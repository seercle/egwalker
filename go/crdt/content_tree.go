package crdt

import "egwalker/bxtree"

// ropeLeafCap bounds the size of a folded rope leaf. Leaves are immutable
// content values; folding two adjacent leaves costs O(result) via Concat, so
// the cap keeps repeated single-character edits amortized and bounds leaf
// fragmentation to ~Len/ropeLeafCap. A single large insert may still be one
// leaf of its full length.
const ropeLeafCap = 256

// runLenSummarizer provides the character-count summary for the content tree:
// each leaf contributes its run length, and a node's summary is the total
// characters under it (bxtree.Size() counts LEAVES, not chars, so all
// positional work goes through this summary).
type runLenSummarizer[C content[C]] struct{}

func (runLenSummarizer[C]) FromItem(c C) int { return c.Len() }
func (runLenSummarizer[C]) Add(a, b int) int { return a + b }
func (runLenSummarizer[C]) Sub(a, b int) int { return a - b }

// contentTree is a run-based content tree (rope): a bxtree whose leaves are
// content runs, positionally indexed by character count. Insert/Delete take
// character positions in [0, Len]. Leaves are immutable content values; the
// tree never stores an empty leaf.
type contentTree[C content[C]] struct {
	tree *bxtree.BxTree[C, int]
}

func newContentTree[C content[C]]() *contentTree[C] {
	tree, err := bxtree.New[C, int](bxtree.WithSummarizer[C, int](runLenSummarizer[C]{}))
	if err != nil {
		panic("crdt: bxtree.New: " + err.Error())
	}
	return &contentTree[C]{tree: tree}
}

// Len returns the number of characters in the tree.
func (ct *contentTree[C]) Len() int {
	if ct == nil || ct.tree.Root() == nil {
		return 0
	}
	return ct.tree.Root().Summary()
}

// leafCount returns the number of leaves (for tests and diagnostics).
func (ct *contentTree[C]) leafCount() int { return ct.tree.Size() }

// locateChar returns the item index of the leaf containing character pos, that
// leaf, and the character offset within it. pos must be in [0, Len). Characters
// are 0-based; a leaf starting at character pos has offset 0.
func (ct *contentTree[C]) locateChar(pos int) (idx int, leaf C, offset int) {
	node, posInNode, acc := ct.tree.FindPath(func(acc, cur int) bool {
		return acc+cur > pos
	})
	if node == nil {
		panic("crdt: rope locateChar out of range")
	}
	leaf = node.Items()[posInNode]
	return node.Index() + posInNode, leaf, pos - acc
}

// replaceLeaf replaces the leaf at item index idx with parts (skipping empty
// parts), keeping document order.
func (ct *contentTree[C]) replaceLeaf(idx int, parts ...C) {
	nonEmpty := parts[:0]
	for _, p := range parts {
		if p.Len() > 0 {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if err := ct.tree.DeleteAt(idx); err != nil {
		panic("crdt: rope DeleteAt: " + err.Error())
	}
	if len(nonEmpty) > 0 {
		if err := ct.tree.InsertRange(idx, nonEmpty); err != nil {
			panic("crdt: rope InsertRange: " + err.Error())
		}
	}
}

// Insert inserts run (Len > 0) at character position pos.
//
// Positions that fall on a leaf boundary fold the run into the left neighbour
// when the result fits ropeLeafCap (keeping append/prepend/contiguous typing to
// ~Len/ropeLeafCap leaves); positions interior to a leaf split the leaf.
func (ct *contentTree[C]) Insert(pos int, run C) {
	if run.Len() == 0 {
		return
	}
	n := ct.Len()
	if n == 0 {
		_ = ct.tree.InsertRange(0, []C{run})
		return
	}
	if pos == 0 {
		// Fold into first leaf if it fits.
		if first, err := ct.tree.GetAt(0); err == nil && (*first).Len()+run.Len() <= ropeLeafCap {
			ct.replaceLeaf(0, run.Concat(*first))
			return
		}
		_ = ct.tree.InsertRange(0, []C{run})
		return
	}
	if pos == n {
		last := ct.tree.Size() - 1
		if tail, err := ct.tree.GetAt(last); err == nil && (*tail).Len()+run.Len() <= ropeLeafCap {
			ct.replaceLeaf(last, (*tail).Concat(run))
			return
		}
		_ = ct.tree.InsertRange(ct.tree.Size(), []C{run})
		return
	}

	// Interior: pos in (0, n). Locate the leaf containing character pos.
	idx, leaf, offset := ct.locateChar(pos)
	if offset == 0 {
		// Boundary between leaves: fold into the left neighbour if it fits.
		if left, err := ct.tree.GetAt(idx - 1); err == nil && (*left).Len()+run.Len() <= ropeLeafCap {
			ct.replaceLeaf(idx-1, (*left).Concat(run))
			return
		}
		_ = ct.tree.InsertRange(idx, []C{run})
		return
	}

	a, b := leaf.SplitAt(offset)
	if a.Len()+run.Len() <= ropeLeafCap {
		ct.replaceLeaf(idx, a.Concat(run), b)
		return
	}
	ct.replaceLeaf(idx, a, run, b)
}

// Delete removes `length` characters starting at character position pos.
//
// Cases:
//   - whole document: delete every leaf;
//   - range inside one leaf: SplitAt the leaf into [before][range][after] and
//     replace it with the before/after halves;
//   - multi-leaf: if the first leaf is only partially in range, split it so the
//     range starts on a leaf boundary; if the last leaf is only partially in
//     range, split it so the range ends on a leaf boundary (dropping only the
//     in-range prefix of that leaf); then DeleteRange the whole interior leaves.
func (ct *contentTree[C]) Delete(pos, length int) {
	if length <= 0 {
		return
	}
	n := ct.Len()
	if n == 0 || pos < 0 || pos+length > n {
		panic("crdt: rope Delete out of range")
	}
	posEnd := pos + length

	if length == n { // pos == 0 necessarily
		if err := ct.tree.DeleteRange(0, ct.tree.Size()); err != nil {
			panic("crdt: rope DeleteRange: " + err.Error())
		}
		return
	}

	iL, L, oL := ct.locateChar(pos)
	iR, _, _ := ct.locateChar(posEnd - 1)

	if iL == iR {
		// Single-leaf deletion: keep [before] and [after].
		before, rest := L.SplitAt(oL)    // before: oL chars; rest: the remainder
		_, after := rest.SplitAt(length) // after: chars after the deleted range
		ct.replaceLeaf(iL, before, after)
		return
	}

	// Multi-leaf. First make the range start on a leaf boundary.
	delStart := iL
	if oL > 0 {
		before, inRange := L.SplitAt(oL) // inRange starts at pos, fully deleted
		ct.replaceLeaf(iL, before, inRange)
		delStart = iL + 1
	}

	// Re-locate the right boundary leaf (indices shifted by the left split).
	iR2, R2, oR2 := ct.locateChar(posEnd - 1)
	end := iR2 + 1 // exclusive index: optimistic, R2 fully in range
	if oR2+1 < R2.Len() {
		// R2 extends past the range: keep its suffix after the last in-range char.
		_, after := R2.SplitAt(oR2 + 1)
		ct.replaceLeaf(iR2, after)
		end = iR2
	}
	if err := ct.tree.DeleteRange(delStart, end-delStart); err != nil {
		panic("crdt: rope DeleteRange: " + err.Error())
	}
}

// ForEachContent visits every leaf's content value in document order.
func (ct *contentTree[C]) ForEachContent(f func(C)) {
	if ct == nil {
		return
	}
	for item := range ct.tree.All() {
		f(item)
	}
}
