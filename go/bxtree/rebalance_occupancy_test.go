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

// rangeInts returns []int{start, start+1, ..., start+n-1}.
func rangeInts(start, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = start + i
	}
	return out
}

// smallLeafOpts builds a tree configured with small node sizes (leaf 4-8,
// internal 2-4) and a count summarizer, matching the configs that expose
// rebalance occupancy bugs most easily.
func smallLeafOpts() []Option[int, int] {
	return []Option[int, int]{
		WithSummarizer[int, int](countSummarizer{}),
		WithLeafNodeSize[int, int](4, 8),
		WithInternalNodeSize[int, int](2, 4),
	}
}

// smallLeafConfig returns a tree configured with the small node sizes.
func smallLeafConfig(t *testing.T) *BxTree[int, int] {
	t.Helper()
	return mustNew(t, smallLeafOpts()...)
}

// TestDeleteRangeRestoresLeafOccupancy is a regression test for the rebalance
// borrow that moved a single item/child. 32 items build four full leaves of 8
// under a single (root) internal node. DeleteRange(2, 6) guts the first leaf
// from 8 items down to 2 in one delete-loop iteration -- well below
// leafMinSize 4. rebalance must redistribute enough items from the neighbouring
// leaf to restore both to >= min; a single +1 borrow leaves the node underfull.
func TestDeleteRangeRestoresLeafOccupancy(t *testing.T) {
	tree := mustNewFromSlice(t, rangeInts(0, 32), smallLeafOpts()...)
	if err := tree.DeleteRange(2, 6); err != nil {
		t.Fatalf("DeleteRange: %v", err)
	}

	var expected []int
	for i := 0; i < 32; i++ {
		if i < 2 || i > 7 {
			expected = append(expected, i)
		}
	}
	verifyTree(t, tree, expected)
}

// TestRebalanceLeafRedistributes verifies that rebalancing an underfull leaf
// next to a full one redistributes items near-evenly (both siblings back within
// [min, max]) rather than borrowing a single item and leaving the node
// underfull or merging the pair.
func TestRebalanceLeafRedistributes(t *testing.T) {
	tree := smallLeafConfig(t)

	leftItems := rangeInts(0, 8)
	rightItems := []int{100}

	left := &Node[int, int]{isLeaf: true, items: leftItems, size: len(leftItems)}
	left.summary = tree.summarizeItems(leftItems)
	right := &Node[int, int]{isLeaf: true, items: rightItems, size: len(rightItems)}
	right.summary = tree.summarizeItems(rightItems)

	root := &Node[int, int]{isLeaf: false, size: left.size + right.size}
	root.summary = tree.summarizer.Add(left.summary, right.summary)
	left.parent = root
	right.parent = root
	root.children = []*Node[int, int]{left, right}

	tree.root = root
	tree.first = left
	tree.last = right
	left.next = right
	right.prev = left

	// right holds a single item (< leafMinSize 4); its only neighbour is the
	// full left leaf. Redistributing the 9 items near-even (left 4, right 5)
	// must raise right back into [4, 8] without merging the pair.
	tree.rebalance(right)

	if tree.root != root {
		t.Fatal("rebalance merged the underfull leaf with its neighbour")
	}
	if len(left.items) < tree.leafMinSize || len(left.items) > tree.leafMaxSize {
		t.Fatalf("left leaf has %d items, want [%d, %d]", len(left.items), tree.leafMinSize, tree.leafMaxSize)
	}
	if len(right.items) < tree.leafMinSize || len(right.items) > tree.leafMaxSize {
		t.Fatalf("right leaf has %d items, want [%d, %d]", len(right.items), tree.leafMinSize, tree.leafMaxSize)
	}
	verifyTree(t, tree, append(leftItems, rightItems...))
}

// rebalanceSelectorRun builds a three-leaf tree (fat left, thin right) around
// an underfull middle leaf and calls rebalance on it, then checks the outcome
// against the expectation that rebalance() partners the middle leaf with its
// thinner sibling.
type rebalanceSelectorCase struct {
	name string

	leftItems, middleItems, rightItems []int

	// Expected post-rebalance leaves. A nil wantMiddle means the middle leaf
	// was absorbed into an (always left) neighbour and no longer holds items.
	wantLeft, wantMiddle, wantRight []int
	wantRootChildren                int
}

func rebalanceSelectorRun(t *testing.T, tc rebalanceSelectorCase) {
	t.Helper()
	tree := mustNew(t,
		WithSummarizer[int, int](countSummarizer{}),
		WithLeafNodeSize[int, int](2, 4),
		WithInternalNodeSize[int, int](2, 4),
	)

	mkLeaf := func(items []int) *Node[int, int] {
		n := &Node[int, int]{isLeaf: true, items: items, size: len(items)}
		n.summary = tree.summarizeItems(items)
		return n
	}
	left, middle, right := mkLeaf(tc.leftItems), mkLeaf(tc.middleItems), mkLeaf(tc.rightItems)

	total := left.size + middle.size + right.size
	root := &Node[int, int]{isLeaf: false, size: total}
	root.summary = tree.summarizer.Add(tree.summarizer.Add(left.summary, middle.summary), right.summary)
	for _, n := range []*Node[int, int]{left, middle, right} {
		n.parent = root
	}
	root.children = []*Node[int, int]{left, middle, right}

	tree.root = root
	tree.first = left
	tree.last = right
	left.next, middle.next = middle, right
	middle.prev, right.prev = left, middle

	tree.rebalance(middle)

	if len(root.children) != tc.wantRootChildren {
		t.Fatalf("root has %d children, want %d", len(root.children), tc.wantRootChildren)
	}
	if len(left.items) != len(tc.wantLeft) {
		t.Fatalf("left leaf has %d items (%v), want %v", len(left.items), left.items, tc.wantLeft)
	}
	for i, v := range tc.wantLeft {
		if left.items[i] != v {
			t.Fatalf("left leaf[%d]=%d, want %d", i, left.items[i], v)
		}
	}
	switch {
	case tc.wantMiddle == nil:
		// Middle was absorbed into its left neighbour; only its content
		// matters, which the chain + verifyTree checks cover.
	case len(middle.items) != len(tc.wantMiddle):
		t.Fatalf("middle leaf has %d items (%v), want %v", len(middle.items), middle.items, tc.wantMiddle)
	default:
		for i, v := range tc.wantMiddle {
			if middle.items[i] != v {
				t.Fatalf("middle leaf[%d]=%d, want %d", i, middle.items[i], v)
			}
		}
	}

	// Leaf-chain integrity: walk first..last and confirm content order.
	var chain []int
	for n := tree.first; n != nil; n = n.next {
		chain = append(chain, n.items...)
	}
	want := append(append(append([]int(nil), tc.wantLeft...), tc.wantMiddle...), tc.wantRight...)
	if len(chain) != len(want) {
		t.Fatalf("leaf chain has %d items (%v), want %v", len(chain), chain, want)
	}
	for i := range want {
		if chain[i] != want[i] {
			t.Fatalf("leaf chain[%d]=%d, want %d", i, chain[i], want[i])
		}
	}
	verifyTree(t, tree, want)
}

// TestRebalancePicksPoorerSibling pins the neighbour-selection policy: when an
// underfull leaf has a fat left sibling and a thin right sibling, rebalance
// must partner with the poorer (fewest items) sibling, leaving the fat leaf
// untouched. With leaf min 2 / max 4, middle (1 item) + right (1 item) cannot
// reach 2*min, so the pair merges: the left node survives, so middle absorbs
// its right sibling.
func TestRebalancePicksPoorerSibling(t *testing.T) {
	rebalanceSelectorRun(t, rebalanceSelectorCase{
		name:             "poorer right sibling is the merge partner",
		leftItems:        rangeInts(0, 4),
		middleItems:      []int{10},
		rightItems:       []int{20},
		wantLeft:         rangeInts(0, 4),
		wantMiddle:       []int{10, 20},
		wantRight:        nil,
		wantRootChildren: 2,
	})
}

// TestRebalanceSiblingTiePrefersLeft pins the tie-break: equal-count siblings
// must resolve to the left one.
func TestRebalanceSiblingTiePrefersLeft(t *testing.T) {
	rebalanceSelectorRun(t, rebalanceSelectorCase{
		name:             "tie resolves to left sibling",
		leftItems:        rangeInts(0, 2),
		middleItems:      []int{10},
		rightItems:       rangeInts(20, 2),
		wantLeft:         []int{0, 1, 10},
		wantMiddle:       nil,
		wantRight:        rangeInts(20, 2),
		wantRootChildren: 2,
	})
}
