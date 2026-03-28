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

func NewFromSlice[T any, S any](items []T, config *SummaryConfig[T, S], onMoved func(T, *Node[T, S])) *BxTree[T, S] {
	tree := New[T, S]()
	tree.SummaryConfig = config
	tree.OnItemMoved = onMoved

	if len(items) == 0 {
		return tree
	}

	// 1. Create all leaf nodes
	var leaves []*Node[T, S]
	var prevLeaf *Node[T, S]

	for i := 0; i < len(items); i += tree.leafMaxSize {
		end := i + tree.leafMaxSize
		if end > len(items) {
			end = len(items)
		}

		leaf := &Node[T, S]{
			isLeaf: true,
			Items:  make([]T, end-i),
			size:   end - i,
		}
		copy(leaf.Items, items[i:end])

		if tree.SummaryConfig != nil {
			var s S
			for j, item := range leaf.Items {
				m := tree.SummaryConfig.FromItem(item)
				if j == 0 {
					s = m
				} else {
					s = tree.SummaryConfig.Add(s, m)
				}
			}
			leaf.Summary = s
		}

		if prevLeaf != nil {
			prevLeaf.next = leaf
		} else {
			tree.First = leaf
		}
		leaves = append(leaves, leaf)
		prevLeaf = leaf

		if tree.OnItemMoved != nil {
			for _, item := range leaf.Items {
				tree.OnItemMoved(item, leaf)
			}
		}
	}
	tree.Last = prevLeaf

	// 2. Build internal nodes bottom-up
	var currentLevel []*Node[T, S] = leaves
	for len(currentLevel) > 1 {
		var nextLevel []*Node[T, S]
		for i := 0; i < len(currentLevel); i += tree.internalMaxSize {
			end := i + tree.internalMaxSize
			if end > len(currentLevel) {
				end = len(currentLevel)
			}

			n := &Node[T, S]{
				isLeaf:   false,
				children: make([]*Node[T, S], end-i),
			}
			copy(n.children, currentLevel[i:end])

			var size int
			var s S
			first := true
			for _, child := range n.children {
				child.parent = n
				size += child.size
				if tree.SummaryConfig != nil {
					if first {
						s = child.Summary
						first = false
					} else {
						s = tree.SummaryConfig.Add(s, child.Summary)
					}
				}
			}
			n.size = size
			n.Summary = s
			nextLevel = append(nextLevel, n)
		}
		currentLevel = nextLevel
	}

	tree.Root = currentLevel[0]
	return tree
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
	for range level {
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
	return tree.insert(index, items)
}

func (tree *BxTree[T, S]) InsertAt(index int, item T) error {
	return tree.insert(index, []T{item})
}

func (tree *BxTree[T, S]) insert(index int, items []T) error {
	if len(items) == 0 {
		return nil
	}
	if index < 0 || index > tree.Size() {
		return ErrIndexOutOfBounds
	}

	for len(items) > 0 {
		var leaf *Node[T, S]
		var pos int

		if tree.Root == nil {
			leaf = &Node[T, S]{
				isLeaf: true,
			}
			tree.Root = leaf
			tree.First = leaf
			tree.Last = leaf
			pos = 0
		} else if index == tree.Size() {
			leaf, pos = tree.Last, tree.Last.size
		} else {
			var err error
			leaf, pos, err = tree.getAt(index)
			if err != nil {
				return err
			}
		}

		// How many items can we fit in this leaf without exceeding leafMaxSize?
		canFit := max(tree.leafMaxSize-len(leaf.Items), 1)
		canFit = min(canFit, len(items))

		// Insert items
		newItems := items[:canFit]
		oldLen := len(leaf.Items)
		leaf.Items = append(leaf.Items, make([]T, len(newItems))...)
		copy(leaf.Items[pos+len(newItems):], leaf.Items[pos:oldLen])
		copy(leaf.Items[pos:], newItems)

		leaf.addSizeUpward(len(newItems))

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
			leaf.AddSummaryUpward(totalDelta, tree)
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
	return tree.delete(index, length)
}

func (tree *BxTree[T, S]) DeleteAt(index int) error {
	return tree.delete(index, 1)
}

func (tree *BxTree[T, S]) delete(index int, length int) error {
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

		canDelete := min(length, leaf.size-pos)

		// Update summary before deleting
		if tree.SummaryConfig != nil {
			var totalDelta S
			for i := range canDelete {
				m := tree.SummaryConfig.FromItem(leaf.Items[pos+i])
				if i == 0 {
					totalDelta = m
				} else {
					totalDelta = tree.SummaryConfig.Add(totalDelta, m)
				}
			}
			leaf.AddSummaryUpward(tree.SummaryConfig.Sub(*new(S), totalDelta), tree)
		}

		// Delete items
		copy(leaf.Items[pos:], leaf.Items[pos+canDelete:])
		leaf.Items = leaf.Items[:len(leaf.Items)-canDelete]
		leaf.addSizeUpward(-canDelete)

		length -= canDelete
		tree.rebalance(leaf)
	}
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

func (n *Node[T, S]) AddSummaryUpward(delta S, tree *BxTree[T, S]) {
	if tree.SummaryConfig == nil {
		return
	}
	curr := n
	for curr != nil {
		curr.Summary = tree.SummaryConfig.Add(curr.Summary, delta)
		curr = curr.parent
	}
}

func (n *Node[T, S]) addSizeUpward(delta int) {
	curr := n
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
