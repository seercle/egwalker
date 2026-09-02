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
		sizes := []int{nForced, nForced + 3*leafMax[ci], 3, leafMax[ci]*(internalMax[ci]+3) + 7}

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
