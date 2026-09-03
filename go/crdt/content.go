package crdt

// content is a run of document content. Text runs hold characters, array runs
// hold elements, map runs hold a single map op. The CRDT machinery only needs a
// run's length and boundaries; the element type is a property of each concrete
// run type and is handled only at the boundaries where content meets the world.
type content[C any] interface {
	Len() int
	SplitAt(k int) (C, C) // split after the k-th character; 0 <= k <= Len
	Concat(C) C
	Collapsible() bool // may two adjacent same-agent runs of this type fuse
}

// runeText is the string-backed run content used by RuneDocument. Len counts
// runes, not bytes, so lv/seq arithmetic matches the per-character model.
type runeText string

func (t runeText) Len() int { return len([]rune(t)) }

// SplitAt splits after the k-th rune. runeText is byte-backed, so the split
// must land on a rune boundary (scan to the k-th rune, then cut bytes).
func (t runeText) SplitAt(k int) (runeText, runeText) {
	if k <= 0 {
		return "", t
	}
	if k >= t.Len() {
		return t, ""
	}
	rs := []rune(t)
	return runeText(string(rs[:k])), runeText(string(rs[k:]))
}

func (t runeText) Concat(o runeText) runeText { return runeText(string(t) + string(o)) }
func (t runeText) Collapsible() bool          { return true }

// itemRun is the slice-backed run content used by ArrayDocument.
type itemRun[E any] []E

func (r itemRun[E]) Len() int { return len(r) }
func (r itemRun[E]) SplitAt(k int) (itemRun[E], itemRun[E]) {
	if k <= 0 {
		return nil, r
	}
	if k >= len(r) {
		return r, nil
	}
	return r[:k:k], r[k:]
}
func (r itemRun[E]) Concat(o itemRun[E]) itemRun[E] {
	return append(append(itemRun[E]{}, r...), o...)
}

// Collapsible reports whether consecutive same-agent runs of this content may
// fuse. Content holding Mergeable elements must stay per-element so the
// recursive-merge path (document.go) can still match ops by id. E is uniform
// across a run, so inspecting the first element is type-accurate.
func (r itemRun[E]) Collapsible() bool {
	if len(r) == 0 {
		return true
	}
	_, mergeable := any(r[0]).(Mergeable)
	return !mergeable
}

// mapRun is the run content for MapDocument ops. Map ops are never merged
// (opType is not opTypeIns), so runs always have length 1.
type mapRun[K comparable, V any] []MapOp[K, V]

func (r mapRun[K, V]) Len() int { return len(r) }
func (r mapRun[K, V]) SplitAt(k int) (mapRun[K, V], mapRun[K, V]) {
	if k <= 0 {
		return nil, r
	}
	if k >= len(r) {
		return r, nil
	}
	return r[:k:k], r[k:]
}
func (r mapRun[K, V]) Concat(o mapRun[K, V]) mapRun[K, V] {
	return append(append(mapRun[K, V]{}, r...), o...)
}
func (r mapRun[K, V]) Collapsible() bool { return false }
