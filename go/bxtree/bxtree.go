package bxtree

import "fmt"

func New[T any]() *BxTree[T] {
	return &BxTree[T]{}
}

func (tree *BxTree[T]) Size() int {
	if tree.root == nil {
		return 0
	}
	return tree.root.size
}

//

func (node *node[T]) printTree(level int) {
	prefix := ""
	for range level {
		prefix += "  "
	}
	if node.isLeaf {
		fmt.Printf("%sLeafNode(len=%d):\n", prefix, len(node.items))
		print(prefix)
		for _, item := range node.items {
			fmt.Printf(" %v", item)
		}
		println()
	} else {
		fmt.Printf("%sInternalNode(len=%d,size=%d):\n", prefix, len(node.children), node.size)
		for i, child := range node.children {
			fmt.Printf("%s  Child %d:\n", prefix, i)
			child.printTree(level + 2)
		}
	}
}

func (tree *BxTree[T]) Print() {
	if tree.root != nil {
		fmt.Printf("Tree size: %d\n", tree.root.size)
		tree.root.printTree(0)
	} else {
		println("Empty tree")
	}
}

//

func (node *node[T]) getAt(index int) (*node[T], int, error) {
	for !node.isLeaf {
		found := false
		for i := 0; i < len(node.children); i++ {
			i_size := node.children[i].size
			if index < i_size {
				node = node.children[i]
				found = true
				break
			}
			index -= i_size
		}
		if !found {
			return nil, -1, ErrIndexOutOfBounds
		}
	}
	return node, index, nil
}

func (tree *BxTree[T]) GetAt(index int) (*T, error) {
	if index < 0 || index >= tree.Size() {
		return nil, ErrIndexOutOfBounds
	}
	node, index, err := tree.root.getAt(index)
	if err != nil {
		return nil, err
	}
	return &node.items[index], nil
}

//

func (_node *node[T]) split() (*node[T], *node[T]) {
	right := &node[T]{
		parent:   _node.parent,
		isLeaf:   _node.isLeaf,
		size:     0,
		items:    nil,
		children: nil,
	}
	if _node.isLeaf {

		right_len := len(_node.items) / 2
		right.items = make([]T, right_len)
		copy(right.items, _node.items[right_len:])
		right.size = len(right.items)
		_node.items = _node.items[:right_len]
		_node.size = len(_node.items)
	} else {

		right_len := len(_node.children) / 2
		right.children = make([]*node[T], right_len)
		for i := range len(right.children) {
			child := _node.children[len(_node.children)-right_len+i]
			right.children[i] = child
			child.parent = right
			right.size += child.size
		}
		_node.children = _node.children[:len(_node.children)-right_len]
		_node.size -= right.size
	}
	return _node, right
}

func (node *node[T]) updateParentSizeUpwards(delta int) {
	for parent := node.parent; parent != nil; parent = parent.parent {
		parent.size += delta
	}
}

func (tree *BxTree[T]) insertInternal(_node *node[T], new_node *node[T], index int) {
	insert := func(n *node[T], at int) {
		n.children = append(n.children, new(node[T]))
		copy(n.children[at+1:], n.children[at:])
		n.children[at] = new_node
		n.size += new_node.size
		new_node.parent = n
	}
	if len(_node.children) < INTERNAL_MAX_SIZE {
		insert(_node, index)
		_node.updateParentSizeUpwards(new_node.size)
		return
	} else {
		_, right := _node.split()
		if index <= len(_node.children) {
			insert(_node, index)
			_node.updateParentSizeUpwards(-right.size + new_node.size)
		} else {
			_node.updateParentSizeUpwards(-right.size)
			insert(right, index-len(_node.children))
		}
		if _node.parent == nil {
			new_root := &node[T]{
				isLeaf:   false,
				parent:   nil,
				size:     _node.size + right.size,
				children: []*node[T]{_node, right},
			}
			_node.parent = new_root
			right.parent = new_root
			tree.root = new_root
			return
		} else {
			for i, parent_child := range _node.parent.children {
				if parent_child == _node {
					tree.insertInternal(_node.parent, right, i+1)
					return
				}
			}
			panic("parent does not contain child")
		}
	}
}

func (tree *BxTree[T]) insertLeaf(_node *node[T], item T, index int) {
	insert := func(node *node[T], at int) {
		node.items = append(node.items, *new(T))
		copy(node.items[at+1:], node.items[at:])
		node.items[at] = item
		node.size++
	}
	if _node.size < LEAF_MAX_SIZE {
		insert(_node, index)
		_node.updateParentSizeUpwards(1)
		return
	} else {
		_, right := _node.split()
		if tree.last == _node {
			tree.last = right
		}

		if index <= _node.size {
			insert(_node, index)
			_node.updateParentSizeUpwards(-right.size + 1)
		} else {
			_node.updateParentSizeUpwards(-right.size)
			insert(right, index-_node.size)
		}

		if _node.parent == nil {
			new_root := &node[T]{
				isLeaf:   false,
				parent:   nil,
				size:     _node.size + right.size,
				children: []*node[T]{_node, right},
			}
			_node.parent = new_root
			right.parent = new_root
			tree.root = new_root
		} else {
			for i, parent_child := range _node.parent.children {
				if parent_child == _node {
					tree.insertInternal(_node.parent, right, i+1)
					return
				}
			}
			panic("parent does not contain child")
		}
	}
}

func (tree *BxTree[T]) InsertAt(index int, item T) error {
	if index < 0 || index > tree.Size() {
		return ErrIndexOutOfBounds
	}
	if tree.root == nil {
		leaf := &node[T]{
			isLeaf: true,
			parent: nil,
			size:   1,
			items:  []T{item},
		}
		tree.root = leaf
		tree.first = leaf
		tree.last = leaf
		return nil
	}

	var leaf *node[T]
	var insert_pos int
	switch index {
	case 0:
		leaf = tree.first
		insert_pos = 0
	case tree.root.size:
		leaf = tree.last
		insert_pos = leaf.size
	default:
		var err error
		leaf, insert_pos, err = tree.root.getAt(insert_pos)
		if err != nil {
			return err
		}
	}
	tree.insertLeaf(leaf, item, insert_pos)
	return nil
}

//
