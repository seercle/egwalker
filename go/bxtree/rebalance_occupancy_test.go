package bxtree

import (
	"fmt"
	"math/rand"
	"testing"
)

// TestDeleteDoesNotOverfillInternalNode is a regression test for a rebalance
// over-merge: after a leaf merge, merge() calls rebalance(parent), and
// rebalance had no guard for "parent is not actually underfull". So a healthy
// internal parent (>= internalMinSize children) got merged with a sibling
// anyway, producing an internal node with more children than internalMaxSize.
// Small node sizes (leaf 4-8, internal 2-4) expose this because the margins
// are tight; default sizes rarely do. The failing operation is a deterministic
// DeleteAt (op 215 of this seed's sequence).
func TestDeleteDoesNotOverfillInternalNode(t *testing.T) {
	tree := mustNew(t,
		WithSummarizer[int, int](countSummarizer{}),
		WithLeafNodeSize[int, int](4, 8),
		WithInternalNodeSize[int, int](2, 4),
	)
	rng := rand.New(rand.NewSource(2))
	var mirror []int

	// Deterministic mixed insert/delete sequence; op 215 is the delete that
	// over-merges a healthy parent (root/1 goes from 4 to 5 children).
	for i := 0; i < 216; i++ {
		if len(mirror) > 0 && rng.Intn(4) >= 2 {
			pos := rng.Intn(len(mirror))
			if err := tree.DeleteAt(pos); err != nil {
				t.Fatalf("DeleteAt(%d) at op %d failed: %v", pos, i, err)
			}
			mirror = append(mirror[:pos], mirror[pos+1:]...)
		} else {
			pos := rng.Intn(len(mirror) + 1)
			v := rng.Intn(100000)
			if err := tree.InsertAt(pos, v); err != nil {
				t.Fatalf("InsertAt(%d) at op %d failed: %v", pos, i, err)
			}
			mirror = append(mirror[:pos], append([]int{v}, mirror[pos:]...)...)
		}
	}

	// verifyTree -> verifyNode asserts content AND the occupancy invariants
	// (max everywhere, min for every non-root node). It must find the internal
	// node with 5 children (> internalMaxSize 4).
	verifyTree(t, tree, mirror)
}

// TestRandomOpsMaintainOccupancy sweeps many seeds at the small node sizes
// that expose the rebalance over-merge, verifying after every batch of ops
// that content and the occupancy invariants hold.
func TestRandomOpsMaintainOccupancy(t *testing.T) {
	for seed := int64(0); seed < 30; seed++ {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			tree := mustNew(t,
				WithSummarizer[int, int](countSummarizer{}),
				WithLeafNodeSize[int, int](4, 8),
				WithInternalNodeSize[int, int](2, 4),
			)
			rng := rand.New(rand.NewSource(seed))
			var mirror []int
			for i := 0; i < 4000; i++ {
				if len(mirror) == 0 || rng.Intn(3) != 0 {
					pos := rng.Intn(len(mirror) + 1)
					v := rng.Intn(100000)
					if err := tree.InsertAt(pos, v); err != nil {
						t.Fatalf("InsertAt(%d) at op %d failed: %v", pos, i, err)
					}
					mirror = append(mirror[:pos], append([]int{v}, mirror[pos:]...)...)
				} else {
					pos := rng.Intn(len(mirror))
					if err := tree.DeleteAt(pos); err != nil {
						t.Fatalf("DeleteAt(%d) at op %d failed: %v", pos, i, err)
					}
					mirror = append(mirror[:pos], mirror[pos+1:]...)
				}
				if i%128 == 0 {
					verifyTree(t, tree, mirror)
				}
			}
			verifyTree(t, tree, mirror)
		})
	}
}
