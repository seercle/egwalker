package crdt

import (
	"reflect"
	"strings"
)

// ==========================================
// Generic Document Methods
// ==========================================

// NewDocument creates a new generic CRDT document.
func NewDocument[T any](agent int) Document[T] {
	return Document[T]{
		opLog:  newOpLog[T](),
		agent:  agent,
		branch: newBranch[T](),
	}
}

// Len returns the number of items in the document.
func (doc *Document[T]) Len() int {
	return doc.branch.snapshot.Size()
}

// Check verifies that the document's local branch matches a full checkout.
func (doc *Document[T]) Check() {
	actualTree := checkout(doc.opLog)
	if actualTree.Size() != doc.branch.snapshot.Size() {
		panic("Document size out of sync")
	}

	actualItems := make([]T, 0, actualTree.Size())
	actualTree.ForEach(func(item T) {
		actualItems = append(actualItems, item)
	})

	docItems := make([]T, 0, doc.branch.snapshot.Size())
	doc.branch.snapshot.ForEach(func(item T) {
		docItems = append(docItems, item)
	})

	if !reflect.DeepEqual(actualItems, docItems) {
		panic("Document content out of sync")
	}
}

// Ins inserts the given items at the specified position.
func (doc *Document[T]) Ins(pos int, items []T) {
	localInsert(doc.opLog, doc.agent, pos, items)

	err := doc.branch.snapshot.InsertRange(pos, items)
	if err != nil {
		panic("Snapshot insert failed")
	}

	doc.branch.frontier = make([]lv, len(doc.opLog.frontier))
	copy(doc.branch.frontier, doc.opLog.frontier)
}

// Del deletes delLen items starting from the specified position.
func (doc *Document[T]) Del(pos int, delLen int) {
	localDelete(doc.opLog, doc.agent, pos, delLen)

	err := doc.branch.snapshot.DeleteRange(pos, delLen)
	if err != nil {
		panic("Snapshot delete failed")
	}

	doc.branch.frontier = make([]lv, len(doc.opLog.frontier))
	copy(doc.branch.frontier, doc.opLog.frontier)
}

// MergeFrom merges changes from another document.
func (doc *Document[T]) MergeFrom(other *Document[T]) {
	if doc == other {
		return
	}
	// Recursive merge for items with the same identity (LV)
	for _, o := range other.opLog.ops {
		if lastSeq, ok := doc.opLog.version[o.id.agent]; ok && lastSeq >= o.id.seq {
			// We have this op. Find our version and merge if mergeable.
			ourLV := idToLV(doc.opLog, o.id)
			if m, ok := any(doc.opLog.ops[ourLV].content).(Mergeable); ok {
				m.MergeFromAny(o.content)
			}
		}
	}
	mergeInto(doc.opLog, other.opLog)
	checkoutFancy(doc.opLog, doc.branch, doc.opLog.frontier)
}

// Reset clears the document state.
func (doc *Document[T]) Reset() {
	doc.opLog = newOpLog[T]()
	doc.branch = newBranch[T]()
}

// ==========================================
// RuneDocument
// ==========================================

// NewRuneDocument creates a new CRDT document for runes with the given agent ID.
func NewRuneDocument(agent int) *RuneDocument {
	return &RuneDocument{
		Document: NewDocument[rune](agent),
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
		Document: NewDocument[T](agent),
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
		opLog:    newOpLog[MapOp[K, V]](),
		keyIndex: make(map[K][]lv),
	}
}

// Set sets the value for the given key.
func (m *MapDocument[K, V]) Set(key K, value V) {
	curLV := lv(len(m.opLog.ops))
	m.opLog.pushLocalOp(m.agent, op[MapOp[K, V]]{
		content: MapOp[K, V]{Key: key, Value: value},
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
		o := m.opLog.ops[l]
		if first || o.id.agent > bestID.agent || (o.id.agent == bestID.agent && o.id.seq > bestID.seq) {
			bestV = o.content.Value
			bestID = o.id
			bestLV = l
			first = false
		}
	}

	// Recursive merge if V is Mergeable
	if mergeable, ok := any(bestV).(Mergeable); ok {
		for _, l := range concurrentLVs {
			if l != bestLV {
				mergeable.MergeFromAny(m.opLog.ops[l].content.Value)
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
	// Recursive merge for items with the same identity (LV)
	for _, o := range other.opLog.ops {
		if lastSeq, ok := m.opLog.version[o.id.agent]; ok && lastSeq >= o.id.seq {
			// We have this op. Find our version and merge if mergeable.
			ourLV := idToLV(m.opLog, o.id)
			if mrg, ok := any(m.opLog.ops[ourLV].content.Value).(Mergeable); ok {
				mrg.MergeFromAny(o.content.Value)
			}
		}
	}
	oldLen := len(m.opLog.ops)
	mergeInto(m.opLog, other.opLog)
	for i := oldLen; i < len(m.opLog.ops); i++ {
		o := m.opLog.ops[i]
		m.keyIndex[o.content.Key] = append(m.keyIndex[o.content.Key], lv(i))
	}
}

// Keys returns all keys that have been set in the map.
func (m *MapDocument[K, V]) Keys() []K {
	keysMap := make(map[K]bool)
	for _, o := range m.opLog.ops {
		keysMap[o.content.Key] = true
	}
	keys := make([]K, 0, len(keysMap))
	for k := range keysMap {
		keys = append(keys, k)
	}
	return keys
}
