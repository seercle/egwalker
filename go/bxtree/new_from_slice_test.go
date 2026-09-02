package bxtree

import (
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
			tree := NewFromSlice(items, cfg.opts...)
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
		tree := NewFromSlice[int, struct{}](nil)
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
		tree := NewFromSlice(items, WithSummarizer(config))

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
		tree := NewFromSlice(items, WithOnItemMoved(func(it *item, n *Node[*item, struct{}]) {
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
	tree := NewFromSlice(items,
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
