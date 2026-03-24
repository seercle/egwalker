package crdt

import "egwalker/bxtree"

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

type op[T any] struct {
	opType  opType
	content T
	pos     int // Original position for local ops
	id      id
	parents []lv
}

type remoteVersion map[int]int

type opLog[T any] struct {
	ops      []op[T]
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
	node        *bxtree.Node[*crdtItem, crdtSummary]
}

type crdtSummary [2]int

type crdtDoc struct {
	items          *bxtree.BxTree[*crdtItem, crdtSummary]
	currentVersion []lv
	delTargets     map[lv]lv        // Map op_lv (delete op) -> target_lv
	itemsByLV      map[lv]*crdtItem // Map lv -> crdt_item
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

// Document represents a generic CRDT document.
type Document[T any] struct {
	opLog  *opLog[T]
	agent  int
	branch *branch[T]
}

// RuneDocument represents a CRDT text document.
type RuneDocument struct {
	Document[rune]
}

// ArrayDocument represents a generic CRDT array document.
type ArrayDocument[T any] struct {
	Document[T]
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
	opLog    *opLog[MapOp[K, V]]
	keyIndex map[K][]lv
}

// Mergeable is an interface for documents that can be merged recursively.
type Mergeable interface {
	MergeFromAny(other any)
}

func (doc *Document[T]) MergeFromAny(other any) {
	if o, ok := other.(*Document[T]); ok {
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
