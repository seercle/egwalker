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
//
// Supported sizes require min >= 2, min <= max, and max >= 2*min-1 so that
// split/merge can keep every non-root node within [min, max]. New returns an
// *InvalidNodeSizeError for any other configuration.
func WithInternalNodeSize[T any, S any](min, max int) Option[T, S] {
	return func(tree *BxTree[T, S]) {
		tree.internalMinSize = min
		tree.internalMaxSize = max
	}
}

// WithLeafNodeSize sets the minimum and maximum number of items for leaf nodes.
// Larger sizes improve cache locality but increase the cost of insertions and deletions.
//
// Supported sizes require min >= 1, min <= max, and max >= 2*min-1 so that
// split/merge can keep every non-root node within [min, max]. New returns an
// *InvalidNodeSizeError for any other configuration.
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
// It returns an error if the configured node sizes are unsupported; see
// WithLeafNodeSize and WithInternalNodeSize for the supported envelope.
func New[T any, S any](opts ...Option[T, S]) (*BxTree[T, S], error) {
	tree := &BxTree[T, S]{
		internalMinSize: DefaultInternalMinSize,
		internalMaxSize: DefaultInternalMaxSize,
		leafMinSize:     DefaultLeafMinSize,
		leafMaxSize:     DefaultLeafMaxSize,
	}
	for _, opt := range opts {
		opt(tree)
	}
	if err := validateNodeSizes(tree); err != nil {
		return nil, err
	}
	return tree, nil
}

// validateNodeSizes checks that the configured leaf/internal node sizes are
// within the envelope the tree's split, borrow, and merge operations can
// maintain. Every non-root node must stay within [min, max]; split divides an
// overflowing max+1 node into two halves (each >= min) and merge combines at
// most (min-1)+min items/children, so both require max >= 2*min-1. Internal
// nodes additionally need min >= 2: a non-root internal node with a single
// child cannot be rebalanced by the delete path.
func validateNodeSizes[T any, S any](tree *BxTree[T, S]) error {
	if tree.leafMinSize < 1 {
		return &InvalidNodeSizeError{Kind: "leaf", Min: tree.leafMinSize, Max: tree.leafMaxSize, Reason: "min must be >= 1"}
	}
	if tree.leafMaxSize < tree.leafMinSize {
		return &InvalidNodeSizeError{Kind: "leaf", Min: tree.leafMinSize, Max: tree.leafMaxSize, Reason: "max must be >= min"}
	}
	if tree.leafMaxSize < 2*tree.leafMinSize-1 {
		return &InvalidNodeSizeError{Kind: "leaf", Min: tree.leafMinSize, Max: tree.leafMaxSize, Reason: "max must be >= 2*min-1"}
	}
	if tree.internalMinSize < 2 {
		return &InvalidNodeSizeError{Kind: "internal", Min: tree.internalMinSize, Max: tree.internalMaxSize, Reason: "min must be >= 2"}
	}
	if tree.internalMaxSize < tree.internalMinSize {
		return &InvalidNodeSizeError{Kind: "internal", Min: tree.internalMinSize, Max: tree.internalMaxSize, Reason: "max must be >= min"}
	}
	if tree.internalMaxSize < 2*tree.internalMinSize-1 {
		return &InvalidNodeSizeError{Kind: "internal", Min: tree.internalMinSize, Max: tree.internalMaxSize, Reason: "max must be >= 2*min-1"}
	}
	return nil
}

// NewFromSlice creates a new BxTree initialized with the provided items.
// The tree is built bottom-up for maximum efficiency. Items are distributed
// across each level so every non-root node holds between its configured min
// and max; only the (single-node) root may hold fewer than the minimum.
// It returns an error if the configured node sizes are unsupported; see
// WithLeafNodeSize and WithInternalNodeSize for the supported envelope.
func NewFromSlice[T any, S any](items []T, opts ...Option[T, S]) (*BxTree[T, S], error) {
	tree, err := New(opts...)
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return tree, nil
	}

	// splitSizes distributes total units across k = ceil(total/hi) sizes that
	// sum to total and are as equal as possible (base = total/k, with the first
	// total%k at base+1). Whenever k > 1 the validated envelope hi >= 2*lo-1
	// guarantees every size is within [lo, hi]; a single size (k == 1) is the
	// future root, which is exempt from the minimum.
	splitSizes := func(total, lo, hi int) []int {
		k := (total + hi - 1) / hi
		sizes := make([]int, k)
		base := total / k
		extra := total % k
		for i := range sizes {
			sizes[i] = base
			if i < extra {
				sizes[i]++
			}
		}
		return sizes
	}

	// 1. Create all leaf nodes
	var leaves []*Node[T, S]
	var prevLeaf *Node[T, S]

	start := 0
	for _, sz := range splitSizes(len(items), tree.leafMinSize, tree.leafMaxSize) {
		end := start + sz

		leaf := &Node[T, S]{
			isLeaf: true,
			items:  make([]T, sz),
			size:   sz,
		}
		copy(leaf.items, items[start:end])

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
		start = end
	}
	tree.last = prevLeaf

	// 2. Build internal nodes bottom-up
	var currentLevel []*Node[T, S] = leaves
	for len(currentLevel) > 1 {
		var nextLevel []*Node[T, S]
		children := currentLevel

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

		// Distribute this level's children across internal nodes within
		// [internalMin, internalMax]. The envelope guarantees no non-root
		// internal node underflows (in particular none holds a single child),
		// so delete rebalancing can always operate on the result.
		start := 0
		for _, sz := range splitSizes(len(children), tree.internalMinSize, tree.internalMaxSize) {
			makeNode(start, start+sz)
			start += sz
		}
		currentLevel = nextLevel
	}

	tree.root = currentLevel[0]
	return tree, nil
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

	// Rebalancing only applies when this node has dropped below its minimum
	// occupancy (the root's single-child collapse is handled above). Without
	// this guard, a healthy parent (still >= its minimum) would be merged with
	// a sibling after a child merge, over-filling an internal node past its
	// internalMaxSize.
	if n.isLeaf {
		if len(n.items) >= tree.leafMinSize {
			return
		}
	} else {
		if len(n.children) >= tree.internalMinSize {
			return
		}
	}

	parent := n.parent
	idx := n.getParentIndex()

	count := func(nd *Node[T, S]) int {
		if nd.isLeaf {
			return len(nd.items)
		}
		return len(nd.children)
	}
	lo, hi := tree.leafMinSize, tree.leafMaxSize
	if !n.isLeaf {
		lo, hi = tree.internalMinSize, tree.internalMaxSize
	}

	// Prefer the richer neighbour (more items/children), so a redistribution
	// raises the underfull node as much as possible.
	var nb *Node[T, S]
	if idx > 0 {
		nb = parent.children[idx-1]
	}
	if idx+1 < len(parent.children) && (nb == nil || count(parent.children[idx+1]) > count(nb)) {
		nb = parent.children[idx+1]
	}

	// If the two nodes can be split so both hold at least the minimum, move
	// items/children from the neighbour into the underfull node until the pair
	// is as evenly filled as the min/max envelope allows. Otherwise the only
	// option is to merge them (merged size < 2*lo <= hi, so it never overfills).
	if count(n)+count(nb) >= 2*lo {
		target := (count(n) + count(nb) + 1) / 2
		if target > hi {
			target = hi
		}
		if n.isLeaf {
			tree.redistributeLeaves(n, nb, target-len(n.items))
		} else {
			tree.redistributeChildren(n, nb, target-len(n.children))
		}
		return
	}

	// Merge the underfull node with its richer neighbour. The left node
	// survives (it absorbs the right one), preserving the leaf chain.
	if idx > 0 && nb.getParentIndex() < idx {
		tree.merge(nb, n)
	} else {
		tree.merge(n, nb)
	}
}

// redistributeLeaves moves `move` items from leaf nb (the richer neighbour)
// into leaf n (the underfull node), keeping both siblings within the tree's
// [min, max] envelope. Items move across the shared boundary only, so global
// order is preserved. The leaf chain is untouched: both leaves survive.
func (tree *BxTree[T, S]) redistributeLeaves(n, nb *Node[T, S], move int) {
	if move <= 0 {
		return
	}

	var moved []T
	if nb.getParentIndex() < n.getParentIndex() {
		// nb is immediately left of n: take nb's trailing items into n's front.
		cut := len(nb.items) - move
		moved = append([]T(nil), nb.items[cut:]...)
		nb.items = nb.items[:cut:cut]
		n.items = append(moved, n.items...)
	} else {
		// nb is immediately right of n: take nb's leading items onto n's back.
		moved = append([]T(nil), nb.items[:move]...)
		nb.items = nb.items[move:]
		n.items = append(n.items, moved...)
	}
	n.size = len(n.items)
	nb.size = len(nb.items)

	if tree.summarizer != nil {
		n.summary = tree.summarizeItems(n.items)
		nb.summary = tree.summarizeItems(nb.items)
	}
	tree.onItemsMoved(n, moved)
}

// redistributeChildren moves `move` child subtrees from internal node nb (the
// richer neighbour) into internal node n (the underfull node), keeping both
// within the tree's [min, max] envelope. Children cross the shared boundary
// only, so key order is preserved.
func (tree *BxTree[T, S]) redistributeChildren(n, nb *Node[T, S], move int) {
	if move <= 0 {
		return
	}

	var moved []*Node[T, S]
	if nb.getParentIndex() < n.getParentIndex() {
		// nb is immediately left of n: take nb's trailing children into n's front.
		cut := len(nb.children) - move
		moved = append([]*Node[T, S](nil), nb.children[cut:]...)
		nb.children = nb.children[:cut:cut]
		n.children = append(moved, n.children...)
	} else {
		// nb is immediately right of n: take nb's leading children onto n's back.
		moved = append([]*Node[T, S](nil), nb.children[:move]...)
		nb.children = nb.children[move:]
		n.children = append(n.children, moved...)
	}
	for _, child := range moved {
		child.parent = n
	}

	recompute := func(nd *Node[T, S]) {
		var size int
		var s S
		for i, child := range nd.children {
			size += child.size
			if tree.summarizer != nil {
				if i == 0 {
					s = child.summary
				} else {
					s = tree.summarizer.Add(s, child.summary)
				}
			}
		}
		nd.size = size
		nd.summary = s
	}
	recompute(n)
	recompute(nb)
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
