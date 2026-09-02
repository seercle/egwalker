package bxtree

import (
	"math/rand"
	"testing"
)

// checkNodeBounds verifies every node respects the configured min/max sizes,
// except the root, which is allowed to underflow.
func checkNodeBounds[T any](t *testing.T, tree *BxTree[T, int]) {
	t.Helper()
	root := tree.Root()
	if root == nil {
		return
	}
	// Note: `var check func(...); check = ...` — a plain `:=` closure cannot
	// reference itself in Go.
	var check func(n *Node[T, int], path string)
	check = func(n *Node[T, int], path string) {
		if n.isLeaf {
			lo, hi := tree.leafMinSize, tree.leafMaxSize
			if n == root && len(n.items) > hi {
				t.Errorf("%s: root leaf has %d items, max %d", path, len(n.items), hi)
			}
			if n != root && (len(n.items) < lo || len(n.items) > hi) {
				t.Errorf("%s: leaf has %d items, want [%d, %d]", path, len(n.items), lo, hi)
			}
			return
		}
		if n != root && (len(n.children) < tree.internalMinSize || len(n.children) > tree.internalMaxSize) {
			t.Errorf("%s: internal node has %d children, want [%d, %d]", path, len(n.children), tree.internalMinSize, tree.internalMaxSize)
		}
		for i, c := range n.children {
			if c.parent != n {
				t.Errorf("%s: child %d has wrong parent", path, i)
			}
			check(c, path+"->child")
		}
	}
	check(root, "root")
}

func TestSmallNodeConfigRandomOps(t *testing.T) {
	// The option funcs are generic; New cannot infer T/S from them alone, so
	// the type arguments must be written explicitly on each option call.
	tree := New(
		WithSummarizer[int, int](countSummarizer{}),
		WithLeafNodeSize[int, int](4, 8),
		WithInternalNodeSize[int, int](2, 4),
	)
	rng := rand.New(rand.NewSource(123))
	var mirror []int

	insert := func(v int) {
		pos := 0
		if len(mirror) > 0 {
			pos = rng.Intn(len(mirror) + 1)
		}
		if err := tree.InsertAt(pos, v); err != nil {
			t.Fatalf("InsertAt(%d) failed: %v", pos, err)
		}
		mirror = append(mirror[:pos], append([]int{v}, mirror[pos:]...)...)
	}
	deleteAt := func() {
		pos := rng.Intn(len(mirror))
		if err := tree.DeleteAt(pos); err != nil {
			t.Fatalf("DeleteAt(%d) failed: %v", pos, err)
		}
		mirror = append(mirror[:pos], mirror[pos+1:]...)
	}

	for i := 0; i < 500; i++ {
		switch rng.Intn(3) {
		case 0:
			insert(i)
		case 1:
			if len(mirror) > 0 {
				deleteAt()
			}
		case 2:
			// Verify a few random point lookups against the mirror.
			if len(mirror) > 0 {
				pos := rng.Intn(len(mirror))
				v, err := tree.GetAt(pos)
				if err != nil || *v != mirror[pos] {
					t.Fatalf("GetAt(%d) = %v (err %v), mirror %d", pos, v, err, mirror[pos])
				}
			}
		}
		checkNodeBounds(t, tree)
	}

	verifyTree(t, tree, mirror)
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
