package bxtree

import (
	"fmt"
	"iter"
)

// WithSummarizer sets the summarizer for the tree. The summarizer is used to
// maintain tree-wide metadata summaries that can be used for efficient searching.
func WithSummarizer[T any, S any](s Summarizer[T, S]) Option[T, S] {
	return func(tree *BxTree[T, S]) {
		tree.summarizer = s
	}
}

// WithOnItemMoved sets the callback for when an item is moved to a new node.
// This is useful for external systems that need to track the current node of an item.
func WithOnItemMoved[T any, S any](f func(item T, node *Node[T, S])) Option[T, S] {
	return func(tree *BxTree[T, S]) {
		tree.onItemMoved = f
	}
}

// WithInternalNodeSize sets the minimum and maximum number of children for internal nodes.
// Larger sizes increase branching factor and reduce tree height but increase work per node.
func WithInternalNodeSize[T any, S any](min, max int) Option[T, S] {
	return func(tree *BxTree[T, S]) {
		tree.internalMinSize = min
		tree.internalMaxSize = max
	}
}

// WithLeafNodeSize sets the minimum and maximum number of items for leaf nodes.
// Larger sizes improve cache locality but increase the cost of insertions and deletions.
func WithLeafNodeSize[T any, S any](min, max int) Option[T, S] {
	return func(tree *BxTree[T, S]) {
		tree.leafMinSize = min
		tree.leafMaxSize = max
	}
}

func (tree *BxTree[T, S]) summarizeItems(items []T) S {
	var s S
	for i, item := range items {
		m := tree.summarizer.FromItem(item)
		if i == 0 {
			s = m
		} else {
			s = tree.summarizer.Add(s, m)
		}
	}
	return s
}

func (tree *BxTree[T, S]) onItemsMoved(node *Node[T, S], items []T) {
	if tree.onItemMoved == nil {
		return
	}
	for _, item := range items {
		tree.onItemMoved(item, node)
	}
}

// New creates a new empty BxTree with the provided options.
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

// NewFromSlice creates a new BxTree initialized with the provided items.
// The tree is built bottom-up for maximum efficiency.
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
			leaf.summary = tree.summarizeItems(leaf.items)
		}

		if prevLeaf != nil {
			prevLeaf.next = leaf
			leaf.prev = prevLeaf
		} else {
			tree.first = leaf
		}
		leaves = append(leaves, leaf)
		prevLeaf = leaf

		tree.onItemsMoved(leaf, leaf.items)
	}
	tree.last = prevLeaf

	// 2. Build internal nodes bottom-up
	var currentLevel []*Node[T, S] = leaves
	for len(currentLevel) > 1 {
		var nextLevel []*Node[T, S]
		children := currentLevel
		n := len(children)
		max := tree.internalMaxSize

		makeNode := func(start, end int) {
			nd := &Node[T, S]{
				isLeaf:   false,
				children: make([]*Node[T, S], end-start),
			}
			copy(nd.children, children[start:end])
			var size int
			var s S
			for j, child := range nd.children {
				child.parent = nd
				size += child.size
				if tree.summarizer != nil {
					if j == 0 {
						s = child.summary
					} else {
						s = tree.summarizer.Add(s, child.summary)
					}
				}
			}
			nd.size = size
			nd.summary = s
			nextLevel = append(nextLevel, nd)
		}

		// Group children into internal nodes of at most max each. Delete
		// rebalancing merges sibling pairs and cannot handle a non-root
		// internal node with a single child, so when the tail would be one
		// lone child, shrink the previous group to max-1 so the tail holds
		// two children instead.
		start := 0
		for n-start > max {
			end := start + max
			if n-end == 1 {
				end--
			}
			makeNode(start, end)
			start = end
		}
		makeNode(start, n)
		currentLevel = nextLevel
	}

	tree.root = currentLevel[0]
	return tree
}

// Size returns the total number of items in the tree.
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

// ForEach calls f for every item in the tree in order.
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

// Print debug prints the structure of the tree to stdout.
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

// GetAt returns a pointer to the item at the specified 0-based index.
// Returns ErrIndexOutOfBounds if the index is out of range.
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

// GetAtNode returns the leaf node containing the item at the specified index,
// along with the item's position within that node.
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
			panic(fmt.Sprintf("bxtree: index %d not found in internal node during traversal", index))
		}
	}
	return curr, index, nil
}

// IsLeaf returns true if this node is a leaf node.
func (n *Node[T, S]) IsLeaf() bool {
	if n == nil {
		panic("bxtree: IsLeaf called on nil node")
	}
	return n.isLeaf
}

// Next returns the next leaf node in the sequence, or nil if this is the last node.
// Only valid for leaf nodes.
func (n *Node[T, S]) Next() *Node[T, S] {
	if n == nil {
		panic("bxtree: Next called on nil node")
	}
	return n.next
}

// Prev returns the previous leaf node in the sequence, or nil if this is the first node.
// Only valid for leaf nodes.
func (n *Node[T, S]) Prev() *Node[T, S] {
	if n == nil {
		panic("bxtree: Prev called on nil node")
	}
	return n.prev
}

// Parent returns the parent of this node, or nil if this is the root.
func (n *Node[T, S]) Parent() *Node[T, S] {
	if n == nil {
		panic("bxtree: Parent called on nil node")
	}
	return n.parent
}

// Children returns the child nodes of this internal node.
// Returns nil for leaf nodes.
func (n *Node[T, S]) Children() []*Node[T, S] {
	if n == nil {
		panic("bxtree: Children called on nil node")
	}
	return n.children
}

// InsertRange inserts a slice of items at the specified index.
func (tree *BxTree[T, S]) InsertRange(index int, items []T) error {
	if tree == nil {
		panic("bxtree: InsertRange called on nil tree")
	}
	return tree.insert(index, items)
}

// InsertAt inserts a single item at the specified index.
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
			leaf.summary = tree.summarizeItems(newItems)
		}

		tree.onItemsMoved(leaf, leaf.items)
		return nil
	}

	if index == tree.Size() {
		leaf := tree.last
		pos := len(leaf.items)
		oldLen := len(leaf.items)
		leaf.items = append(leaf.items, newItems...)
		copy(leaf.items[pos+len(newItems):], leaf.items[pos:oldLen])
		copy(leaf.items[pos:], newItems)

		var deltaSummary S
		if tree.summarizer != nil {
			deltaSummary = tree.summarizeItems(newItems)
		}
		leaf.addUpward(len(newItems), deltaSummary, tree)

		tree.onItemsMoved(leaf, newItems)

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

	var deltaSummary S
	if tree.summarizer != nil {
		deltaSummary = tree.summarizeItems(newItems)
	}
	leaf.addUpward(len(newItems), deltaSummary, tree)

	tree.onItemsMoved(leaf, newItems)

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
		panic("bxtree: split node must have a parent")
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
			rightSummary := tree.summarizeItems(right.items)
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

		tree.onItemsMoved(right, right.items)
	} else {
		mid := len(n.children) / 2
		right.children = make([]*Node[T, S], len(n.children)-mid)
		copy(right.children, n.children[mid:])

		n.size = 0
		right.size = 0

		if tree.summarizer != nil {
			for j, child := range n.children[:mid] {
				n.size += child.size
				if j == 0 {
					n.summary = child.summary
				} else {
					n.summary = tree.summarizer.Add(n.summary, child.summary)
				}
			}
			for j, child := range right.children {
				child.parent = right
				right.size += child.size
				if j == 0 {
					right.summary = child.summary
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

// DeleteRange removes length items starting from the specified index.
func (tree *BxTree[T, S]) DeleteRange(index int, length int) error {
	if tree == nil {
		panic("bxtree: DeleteRange called on nil tree")
	}
	return tree.delete(index, length)
}

// DeleteAt removes the item at the specified index.
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
		var deltaSummary S
		if tree.summarizer != nil {
			deletedItems := leaf.items[pos : pos+canDelete]
			totalDelta := tree.summarizeItems(deletedItems)
			deltaSummary = tree.summarizer.Sub(S(*new(S)), totalDelta)
		}
		leaf.addUpward(-canDelete, deltaSummary, tree)

		leaf.items = append(leaf.items[:pos], leaf.items[pos+canDelete:]...)

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

				tree.onItemsMoved(n, []T{item})
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

				tree.onItemsMoved(n, []T{item})
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
		panic("bxtree: cannot merge nodes without a parent")
	}
	if left.isLeaf {
		left.items = append(left.items, right.items...)
		left.size = len(left.items)

		tree.onItemsMoved(left, right.items)

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

func (n *Node[T, S]) addUpward(deltaSize int, deltaSummary S, tree *BxTree[T, S]) {
	if tree == nil {
		panic("bxtree: addUpward called with nil tree")
	}
	curr := n
	for curr != nil {
		curr.size += deltaSize
		if tree.summarizer != nil {
			curr.summary = tree.summarizer.Add(curr.summary, deltaSummary)
		}
		curr = curr.parent
	}
}

// SummaryAddUpward adds the delta to the summary of this node and all of its ancestors.
func (n *Node[T, S]) SummaryAddUpward(delta S, tree *BxTree[T, S]) {
	if n == nil {
		panic("bxtree: SummaryAddUpward called on nil node")
	}
	if tree == nil {
		panic("bxtree: SummaryAddUpward called with nil tree")
	}
	curr := n
	for curr != nil {
		if tree.summarizer != nil {
			curr.summary = tree.summarizer.Add(curr.summary, delta)
		}
		curr = curr.parent
	}
}

// UpdateSummary recomputes the summary for this node based on its children or items.
func (n *Node[T, S]) UpdateSummary(tree *BxTree[T, S]) {
	if tree.summarizer == nil {
		return
	}
	if n.isLeaf {
		n.summary = tree.summarizeItems(n.items)
	} else {
		var s S
		for i, child := range n.children {
			if i == 0 {
				s = child.summary
			} else {
				s = tree.summarizer.Add(s, child.summary)
			}
		}
		n.summary = s
	}
}

// UpdateSummaryUpward recomputes the summary for this node and all its ancestors.
func (n *Node[T, S]) UpdateSummaryUpward(tree *BxTree[T, S]) {
	curr := n
	for curr != nil {
		curr.UpdateSummary(tree)
		curr = curr.parent
	}
}

// SummaryBefore returns the accumulated summary of all items before this node.
func (n *Node[T, S]) SummaryBefore(tree *BxTree[T, S]) S {
	if n == nil {
		panic("bxtree: SummaryBefore called on nil node")
	}
	var s S
	first := true

	curr := n
	for curr.parent != nil {
		parent := curr.parent
		for _, child := range parent.children {
			if child == curr {
				break
			}
			if first {
				s = child.summary
				first = false
			} else {
				s = tree.summarizer.Add(s, child.summary)
			}
		}
		curr = parent
	}
	return s
}

func (n *Node[T, S]) getParentIndex() int {
	if n == nil {
		panic("bxtree: getParentIndex called on nil node")
	}
	if n.parent == nil {
		panic("bxtree: getParentIndex called on root node")
	}
	for i, child := range n.parent.children {
		if child == n {
			return i
		}
	}
	panic("bxtree: node not found in parent's children")
}

// Index returns the absolute 0-based index of the first item in this node.
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

// FindPath navigates the tree using a predicate on accumulated summaries.
//
// It returns the leaf node, the position within that leaf, and the accumulated
// summary value just before the matching item.
//
// The predicate is called with (accumulated summary, current node summary).
// It should return true when the search criteria is met.
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
		panic("bxtree: FindPath called on tree without Summarizer")
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
			panic("bxtree: FindPath failed to find child matching predicate")
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
