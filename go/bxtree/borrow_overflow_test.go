// Regression tests for the overflow policy: an overflowing node first tries
// to donate its excess to the adjacent sibling with the most headroom, and
// only splits when no (or too little) headroom exists.

package bxtree

import (
	"slices"
	"testing"
)

// borrowLeafFixture builds a three-leaf tree (single internal root) with the
// given item groups, so borrowOverflow can be exercised in isolation.
func borrowLeafFixture(t *testing.T, leafMin, leafMax int, groups ...[]int) (*BxTree[int, int], []*Node[int, int]) {
	t.Helper()
	tree := mustNew(t,
		WithSummarizer[int, int](countSummarizer{}),
		WithLeafNodeSize[int, int](leafMin, leafMax),
		WithInternalNodeSize[int, int](leafMin, leafMax),
	)

	leaves := make([]*Node[int, int], len(groups))
	root := &Node[int, int]{isLeaf: false}
	for i, items := range groups {
		leaves[i] = &Node[int, int]{isLeaf: true, items: append([]int(nil), items...), size: len(items)}
		leaves[i].summary = tree.summarizeItems(items)
		leaves[i].parent = root
		root.children = append(root.children, leaves[i])
		root.size += leaves[i].size
		root.summary += leaves[i].summary
	}
	tree.root = root
	tree.first = leaves[0]
	tree.last = leaves[len(leaves)-1]
	for i := 1; i < len(leaves); i++ {
		leaves[i-1].next = leaves[i]
		leaves[i].prev = leaves[i-1]
	}
	return tree, leaves
}

func TestBorrowOverflowLeafBorrowInsteadOfSplit(t *testing.T) {
	// leafMax 4: middle leaf at 5 (overflow), left and right at 2 (headroom 2
	// each). Tie resolves to left, so the donor gives 1 item to the left
	// sibling and no split happens.
	tree, leaves := borrowLeafFixture(t, 2, 4, rangeInts(0, 2), rangeInts(10, 5), rangeInts(20, 2))
	left, mid := leaves[0], leaves[1]
	root := tree.root

	tree.borrowOverflow(mid)

	if len(root.children) != 3 {
		t.Fatalf("root has %d children, want 3 (no split allowed)", len(root.children))
	}
	if len(mid.items) != 4 {
		t.Fatalf("overflowing leaf has %d items, want 4 after borrow", len(mid.items))
	}
	if len(left.items) != 3 {
		t.Fatalf("left sibling has %d items, want 3 (received 1)", len(left.items))
	}
	wantLeft := []int{0, 1, 10}
	if !slices.Equal(left.items, wantLeft) {
		t.Fatalf("left items %v, want %v", left.items, wantLeft)
	}
	want := append(rangeInts(0, 2), append(rangeInts(10, 5), rangeInts(20, 2)...)...)
	verifyTree(t, tree, want)
}

func TestBorrowOverflowInternalChildren(t *testing.T) {
	// One level up: an internal node with 5 children (overflow) next to
	// internal siblings with 2 each; a boundary child moves into the left
	// sibling instead of splitting the overflowing node.
	tree := mustNew(t,
		WithSummarizer[int, int](countSummarizer{}),
		WithLeafNodeSize[int, int](2, 4),
		WithInternalNodeSize[int, int](2, 4),
	)

	mkLeaf := func(start, count int) *Node[int, int] {
		items := rangeInts(start, count)
		n := &Node[int, int]{isLeaf: true, items: items, size: count}
		n.summary = tree.summarizeItems(items)
		return n
	}
	mkInner := func(leaves []*Node[int, int]) *Node[int, int] {
		n := &Node[int, int]{isLeaf: false, children: leaves}
		for _, c := range leaves {
			c.parent = n
			n.size += c.size
			n.summary += c.summary
		}
		return n
	}

	left := mkInner([]*Node[int, int]{mkLeaf(0, 2), mkLeaf(2, 2)})
	mid := mkInner([]*Node[int, int]{mkLeaf(10, 2), mkLeaf(12, 2), mkLeaf(14, 2), mkLeaf(16, 2), mkLeaf(18, 2)})
	right := mkInner([]*Node[int, int]{mkLeaf(20, 2), mkLeaf(22, 2)})

	root := &Node[int, int]{isLeaf: false}
	for _, n := range []*Node[int, int]{left, mid, right} {
		n.parent = root
		root.children = append(root.children, n)
		root.size += n.size
		root.summary += n.summary
	}
	tree.root = root
	tree.first = left.children[0]
	tree.last = right.children[len(right.children)-1]

	var prev *Node[int, int]
	for _, inner := range []*Node[int, int]{left, mid, right} {
		for _, leaf := range inner.children {
			if prev != nil {
				prev.next = leaf
				leaf.prev = prev
			}
			prev = leaf
		}
	}

	tree.borrowOverflow(mid)

	if len(root.children) != 3 {
		t.Fatalf("root has %d children, want 3 (no split allowed)", len(root.children))
	}
	if len(left.children) != 3 {
		t.Fatalf("left internal node has %d children, want 3 (received 1)", len(left.children))
	}
	if len(mid.children) != 4 {
		t.Fatalf("overflowing internal node has %d children, want 4 after borrow", len(mid.children))
	}
	if len(right.children) != 2 {
		t.Fatalf("right internal node changed: %d children, want 2", len(right.children))
	}

	var items []int
	first := tree.first
	for n := first; n != nil; n = n.next {
		items = append(items, n.items...)
	}
	verifyTree(t, tree, items)
}

func TestBorrowOverflowTiePrefersLeft(t *testing.T) {
	// Both siblings have equal headroom; the tie must resolve to the left
	// sibling (mirrors rebalance's tie-break).
	tree, leaves := borrowLeafFixture(t, 2, 4, rangeInts(0, 2), rangeInts(10, 5), rangeInts(20, 2))

	tree.borrowOverflow(leaves[1])

	if len(leaves[0].items) != 3 {
		t.Fatalf("left has %d items, want 3 — tie must resolve to the left sibling", len(leaves[0].items))
	}
	if len(leaves[2].items) != 2 {
		t.Fatalf("right has %d items, want 2 (untouched)", len(leaves[2].items))
	}
	want := append(rangeInts(0, 2), append(rangeInts(10, 5), rangeInts(20, 2)...)...)
	verifyTree(t, tree, want)
}

func TestBorrowOverflowPicksMostHeadroom(t *testing.T) {
	// Left sibling has headroom 1, right has headroom 3: the right sibling
	// (most headroom) must receive the excess.
	tree, leaves := borrowLeafFixture(t, 2, 4, rangeInts(0, 3), rangeInts(10, 5), rangeInts(20, 2))

	tree.borrowOverflow(leaves[1])

	if len(leaves[2].items) != 3 {
		t.Fatalf("right has %d items, want 3 — it had the most headroom", len(leaves[2].items))
	}
	if len(leaves[0].items) != 3 {
		t.Fatalf("left has %d items, want 3 (unchanged)", len(leaves[0].items))
	}
	verifyTree(t, tree, append(rangeInts(0, 3), append(rangeInts(10, 5), rangeInts(20, 2)...)...))
}

func TestBorrowOverflowThenSplitOnPartialHeadroom(t *testing.T) {
	// donor at 6 with a sibling at 3 (headroom 1): borrow 1, still 5 > max 4,
	// so the caller splits what remains — the insert path's borrow-then-split.
	tree, leaves := borrowLeafFixture(t, 2, 4, rangeInts(0, 3), rangeInts(10, 6))
	mid := leaves[1]

	tree.borrowOverflow(mid)
	if len(mid.items) != 5 {
		t.Fatalf("mid has %d items, want 5 after partial borrow", len(mid.items))
	}
	tree.split(mid)

	if len(leaves[0].items) != 4 {
		t.Fatalf("left has %d items, want 4 after borrow", len(leaves[0].items))
	}
	if len(mid.items) != 2 {
		t.Fatalf("mid has %d items, want 2 after borrow+split", len(mid.items))
	}
	if len(tree.root.children) != 3 {
		t.Fatalf("root has %d children, want 3 (left, mid, split twin)", len(tree.root.children))
	}
	verifyTree(t, tree, append(rangeInts(0, 3), rangeInts(10, 6)...))
}

func TestBorrowOverflowNoSiblingOrNoRoom(t *testing.T) {
	// No adjacent sibling: borrow is a no-op, so the insert path splits.
	tree, leaves := borrowLeafFixture(t, 2, 4, rangeInts(0, 5))
	tree.borrowOverflow(leaves[0])
	if len(leaves[0].items) != 5 {
		t.Fatalf("only-child leaf changed: %d items", len(leaves[0].items))
	}

	// Full siblings give no headroom: no-op again.
	tree, leaves = borrowLeafFixture(t, 2, 4, rangeInts(0, 4), rangeInts(10, 5))
	tree.borrowOverflow(leaves[1])
	if len(leaves[1].items) != 5 {
		t.Fatalf("no-room borrow changed mid: %d items, want 5", len(leaves[1].items))
	}
}

func TestBorrowOverflowEqualizesOneShot(t *testing.T) {
	// The policy is one-shot equalisation, not dribbling: a leaf overflowed
	// to 129 next to a sibling at 40 (leaf size 64..128) balances the pair
	// to ceil(169/2) = 85 and 84 in a single transfer, with no split.
	tree, leaves := borrowLeafFixture(t, 64, 128, rangeInts(0, 40), rangeInts(1000, 129))
	left, mid := leaves[0], leaves[1]

	tree.borrowOverflow(mid)

	if len(mid.items) != 85 {
		t.Fatalf("overflowing leaf has %d items, want 85 (equal point)", len(mid.items))
	}
	if len(left.items) != 84 {
		t.Fatalf("neighbour has %d items, want 84 (equal point)", len(left.items))
	}
	if tree.root.isLeaf {
		t.Fatal("root split; borrow alone must absorb the overflow")
	}
	want := append(rangeInts(0, 40), rangeInts(1000, 129)...)
	verifyTree(t, tree, want)
}

func TestBorrowOverflowEqualizesInternalChildren(t *testing.T) {
	// Same one-shot equalisation one level up: an internal node with 129
	// children next to 40, leaf size 64..128 with leaves of 1 item each.
	tree := mustNew(t,
		WithSummarizer[int, int](countSummarizer{}),
		WithLeafNodeSize[int, int](1, 128),
		WithInternalNodeSize[int, int](64, 128),
	)

	mkInner := func(firstChild int, count int) *Node[int, int] {
		n := &Node[int, int]{isLeaf: false}
		for i := firstChild; i < firstChild+count; i++ {
			leaf := &Node[int, int]{isLeaf: true, items: []int{i}, size: 1, summary: 1}
			if n.children != nil {
				leaf.prev = n.children[len(n.children)-1]
				n.children[len(n.children)-1].next = leaf
			}
			leaf.parent = n
			n.children = append(n.children, leaf)
			n.size++
			n.summary++
		}
		return n
	}

	left := mkInner(0, 40)
	mid := mkInner(1000, 129)
	root := &Node[int, int]{isLeaf: false}
	for _, n := range []*Node[int, int]{left, mid} {
		n.parent = root
		root.children = append(root.children, n)
		root.size += n.size
		root.summary += n.summary
	}
	tree.root = root
	tree.first = left.children[0]
	tree.last = mid.children[len(mid.children)-1]
	left.children[len(left.children)-1].next = mid.children[0]
	mid.children[0].prev = left.children[len(left.children)-1]

	tree.borrowOverflow(mid)

	if len(left.children) != 84 {
		t.Fatalf("left has %d children, want 84 (equal point)", len(left.children))
	}
	if len(mid.children) != 85 {
		t.Fatalf("overflowing internal node has %d children, want 85 (equal point)", len(mid.children))
	}
	for i, child := range left.children {
		if child.parent != left {
			t.Fatalf("left child %d parent not relinked", i)
		}
	}
	if left.children[40].items[0] != 1000 || mid.children[0].items[0] != 1044 {
		t.Fatalf("boundary order broken: first moved %v, first kept %v", left.children[40].items, mid.children[0].items)
	}
	verifyTree(t, tree, append(rangeInts(0, 40), rangeInts(1000, 129)...))
}
