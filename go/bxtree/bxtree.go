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
		fmt.Printf("%sInternalNode(len=%d):\n", prefix, internal._len)
		for i := 0; i < internal._len; i++ {
			fmt.Printf("%s  Child %d (size=%d):\n", prefix, i, internal.sizes[i])
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
	if node.isLeaf() {
		return node.(*LeafNode[T]).getAt(index)
	}
	return node.(*InternalNode[T]).getAt(index)
}

func (node *InternalNode[T]) getAt(index int) (*LeafNode[T], int, error) {
	for i := 0; i < node._len; i++ {
		i_size := node.sizes[i]
		if index < i_size {
			return getAt(node.children[i], index)
		}
		index -= i_size
	}
	return nil, -1, ErrIndexOutOfBounds
}

func (node *LeafNode[T]) getAt(index int) (*LeafNode[T], int, error) {
	if index < 0 || index >= node._len {
		return nil, -1, ErrIndexOutOfBounds
	}
	return node, index, nil
}

func (tree *BxTree[T]) GetAt(index int) (*T, error) {
	if index < 0 || index >= tree.size {
		return nil, ErrIndexOutOfBounds
	}
	leaf, pos, err := getAt(tree.root, index)
	if err != nil {
		return nil, err
	}
	return &leaf.items[pos], nil
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
		_len:    node._len / 2,
	}
	copy(right.children[0:right._len], node.children[node._len-right._len:node._len])
	copy(right.sizes[0:right._len], node.sizes[node._len-right._len:node._len])
	node._len = node._len - right._len
	//update parent pointers for children
	for i := 0; i < right._len; i++ {
		right.children[i].setParent(right)
	}
	return node, right
}

func updateParentSizeUpwards[T any](node Node[T]) {
	for ; node.parent() != nil; node = node.parent() {
		for i := range node.parent()._len {
			if node.parent().children[i] == node {
				node.parent().sizes[i] = node.count()
				break
			}
		}
	}
}

func (node *InternalNode[T]) insertAt(children_index int, new_node Node[T]) (*InternalNode[T], bool) {
	insert := func(_node *InternalNode[T], _children_index int) {
		copy(_node.children[_children_index+1:_node._len+1], _node.children[_children_index:_node._len])
		copy(_node.sizes[_children_index+1:_node._len+1], _node.sizes[_children_index:_node._len])
		_node.children[_children_index] = new_node
		_node.sizes[_children_index] = new_node.count()
		new_node.setParent(_node)
		_node._len++
	}
	if node._len < INTERNAL_MAX_SIZE {
		insert(node, children_index)
		updateParentSizeUpwards(node)
		return node, false
	} else {
		_, right := node.split()
		if children_index <= node._len {
			insert(node, children_index)
		} else {
			insert(right, children_index-node._len)
		}
		updateParentSizeUpwards(node)

		if node._parent == nil {
			new_root := &InternalNode[T]{
				_parent: nil,
				_len:    2,
			}
			new_root.children[0] = node
			new_root.sizes[0] = node.count()
			new_root.children[1] = right
			new_root.sizes[1] = right.count()
			node._parent = new_root
			right._parent = new_root
			return new_root, true
		} else {
			for i := range node._parent._len {
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
		updateParentSizeUpwards(node)
		return nil, false
	} else {
		_, right := node.split()
		if pos <= node._len {
			insert(node, pos)
		} else {
			insert(right, pos-node._len)
		}
		updateParentSizeUpwards(node)

		if node._parent == nil {
			new_root := &InternalNode[T]{
				_parent: nil,
				_len:    2,
			}
			new_root.children[0] = node
			new_root.sizes[0] = node.count()
			new_root.children[1] = right
			new_root.sizes[1] = right.count()
			node._parent = new_root
			right._parent = new_root
			return new_root, true
		} else {
			for i := range node._parent._len {
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
