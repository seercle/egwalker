package bxtree

import (
	"testing"
)

// TestBulkInsertRangeEmptyRootInsertsOneLeaf covers the pre-fix bug: inserting
// a large range into an empty tree created a single oversized leaf that was
// never split.
func TestBulkInsertRangeEmptyRootInsertsOneLeaf(t *testing.T) {
	var items []int
	for i := 0; i < 300; i++ {
		items = append(items, i)
	}

	tree := mustNew[int, int](t, WithSummarizer(countSummarizer{}))
	if err := tree.InsertRange(0, items); err != nil {
		t.Fatalf("InsertRange: %v", err)
	}

	verifyTree(t, tree, items)
}

func TestBulkInsertRangeDownsizedLeaves(t *testing.T) {
	var items []int
	for i := 0; i < 17; i++ {
		items = append(items, i)
	}

	tree := mustNew[int, int](t, WithSummarizer(countSummarizer{}),
		WithLeafNodeSize[int, int](2, 4))
	if err := tree.InsertRange(0, items); err != nil {
		t.Fatalf("InsertRange: %v", err)
	}

	// Exercise the append-at-end overflow path too (index == Size).
	if err := tree.InsertRange(17, []int{17, 18, 19, 20, 21}); err != nil {
		t.Fatalf("InsertRange: %v", err)
	}
	items = append(items, 17, 18, 19, 20, 21)

	// Exercise the middle-insert overflow path.
	if err := tree.InsertRange(7, []int{100, 101, 102, 103, 104}); err != nil {
		t.Fatalf("InsertRange: %v", err)
	}
	items = append(items[:7:7], append([]int{100, 101, 102, 103, 104}, items[7:]...)...)

	verifyTree(t, tree, items)

	for n := tree.First(); n != nil; n = n.Next() {
		if n != tree.Root() && len(n.Items()) > 4 {
			t.Errorf("leaf has %d items, max 4", len(n.Items()))
		}
	}
}

func TestBulkInsertRangeInternalOverfill(t *testing.T) {
	var items []int
	for i := 0; i < 100; i++ {
		items = append(items, i)
	}

	tree := mustNew[int, int](t, WithSummarizer(countSummarizer{}),
		WithLeafNodeSize[int, int](2, 4),
		WithInternalNodeSize[int, int](2, 4))
	if err := tree.InsertRange(0, items); err != nil {
		t.Fatalf("InsertRange: %v", err)
	}

	verifyTree(t, tree, items)
	checkNodeBounds(t, tree)

	// A second bulk range into the middle must keep internal nodes within
	// bounds as they split and cascade upward.
	mid := []int{1000, 1001, 1002, 1003, 1004, 1005, 1006, 1007, 1008, 1009}
	if err := tree.InsertRange(50, mid); err != nil {
		t.Fatalf("InsertRange: %v", err)
	}
	items = append(items[:50:50], append(append([]int{}, mid...), items[50:]...)...)

	verifyTree(t, tree, items)
	checkNodeBounds(t, tree)
}

func TestBulkInsertRangeThenDeleteRechecks(t *testing.T) {
	var items []int
	for i := 0; i < 120; i++ {
		items = append(items, i)
	}

	tree := mustNew[int, int](t, WithSummarizer(countSummarizer{}),
		WithLeafNodeSize[int, int](2, 4),
		WithInternalNodeSize[int, int](2, 4))
	if err := tree.InsertRange(0, items); err != nil {
		t.Fatalf("InsertRange: %v", err)
	}

	for i, remaining := 119, 119; remaining >= 0; i, remaining = i-1, remaining-1 {
		if err := tree.DeleteAt(remaining); err != nil {
			t.Fatalf("DeleteAt(%d): %v", remaining, err)
		}
		items = items[:remaining]
		if remaining%7 == 0 {
			verifyTree(t, tree, items)
			checkNodeBounds(t, tree)
		}
	}
	if tree.Root() != nil {
		t.Errorf("expected empty tree, root still present")
	}
}
