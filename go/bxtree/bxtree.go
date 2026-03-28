package bxtree

import (
	"fmt"
	"iter"
)

// WithSummarizer sets the summarizer for the tree.
func WithSummarizer[T any, S any](s Summarizer[T, S]) Option[T, S] {
	return func(tree *BxTree[T, S]) {
		tree.summarizer = s
	}
}

// WithOnItemMoved sets the callback for when an item is moved to a new node.
func WithOnItemMoved[T any, S any](f func(item T, node *Node[T, S])) Option[T, S] {
	return func(tree *BxTree[T, S]) {
		tree.onItemMoved = f
	}
}

// WithInternalNodeSize sets the minimum and maximum number of children for internal nodes.
func WithInternalNodeSize[T any, S any](min, max int) Option[T, S] {
	return func(tree *BxTree[T, S]) {
		tree.internalMinSize = min
		tree.internalMaxSize = max
	}
}

// WithLeafNodeSize sets the minimum and maximum number of items for leaf nodes.
func WithLeafNodeSize[T any, S any](min, max int) Option[T, S] {
	return func(tree *BxTree[T, S]) {
		tree.leafMinSize = min
		tree.leafMaxSize = max
	}
}

func New[T any, S any](opts ...Option[T, S]) *BxTree[T, S] {
	tree := &BxTree[T, S]{
		internalMinSize: DefaultInternalMinSize,
		internalMaxSize: DefaultInternalMaxSize,
		leafMinSize:     DefaultLeafMinSize,
		leafMaxSize:     DefaultLeafMaxSize,
	}
	for _, opt := range opts {
		opt(tree)
	}
	return tree
}

func NewFromSlice[T any, S any](items []T, opts ...Option[T, S]) *BxTree[T, S] {
	tree := New(opts...)

	if len(items) == 0 {
		return tree
	}

	// 1. Create all leaf nodes
	var leaves []*Node[T, S]
	var prevLeaf *Node[T, S]

	for i := 0; i < len(items); i += tree.leafMaxSize {
		end := min(i+tree.leafMaxSize, len(items))

		leaf := &Node[T, S]{
			isLeaf: true,
			items:  make([]T, end-i),
			size:   end - i,
		}
		copy(leaf.items, items[i:end])

		if tree.summarizer != nil {
			var s S
			for j, item := range leaf.items {
				m := tree.summarizer.FromItem(item)
				if j == 0 {
					s = m
				} else {
					s = tree.summarizer.Add(s, m)
				}
			}
			leaf.summary = s
		}

		if prevLeaf != nil {
			prevLeaf.next = leaf
			leaf.prev = prevLeaf
		} else {
			tree.first = leaf
		}
		leaves = append(leaves, leaf)
		prevLeaf = leaf

		if tree.onItemMoved != nil {
			for _, item := range leaf.items {
				tree.onItemMoved(item, leaf)
			}
		}
	}
	tree.last = prevLeaf

	// 2. Build internal nodes bottom-up
	var currentLevel []*Node[T, S] = leaves
	for len(currentLevel) > 1 {
		var nextLevel []*Node[T, S]
		for i := 0; i < len(currentLevel); i += tree.internalMaxSize {
			end := min(i+tree.internalMaxSize, len(currentLevel))

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
				if tree.summarizer != nil {
					if first {
						s = child.summary
						first = false
					} else {
						s = tree.summarizer.Add(s, child.summary)
					}
				}
			}
			n.size = size
			n.summary = s
			nextLevel = append(nextLevel, n)
		}
		currentLevel = nextLevel
	}

	tree.root = currentLevel[0]
	return tree
}

func (tree *BxTree[T, S]) Size() int {
	if tree == nil {
		panic("bxtree: Size called on nil tree")
	}
	if tree.root == nil {
		return 0
	}
	return tree.root.size
}

// All returns an iterator over all items in the tree in order.
func (tree *BxTree[T, S]) All() iter.Seq[T] {
	if tree == nil {
		panic("bxtree: All called on nil tree")
	}
	return func(yield func(T) bool) {
		curr := tree.first
		for curr != nil {
			for _, item := range curr.items {
				if !yield(item) {
					return
				}
			}
			curr = curr.next
		}
	}
}

// Reverse returns an iterator over all items in the tree in reverse order.
func (tree *BxTree[T, S]) Reverse() iter.Seq[T] {
	if tree == nil {
		panic("bxtree: Reverse called on nil tree")
	}
	return func(yield func(T) bool) {
		curr := tree.last
		for curr != nil {
			for i := len(curr.items) - 1; i >= 0; i-- {
				if !yield(curr.items[i]) {
					return
				}
			}
			curr = curr.prev
		}
	}
}

func (tree *BxTree[T, S]) ForEach(f func(item T)) {
	if tree == nil {
		panic("bxtree: ForEach called on nil tree")
	}
	if f == nil {
		panic("bxtree: ForEach called with nil function")
	}
	for item := range tree.All() {
		f(item)
	}
}

func (tree *BxTree[T, S]) Print() {
	if tree == nil {
		panic("bxtree: Print called on nil tree")
	}
	if tree.root != nil {
		fmt.Printf("Tree size: %d\n", tree.root.size)
		tree.root.printTree(0)
	} else {
		fmt.Println("Empty tree")
	}
}

func (n *Node[T, S]) printTree(level int) {
	if n == nil {
		panic("bxtree: printTree called on nil node")
	}
	prefix := ""
	for range level {
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

func (tree *BxTree[T, S]) GetAt(index int) (*T, error) {
	if tree == nil {
		panic("bxtree: GetAt called on nil tree")
	}
	leaf, pos, err := tree.getAt(index)
	if err != nil {
		return nil, err
	}
	return &leaf.items[pos], nil
}

func (tree *BxTree[T, S]) GetAtNode(index int) (*Node[T, S], int, error) {
	if tree == nil {
		panic("bxtree: GetAtNode called on nil tree")
	}
	return tree.getAt(index)
}

func (tree *BxTree[T, S]) getAt(index int) (*Node[T, S], int, error) {
	if tree == nil {
		panic("bxtree: getAt called on nil tree")
	}
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
			panic(fmt.Sprintf("internal inconsistency: index %d not found in internal node during traversal", index))
		}
	}
	return curr, index, nil
}

func (n *Node[T, S]) IsLeaf() bool {
	if n == nil {
		panic("bxtree: IsLeaf called on nil node")
	}
	return n.isLeaf
}

func (n *Node[T, S]) Next() *Node[T, S] {
	if n == nil {
		panic("bxtree: Next called on nil node")
	}
	return n.next
}

func (n *Node[T, S]) Prev() *Node[T, S] {
	if n == nil {
		panic("bxtree: Prev called on nil node")
	}
	return n.prev
}

func (n *Node[T, S]) Parent() *Node[T, S] {
	if n == nil {
		panic("bxtree: Parent called on nil node")
	}
	return n.parent
}

func (n *Node[T, S]) Children() []*Node[T, S] {
	if n == nil {
		panic("bxtree: Children called on nil node")
	}
	return n.children
}

func (tree *BxTree[T, S]) InsertRange(index int, items []T) error {
	if tree == nil {
		panic("bxtree: InsertRange called on nil tree")
	}
	return tree.insert(index, items)
}

func (tree *BxTree[T, S]) InsertAt(index int, item T) error {
	if tree == nil {
		panic("bxtree: InsertAt called on nil tree")
	}
	return tree.insert(index, []T{item})
}

func (tree *BxTree[T, S]) insert(index int, newItems []T) error {
	if tree == nil {
		panic("bxtree: insert called on nil tree")
	}
	if tree.root == nil {
		if index != 0 {
			return ErrIndexOutOfBounds
		}
		leaf := &Node[T, S]{
			isLeaf: true,
			items:  make([]T, len(newItems)),
			size:   len(newItems),
		}
		copy(leaf.items, newItems)
		tree.root = leaf
		tree.first = leaf
		tree.last = leaf

		if tree.summarizer != nil {
			var s S
			for i, item := range newItems {
				m := tree.summarizer.FromItem(item)
				if i == 0 {
					s = m
				} else {
					s = tree.summarizer.Add(s, m)
				}
			}
			leaf.summary = s
		}

		if tree.onItemMoved != nil {
			for _, item := range leaf.items {
				tree.onItemMoved(item, leaf)
			}
		}
		return nil
	}

	if index == tree.Size() {
		leaf := tree.last
		pos := len(leaf.items)
		oldLen := len(leaf.items)
		leaf.items = append(leaf.items, newItems...)
		copy(leaf.items[pos+len(newItems):], leaf.items[pos:oldLen])
		copy(leaf.items[pos:], newItems)

		leaf.sizeAddUpward(len(newItems))

		if tree.summarizer != nil {
			var totalDelta S
			for i, item := range newItems {
				m := tree.summarizer.FromItem(item)
				if i == 0 {
					totalDelta = m
				} else {
					totalDelta = tree.summarizer.Add(totalDelta, m)
				}
			}
			leaf.SummaryAddUpward(totalDelta, tree)
		}

		if tree.onItemMoved != nil {
			for _, item := range newItems {
				tree.onItemMoved(item, leaf)
			}
		}

		if len(leaf.items) > tree.leafMaxSize {
			tree.split(leaf)
		}
		return nil
	}

	leaf, pos, err := tree.getAt(index)
	if err != nil {
		return err
	}

	oldLen := len(leaf.items)
	leaf.items = append(leaf.items, newItems...)
	copy(leaf.items[pos+len(newItems):], leaf.items[pos:oldLen])
	copy(leaf.items[pos:], newItems)

	leaf.sizeAddUpward(len(newItems))

	if tree.summarizer != nil {
		var totalDelta S
		for i, item := range newItems {
			m := tree.summarizer.FromItem(item)
			if i == 0 {
				totalDelta = m
			} else {
				totalDelta = tree.summarizer.Add(totalDelta, m)
			}
		}
		leaf.SummaryAddUpward(totalDelta, tree)
	}

	if tree.onItemMoved != nil {
		for _, item := range newItems {
			tree.onItemMoved(item, leaf)
		}
	}

	if len(leaf.items) > tree.leafMaxSize {
		tree.split(leaf)
	}

	return nil
}

func (tree *BxTree[T, S]) split(n *Node[T, S]) {
	if n == nil {
		panic("bxtree: split called with nil node")
	}
	if n.parent == nil {
		newRoot := &Node[T, S]{
			isLeaf: false,
			size:   n.size,
		}
		if tree.summarizer != nil {
			newRoot.summary = n.summary
		}
		tree.root = newRoot
		n.parent = newRoot
		newRoot.children = []*Node[T, S]{n}
	}

	if n.parent == nil {
		panic("internal inconsistency: split node must have a parent")
	}

	right := &Node[T, S]{
		isLeaf: n.isLeaf,
		parent: n.parent,
	}

	if n.isLeaf {
		mid := len(n.items) / 2
		right.items = make([]T, len(n.items)-mid)
		copy(right.items, n.items[mid:])

		if tree.summarizer != nil {
			var rightSummary S
			for i, item := range right.items {
				m := tree.summarizer.FromItem(item)
				if i == 0 {
					rightSummary = m
				} else {
					rightSummary = tree.summarizer.Add(rightSummary, m)
				}
			}
			right.summary = rightSummary
			n.summary = tree.summarizer.Sub(n.summary, rightSummary)
		}

		n.items = n.items[:mid]
		n.size = len(n.items)
		right.size = len(right.items)

		right.next = n.next
		n.next = right
		right.prev = n
		if right.next != nil {
			right.next.prev = right
		}
		if tree.last == n {
			tree.last = right
		}

		if tree.onItemMoved != nil {
			for _, item := range right.items {
				tree.onItemMoved(item, right)
			}
		}
	} else {
		mid := len(n.children) / 2
		right.children = make([]*Node[T, S], len(n.children)-mid)
		copy(right.children, n.children[mid:])

		n.size = 0
		right.size = 0

		if tree.summarizer != nil {
			firstN := true
			firstR := true

			for _, child := range n.children[:mid] {
				n.size += child.size
				if firstN {
					n.summary = child.summary
					firstN = false
				} else {
					n.summary = tree.summarizer.Add(n.summary, child.summary)
				}
			}
			for _, child := range right.children {
				child.parent = right
				right.size += child.size
				if firstR {
					right.summary = child.summary
					firstR = false
				} else {
					right.summary = tree.summarizer.Add(right.summary, child.summary)
				}
			}
		} else {

			for _, child := range n.children[:mid] {
				n.size += child.size
			}
			for _, child := range right.children {
				child.parent = right
				right.size += child.size
			}
		}
		n.children = n.children[:mid]
	}

	parent := n.parent
	idx := n.getParentIndex()
	parent.children = append(parent.children, nil)
	copy(parent.children[idx+2:], parent.children[idx+1:])
	parent.children[idx+1] = right

	if len(parent.children) > tree.internalMaxSize {
		tree.split(parent)
	}
}

func (tree *BxTree[T, S]) DeleteRange(index int, length int) error {
	if tree == nil {
		panic("bxtree: DeleteRange called on nil tree")
	}
	return tree.delete(index, length)
}

func (tree *BxTree[T, S]) DeleteAt(index int) error {
	if tree == nil {
		panic("bxtree: DeleteAt called on nil tree")
	}
	return tree.delete(index, 1)
}

func (tree *BxTree[T, S]) delete(index int, length int) error {
	if tree == nil {
		panic("bxtree: delete called on nil tree")
	}
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
		if tree.summarizer != nil {
			var totalDelta S
			for i := range canDelete {
				m := tree.summarizer.FromItem(leaf.items[pos+i])
				if i == 0 {
					totalDelta = m
				} else {
					totalDelta = tree.summarizer.Add(totalDelta, m)
				}
			}
			leaf.SummaryAddUpward(tree.summarizer.Sub(S(*new(S)), totalDelta), tree)
		}

		leaf.sizeAddUpward(-canDelete)
		leaf.items = append(leaf.items[:pos], leaf.items[pos+canDelete:]...)
		leaf.size = len(leaf.items)

		length -= canDelete

		if tree.root.size == 0 {
			tree.root = nil
			tree.first = nil
			tree.last = nil
		} else if leaf.size < tree.leafMinSize && leaf.parent != nil {
			tree.rebalance(leaf)
		}
	}

	return nil
}

func (tree *BxTree[T, S]) rebalance(n *Node[T, S]) {
	if n == nil {
		panic("bxtree: rebalance called with nil node")
	}
	if n.parent == nil {
		if !n.isLeaf && len(n.children) == 1 {
			tree.root = n.children[0]
			tree.root.parent = nil
		}
		return
	}

	parent := n.parent
	idx := n.getParentIndex()

	if n.isLeaf {
		if idx > 0 {
			left := parent.children[idx-1]
			if len(left.items) > tree.leafMinSize {
				// Borrow from left
				item := left.items[len(left.items)-1]
				left.items = left.items[:len(left.items)-1]
				left.size--
				n.items = append([]T{item}, n.items...)
				n.size++

				if tree.summarizer != nil {
					m := tree.summarizer.FromItem(item)
					left.summary = tree.summarizer.Sub(left.summary, m)
					n.summary = tree.summarizer.Add(n.summary, m)
				}

				if tree.onItemMoved != nil {
					tree.onItemMoved(item, n)
				}
				return
			}
		}
		if idx < len(parent.children)-1 {
			right := parent.children[idx+1]
			if len(right.items) > tree.leafMinSize {
				// Borrow from right
				item := right.items[0]
				right.items = right.items[1:]
				right.size--
				n.items = append(n.items, item)
				n.size++

				if tree.summarizer != nil {
					m := tree.summarizer.FromItem(item)
					right.summary = tree.summarizer.Sub(right.summary, m)
					n.summary = tree.summarizer.Add(n.summary, m)
				}

				if tree.onItemMoved != nil {
					tree.onItemMoved(item, n)
				}
				return
			}
		}
	} else {
		if idx > 0 {
			left := parent.children[idx-1]
			if len(left.children) > tree.internalMinSize {
				child := left.children[len(left.children)-1]
				left.children = left.children[:len(left.children)-1]
				left.size -= child.size
				n.children = append([]*Node[T, S]{child}, n.children...)
				n.size += child.size
				child.parent = n

				if tree.summarizer != nil {
					left.summary = tree.summarizer.Sub(left.summary, child.summary)
					n.summary = tree.summarizer.Add(n.summary, child.summary)
				}

				return
			}
		}
		if idx < len(parent.children)-1 {
			right := parent.children[idx+1]
			if len(right.children) > tree.internalMinSize {
				child := right.children[0]
				right.children = right.children[1:]
				right.size -= child.size
				n.children = append(n.children, child)
				n.size += child.size
				child.parent = n

				if tree.summarizer != nil {
					right.summary = tree.summarizer.Sub(right.summary, child.summary)
					n.summary = tree.summarizer.Add(n.summary, child.summary)
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
	if left == nil {
		panic("bxtree: merge called with nil left node")
	}
	if right == nil {
		panic("bxtree: merge called with nil right node")
	}
	parent := left.parent
	if parent == nil {
		panic("internal inconsistency: cannot merge nodes without a parent")
	}
	if left.isLeaf {
		if tree.onItemMoved != nil {
			for _, item := range right.items {
				tree.onItemMoved(item, left)
			}
		}
		left.items = append(left.items, right.items...)
		left.size = len(left.items)
		if tree.summarizer != nil {
			left.summary = tree.summarizer.Add(left.summary, right.summary)
		}
		left.next = right.next
		if left.next != nil {
			left.next.prev = left
		}
		if tree.last == right {
			tree.last = left
		}
	} else {
		for _, child := range right.children {
			child.parent = left
			left.children = append(left.children, child)
			left.size += child.size
			if tree.summarizer != nil {
				left.summary = tree.summarizer.Add(left.summary, child.summary)
			}
		}
	}

	idx := right.getParentIndex()
	copy(parent.children[idx:], parent.children[idx+1:])
	parent.children = parent.children[:len(parent.children)-1]

	tree.rebalance(parent)
}

func (n *Node[T, S]) SummaryAddUpward(delta S, tree *BxTree[T, S]) {
	if n == nil {
		panic("bxtree: SummaryAddUpward called on nil node")
	}
	if tree == nil {
		panic("bxtree: SummaryAddUpward called with nil tree")
	}
	if tree.summarizer == nil {
		return
	}
	curr := n
	for curr != nil {
		curr.summary = tree.summarizer.Add(curr.summary, delta)
		curr = curr.parent
	}
}

func (n *Node[T, S]) sizeAddUpward(delta int) {
	if n == nil {
		panic("bxtree: sizeAddUpward called on nil node")
	}
	curr := n
	for curr != nil {
		curr.size += delta
		curr = curr.parent
	}
}

func (n *Node[T, S]) getParentIndex() int {
	if n == nil {
		panic("bxtree: getParentIndex called on nil node")
	}
	if n.parent == nil {
		panic("getParentIndex called on root node")
	}
	for i, child := range n.parent.children {
		if child == n {
			return i
		}
	}
	panic("node not found in parent's children")
}

func (n *Node[T, S]) Index() int {
	if n == nil {
		panic("bxtree: Index called on nil node")
	}
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

func (tree *BxTree[T, S]) FindPath(predicate func(acc S, cur S) bool) (*Node[T, S], int, S) {
	if tree == nil {
		panic("bxtree: FindPath called on nil tree")
	}
	if predicate == nil {
		panic("bxtree: FindPath called with nil predicate")
	}
	if tree.root == nil {
		return nil, -1, *new(S)
	}

	if tree.summarizer == nil {
		panic("FindPath called on tree without Summarizer")
	}

	var acc S
	first := true
	curr := tree.root

	for !curr.isLeaf {
		found := false
		for _, child := range curr.children {
			var nextAcc S
			if first {
				nextAcc = child.summary
			} else {
				nextAcc = tree.summarizer.Add(acc, child.summary)
			}

			if predicate(acc, child.summary) {
				curr = child
				found = true
				break
			}
			acc = nextAcc
			first = false
		}
		if !found {
			panic("internal inconsistency: FindPath failed to find child matching predicate")
		}
	}

	// In leaf
	for i, item := range curr.items {
		m := tree.summarizer.FromItem(item)
		if predicate(acc, m) {
			return curr, i, acc
		}
		if first {
			acc = m
			first = false
		} else {
			acc = tree.summarizer.Add(acc, m)
		}
	}

	return nil, -1, acc
}
