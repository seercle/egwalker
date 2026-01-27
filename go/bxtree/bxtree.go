package bxtree

import "fmt"

func New[T any]() *BxTree[T] {
	return &BxTree[T]{}
}

//

func printTree[T any](node Node[T], level int) {
	prefix := ""
	for range level {
		prefix += "  "
	}
	if node.isLeaf() {
		leaf := node.(*LeafNode[T])
		fmt.Printf("%sLeafNode(len=%d):\n", prefix, leaf._len)
		print(prefix)
		for i := 0; i < leaf._len; i++ {
			fmt.Printf(" %v", leaf.items[i])
		}
		println()
	} else {
		internal := node.(*InternalNode[T])
		fmt.Printf("%sInternalNode(len=%d,size=%d):\n", prefix, internal.len, internal._size)
		for i := 0; i < internal.len; i++ {
			fmt.Printf("%s  Child %d:\n", prefix, i)
			printTree(internal.children[i], level+2)
		}
	}
}

func (tree *BxTree[T]) Print() {
	if tree.root != nil {
		printTree(tree.root, 0)
	} else {
		println("Empty tree")
	}
}

//

func getAt[T any](node Node[T], index int) (*LeafNode[T], int, error) {
	for !node.isLeaf() {
		int_node := node.(*InternalNode[T])
		found := false
		for i := 0; i < int_node.len; i++ {
			i_size := int_node.children[i].size()
			if index < i_size {
				node = int_node.children[i]
				found = true
				break
			}
			index -= i_size
		}
		if !found {
			return nil, -1, ErrIndexOutOfBounds
		}
	}
	return node.(*LeafNode[T]), index, nil
}

func (tree *BxTree[T]) GetAt(index int) (*T, error) {
	if index < 0 || index >= tree.size {
		return nil, ErrIndexOutOfBounds
	}
	node, index, err := getAt(tree.root, index)
	if err != nil {
		return nil, err
	}
	return &node.items[index], nil
}

//

func (node *LeafNode[T]) split() (*LeafNode[T], *LeafNode[T]) {
	right := &LeafNode[T]{
		_parent: node._parent,
		_len:    node._len / 2,
		next:    node.next,
	}
	copy(right.items[0:right._len], node.items[node._len-right._len:node._len])
	node._len = node._len - right._len
	node.next = right
	return node, right
}

func (node *InternalNode[T]) split() (*InternalNode[T], *InternalNode[T]) {
	right := &InternalNode[T]{
		_parent: node._parent,
		len:     node.len / 2,
		_size:   0,
	}
	for i := 0; i < right.len; i++ {
		right.children[i] = node.children[node.len-right.len+i]
		right.children[i].setParent(right)
		right._size += right.children[i].size()
	}
	node.len = node.len - right.len
	node._size = node._size - right._size
	return node, right
}

func updateParentSizeUpwards[T any](node Node[T], delta int) {
	for parent := node.parent(); parent != nil; parent = parent._parent {
		parent._size += delta
	}
}

func (node *InternalNode[T]) insertAt(children_index int, new_node Node[T]) (*InternalNode[T], bool) {
	insert := func(_node *InternalNode[T], _children_index int) {
		copy(_node.children[_children_index+1:_node.len+1], _node.children[_children_index:_node.len])
		_node.children[_children_index] = new_node
		_node._size += new_node.size()
		_node.len++
		new_node.setParent(_node)
	}
	if node.len < INTERNAL_MAX_SIZE {
		insert(node, children_index)
		updateParentSizeUpwards(node, new_node.size())
		return node, false
	} else {
		_, right := node.split()
		if children_index <= node.len {
			insert(node, children_index)
			updateParentSizeUpwards(node, -right._size+new_node.size())
		} else {
			updateParentSizeUpwards(node, -right._size)
			insert(right, children_index-node.len)
		}
		if node._parent == nil {
			new_root := &InternalNode[T]{
				_parent: nil,
				len:     2,
			}
			new_root.children[0] = node
			new_root.children[1] = right
			new_root._size = node._size + right._size
			node._parent = new_root
			right._parent = new_root
			return new_root, true
		} else {
			for i := range node._parent.len {
				if node._parent.children[i] == node {
					return node._parent.insertAt(i+1, right)
				}
			}
			panic("parent does not contain child")
		}
	}
}

func (node *LeafNode[T]) insertAt(pos int, item T) (*InternalNode[T], bool) {
	insert := func(_node *LeafNode[T], _pos int) {
		copy(_node.items[_pos+1:_node._len+1], _node.items[_pos:_node._len])
		_node.items[_pos] = item
		_node._len++
	}
	if node._len < LEAF_MAX_SIZE {
		insert(node, pos)
		updateParentSizeUpwards(node, 1)
		return nil, false
	} else {
		_, right := node.split()
		if pos <= node._len {
			insert(node, pos)
			updateParentSizeUpwards(node, -right._len+1)
		} else {
			updateParentSizeUpwards(node, -right._len)
			insert(right, pos-node._len)
		}

		if node._parent == nil {
			new_root := &InternalNode[T]{
				_parent: nil,
				len:     2,
			}
			new_root.children[0] = node
			new_root.children[1] = right
			new_root._size = node._len + right._len
			node._parent = new_root
			right._parent = new_root
			return new_root, true
		} else {
			for i := range node._parent.len {
				if node._parent.children[i] == node {
					return node._parent.insertAt(i+1, right)
				}
			}
			panic("parent does not contain child")
		}
	}
}

func (tree *BxTree[T]) InsertAt(index int, item T) error {
	if index < 0 || index > tree.size {
		return ErrIndexOutOfBounds
	}
	if tree.root == nil {
		leaf := &LeafNode[T]{}
		leaf.items[0] = item
		leaf._len = 1
		tree.root = leaf
		tree.size = 1
		return nil
	}

	insert_end := false
	search_pos := index
	if index == tree.size {
		insert_end = true
		search_pos = index - 1
	}
	leaf, pos, err := getAt(tree.root, search_pos)
	if err != nil {
		return err
	}

	insert_pos := pos
	if insert_end {
		insert_pos = leaf._len
	}
	root, root_changed := leaf.insertAt(insert_pos, item)
	if root_changed {
		tree.root = root
	}
	tree.size++
	return nil
}
