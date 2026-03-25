package bxtree

import "fmt"

func New[T any, S any]() *BxTree[T, S] {
	return &BxTree[T, S]{
		Root:            nil,
		First:           nil,
		Last:            nil,
		internalMinSize: DefaultInternalMinSize,
		internalMaxSize: DefaultInternalMaxSize,
		leafMinSize:     DefaultLeafMinSize,
		leafMaxSize:     DefaultLeafMaxSize,
	}
}

func (tree *BxTree[T, S]) Size() int {
	if tree.Root == nil {
		return 0
	}
	return tree.Root.size
}

func (tree *BxTree[T, S]) ForEach(f func(item T)) {
	curr := tree.First
	for curr != nil {
		for _, item := range curr.Items {
			f(item)
		}
		curr = curr.next
	}
}

func (tree *BxTree[T, S]) Print() {
	if tree.Root != nil {
		fmt.Printf("Tree size: %d\n", tree.Root.size)
		tree.Root.printTree(0)
	} else {
		fmt.Println("Empty tree")
	}
}

func (n *Node[T, S]) printTree(level int) {
	prefix := ""
	for i := 0; i < level; i++ {
		prefix += "  "
	}
	if n.isLeaf {
		fmt.Printf("%sLeafNode(len=%d):\n", prefix, len(n.Items))
		fmt.Print(prefix)
		for _, item := range n.Items {
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

func (tree *BxTree[T, S]) GetAt(index int) (*T, error) {
	leaf, pos, err := tree.getAt(index)
	if err != nil {
		return nil, err
	}
	return &leaf.Items[pos], nil
}

func (tree *BxTree[T, S]) GetAtNode(index int) (*Node[T, S], int, error) {
	return tree.getAt(index)
}

func (tree *BxTree[T, S]) getAt(index int) (*Node[T, S], int, error) {
	if index < 0 || index >= tree.Size() {
		return nil, -1, ErrIndexOutOfBounds
	}
	if index < tree.First.size {
		return tree.First, index, nil
	}
	if index >= tree.Size()-tree.Last.size {
		return tree.Last, index - (tree.Size() - tree.Last.size), nil
	}

	curr := tree.Root
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

func (n *Node[T, S]) IsLeaf() bool {
	return n.isLeaf
}

func (n *Node[T, S]) Next() *Node[T, S] {
	return n.next
}

func (tree *BxTree[T, S]) InsertRange(index int, items []T) error {
	if len(items) == 0 {
		return nil
	}
	if index < 0 || index > tree.Size() {
		return ErrIndexOutOfBounds
	}

	for len(items) > 0 {
		if tree.Root == nil {
			// Special case for empty tree
			err := tree.InsertAt(index, items[0])
			if err != nil {
				return err
			}
			index++
			items = items[1:]
			continue
		}

		var leaf *Node[T, S]
		var pos int
		if index == tree.Size() {
			leaf, pos = tree.Last, tree.Last.size
		} else {
			var err error
			leaf, pos, err = tree.getAt(index)
			if err != nil {
				return err
			}
		}

		// How many items can we fit in this leaf without exceeding leafMaxSize?
		canFit := tree.leafMaxSize - len(leaf.Items)
		if canFit < 1 {
			canFit = 1 // At least one, will trigger split
		}
		if canFit > len(items) {
			canFit = len(items)
		}

		// Insert items
		newItems := items[:canFit]
		oldLen := len(leaf.Items)
		leaf.Items = append(leaf.Items, make([]T, len(newItems))...)
		copy(leaf.Items[pos+len(newItems):], leaf.Items[pos:oldLen])
		copy(leaf.Items[pos:], newItems)

		leaf.size += len(newItems)
		leaf.updateParentSizeUpwards(len(newItems))

		if tree.SummaryConfig != nil {
			var totalDelta S
			for i, item := range newItems {
				m := tree.SummaryConfig.FromItem(item)
				if i == 0 {
					totalDelta = m
				} else {
					totalDelta = tree.SummaryConfig.Add(totalDelta, m)
				}
			}
			leaf.UpdateSummaryUpwards(totalDelta, tree)
		}

		if tree.OnItemMoved != nil {
			for _, item := range newItems {
				tree.OnItemMoved(item, leaf)
			}
		}

		items = items[canFit:]
		index += canFit

		if len(leaf.Items) > tree.leafMaxSize {
			tree.split(leaf)
		}
	}
	return nil
}

func (tree *BxTree[T, S]) InsertAt(index int, item T) error {
	if index < 0 || index > tree.Size() {
		return ErrIndexOutOfBounds
	}

	if tree.Root == nil {
		n := &Node[T, S]{
			isLeaf: true,
			size:   1,
			Items:  []T{item},
		}
		if tree.SummaryConfig != nil {
			n.Summary = tree.SummaryConfig.FromItem(item)
		}
		tree.Root = n
		tree.First = n
		tree.Last = n
		if tree.OnItemMoved != nil {
			tree.OnItemMoved(item, n)
		}
		return nil
	}

	var leaf *Node[T, S]
	var pos int
	if index == tree.Size() {
		leaf, pos = tree.Last, tree.Last.size
	} else {
		var err error
		leaf, pos, err = tree.getAt(index)
		if err != nil {
			return err
		}
	}

	leaf.Items = append(leaf.Items, *new(T))
	copy(leaf.Items[pos+1:], leaf.Items[pos:])
	leaf.Items[pos] = item
	leaf.size++
	leaf.updateParentSizeUpwards(1)

	if tree.SummaryConfig != nil {
		delta := tree.SummaryConfig.FromItem(item)
		leaf.UpdateSummaryUpwards(delta, tree)
	}

	if tree.OnItemMoved != nil {
		tree.OnItemMoved(item, leaf)
	}

	if len(leaf.Items) > tree.leafMaxSize {
		tree.split(leaf)
	}
	return nil
}

func (tree *BxTree[T, S]) split(n *Node[T, S]) {
	if n.parent == nil {
		newRoot := &Node[T, S]{
			isLeaf: false,
			size:   n.size,
		}
		if tree.SummaryConfig != nil {
			newRoot.Summary = n.Summary
		}
		tree.Root = newRoot
		n.parent = newRoot
		newRoot.children = []*Node[T, S]{n}
	}

	parent := n.parent
	right := &Node[T, S]{
		isLeaf: n.isLeaf,
		parent: parent,
	}

	if n.isLeaf {
		mid := len(n.Items) / 2
		right.Items = make([]T, len(n.Items)-mid)
		copy(right.Items, n.Items[mid:])

		if tree.SummaryConfig != nil {
			var rightSummary S
			for i, item := range right.Items {
				m := tree.SummaryConfig.FromItem(item)
				if i == 0 {
					rightSummary = m
				} else {
					rightSummary = tree.SummaryConfig.Add(rightSummary, m)
				}
			}
			right.Summary = rightSummary
			n.Summary = tree.SummaryConfig.Sub(n.Summary, rightSummary)
		}

		if tree.OnItemMoved != nil {
			for _, item := range right.Items {
				tree.OnItemMoved(item, right)
			}
		}

		n.Items = n.Items[:mid]
		n.size = len(n.Items)
		right.size = len(right.Items)

		right.next = n.next
		n.next = right
		if tree.Last == n {
			tree.Last = right
		}
	} else {
		mid := len(n.children) / 2
		right.children = make([]*Node[T, S], len(n.children)-mid)
		copy(right.children, n.children[mid:])
		n.children = n.children[:mid]

		n.size = 0
		right.size = 0
		firstN := true
		firstR := true

		for _, child := range n.children {
			n.size += child.size
			if tree.SummaryConfig != nil {
				if firstN {
					n.Summary = child.Summary
					firstN = false
				} else {
					n.Summary = tree.SummaryConfig.Add(n.Summary, child.Summary)
				}
			}
		}
		for _, child := range right.children {
			child.parent = right
			right.size += child.size
			if tree.SummaryConfig != nil {
				if firstR {
					right.Summary = child.Summary
					firstR = false
				} else {
					right.Summary = tree.SummaryConfig.Add(right.Summary, child.Summary)
				}
			}
		}
	}

	idx := n.getParentIndex()
	parent.children = append(parent.children, nil)
	copy(parent.children[idx+2:], parent.children[idx+1:])
	parent.children[idx+1] = right

	if len(parent.children) > tree.internalMaxSize {
		tree.split(parent)
	}
}

func (tree *BxTree[T, S]) DeleteRange(index int, length int) error {
	if length == 0 {
		return nil
	}
	if index < 0 || index+length > tree.Size() {
		return ErrIndexOutOfBounds
	}

	for length > 0 {
		leaf, pos, err := tree.getAt(index)
		if err != nil {
			return err
		}

		canDelete := leaf.size - pos
		if canDelete > length {
			canDelete = length
		}

		// Update summary before deleting
		if tree.SummaryConfig != nil {
			var totalDelta S
			for i := 0; i < canDelete; i++ {
				m := tree.SummaryConfig.FromItem(leaf.Items[pos+i])
				if i == 0 {
					totalDelta = m
				} else {
					totalDelta = tree.SummaryConfig.Add(totalDelta, m)
				}
			}
			leaf.UpdateSummaryUpwards(tree.SummaryConfig.Sub(*new(S), totalDelta), tree)
		}

		// Delete items
		copy(leaf.Items[pos:], leaf.Items[pos+canDelete:])
		leaf.Items = leaf.Items[:len(leaf.Items)-canDelete]
		leaf.size -= canDelete
		leaf.updateParentSizeUpwards(-canDelete)

		length -= canDelete
		tree.rebalance(leaf)
	}
	return nil
}

func (tree *BxTree[T, S]) DeleteAt(index int) error {
	if index < 0 || index >= tree.Size() {
		return ErrIndexOutOfBounds
	}
	leaf, pos, err := tree.getAt(index)
	if err != nil {
		return err
	}

	item := leaf.Items[pos]
	if tree.SummaryConfig != nil {
		delta := tree.SummaryConfig.FromItem(item)
		leaf.UpdateSummaryUpwards(tree.SummaryConfig.Sub(*new(S), delta), tree)
	}

	copy(leaf.Items[pos:], leaf.Items[pos+1:])
	leaf.Items = leaf.Items[:len(leaf.Items)-1]
	leaf.size--
	leaf.updateParentSizeUpwards(-1)

	tree.rebalance(leaf)
	return nil
}

func (tree *BxTree[T, S]) rebalance(n *Node[T, S]) {
	if n.parent == nil {
		if !n.isLeaf && len(n.children) == 1 {
			tree.Root = n.children[0]
			tree.Root.parent = nil
		} else if n.isLeaf && n.size == 0 {
			tree.Root = nil
			tree.First = nil
			tree.Last = nil
		}
		return
	}

	minSize := tree.internalMinSize
	if n.isLeaf {
		minSize = tree.leafMinSize
	}

	currLen := len(n.children)
	if n.isLeaf {
		currLen = len(n.Items)
	}

	if currLen >= minSize {
		return
	}

	idx := n.getParentIndex()
	parent := n.parent

	if idx > 0 {
		left := parent.children[idx-1]
		if n.isLeaf {
			if len(left.Items) > tree.leafMinSize {
				item := left.Items[len(left.Items)-1]
				left.Items = left.Items[:len(left.Items)-1]
				left.size--
				n.Items = append([]T{item}, n.Items...)
				n.size++
				left.updateParentSizeUpwards(-1)
				n.updateParentSizeUpwards(1)

				if tree.SummaryConfig != nil {
					m := tree.SummaryConfig.FromItem(item)
					left.Summary = tree.SummaryConfig.Sub(left.Summary, m)
					n.Summary = tree.SummaryConfig.Add(n.Summary, m)
				}

				if tree.OnItemMoved != nil {
					tree.OnItemMoved(item, n)
				}
				return
			}
		} else {
			if len(left.children) > tree.internalMinSize {
				child := left.children[len(left.children)-1]
				left.children = left.children[:len(left.children)-1]
				left.size -= child.size
				n.children = append([]*Node[T, S]{child}, n.children...)
				n.size += child.size
				child.parent = n
				left.updateParentSizeUpwards(-child.size)
				n.updateParentSizeUpwards(child.size)

				if tree.SummaryConfig != nil {
					left.Summary = tree.SummaryConfig.Sub(left.Summary, child.Summary)
					n.Summary = tree.SummaryConfig.Add(n.Summary, child.Summary)
				}

				return
			}
		}
	}

	if idx < len(parent.children)-1 {
		right := parent.children[idx+1]
		if n.isLeaf {
			if len(right.Items) > tree.leafMinSize {
				item := right.Items[0]
				right.Items = right.Items[1:]
				right.size--
				n.Items = append(n.Items, item)
				n.size++
				right.updateParentSizeUpwards(-1)
				n.updateParentSizeUpwards(1)

				if tree.SummaryConfig != nil {
					m := tree.SummaryConfig.FromItem(item)
					right.Summary = tree.SummaryConfig.Sub(right.Summary, m)
					n.Summary = tree.SummaryConfig.Add(n.Summary, m)
				}

				if tree.OnItemMoved != nil {
					tree.OnItemMoved(item, n)
				}
				return
			}
		} else {
			if len(right.children) > tree.internalMinSize {
				child := right.children[0]
				right.children = right.children[1:]
				right.size -= child.size
				n.children = append(n.children, child)
				n.size += child.size
				child.parent = n
				right.updateParentSizeUpwards(-child.size)
				n.updateParentSizeUpwards(child.size)

				if tree.SummaryConfig != nil {
					right.Summary = tree.SummaryConfig.Sub(right.Summary, child.Summary)
					n.Summary = tree.SummaryConfig.Add(n.Summary, child.Summary)
				}

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

func (tree *BxTree[T, S]) merge(left, right *Node[T, S]) {
	parent := left.parent
	if left.isLeaf {
		if tree.OnItemMoved != nil {
			for _, item := range right.Items {
				tree.OnItemMoved(item, left)
			}
		}
		left.Items = append(left.Items, right.Items...)
		left.size = len(left.Items)
		if tree.SummaryConfig != nil {
			left.Summary = tree.SummaryConfig.Add(left.Summary, right.Summary)
		}
		left.next = right.next
		if tree.Last == right {
			tree.Last = left
		}
	} else {
		for _, child := range right.children {
			child.parent = left
			left.children = append(left.children, child)
			left.size += child.size
			if tree.SummaryConfig != nil {
				left.Summary = tree.SummaryConfig.Add(left.Summary, child.Summary)
			}
		}
	}

	idx := right.getParentIndex()
	copy(parent.children[idx:], parent.children[idx+1:])
	parent.children = parent.children[:len(parent.children)-1]

	tree.rebalance(parent)
}

func (n *Node[T, S]) UpdateSummaryUpwards(delta S, tree *BxTree[T, S]) {
	if tree.SummaryConfig == nil {
		return
	}
	curr := n
	for curr != nil {
		curr.Summary = tree.SummaryConfig.Add(curr.Summary, delta)
		curr = curr.parent
	}
}

func (n *Node[T, S]) updateParentSizeUpwards(delta int) {
	curr := n.parent
	for curr != nil {
		curr.size += delta
		curr = curr.parent
	}
}

func (n *Node[T, S]) getParentIndex() int {
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

func (n *Node[T, S]) Index() int {
	index := 0
	curr := n
	for curr.parent != nil {
		parent := curr.parent
		for _, child := range parent.children {
			if child == curr {
				break
			}
			index += child.size
		}
		curr = parent
	}
	return index
}

// FindPath traverses the tree using a predicate.
// The predicate takes the current accumulated summary and a candidate child/item summary,
// and returns true if the target is within or beyond that child/item.
func (tree *BxTree[T, S]) FindPath(predicate func(acc S, cur S) bool) (*Node[T, S], int, S) {
	if tree.Root == nil || tree.SummaryConfig == nil {
		return nil, -1, *new(S)
	}

	var acc S
	curr := tree.Root
	first := true

	for !curr.isLeaf {
		found := false
		for _, child := range curr.children {
			if predicate(acc, child.Summary) {
				curr = child
				found = true
				break
			}
			if first {
				acc = child.Summary
				first = false
			} else {
				acc = tree.SummaryConfig.Add(acc, child.Summary)
			}
		}
		if !found {
			return nil, -1, acc
		}
	}

	for i, item := range curr.Items {
		m := tree.SummaryConfig.FromItem(item)
		if predicate(acc, m) {
			return curr, i, acc
		}
		if first {
			acc = m
			first = false
		} else {
			acc = tree.SummaryConfig.Add(acc, m)
		}
	}

	return nil, -1, acc
}
