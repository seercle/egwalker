package crdt

import "egwalker/bxtree"

// ropeLeafCap bounds the size of a folded rope leaf. Leaves are immutable
// content values; folding two adjacent leaves costs O(result) via Concat, so
// the cap keeps repeated single-character edits amortized and bounds leaf
// fragmentation to ~Len/ropeLeafCap. A single large insert may still be one
// leaf of its full length.
const ropeLeafCap = 256

// ropeLeaf is one rope leaf: an immutable content run plus its cached
// character count. Caches exist because C is a byte-backed value (runeText)
// whose Len() would otherwise rescan the whole run; the summarizer and the
// fold checks run on every positional operation, so they must be O(1).
// Counts are maintained arithmetically at split/concat time (SplitAt splits
// after the k-th character, so half lengths are known without rescanning);
// only the initial run.Len() at Insert time ever scans.
type ropeLeaf[C content[C]] struct {
	c C
	n int
}

// ropeSummarizer provides the character-count summary for the content tree:
// each leaf contributes its cached run length, and a node's summary is the
// total characters under it (bxtree.Size() counts LEAVES, not chars, so all
// positional work goes through this summary).
type ropeSummarizer[C content[C]] struct{}

func (ropeSummarizer[C]) FromItem(l ropeLeaf[C]) int { return l.n }
func (ropeSummarizer[C]) Add(a, b int) int           { return a + b }
func (ropeSummarizer[C]) Sub(a, b int) int           { return a - b }

// contentTree is a run-based content tree (rope): a bxtree whose leaves are
// content runs, positionally indexed by character count. Insert/Delete take
// character positions in [0, Len]. Leaves are immutable content values; the
// tree never stores an empty leaf.
type contentTree[C content[C]] struct {
	tree *bxtree.BxTree[ropeLeaf[C], int]
}

func newContentTree[C content[C]]() *contentTree[C] {
	tree, err := bxtree.New[ropeLeaf[C], int](bxtree.WithSummarizer[ropeLeaf[C], int](ropeSummarizer[C]{}))
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

// locate returns the item index of the leaf containing content position pos,
// that leaf, and the offset within it. pos must be in [0, Len) and is in
// content units — characters for runeText, elements for itemRun — matching the
// Len() unit of the content contract. A leaf starting at pos has offset 0.
func (ct *contentTree[C]) locate(pos int) (idx int, leaf ropeLeaf[C], offset int) {
	node, posInNode, acc := ct.tree.FindPath(func(acc, cur int) bool {
		return acc+cur > pos
	})
	if node == nil {
		panic("crdt: rope locate out of range")
	}
	leaf = node.Items()[posInNode]
	return node.Index() + posInNode, leaf, pos - acc
}

// replaceLeaf replaces the leaf at item index idx with parts (skipping empty
// parts), keeping document order.
func (ct *contentTree[C]) replaceLeaf(idx int, parts ...ropeLeaf[C]) {
	nonEmpty := parts[:0]
	for _, p := range parts {
		if p.n > 0 {
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

// mergeWithLeft merges the leaf at item index idx into its left neighbour
// (idx-1) when the combined length fits ropeLeafCap. It is a no-op when idx
// is out of range or the pair does not fit. Used at the seams a Delete
// creates, mirroring Insert's boundary folding: edit points do not
// accumulate fragmentation.
func (ct *contentTree[C]) mergeWithLeft(idx int) {
	if idx <= 0 || idx >= ct.tree.Size() {
		return
	}
	left, errL := ct.tree.GetAt(idx - 1)
	cur, errR := ct.tree.GetAt(idx)
	if errL != nil || errR != nil || left.n+cur.n > ropeLeafCap {
		return
	}
	// GetAt returns pointers into node storage; build merged before the
	// DeleteAt calls below, which shift items under those pointers.
	merged := ropeLeaf[C]{c: left.c.Concat(cur.c), n: left.n + cur.n}
	if err := ct.tree.DeleteAt(idx); err != nil {
		panic("crdt: rope DeleteAt: " + err.Error())
	}
	if err := ct.tree.DeleteAt(idx - 1); err != nil {
		panic("crdt: rope DeleteAt: " + err.Error())
	}
	if err := ct.tree.InsertRange(idx-1, []ropeLeaf[C]{merged}); err != nil {
		panic("crdt: rope InsertRange: " + err.Error())
	}
}

// Insert inserts run (Len > 0) at character position pos.
//
// Positions that fall on a leaf boundary fold the run into the left neighbour
// when the result fits ropeLeafCap (keeping append/prepend/contiguous typing to
// ~Len/ropeLeafCap leaves); positions interior to a leaf split the leaf.
func (ct *contentTree[C]) Insert(pos int, run C) {
	rn := run.Len()
	if rn == 0 {
		return
	}
	rl := ropeLeaf[C]{c: run, n: rn}
	n := ct.Len()
	if n == 0 {
		_ = ct.tree.InsertRange(0, []ropeLeaf[C]{rl})
		return
	}
	if pos == 0 {
		// Fold into first leaf if it fits.
		if first, err := ct.tree.GetAt(0); err == nil && first.n+rn <= ropeLeafCap {
			ct.replaceLeaf(0, ropeLeaf[C]{c: run.Concat(first.c), n: rn + first.n})
			return
		}
		_ = ct.tree.InsertRange(0, []ropeLeaf[C]{rl})
		return
	}
	if pos == n {
		last := ct.tree.Size() - 1
		if tail, err := ct.tree.GetAt(last); err == nil && tail.n+rn <= ropeLeafCap {
			ct.replaceLeaf(last, ropeLeaf[C]{c: tail.c.Concat(run), n: tail.n + rn})
			return
		}
		_ = ct.tree.InsertRange(ct.tree.Size(), []ropeLeaf[C]{rl})
		return
	}

	// Interior: pos in (0, n). Locate the leaf containing character pos.
	idx, leaf, offset := ct.locate(pos)
	if offset == 0 {
		// Boundary between leaves: fold into the left neighbour if it fits.
		if left, err := ct.tree.GetAt(idx - 1); err == nil && left.n+rn <= ropeLeafCap {
			ct.replaceLeaf(idx-1, ropeLeaf[C]{c: left.c.Concat(run), n: left.n + rn})
			return
		}
		_ = ct.tree.InsertRange(idx, []ropeLeaf[C]{rl})
		return
	}

	// SplitAt splits after the k-th character, so the half lengths are exactly
	// offset and leaf.n-offset; no rescanning needed.
	a, b := leaf.c.SplitAt(offset)
	if offset+rn <= ropeLeafCap {
		ct.replaceLeaf(idx, ropeLeaf[C]{c: a.Concat(run), n: offset + rn}, ropeLeaf[C]{c: b, n: leaf.n - offset})
		return
	}
	ct.replaceLeaf(idx, ropeLeaf[C]{c: a, n: offset}, rl, ropeLeaf[C]{c: b, n: leaf.n - offset})
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
//     in-range prefix of that leaf); then DeleteRange the whole interior leaves;
//   - every structural seam is coalesced afterwards (mergeWithLeft), so
//     repeated edits do not accumulate fragmented leaves.
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

	iL, L, oL := ct.locate(pos)
	// For length == 1 the right edge is pos itself, so a second locate would
	// return exactly what we already hold — reuse it.
	iR := iL
	if length > 1 {
		iR, _, _ = ct.locate(posEnd - 1)
	}

	if iL == iR {
		// Single-leaf deletion: keep [before] and [after]. Half lengths are
		// derived from the offset and the leaf's cached count.
		before, rest := L.c.SplitAt(oL)  // before: oL chars; rest: the remainder
		_, after := rest.SplitAt(length) // after: chars after the deleted range
		ct.replaceLeaf(iL, ropeLeaf[C]{c: before, n: oL}, ropeLeaf[C]{c: after, n: L.n - oL - length})
		if oL == 0 {
			ct.mergeWithLeft(iL) // leaf start: seam is (iL-1, survivor|old-next)
		} else {
			ct.mergeWithLeft(iL + 1) // split point: seam is (before, after|old-next)
		}
		return
	}

	// Multi-leaf. First make the range start on a leaf boundary.
	delStart := iL
	if oL > 0 {
		before, inRange := L.c.SplitAt(oL) // inRange starts at pos, fully deleted
		ct.replaceLeaf(iL, ropeLeaf[C]{c: before, n: oL}, ropeLeaf[C]{c: inRange, n: L.n - oL})
		delStart = iL + 1
	}

	// Re-locate the right boundary leaf (indices shifted by the left split).
	iR2, R2, oR2 := ct.locate(posEnd - 1)
	end := iR2 + 1 // exclusive index: optimistic, R2 fully in range
	if oR2+1 < R2.n {
		// R2 extends past the range: keep its suffix after the last in-range char.
		_, after := R2.c.SplitAt(oR2 + 1)
		ct.replaceLeaf(iR2, ropeLeaf[C]{c: after, n: R2.n - oR2 - 1})
		end = iR2
	}
	if err := ct.tree.DeleteRange(delStart, end-delStart); err != nil {
		panic("crdt: rope DeleteRange: " + err.Error())
	}
	ct.mergeWithLeft(delStart) // re-join the neighbours of the excised range
}

// ForEachContent visits every leaf's content value in document order.
func (ct *contentTree[C]) ForEachContent(f func(C)) {
	if ct == nil {
		return
	}
	for item := range ct.tree.All() {
		f(item.c)
	}
}
