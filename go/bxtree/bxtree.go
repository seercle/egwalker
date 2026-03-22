package bxtree

import "fmt"

func New[T any]() *BxTree[T] {
	return &BxTree[T]{
		root:  nil,
		first: nil,
		last:  nil,
	}
}

func (tree *BxTree[T]) Size() int {
	if tree.root == nil {
		return 0
	}
	return tree.root.size
}

func (tree *BxTree[T]) ForEach(f func(item T)) {
	curr := tree.first
	for curr != nil {
		for _, item := range curr.items {
			f(item)
		}
		curr = curr.next
	}
}

func (tree *BxTree[T]) Print() {
	if tree.root != nil {
		fmt.Printf("Tree size: %d\n", tree.root.size)
		tree.root.printTree(0)
	} else {
		fmt.Println("Empty tree")
	}
}

func (n *node[T]) printTree(level int) {
	prefix := ""
	for i := 0; i < level; i++ {
		prefix += "  "
	}
	if n.isLeaf {
		fmt.Printf("%sLeafNode(len=%d):\n", prefix, len(n.items))
		fmt.Print(prefix)
		for _, item := range n.items {
			fmt.Printf(" %v", item)
		}
		fmt.Println()
	} else {
		fmt.Printf("%sInternalNode(len=%d,size=%d):\n", prefix, len(n.children), n.size)
		for i, child := range n.children {
			fmt.Printf("%s  Child %d:\n", prefix, i)
			child.printTree(level + 1)
		}
	}
}

func (tree *BxTree[T]) GetAt(index int) (*T, error) {
	leaf, pos, err := tree.getAt(index)
	if err != nil {
		return nil, err
	}
	return &leaf.items[pos], nil
}

func (tree *BxTree[T]) getAt(index int) (*node[T], int, error) {
	if index < 0 || index >= tree.Size() {
		return nil, -1, ErrIndexOutOfBounds
	}
	if index < tree.first.size {
		return tree.first, index, nil
	}
	if index >= tree.Size()-tree.last.size {
		return tree.last, index - (tree.Size() - tree.last.size), nil
	}

	curr := tree.root
	for !curr.isLeaf {
		found := false
		for _, child := range curr.children {
			if index < child.size {
				curr = child
				found = true
				break
			}
			index -= child.size
		}
		if !found {
			return nil, -1, ErrIndexOutOfBounds
		}
	}
	return curr, index, nil
}

func (tree *BxTree[T]) InsertRange(index int, items []T) error {
	for i, item := range items {
		if err := tree.InsertAt(index+i, item); err != nil {
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
		n := &node[T]{
			isLeaf: true,
			size:   1,
			items:  []T{item},
		}
		tree.root = n
		tree.first = n
		tree.last = n
		return nil
	}

	var leaf *node[T]
	var pos int
	if index == tree.Size() {
		leaf, pos = tree.last, tree.last.size
	} else {
		var err error
		leaf, pos, err = tree.getAt(index)
		if err != nil {
			return err
		}
	}

	leaf.items = append(leaf.items, *new(T))
	copy(leaf.items[pos+1:], leaf.items[pos:])
	leaf.items[pos] = item
	leaf.size++
	leaf.updateParentSizeUpwards(1)

	if len(leaf.items) > LeafMaxSize {
		tree.split(leaf)
	}
	return nil
}

func (tree *BxTree[T]) split(n *node[T]) {
	if n.parent == nil {
		newRoot := &node[T]{
			isLeaf: false,
			size:   n.size,
		}
		tree.root = newRoot
		n.parent = newRoot
		newRoot.children = []*node[T]{n}
	}

	parent := n.parent
	right := &node[T]{
		isLeaf: n.isLeaf,
		parent: parent,
	}

	if n.isLeaf {
		mid := len(n.items) / 2
		right.items = make([]T, len(n.items)-mid)
		copy(right.items, n.items[mid:])
		n.items = n.items[:mid]
		n.size = len(n.items)
		right.size = len(right.items)

		right.next = n.next
		n.next = right
		if tree.last == n {
			tree.last = right
		}
	} else {
		mid := len(n.children) / 2
		right.children = make([]*node[T], len(n.children)-mid)
		copy(right.children, n.children[mid:])
		n.children = n.children[:mid]

		n.size = 0
		for _, child := range n.children {
			n.size += child.size
		}
		right.size = 0
		for _, child := range right.children {
			child.parent = right
			right.size += child.size
		}
	}

	idx := n.getParentIndex()
	parent.children = append(parent.children, nil)
	copy(parent.children[idx+2:], parent.children[idx+1:])
	parent.children[idx+1] = right

	if len(parent.children) > InternalMaxSize {
		tree.split(parent)
	}
}

func (tree *BxTree[T]) DeleteRange(index int, length int) error {
	for i := 0; i < length; i++ {
		if err := tree.DeleteAt(index); err != nil {
			return err
		}
	}
	return nil
}

func (tree *BxTree[T]) DeleteAt(index int) error {
	if index < 0 || index >= tree.Size() {
		return ErrIndexOutOfBounds
	}
	leaf, pos, err := tree.getAt(index)
	if err != nil {
		return err
	}

	copy(leaf.items[pos:], leaf.items[pos+1:])
	leaf.items = leaf.items[:len(leaf.items)-1]
	leaf.size--
	leaf.updateParentSizeUpwards(-1)

	tree.rebalance(leaf)
	return nil
}

func (tree *BxTree[T]) rebalance(n *node[T]) {
	if n.parent == nil {
		if !n.isLeaf && len(n.children) == 1 {
			tree.root = n.children[0]
			tree.root.parent = nil
		} else if n.isLeaf && n.size == 0 {
			tree.root = nil
			tree.first = nil
			tree.last = nil
		}
		return
	}

	minSize := InternalMinSize
	if n.isLeaf {
		minSize = LeafMinSize
	}

	currLen := len(n.children)
	if n.isLeaf {
		currLen = len(n.items)
	}

	if currLen >= minSize {
		return
	}

	idx := n.getParentIndex()
	parent := n.parent

	if idx > 0 {
		left := parent.children[idx-1]
		if n.isLeaf {
			if len(left.items) > LeafMinSize {
				item := left.items[len(left.items)-1]
				left.items = left.items[:len(left.items)-1]
				left.size--
				n.items = append([]T{item}, n.items...)
				n.size++
				left.updateParentSizeUpwards(-1)
				n.updateParentSizeUpwards(1)
				return
			}
		} else {
			if len(left.children) > InternalMinSize {
				child := left.children[len(left.children)-1]
				left.children = left.children[:len(left.children)-1]
				left.size -= child.size
				n.children = append([]*node[T]{child}, n.children...)
				n.size += child.size
				child.parent = n
				left.updateParentSizeUpwards(-child.size)
				n.updateParentSizeUpwards(child.size)
				return
			}
		}
	}

	if idx < len(parent.children)-1 {
		right := parent.children[idx+1]
		if n.isLeaf {
			if len(right.items) > LeafMinSize {
				item := right.items[0]
				right.items = right.items[1:]
				right.size--
				n.items = append(n.items, item)
				n.size++
				right.updateParentSizeUpwards(-1)
				n.updateParentSizeUpwards(1)
				return
			}
		} else {
			if len(right.children) > InternalMinSize {
				child := right.children[0]
				right.children = right.children[1:]
				right.size -= child.size
				n.children = append(n.children, child)
				n.size += child.size
				child.parent = n
				right.updateParentSizeUpwards(-child.size)
				n.updateParentSizeUpwards(child.size)
				return
			}
		}
	}

	if idx > 0 {
		tree.merge(parent.children[idx-1], n)
	} else {
		tree.merge(n, parent.children[idx+1])
	}
}

func (tree *BxTree[T]) merge(left, right *node[T]) {
	parent := left.parent
	if left.isLeaf {
		left.items = append(left.items, right.items...)
		left.size = len(left.items)
		left.next = right.next
		if tree.last == right {
			tree.last = left
		}
	} else {
		for _, child := range right.children {
			child.parent = left
			left.children = append(left.children, child)
			left.size += child.size
		}
	}

	idx := right.getParentIndex()
	copy(parent.children[idx:], parent.children[idx+1:])
	parent.children = parent.children[:len(parent.children)-1]

	tree.rebalance(parent)
}

func (n *node[T]) updateParentSizeUpwards(delta int) {
	curr := n.parent
	for curr != nil {
		curr.size += delta
		curr = curr.parent
	}
}

func (n *node[T]) getParentIndex() int {
	if n.parent == nil {
		return -1
	}
	for i, child := range n.parent.children {
		if child == n {
			return i
		}
	}
	return -1
}
