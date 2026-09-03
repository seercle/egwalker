package crdt

import (
	"reflect"
	"strings"
)

// ==========================================
// Content-generic core
// ==========================================

// doc is the content-generic CRDT core shared by every document family. The
// visible document is the branch's contentTree (a rope of content runs);
// element type E has been erased from the engine and lives only in the family
// wrappers and concrete run types.
type doc[C content[C]] struct {
	opLog  *opLog[C]
	agent  int
	branch *branch[C]
}

func newDoc[C content[C]](agent int) *doc[C] {
	return &doc[C]{opLog: newOpLog[C](), agent: agent, branch: newBranch[C]()}
}

// Len returns the number of characters (elements) in the visible document.
func (d *doc[C]) Len() int { return d.branch.snapshot.Len() }

// syncRun inserts run content into the visible snapshot and copies the frontier
// to the branch. pos is the character position of the run's first element.
func (d *doc[C]) syncRun(pos int, run C) {
	d.branch.snapshot.Insert(pos, run)
	d.branch.frontier = make([]lv, len(d.opLog.frontier))
	copy(d.branch.frontier, d.opLog.frontier)
}

// InsRun pushes one whole insert run op (collapsing into the tail run when
// allowed — see op_log.go pushLocalOp) and syncs the snapshot. Callers must
// only pass collapsible content (runeText, or itemRun whose elements are not
// Mergeable), which is exactly the old whole-run localInsert path.
func (d *doc[C]) InsRun(pos int, run C) {
	if run.Len() == 0 {
		return
	}
	d.opLog.pushLocalOp(d.agent, op[C]{opType: opTypeIns, content: run, pos: pos})
	d.syncRun(pos, run)
}

// Del deletes delLen characters starting at pos. Delete ops carry no content:
// localDelete pushes a run op with a zero C and an authoritative length.
func (d *doc[C]) Del(pos, delLen int) {
	localDelete(d.opLog, d.agent, pos, delLen)
	d.branch.snapshot.Delete(pos, delLen)
	d.branch.frontier = make([]lv, len(d.opLog.frontier))
	copy(d.branch.frontier, d.opLog.frontier)
}

// Reset clears the document state.
func (d *doc[C]) Reset() {
	d.opLog = newOpLog[C]()
	d.branch = newBranch[C]()
}

// mergeFrom performs the batch log merge and a single checkoutFancy. Element
// recursion is the families' job and must happen BEFORE this call.
func (d *doc[C]) mergeFrom(other *doc[C]) {
	if d == other {
		return
	}
	mergeInto(d.opLog, other.opLog)
	checkoutFancy(d.opLog, d.branch, d.opLog.frontier)
}

// check replays the full log and compares visible content with the branch
// snapshot. equal renders both sides through the family so leaf-boundary
// differences never matter.
func (d *doc[C]) check(equal func(a, b *contentTree[C]) bool) {
	replay := checkout(d.opLog)
	if !equal(replay, d.branch.snapshot) {
		panic("Document content out of sync")
	}
}

// ==========================================
// RuneDocument
// ==========================================

// RuneDocument represents a CRDT text document. It has no element type
// parameter; its run content is runeText.
type RuneDocument struct {
	doc *doc[runeText]
}

// NewRuneDocument creates a new CRDT document for runes with the given agent ID.
func NewRuneDocument(agent int) *RuneDocument {
	return &RuneDocument{doc: newDoc[runeText](agent)}
}

// Ins inserts the given text at the specified position.
func (doc *RuneDocument) Ins(pos int, text string) {
	doc.doc.InsRun(pos, runeText(text))
}

// GetString returns the current text content of the document.
func (doc *RuneDocument) GetString() string {
	var sb strings.Builder
	doc.doc.branch.snapshot.ForEachContent(func(r runeText) { sb.WriteString(string(r)) })
	return sb.String()
}

// Len returns the number of runes in the document.
func (doc *RuneDocument) Len() int { return doc.doc.Len() }

// Del deletes n runes starting from the specified position.
func (doc *RuneDocument) Del(pos, n int) {
	doc.doc.Del(pos, n)
}

// MergeFrom merges changes from another document. Runes are never Mergeable, so
// there is no recursion pass.
func (doc *RuneDocument) MergeFrom(other *RuneDocument) {
	doc.doc.mergeFrom(other.doc)
}

// Reset clears the document state.
func (doc *RuneDocument) Reset() { doc.doc.Reset() }

// Check verifies that the document's local branch matches a full checkout.
func (doc *RuneDocument) Check() {
	doc.doc.check(func(a, b *contentTree[runeText]) bool {
		var sa, sb strings.Builder
		a.ForEachContent(func(r runeText) { sa.WriteString(string(r)) })
		b.ForEachContent(func(r runeText) { sb.WriteString(string(r)) })
		return sa.String() == sb.String()
	})
}

func (doc *RuneDocument) MergeFromAny(other any) {
	if o, ok := other.(*RuneDocument); ok {
		doc.MergeFrom(o)
	}
}

// ==========================================
// ArrayDocument
// ==========================================

// ArrayDocument represents a generic CRDT array document.
type ArrayDocument[T any] struct {
	doc *doc[itemRun[T]]
}

// NewArrayDocument creates a new generic CRDT array document.
func NewArrayDocument[T any](agent int) *ArrayDocument[T] {
	return &ArrayDocument[T]{doc: newDoc[itemRun[T]](agent)}
}

// Ins inserts the given items at the specified position. Inserts fan out per
// element ONLY when elements are Mergeable (they must keep their own length-1
// op so recursion can match by id); otherwise the whole slice is pushed as one
// run op. The snapshot sync is one whole-run insert either way.
func (doc *ArrayDocument[T]) Ins(pos int, items []T) {
	if len(items) > 0 {
		if _, ok := any(items[0]).(Mergeable); ok {
			for i, item := range items {
				doc.doc.opLog.pushLocalOp(doc.doc.agent, op[itemRun[T]]{
					opType:  opTypeIns,
					content: itemRun[T]{item},
					pos:     pos + i,
				})
			}
			doc.doc.syncRun(pos, itemRun[T](items))
			return
		}
	}
	doc.doc.InsRun(pos, itemRun[T](items))
}

// GetItems returns all items in the array.
func (doc *ArrayDocument[T]) GetItems() []T {
	out := make([]T, 0, doc.Len())
	doc.doc.branch.snapshot.ForEachContent(func(r itemRun[T]) { out = append(out, []T(r)...) })
	return out
}

// MergeFrom merges changes from another document. Mergeable children are
// reconciled by identity first (mergeRecursive), then the log is merged.
func (doc *ArrayDocument[T]) MergeFrom(other *ArrayDocument[T]) {
	if doc == other {
		return
	}
	doc.mergeRecursive(other)
	doc.doc.mergeFrom(other.doc)
}

// Len returns the number of elements in the document.
func (doc *ArrayDocument[T]) Len() int { return doc.doc.Len() }

// Del deletes n elements starting from the specified position.
func (doc *ArrayDocument[T]) Del(pos, n int) {
	doc.doc.Del(pos, n)
}

// Reset clears the document state.
func (doc *ArrayDocument[T]) Reset() { doc.doc.Reset() }

// Check verifies that the document's local branch matches a full checkout.
func (doc *ArrayDocument[T]) Check() {
	doc.doc.check(func(a, b *contentTree[itemRun[T]]) bool {
		var fa, fb []T
		a.ForEachContent(func(r itemRun[T]) { fa = append(fa, []T(r)...) })
		b.ForEachContent(func(r itemRun[T]) { fb = append(fb, []T(r)...) })
		return reflect.DeepEqual(fa, fb)
	})
}

func (doc *ArrayDocument[T]) MergeFromAny(other any) {
	if o, ok := other.(*ArrayDocument[T]); ok {
		doc.MergeFrom(o)
	}
}

// mergeRecursive mirrors the old Document[E,C] recursion pass, with itemRun
// element access inlined: for each op in other whose first element is Mergeable
// and which we already hold, resolve our op by id and call MergeFromAny on the
// element. Runs whose elements are Mergeable never collapse, so ids match.
func (doc *ArrayDocument[T]) mergeRecursive(other *ArrayDocument[T]) {
	oLog, mLog := doc.doc.opLog, other.doc.opLog
	for _, o := range mLog.ops {
		elems := []T(o.content)
		if len(elems) == 0 {
			continue
		}
		if _, ok := any(elems[0]).(Mergeable); !ok {
			continue
		}
		if lastSeq, ok := oLog.version[o.id.agent]; ok && lastSeq >= o.id.seq {
			ourLV := idToLV(oLog, o.id)
			our := []T(oLog.opAt(ourLV).content)
			if len(our) > 0 && len(elems) > 0 {
				if m, ok := any(our[0]).(Mergeable); ok {
					m.MergeFromAny(elems[0])
				}
			}
		}
	}
}

// ==========================================
// MapDocument (LWW)
// ==========================================

// MapDocument represents a CRDT map document using LWW strategy.
type MapDocument[K comparable, V any] struct {
	agent    int
	opLog    *opLog[mapRun[K, V]]
	keyIndex map[K][]lv
}

// NewMapDocument creates a new generic CRDT map document.
func NewMapDocument[K comparable, V any](agent int) *MapDocument[K, V] {
	return &MapDocument[K, V]{
		agent:    agent,
		opLog:    newOpLog[mapRun[K, V]](),
		keyIndex: make(map[K][]lv),
	}
}

// Set sets the value for the given key.
func (m *MapDocument[K, V]) Set(key K, value V) {
	curLV := m.opLog.pushLocalOp(m.agent, op[mapRun[K, V]]{
		content: mapRun[K, V]{{Key: key, Value: value}},
	})
	m.keyIndex[key] = append(m.keyIndex[key], curLV)
}

// Get returns the value for the given key using LWW strategy, with recursive merging for Mergeable values.
func (m *MapDocument[K, V]) Get(key K) (V, bool) {
	var concurrentLVs []lv

	for _, curLV := range m.keyIndex[key] {
		// Filter out any existing concurrent LVs that are ancestors of this one
		nextConcurrent := []lv{curLV}
		for _, existingLV := range concurrentLVs {
			if !m.opLog.isAncestor(existingLV, curLV) {
				nextConcurrent = append(nextConcurrent, existingLV)
			}
		}
		concurrentLVs = nextConcurrent
	}

	if len(concurrentLVs) == 0 {
		return *new(V), false
	}

	// Pick the "LWW" winner as the primary value
	var bestV V
	var bestID id
	var bestLV lv
	first := true

	for _, l := range concurrentLVs {
		o := m.opLog.opAt(l)
		if first || o.id.agent > bestID.agent || (o.id.agent == bestID.agent && o.id.seq > bestID.seq) {
			bestV = o.content[0].Value
			bestID = o.id
			bestLV = l
			first = false
		}
	}

	// Recursive merge if V is Mergeable
	if mergeable, ok := any(bestV).(Mergeable); ok {
		for _, l := range concurrentLVs {
			if l != bestLV {
				mergeable.MergeFromAny(m.opLog.opAt(l).content[0].Value)
			}
		}
	}

	return bestV, true
}

// MergeFrom merges changes from another map document.
func (m *MapDocument[K, V]) MergeFrom(other *MapDocument[K, V]) {
	if m == other {
		return
	}
	// Recursive merge for items with the same identity (LV). Map run content
	// always holds exactly one MapOp; recursion applies to its .Value element
	// when that value is Mergeable (map ops never collapse, so ids match).
	for _, o := range other.opLog.ops {
		otherElems := o.content
		if len(otherElems) == 0 {
			continue
		}
		if _, ok := any(otherElems[0].Value).(Mergeable); !ok {
			continue
		}
		if lastSeq, ok := m.opLog.version[o.id.agent]; ok && lastSeq >= o.id.seq {
			// We have this op. Find our version and merge if mergeable.
			ourLV := idToLV(m.opLog, o.id)
			ourElems := m.opLog.opAt(ourLV).content
			if len(ourElems) > 0 && len(otherElems) > 0 {
				if mrg, ok := any(ourElems[0].Value).(Mergeable); ok {
					mrg.MergeFromAny(otherElems[0].Value)
				}
			}
		}
	}
	oldLen := len(m.opLog.ops)
	mergeInto(m.opLog, other.opLog)
	for i := oldLen; i < len(m.opLog.ops); i++ {
		o := m.opLog.ops[i]
		m.keyIndex[o.content[0].Key] = append(m.keyIndex[o.content[0].Key], m.opLog.opLV[i])
	}
}

func (m *MapDocument[K, V]) MergeFromAny(other any) {
	if o, ok := other.(*MapDocument[K, V]); ok {
		m.MergeFrom(o)
	}
}

// Keys returns all keys that have been set in the map.
func (m *MapDocument[K, V]) Keys() []K {
	keysMap := make(map[K]bool)
	for _, o := range m.opLog.ops {
		keysMap[o.content[0].Key] = true
	}
	keys := make([]K, 0, len(keysMap))
	for k := range keysMap {
		keys = append(keys, k)
	}
	return keys
}
