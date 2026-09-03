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

type op[C content[C]] struct {
	opType  opType
	content C
	length  int // Number of characters/elements this op spans (set at push)
	pos     int // Original position for local ops
	id      id
	parents []lv
}

type remoteVersion map[int]int

type opLog[C content[C]] struct {
	ops      []op[C]
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

type crdtSummary struct {
	presentLen int // Characters whose curState == stateInserted: the positional ordering key.
	liveLen    int // Characters in items that are not deleted.
}

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

type branch[C content[C]] struct {
	snapshot *contentTree[C]
	frontier []lv
}

const (
	stateNotYetInserted = -1
	stateInserted       = 0
)

// MapOp represents a set operation on a MapDocument.
type MapOp[K comparable, V any] struct {
	Key      K
	Value    V
	IsDelete bool
}

// Mergeable is an interface for documents that can be merged recursively.
type Mergeable interface {
	MergeFromAny(other any)
}
