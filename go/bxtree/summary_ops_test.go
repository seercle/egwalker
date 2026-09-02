package bxtree

import (
	"testing"
)

// buildCountTree returns a count-summarized tree holding items 0..size-1.
func buildCountTree(size int) *BxTree[int, int] {
	tree := New(WithSummarizer(countSummarizer{}))
	for i := 0; i < size; i++ {
		if err := tree.InsertAt(i, i); err != nil {
			panic(err)
		}
	}
	return tree
}

// sumSummary sums item values (FromItem returns the value itself).
type sumSummary struct{}

func (sumSummary) FromItem(item int) int { return item }
func (sumSummary) Add(a, b int) int      { return a + b }
func (sumSummary) Sub(a, b int) int      { return a - b }

func TestNodeSummaryBeforeMatchesIndex(t *testing.T) {
	tree := buildCountTree(300) // multiple leaves and internal levels
	for n := tree.First(); n != nil; n = n.Next() {
		if got, want := n.SummaryBefore(tree), n.Index(); got != want {
			t.Errorf("leaf at Index %d: SummaryBefore=%d, want %d", n.Index(), got, want)
		}
	}
	if got := tree.Last().SummaryBefore(tree) + len(tree.Last().Items()); got != 300 {
		t.Errorf("last leaf SummaryBefore+len = %d, want 300", got)
	}
}

func TestUpdateSummaryAndUpward(t *testing.T) {
	const size = 300 // > leafMax(128), so the root is an internal node
	tree := buildCountTree(size)

	// Corrupt the root summary, then recompute from a leaf upward.
	leaf := tree.First()
	tree.Root().summary = 0
	leaf.UpdateSummaryUpward(tree)
	if got := tree.Root().Summary(); got != size {
		t.Errorf("after UpdateSummaryUpward root summary=%d, want %d", got, size)
	}

	// Corrupt a single leaf summary, then recompute just that node.
	leaf.summary = 9999
	leaf.UpdateSummary(tree)
	if got := leaf.Summary(); got != len(leaf.Items()) {
		t.Errorf("after UpdateSummary leaf summary=%d, want %d", got, len(leaf.Items()))
	}

	// Internal nodes must also be recomputed by UpdateSummary. The root is
	// internal at this size; corrupt it and recompute it from its children.
	internal := tree.Root()
	if internal.isLeaf {
		t.Fatal("expected an internal root for a 300-item tree")
	}
	internal.summary = 0
	internal.UpdateSummary(tree)
	want := 0
	for _, c := range internal.children {
		want += c.Summary()
	}
	if internal.Summary() != want {
		t.Errorf("internal summary=%d, want %d", internal.Summary(), want)
	}
}

func TestFindPathEdges(t *testing.T) {
	tree := New[int, int](WithSummarizer(sumSummary{}))
	if err := tree.InsertRange(0, []int{10, 20, 30, 40}); err != nil {
		t.Fatal(err)
	}

	// Predicate that becomes true at 30 (acc before it is 30).
	node, pos, acc := tree.FindPath(func(acc, cur int) bool { return acc+cur > 45 })
	if node == nil {
		t.Fatal("FindPath(>45) returned a nil node")
	}
	if got := node.Items()[pos]; got != 30 || acc != 30 {
		t.Errorf("FindPath(>45) = item %d at pos %d acc %d, want 30/2/30", got, pos, acc)
	}

	// Always-true predicate stops at the first item with acc 0.
	node, pos, acc = tree.FindPath(func(acc, cur int) bool { return true })
	if node == nil {
		t.Fatal("FindPath(true) returned a nil node")
	}
	if got := node.Items()[pos]; got != 10 || acc != 0 {
		t.Errorf("FindPath(true) = item %d acc %d, want 10/0", got, acc)
	}

	// Empty tree: FindPath returns (nil, -1, zero).
	empty := New[int, int](WithSummarizer(sumSummary{}))
	if n, p, a := empty.FindPath(func(acc, cur int) bool { return true }); n != nil || p != -1 || a != 0 {
		t.Errorf("FindPath on empty tree = (%v, %d, %d), want (nil, -1, 0)", n, p, a)
	}
}

func TestDeepNodeAccessors(t *testing.T) {
	tree := buildCountTree(5000) // > leafMax*internalMax: forces multiple internal levels

	// For every leaf, GetAtNode(leaf.Index()) must land on that leaf at pos 0,
	// and the leaf must be reachable through parent/children links.
	for n := tree.First(); n != nil; n = n.Next() {
		node, pos, err := tree.GetAtNode(n.Index())
		if err != nil {
			t.Fatalf("GetAtNode(%d) failed: %v", n.Index(), err)
		}
		if node != n || pos != 0 {
			t.Fatalf("GetAtNode(%d) = (node %p, pos %d), want (leaf %p, pos 0)", n.Index(), node, pos, n)
		}
		if n.Parent() != nil {
			found := false
			for _, c := range n.Parent().Children() {
				if c == n {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("leaf %p not found in parent.Children()", n)
			}
		}
	}

	// Index() must equal the count of items before the leaf.
	idx := 0
	for n := tree.First(); n != nil; n = n.Next() {
		if n.Index() != idx {
			t.Errorf("leaf Index=%d, want %d", n.Index(), idx)
		}
		idx += len(n.Items())
	}
}
