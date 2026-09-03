package crdt

import (
	"reflect"
	"strings"
)

// ==========================================
// Generic Document Methods
// ==========================================

// NewDocument creates a new generic CRDT document.
func NewDocument[E any, C content[E]](agent int) Document[E, C] {
	return Document[E, C]{
		opLog:  newOpLog[E, C](),
		agent:  agent,
		branch: newBranch[E](),
	}
}

// Len returns the number of items in the document.
func (doc *Document[E, C]) Len() int {
	return doc.branch.snapshot.Size()
}

// Check verifies that the document's local branch matches a full checkout.
func (doc *Document[E, C]) Check() {
	actualTree := checkout(doc.opLog)
	if actualTree.Size() != doc.branch.snapshot.Size() {
		panic("Document size out of sync")
	}

	actualItems := make([]E, 0, actualTree.Size())
	actualTree.ForEach(func(item E) {
		actualItems = append(actualItems, item)
	})

	docItems := make([]E, 0, doc.branch.snapshot.Size())
	doc.branch.snapshot.ForEach(func(item E) {
		docItems = append(docItems, item)
	})

	if !reflect.DeepEqual(actualItems, docItems) {
		panic("Document content out of sync")
	}
}

// Ins inserts the given items at the specified position. The whole slice is
// pushed as a single run op (consecutive adjacent edits from the same agent
// collapse into one run).
func (doc *Document[E, C]) Ins(pos int, items []E) {
	localInsert(doc.opLog, doc.agent, pos, items)

	err := doc.branch.snapshot.InsertRange(pos, items)
	if err != nil {
		panic("Snapshot insert failed")
	}

	doc.branch.frontier = make([]lv, len(doc.opLog.frontier))
	copy(doc.branch.frontier, doc.opLog.frontier)
}

// Del deletes delLen items starting from the specified position.
func (doc *Document[E, C]) Del(pos int, delLen int) {
	localDelete(doc.opLog, doc.agent, pos, delLen)

	err := doc.branch.snapshot.DeleteRange(pos, delLen)
	if err != nil {
		panic("Snapshot delete failed")
	}

	doc.branch.frontier = make([]lv, len(doc.opLog.frontier))
	copy(doc.branch.frontier, doc.opLog.frontier)
}

// MergeFrom merges changes from another document.
func (doc *Document[E, C]) MergeFrom(other *Document[E, C]) {
	if doc == other {
		return
	}
	// 1. Recursive merge for items with the same identity (LV). Content is a
	// run; recursion applies to the elements it holds. Only single-element ops
	// whose element is Mergeable participate: runs whose elements are Mergeable
	// never collapse, so both replicas hold such ops under the same id and a
	// plain id lookup resolves them (run ops of plain content may live under
	// different boundaries on each replica and are skipped here).
	for _, o := range other.opLog.ops {
		otherElems := o.content.Elems()
		if len(otherElems) == 0 {
			continue
		}
		if _, ok := any(otherElems[0]).(Mergeable); !ok {
			continue
		}
		if lastSeq, ok := doc.opLog.version[o.id.agent]; ok && lastSeq >= o.id.seq {
			// We have this op. Find our version and merge if mergeable.
			ourLV := idToLV(doc.opLog, o.id)
			ourElems := doc.opLog.opAt(ourLV).content.Elems()
			if len(ourElems) > 0 && len(otherElems) > 0 {
				if m, ok := any(ourElems[0]).(Mergeable); ok {
					m.MergeFromAny(otherElems[0])
				}
			}
		}
	}

	// 2. Batch merge into opLog
	mergeInto(doc.opLog, other.opLog)

	// 3. Perform a single checkoutFancy to move the state
	checkoutFancy(doc.opLog, doc.branch, doc.opLog.frontier)
}

// Reset clears the document state.
func (doc *Document[E, C]) Reset() {
	doc.opLog = newOpLog[E, C]()
	doc.branch = newBranch[E]()
}

// ==========================================
// RuneDocument
// ==========================================

// NewRuneDocument creates a new CRDT document for runes with the given agent ID.
func NewRuneDocument(agent int) *RuneDocument {
	return &RuneDocument{
		Document: NewDocument[rune, runeText](agent),
	}
}

// Ins inserts the given text at the specified position.
func (doc *RuneDocument) Ins(pos int, text string) {
	chars := []rune(text)
	doc.Document.Ins(pos, chars)
}

// MergeFrom merges changes from another document.
func (doc *RuneDocument) MergeFrom(other *RuneDocument) {
	if doc == other {
		return
	}
	doc.Document.MergeFrom(&other.Document)
}

// GetString returns the current text content of the document.
func (doc *RuneDocument) GetString() string {
	var sb strings.Builder
	doc.branch.snapshot.ForEach(func(r rune) {
		sb.WriteRune(r)
	})
	return sb.String()
}

// ==========================================
// ArrayDocument
// ==========================================

// NewArrayDocument creates a new generic CRDT array document.
func NewArrayDocument[T any](agent int) *ArrayDocument[T] {
	return &ArrayDocument[T]{
		Document: NewDocument[T, itemRun[T]](agent),
	}
}

// MergeFrom merges changes from another document.
func (doc *ArrayDocument[T]) MergeFrom(other *ArrayDocument[T]) {
	if doc == other {
		return
	}
	doc.Document.MergeFrom(&other.Document)
}

// GetItems returns all items in the array.
func (doc *ArrayDocument[T]) GetItems() []T {
	items := make([]T, 0, doc.Len())
	doc.branch.snapshot.ForEach(func(item T) {
		items = append(items, item)
	})
	return items
}

// ==========================================
// MapDocument (LWW)
// ==========================================

// NewMapDocument creates a new generic CRDT map document.
func NewMapDocument[K comparable, V any](agent int) *MapDocument[K, V] {
	return &MapDocument[K, V]{
		agent:    agent,
		opLog:    newOpLog[MapOp[K, V], mapRun[K, V]](),
		keyIndex: make(map[K][]lv),
	}
}

// Set sets the value for the given key.
func (m *MapDocument[K, V]) Set(key K, value V) {
	curLV := m.opLog.pushLocalOp(m.agent, op[MapOp[K, V], mapRun[K, V]]{
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
			bestV = o.content.Elems()[0].Value
			bestID = o.id
			bestLV = l
			first = false
		}
	}

	// Recursive merge if V is Mergeable
	if mergeable, ok := any(bestV).(Mergeable); ok {
		for _, l := range concurrentLVs {
			if l != bestLV {
				mergeable.MergeFromAny(m.opLog.opAt(l).content.Elems()[0].Value)
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
		otherElems := o.content.Elems()
		if len(otherElems) == 0 {
			continue
		}
		if _, ok := any(otherElems[0].Value).(Mergeable); !ok {
			continue
		}
		if lastSeq, ok := m.opLog.version[o.id.agent]; ok && lastSeq >= o.id.seq {
			// We have this op. Find our version and merge if mergeable.
			ourLV := idToLV(m.opLog, o.id)
			ourElems := m.opLog.opAt(ourLV).content.Elems()
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
		m.keyIndex[o.content.Elems()[0].Key] = append(m.keyIndex[o.content.Elems()[0].Key], m.opLog.opLV[i])
	}
}

// Keys returns all keys that have been set in the map.
func (m *MapDocument[K, V]) Keys() []K {
	keysMap := make(map[K]bool)
	for _, o := range m.opLog.ops {
		keysMap[o.content.Elems()[0].Key] = true
	}
	keys := make([]K, 0, len(keysMap))
	for k := range keysMap {
		keys = append(keys, k)
	}
	return keys
}
