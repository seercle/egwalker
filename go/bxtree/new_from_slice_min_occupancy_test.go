//go:build known_limitations

package bxtree

import "testing"

// TestExpectationNewFromSliceMinOccupancy documents a known limitation (see
// CONTEXT.md, "Known Limitations"): the bulk loader fills internal/leaf nodes
// to their configured maximum and leaves whatever remains in the rightmost
// node, so bulk-loaded trees can have non-root nodes below their configured
// minimum (e.g. 129 items at default sizes produce a trailing 1-item leaf,
// below leafMinSize 64). The strict occupancy invariant — every non-root node
// within [min, max], root exempt from the minimum — therefore does NOT hold
// for bulk-loaded trees, only for trees mutated through Insert/Delete.
//
// This test is expected to FAIL. When bulk loading is changed to honor the
// minimum occupancies too, move it into the regular suite.
func TestExpectationNewFromSliceMinOccupancy(t *testing.T) {
	const n = 129 // one full leaf (128) + a trailing 1-item leaf < leafMinSize 64
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}
	tree := mustNewFromSlice(t, items, WithSummarizer[int, int](countSummarizer{}))

	violations := 0
	root := tree.Root()
	var walk func(node *Node[int, int])
	walk = func(node *Node[int, int]) {
		if node.isLeaf {
			if node != root && len(node.items) < tree.leafMinSize {
				violations++
			}
			return
		}
		if node != root && len(node.children) < tree.internalMinSize {
			violations++
		}
		for _, c := range node.children {
			walk(c)
		}
	}
	if root != nil {
		walk(root)
	}

	if violations > 0 {
		t.Errorf("NewFromSlice does not honor min occupancy: %d non-root node(s) below minimum for n=%d (known limitation)", violations, n)
	}
}
