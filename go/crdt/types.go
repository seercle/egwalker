package crdt

import "egwalker/bxtree"

// newBxTree constructs a bxtree, panicking if construction fails. crdt only
// ever builds trees with the default node sizes (it passes no With*NodeSize
// options), which are always valid, so the error is unreachable.
func newBxTree[T any, S any](opts ...bxtree.Option[T, S]) *bxtree.BxTree[T, S] {
	tree, err := bxtree.New(opts...)
	if err != nil {
		panic("crdt: bxtree.New: " + err.Error())
	}
	return tree
}

type lv int

type id struct {
	agent int
	seq   int
}

type opType string

const (
	opTypeIns opType = "ins"
	opTypeDel opType = "del"
)

type op[E any, C content[E]] struct {
	opType  opType
	content C
	length  int // Number of characters/elements this op spans (set at push)
	pos     int // Original position for local ops
	id      id
	parents []lv
}

type remoteVersion map[int]int

type opLog[E any, C content[E]] struct {
	ops      []op[E, C]
	opLV     []lv // opLV[i] is op i's first LV
	totalLV  lv   // Running character count covered by ops
	frontier []lv
	version  remoteVersion
	idToLV   map[id]lv
}

type diffResult struct {
	aOnly []lv
	bOnly []lv
}

type diffFlag int

const (
	diffFlagA diffFlag = iota
	diffFlagB
	diffFlagShared
)

type crdtItem struct {
	lv          lv
	originLeft  lv // -1 if none
	originRight lv // -1 if none
	deleted     bool
	curState    int
	length      int // Number of characters this item represents
	node        *bxtree.Node[*crdtItem, crdtSummary]
}

type crdtSummary [2]int

type crdtDoc struct {
	items          *bxtree.BxTree[*crdtItem, crdtSummary]
	currentVersion []lv
	delTargets     map[lv]lv   // Map op_lv (delete op) -> target_lv
	sortedItems    []*crdtItem // Sorted list of items by start LV
}

type opsToVisit struct {
	commonVersion []lv
	sharedOps     []lv
	bOnlyOps      []lv
}

type mergePoint struct {
	v     []lv // Sorted in inverse order
	isInA bool
}

type branch[T any] struct {
	snapshot *bxtree.BxTree[T, struct{}]
	frontier []lv
}

const (
	stateNotYetInserted = -1
	stateInserted       = 0
)

// Document represents a generic CRDT document. E is the element type held in
// the document's snapshot; C is the run content type used to store inserts in
// the op log (an op's content is a length-1 run today, collapsed to whole
// runs by the RLE optimization).
type Document[E any, C content[E]] struct {
	opLog  *opLog[E, C]
	agent  int
	branch *branch[E]
}

// RuneDocument represents a CRDT text document.
type RuneDocument struct {
	Document[rune, runeText]
}

// ArrayDocument represents a generic CRDT array document.
type ArrayDocument[T any] struct {
	Document[T, itemRun[T]]
}

// MapOp represents a set operation on a MapDocument.
type MapOp[K comparable, V any] struct {
	Key      K
	Value    V
	IsDelete bool
}

// MapDocument represents a CRDT map document using LWW strategy.
type MapDocument[K comparable, V any] struct {
	agent    int
	opLog    *opLog[MapOp[K, V], mapRun[K, V]]
	keyIndex map[K][]lv
}

// Mergeable is an interface for documents that can be merged recursively.
type Mergeable interface {
	MergeFromAny(other any)
}

func (doc *Document[E, C]) MergeFromAny(other any) {
	if o, ok := other.(*Document[E, C]); ok {
		doc.MergeFrom(o)
	}
}

func (doc *RuneDocument) MergeFromAny(other any) {
	if o, ok := other.(*RuneDocument); ok {
		doc.MergeFrom(o)
	}
}

func (doc *ArrayDocument[T]) MergeFromAny(other any) {
	if o, ok := other.(*ArrayDocument[T]); ok {
		doc.MergeFrom(o)
	}
}

func (doc *MapDocument[K, V]) MergeFromAny(other any) {
	if o, ok := other.(*MapDocument[K, V]); ok {
		doc.MergeFrom(o)
	}
}
