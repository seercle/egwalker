package bxtree

import (
	"sort"
	"testing"
)

// hasSingletonInternal walks the tree and reports whether any non-root
// internal node holds exactly one child.
func hasSingletonInternal(tree *BxTree[int, int]) bool {
	root := tree.Root()
	if root == nil {
		return false
	}
	bad := false
	var walk func(n *Node[int, int])
	walk = func(n *Node[int, int]) {
		if n != root && !n.isLeaf && len(n.children) == 1 {
			bad = true
			return
		}
		if !n.isLeaf {
			for _, c := range n.children {
				walk(c)
			}
		}
	}
	walk(root)
	return bad
}

func TestNewFromSliceNoSingletonInternal(t *testing.T) {
	configs := []struct {
		name string
		opts []Option[int, int]
	}{
		{"small", []Option[int, int]{
			WithLeafNodeSize[int, int](4, 8),
			WithInternalNodeSize[int, int](2, 4),
			WithSummarizer[int, int](countSummarizer{}),
		}},
		{"default", []Option[int, int]{
			WithSummarizer[int, int](countSummarizer{}),
		}},
	}

	leafMax := []int{8, 128}
	internalMax := []int{4, 32}

	for ci, cfg := range configs {
		// leafMax*(internalMax+1) forces exactly internalMax+1 leaves, so the
		// internal level groups into internalMax + 1 children -> trailing
		// singleton without the fix.
		nForced := leafMax[ci] * (internalMax[ci] + 1)
		// (internalMax+1)^2 leaves force an upper-level grouping (one full group
		// of internalMax plus a tail of internalMax+1) that also overflows one
		// internal node, i.e. a ≡ 1 (mod max) tail one level further up.
		nUpper := leafMax[ci] * (internalMax[ci] + 1) * (internalMax[ci] + 1)
		sizes := []int{nForced, nForced + 3*leafMax[ci], 3, leafMax[ci]*(internalMax[ci]+3) + 7, nUpper}

		for _, n := range sizes {
			items := make([]int, n)
			for i := range items {
				items[i] = i
			}
			tree := mustNewFromSlice(t, items, cfg.opts...)
			if tree.Size() != n {
				t.Fatalf("%s config, n=%d: Size=%d", cfg.name, n, tree.Size())
			}
			if hasSingletonInternal(tree) {
				t.Fatalf("%s config, n=%d: NewFromSlice built a non-root internal node with a single child", cfg.name, n)
			}

			// Draining from the back previously panicked on such trees.
			for tree.Size() > 0 {
				if err := tree.DeleteAt(tree.Size() - 1); err != nil {
					t.Fatalf("%s config, n=%d: DeleteAt from back failed: %v", cfg.name, n, err)
				}
			}
		}
	}
}

func TestNewFromSlice(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		tree := mustNewFromSlice[int, struct{}](t, nil)
		if tree.Size() != 0 {
			t.Errorf("Expected size 0, got %d", tree.Size())
		}
	})

	t.Run("LargeWithSummary", func(t *testing.T) {
		size := 10000
		items := make([]int, size)
		for i := range items {
			items[i] = i
		}

		config := setupCountSummary()
		tree := mustNewFromSlice(t, items, WithSummarizer(config))

		if tree.Size() != size {
			t.Errorf("Expected size %d, got %d", size, tree.Size())
		}

		if tree.Root().Summary() != size {
			t.Errorf("Expected root summary %d, got %d", size, tree.Root().Summary())
		}

		// Verify structure
		idx := 0
		tree.ForEach(func(v int) {
			if v != idx {
				t.Errorf("Mismatch at index %d: expected %d, got %d", idx, idx, v)
			}
			idx++
		})

		// Verify pointers
		if tree.First() == nil || tree.Last() == nil {
			t.Fatal("First/Last pointers should be set")
		}
	})

	t.Run("OnItemMoved", func(t *testing.T) {
		type item struct {
			node *Node[*item, struct{}]
		}
		items := []*item{{}, {}, {}}
		tree := mustNewFromSlice(t, items, WithOnItemMoved(func(it *item, n *Node[*item, struct{}]) {
			it.node = n
		}))

		if tree.Size() != 3 {
			t.Errorf("Expected size 3, got %d", tree.Size())
		}

		for i, it := range items {
			if it.node == nil {
				t.Errorf("Item %d node is nil", i)
			}
		}
	})
}

func TestNewFromSliceRespectsSizeOptions(t *testing.T) {
	items := make([]int, 1000)
	for i := range items {
		items[i] = i
	}
	tree := mustNewFromSlice(t, items,
		WithSummarizer[int, int](countSummarizer{}),
		WithLeafNodeSize[int, int](4, 8),
		WithInternalNodeSize[int, int](2, 4),
	)
	if tree.Size() != len(items) {
		t.Fatalf("size = %d, want %d", tree.Size(), len(items))
	}
	checkNodeBounds(t, tree)
	verifyTree(t, tree, items)
}

// occupancySizes returns tree sizes designed to exercise min/max occupancy
// boundaries at every level for a config with the given node sizes: item tails
// at each leaf fan-out multiple, leaf counts just above/below each internal
// fan-out multiple, a full contiguous range for structurally cheap configs, and
// a deterministic spread over larger sizes.
func occupancySizes(leafMin, leafMax, internalMax int) []int {
	set := map[int]bool{}
	add := func(n int) {
		if n >= 0 {
			set[n] = true
		}
	}

	// Exhaustive contiguous sweep for configs small enough to make it cheap.
	if leafMax*internalMax <= 300 {
		for n := 0; n <= leafMax*internalMax*internalMax+leafMax*internalMax; n++ {
			add(n)
		}
	}

	// Leaf counts around internal grouping boundaries, with item offsets that
	// land leaves on every min/max edge. internalMax+2 leaves force one level
	// above internalMax, and internalMax^2+1 leaves force two levels.
	leafCounts := make([]int, 0, internalMax+2+3)
	for r := 1; r <= internalMax+2; r++ {
		leafCounts = append(leafCounts, r)
	}
	leafCounts = append(leafCounts, internalMax*internalMax-1, internalMax*internalMax, internalMax*internalMax+1)
	for _, leaves := range leafCounts {
		for _, t := range []int{0, 1, leafMin - 1, leafMin, leafMin + 1, leafMax - 1, leafMax} {
			add(leaves*leafMax + t)
		}
	}

	// Deterministic spread across larger multi-level sizes.
	for i := 0; i < 40; i++ {
		add((i*7919)%(leafMax*internalMax*internalMax) + leafMax*internalMax)
	}

	sizes := make([]int, 0, len(set))
	for n := range set {
		sizes = append(sizes, n)
	}
	sort.Ints(sizes)
	return sizes
}

// TestNewFromSliceMinOccupancy verifies that bulk loading distributes items
// across leaf and internal levels so every non-root node lands within its
// configured [min, max]; only the root is exempt from the minimum. Previously
// the loader max-filled nodes and dumped the remainder into a trailing node
// that could fall below min (e.g. 129 items at default sizes made a 1-item
// leaf).
func TestNewFromSliceMinOccupancy(t *testing.T) {
	configs := []struct {
		name                     string
		leafMin, leafMax         int
		internalMin, internalMax int
	}{
		{"default", 64, 128, 16, 32},
		{"small", 4, 8, 2, 4},
		{"odd", 3, 5, 2, 3},
		{"single-item-leaves", 1, 1, 16, 32},
	}

	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			opts := []Option[int, int]{
				WithLeafNodeSize[int, int](cfg.leafMin, cfg.leafMax),
				WithInternalNodeSize[int, int](cfg.internalMin, cfg.internalMax),
				WithSummarizer[int, int](countSummarizer{}),
			}

			for _, n := range occupancySizes(cfg.leafMin, cfg.leafMax, cfg.internalMax) {
				items := make([]int, n)
				for i := range items {
					items[i] = i
				}
				tree := mustNewFromSlice(t, items, opts...)

				if n == 0 {
					if tree.Size() != 0 {
						t.Fatalf("n=%d: Size=%d, want 0", n, tree.Size())
					}
					continue
				}
				if tree.Size() != n {
					t.Fatalf("n=%d: Size=%d", n, tree.Size())
				}

				// verifyNode checks every node stays within [min, max] (root
				// exempt from the minimum) plus size/summary consistency.
				verifyNode(t, tree.Root(), tree)

				// Full structural + content check for reasonably-sized trees.
				if n <= 20000 {
					verifyTree(t, tree, items)
				} else {
					got := 0
					ok := true
					tree.ForEach(func(v int) {
						if v != got {
							ok = false
						}
						got++
					})
					if !ok || got != n {
						t.Fatalf("n=%d: ForEach content/order mismatch", n)
					}
				}
			}
		})
	}
}
