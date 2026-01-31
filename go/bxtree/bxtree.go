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

func (tree *BxTree[T]) Print() {
	if tree.root != nil {
		fmt.Printf("Tree size: %d\n", tree.root.size)
		tree.root.printTree(0)
	} else {
		println("Empty tree")
	}
}

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

//

func (tree *BxTree[T]) GetAt(index int) (*T, error) {
	node, index, err := tree.getAt(index)
	if err != nil {
		return nil, err
	}
	return &node.items[index], nil
}

func (tree *BxTree[T]) getAt(index int) (*node[T], int, error) {
	if index < 0 || index >= tree.Size() {
		return nil, -1, ErrIndexOutOfBounds
	}
	if index < tree.first.size {
		return tree.first, index, nil
	} else if index >= tree.Size()-tree.last.size {
		return tree.last, index - tree.Size() + tree.last.size, nil
	} else {
		return tree.root.getAt(index)
	}
}

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

//

// OPTIMIZE: do not go down the tree for each insert, but adapt
// the leaf & position if the insert caused a rebalance
func (tree *BxTree[T]) InsertRange(index int, items []T) error {
	for i, item := range items {
		err := tree.InsertAt(index+i, item)
		if err != nil {
			return err
		}
	}
	return nil
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

	if index == tree.Size() {
		return tree.insertLeaf(tree.last, item, tree.last.size)
	}
	leaf, position, err := tree.getAt(index)
	if err != nil {
		return err
	}
	return tree.insertLeaf(leaf, item, position)
}

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

func (tree *BxTree[T]) insertInternal(_node *node[T], new_node *node[T], index int, upward_change int) error {
	insert := func(n *node[T], at int) {
		n.children = append(n.children, new(node[T]))
		copy(n.children[at+1:], n.children[at:])
		n.children[at] = new_node
		n.size += new_node.size
		new_node.parent = n
	}

	_node.size += upward_change
	if len(_node.children) < INTERNAL_MAX_SIZE {
		insert(_node, index)
		_node.updateParentSizeUpwards(upward_change + new_node.size)
		return nil
	}

	_, right := _node.split()
	upward_change -= right.size
	if index <= len(_node.children) {
		insert(_node, index)
		upward_change += new_node.size
	} else {
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
		return nil
	}
	parent_index := _node.getParentIndex()
	if parent_index == -1 {
		return ErrParentDoesNotHaveChild
	}
	return tree.insertInternal(_node.parent, right, _node.getParentIndex()+1, upward_change)
}

func (tree *BxTree[T]) insertLeaf(leaf *node[T], item T, index int) error {
	insert := func(node *node[T], at int) {
		node.items = append(node.items, *new(T))
		copy(node.items[at+1:], node.items[at:])
		node.items[at] = item
		node.size++
	}
	if leaf.size < LEAF_MAX_SIZE {
		insert(leaf, index)
		leaf.updateParentSizeUpwards(1)
		return nil
	}
	_, right := leaf.split()
	if tree.last == leaf {
		tree.last = right
	}

	upward_change := -right.size
	if index <= leaf.size {
		insert(leaf, index)
		upward_change += 1
	} else {
		insert(right, index-leaf.size)
	}

	if leaf.parent == nil {
		new_root := &node[T]{
			isLeaf:   false,
			parent:   nil,
			size:     leaf.size + right.size,
			children: []*node[T]{leaf, right},
		}
		leaf.parent = new_root
		right.parent = new_root
		tree.root = new_root
		return nil
	}
	parent_index := leaf.getParentIndex()
	if parent_index == -1 {
		return ErrParentDoesNotHaveChild
	}
	return tree.insertInternal(leaf.parent, right, parent_index+1, upward_change)
}

//

// OPTIMIZE: do not go down the tree for each delete, but adapt
// the leaf & position if the delete caused a rebalance
func (tree *BxTree[T]) DeleteRange(index int, length int) error {
	for range length {
		err := tree.DeleteAt(index)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tree *BxTree[T]) DeleteAt(index int) error {
	if index < 0 || index >= tree.Size() {
		return ErrIndexOutOfBounds
	}
	leaf, position, err := tree.getAt(index)
	if err != nil {
		return err
	}
	return tree.deleteLeaf(leaf, position)
}

func (tree *BxTree[T]) deleteInternal(_node *node[T], index int, upward_change int) error {
	del_size := _node.children[index].size
	copy(_node.children[index:], _node.children[index+1:])
	_node.children = _node.children[:len(_node.children)-1]
	_node.size -= del_size

	upward_change -= del_size
	if len(_node.children) >= INTERNAL_MIN_SIZE || (_node.parent == nil && len(_node.children) > 1) {
		_node.updateParentSizeUpwards(upward_change)
		return nil
	}
	if _node.parent == nil && len(_node.children) == 1 {
		tree.root = _node.children[0]
		tree.root.parent = nil
		return nil
	}
	parent_index := _node.getParentIndex()
	if parent_index > 0 {
		left_sibling := _node.parent.children[parent_index-1]
		if tryBorrowFromLeftSibling(_node, left_sibling) {
			_node.updateParentSizeUpwards(upward_change)
			return nil
		} else {
			tree.merge(left_sibling, _node)
			return tree.deleteInternal(left_sibling.parent, parent_index, upward_change+_node.size)
		}
	}
	if parent_index < len(_node.parent.children)-1 {
		right_sibling := _node.parent.children[parent_index+1]
		if tryBorrowFromRightSibling(_node, right_sibling) {
			_node.updateParentSizeUpwards(upward_change)
			return nil
		} else {
			tree.merge(_node, right_sibling)
			return tree.deleteInternal(_node.parent, parent_index+1, upward_change+right_sibling.size)
		}
	}
	return ErrNotRootAndOneChild
}

func (tree *BxTree[T]) deleteLeaf(leaf *node[T], index int) error {
	copy(leaf.items[index:], leaf.items[index+1:])
	leaf.items = leaf.items[:leaf.size-1]
	leaf.size -= 1

	if leaf.size >= LEAF_MIN_SIZE || leaf.parent == nil {
		leaf.updateParentSizeUpwards(-1)
		return nil
	}
	parent_index := leaf.getParentIndex()
	if parent_index > 0 {
		left_sibling := leaf.parent.children[parent_index-1]
		if tryBorrowFromLeftSibling(leaf, left_sibling) {
			leaf.updateParentSizeUpwards(-1)
			return nil
		} else {
			tree.merge(left_sibling, leaf)
			return tree.deleteInternal(left_sibling.parent, parent_index, leaf.size-1)
		}
	}
	if parent_index < len(leaf.parent.children)-1 {
		right_sibling := leaf.parent.children[parent_index+1]
		if tryBorrowFromRightSibling(leaf, right_sibling) {
			leaf.updateParentSizeUpwards(-1)
			return nil
		} else {
			tree.merge(leaf, right_sibling)
			return tree.deleteInternal(leaf.parent, parent_index+1, right_sibling.size-1)
		}
	}
	return ErrNotRootAndOneChild
}

func (tree *BxTree[T]) merge(left *node[T], right *node[T]) {
	left.size += right.size
	if left.isLeaf {
		left.items = append(left.items, right.items...)
		if tree.last == right {
			tree.last = left
		}
	} else {
		for _, child := range right.children {
			child.parent = left
			left.children = append(left.children, child)
		}
	}
}

// OPTIMIZE: do not borrow only 1 element but make both nodes of equal length
func tryBorrowFromLeftSibling[T any](_node *node[T], sibling *node[T]) bool {
	if _node.isLeaf {
		if _node.size <= LEAF_MIN_SIZE {
			return false
		}
		borrowed := sibling.items[sibling.size-1]
		sibling.items = sibling.items[:sibling.size-1]
		_node.items = append([]T{borrowed}, _node.items...)
		sibling.size -= 1
		_node.size += 1
		return true
	} else {
		if len(_node.children) <= INTERNAL_MIN_SIZE {
			return false
		}
		borrowed := sibling.children[len(sibling.children)-1]
		sibling.children = sibling.children[:len(sibling.children)-1]
		_node.children = append([]*node[T]{borrowed}, _node.children...)
		sibling.size -= borrowed.size
		_node.size += borrowed.size
		return true
	}

}

// OPTIMIZE: do not borrow only 1 element but make both nodes of equal length
func tryBorrowFromRightSibling[T any](_node *node[T], sibling *node[T]) bool {
	if _node.isLeaf {
		if _node.size <= LEAF_MIN_SIZE {
			return false
		}
		borrowed := sibling.items[0]
		sibling.items = sibling.items[1:]
		_node.items = append(_node.items, borrowed)
		sibling.size -= 1
		_node.size += 1
		return true
	} else {
		if len(_node.children) <= INTERNAL_MIN_SIZE {
			return false
		}
		borrowed := sibling.children[0]
		sibling.children = sibling.children[1:]
		_node.children = append(_node.children, borrowed)
		sibling.size -= borrowed.size
		_node.size += borrowed.size
		return true
	}
}

//

func (_node *node[T]) getParentIndex() int {
	if _node.parent == nil {
		return -1
	}
	for i, child := range _node.parent.children {
		if child == _node {
			return i
		}
	}
	return -1
}

func (node *node[T]) updateParentSizeUpwards(delta int) {
	for parent := node.parent; parent != nil; parent = parent.parent {
		parent.size += delta
	}
}
