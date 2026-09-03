package crdt

// content is the payload of an operation. Text/array logs store a run of
// E elements (a single-character insert is a run of length 1); map logs store
// a 1-element run. Len() is the number of characters/elements the run spans,
// which equals the number of logical versions (lvs) and per-agent sequence
// numbers the op covers.
type content[E any] interface {
	Len() int
	Elems() []E
	Concat(content[E]) content[E]
	fromOne(E) content[E]     // returns a length-1 run wrapping e
	fromElems([]E) content[E] // returns a run wrapping the whole slice
}

// runeText is the string-backed run content used by RuneDocument. Len counts
// runes, not bytes, so lv/seq arithmetic matches the per-character model.
type runeText string

func (t runeText) Len() int { return len([]rune(t)) }
func (t runeText) Elems() []rune {
	return []rune(t)
}
func (t runeText) Concat(o content[rune]) content[rune] {
	return runeText(string(t) + string(o.(runeText)))
}
func (t runeText) fromOne(e rune) content[rune] {
	return runeText(string(e))
}
func (t runeText) fromElems(e []rune) content[rune] {
	return runeText(string(e))
}

// itemRun is the slice-backed run content used by ArrayDocument and MapDocument.
type itemRun[E any] []E

func (r itemRun[E]) Len() int { return len(r) }
func (r itemRun[E]) Elems() []E {
	return r
}
func (r itemRun[E]) Concat(o content[E]) content[E] {
	return append(append(itemRun[E]{}, r...), o.(itemRun[E])...)
}
func (r itemRun[E]) fromOne(e E) content[E] {
	return itemRun[E]{e}
}
func (r itemRun[E]) fromElems(e []E) content[E] {
	return itemRun[E](append([]E(nil), e...))
}

// mapRun is the run content for MapDocument ops. Map ops are never merged
// (opType is not opTypeIns), so runs always have length 1.
type mapRun[K comparable, V any] []MapOp[K, V]

func (r mapRun[K, V]) Len() int { return len(r) }
func (r mapRun[K, V]) Elems() []MapOp[K, V] {
	return r
}
func (r mapRun[K, V]) Concat(o content[MapOp[K, V]]) content[MapOp[K, V]] {
	return append(append(mapRun[K, V]{}, r...), o.(mapRun[K, V])...)
}
func (r mapRun[K, V]) fromOne(e MapOp[K, V]) content[MapOp[K, V]] {
	return mapRun[K, V]{e}
}
func (r mapRun[K, V]) fromElems(e []MapOp[K, V]) content[MapOp[K, V]] {
	return mapRun[K, V](append([]MapOp[K, V](nil), e...))
}
