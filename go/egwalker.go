package main

import (
	"container/heap"
	"fmt"
	"sort"
)

// ==========================================
// Types
// ==========================================

type LV int

type Id struct {
	Agent int
	Seq   int
}

type OpType string

const (
	OpTypeIns OpType = "ins"
	OpTypeDel OpType = "del"
)

type Op[T any] struct {
	Type    OpType
	Content T
	Pos     int // Original position for local ops
	Id      Id
	Parents []LV
}

type RemoteVersion map[int]int

type OpLog[T any] struct {
	Ops      []Op[T]
	Frontier []LV
	Version  RemoteVersion
}

// ==========================================
// OpLog Functions
// ==========================================

func NewOpLog[T any]() *OpLog[T] {
	return &OpLog[T]{
		Ops:      []Op[T]{},
		Frontier: []LV{},
		Version:  make(RemoteVersion),
	}
}

func (log *OpLog[T]) PushLocalOp(agent int, op Op[T]) {
	lastSeq, ok := log.Version[agent]
	if !ok {
		lastSeq = -1
	}
	seq := lastSeq + 1

	lv := LV(len(log.Ops))
	op.Id = Id{Agent: agent, Seq: seq}
	op.Parents = log.Frontier // Copy frontier? Slices are refs, but frontier is replaced below.
	// We should probably copy the slice to be safe, though the logic replaces log.Frontier immediately.
	parentsCopy := make([]LV, len(log.Frontier))
	copy(parentsCopy, log.Frontier)
	op.Parents = parentsCopy

	log.Ops = append(log.Ops, op)
	log.Frontier = []LV{lv}
	log.Version[agent] = seq
}

func LocalInsert[T any](log *OpLog[T], agent int, pos int, content []T) {
	currentPos := pos
	for _, c := range content {
		LocalInsertOne(log, agent, currentPos, c)
		currentPos++
	}
}

func LocalInsertOne[T any](log *OpLog[T], agent int, pos int, content T) {
	log.PushLocalOp(agent, Op[T]{
		Type:    OpTypeIns,
		Content: content,
		Pos:     pos,
	})
}

func LocalDelete[T any](log *OpLog[T], agent int, pos int, delLen int) {
	for delLen > 0 {
		log.PushLocalOp(agent, Op[T]{
			Type: OpTypeDel,
			Pos:  pos,
		})
		delLen--
	}
}

func IdEq(a, b Id) bool {
	return a.Agent == b.Agent && a.Seq == b.Seq
}

func IdToLV[T any](log *OpLog[T], id Id) LV {
	for i, op := range log.Ops {
		if IdEq(op.Id, id) {
			return LV(i)
		}
	}
	panic("Could not find id in oplog")
}

func SortLVs(frontier []LV) []LV {
	sort.Slice(frontier, func(i, j int) bool {
		return frontier[i] < frontier[j]
	})
	return frontier
}

func AdvanceFrontier(frontier []LV, lv LV, parents []LV) []LV {
	// f = frontier.filter(v => !parents.includes(v))
	f := []LV{}
	parentMap := make(map[LV]bool)
	for _, p := range parents {
		parentMap[p] = true
	}

	for _, v := range frontier {
		if !parentMap[v] {
			f = append(f, v)
		}
	}
	f = append(f, lv)
	return SortLVs(f)
}

func PushRemoteOp[T any](log *OpLog[T], op Op[T], parentIds []Id) {
	agent := op.Id.Agent
	seq := op.Id.Seq

	lastKnownSeq, ok := log.Version[agent]
	if !ok {
		lastKnownSeq = -1
	}

	if lastKnownSeq >= seq {
		return // Already have the op
	}

	lv := LV(len(log.Ops))

	// Resolve parents
	parents := make([]LV, len(parentIds))
	for i, pid := range parentIds {
		parents[i] = IdToLV(log, pid)
	}
	op.Parents = SortLVs(parents)

	log.Ops = append(log.Ops, op)
	log.Frontier = AdvanceFrontier(log.Frontier, lv, op.Parents)

	if seq != lastKnownSeq+1 {
		panic("Seq numbers out of order")
	}
	log.Version[agent] = seq
}

func MergeInto[T any](dest *OpLog[T], src *OpLog[T]) {
	for _, op := range src.Ops {
		parentIds := make([]Id, len(op.Parents))
		for i, pLV := range op.Parents {
			parentIds[i] = src.Ops[pLV].Id
		}
		// Create a copy of op to avoid mutating source if we were to modify it (we don't, but safe practice)
		newOp := op
		PushRemoteOp(dest, newOp, parentIds)
	}
}

// ==========================================
// Priority Queue / Heap Helpers
// ==========================================

// IntMaxHeap for Diff (LVs)
type IntMaxHeap []LV

func (h IntMaxHeap) Len() int           { return len(h) }
func (h IntMaxHeap) Less(i, j int) bool { return h[i] > h[j] } // Max Heap
func (h IntMaxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntMaxHeap) Push(x any)        { *h = append(*h, x.(LV)) }
func (h *IntMaxHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// ==========================================
// Diff Algorithm
// ==========================================

type DiffResult struct {
	AOnly []LV
	BOnly []LV
}

type DiffFlag int

const (
	DiffFlagA DiffFlag = iota
	DiffFlagB
	DiffFlagShared
)

func Diff[T any](log *OpLog[T], a []LV, b []LV) DiffResult {
	flags := make(map[LV]DiffFlag)
	numShared := 0

	pq := &IntMaxHeap{}
	heap.Init(pq)

	enq := func(v LV, flag DiffFlag) {
		oldFlag, exists := flags[v]
		if !exists {
			heap.Push(pq, v)
			flags[v] = flag
			if flag == DiffFlagShared {
				numShared++
			}
		} else if flag != oldFlag && oldFlag != DiffFlagShared {
			flags[v] = DiffFlagShared
			numShared++
		}
	}

	for _, aa := range a {
		enq(aa, DiffFlagA)
	}
	for _, bb := range b {
		enq(bb, DiffFlagB)
	}

	var aOnly, bOnly []LV

	for pq.Len() > numShared {
		lv := heap.Pop(pq).(LV)
		flag := flags[lv]

		switch flag {
		case DiffFlagShared:
			numShared--
		case DiffFlagA:
			aOnly = append(aOnly, lv)
		case DiffFlagB:
			bOnly = append(bOnly, lv)
		}

		op := log.Ops[lv]
		for _, p := range op.Parents {
			enq(p, flag)
		}
	}

	return DiffResult{AOnly: aOnly, BOnly: bOnly}
}

// ==========================================
// CRDT Logic
// ==========================================

const (
	StateNotYetInserted = -1
	StateInserted       = 0
	// StateDeleted >= 1
)

type CRDTItem struct {
	LV          LV
	OriginLeft  LV // -1 if none
	OriginRight LV // -1 if none
	Deleted     bool
	CurState    int
}

type CRDTDoc struct {
	Items          []*CRDTItem
	CurrentVersion []LV
	DelTargets     map[LV]LV        // Map opLV (delete op) -> targetLV
	ItemsByLV      map[LV]*CRDTItem // Map LV -> CRDTItem
}

func Retreat[T any](doc *CRDTDoc, log *OpLog[T], opLv LV) {
	op := log.Ops[opLv]
	var targetLV LV
	if op.Type == OpTypeIns {
		targetLV = opLv
	} else {
		targetLV = doc.DelTargets[opLv]
	}

	item := doc.ItemsByLV[targetLV]
	item.CurState--
}

func Advance[T any](doc *CRDTDoc, log *OpLog[T], opLv LV) {
	op := log.Ops[opLv]
	var targetLV LV
	if op.Type == OpTypeIns {
		targetLV = opLv
	} else {
		targetLV = doc.DelTargets[opLv]
	}

	item := doc.ItemsByLV[targetLV]
	item.CurState++
}

func FindItemIdxAtLV(items []*CRDTItem, lv LV) int {
	for i, item := range items {
		if item.LV == lv {
			return i
		}
	}
	panic("Could not find item")
}

func Integrate[T any](doc *CRDTDoc, log *OpLog[T], newItem *CRDTItem, idx int, endPos int, snapshot *[]T) {
	scanIdx := idx
	scanEndPos := endPos

	left := scanIdx - 1
	right := len(doc.Items)
	if newItem.OriginRight != -1 {
		right = FindItemIdxAtLV(doc.Items, newItem.OriginRight)
	}

	scanning := false

	for scanIdx < right {
		other := doc.Items[scanIdx]

		if other.CurState != StateNotYetInserted {
			break
		}

		oleft := -1
		if other.OriginLeft != -1 {
			oleft = FindItemIdxAtLV(doc.Items, other.OriginLeft)
		}

		oright := len(doc.Items)
		if other.OriginRight != -1 {
			oright = FindItemIdxAtLV(doc.Items, other.OriginRight)
		}

		newItemAgent := log.Ops[newItem.LV].Id.Agent
		otherAgent := log.Ops[other.LV].Id.Agent

		// Concurrent insert ordering logic
		if oleft < left || (oleft == left && oright == right && newItemAgent < otherAgent) {
			break
		}

		if oleft == left {
			scanning = oright < right
		}

		if !other.Deleted {
			scanEndPos++
		}
		scanIdx++

		if !scanning {
			idx = scanIdx
			endPos = scanEndPos
		}
	}

	// Insert into document list
	// Go slice insertion: append(items[:idx], append([]*Item{newItem}, items[idx:]...)...)
	doc.Items = append(doc.Items[:idx], append([]*CRDTItem{newItem}, doc.Items[idx:]...)...)

	op := log.Ops[newItem.LV]
	if op.Type != OpTypeIns {
		panic("Cannot insert a delete")
	}

	if snapshot != nil {
		// snapshot splice
		*snapshot = append((*snapshot)[:endPos], append([]T{op.Content}, (*snapshot)[endPos:]...)...)
	}
}

func FindByCurrentPos(items []*CRDTItem, targetPos int) (int, int) {
	curPos := 0
	endPos := 0
	idx := 0

	for ; curPos < targetPos; idx++ {
		if idx >= len(items) {
			panic("Past end of items list")
		}
		item := items[idx]
		if item.CurState == StateInserted {
			curPos++
		}
		if !item.Deleted {
			endPos++
		}
	}
	return idx, endPos
}

func Apply[T any](doc *CRDTDoc, log *OpLog[T], snapshot *[]T, opLv LV) {
	op := log.Ops[opLv]

	if op.Type == OpTypeDel {
		// Delete
		idx, endPos := FindByCurrentPos(doc.Items, op.Pos)

		// Scan forward to find actual item
		for doc.Items[idx].CurState != StateInserted {
			if !doc.Items[idx].Deleted {
				endPos++
			}
			idx++
		}

		item := doc.Items[idx]

		if !item.Deleted {
			item.Deleted = true
			if snapshot != nil {
				// snapshot splice remove 1
				*snapshot = append((*snapshot)[:endPos], (*snapshot)[endPos+1:]...)
			}
		}

		item.CurState = 1 // Deleted(1)
		doc.DelTargets[opLv] = item.LV

	} else {
		// Insert
		idx, endPos := FindByCurrentPos(doc.Items, op.Pos)

		if idx >= 1 && doc.Items[idx-1].CurState != StateInserted {
			panic("Item to the left is not inserted!")
		}

		originLeft := LV(-1)
		if idx > 0 {
			originLeft = doc.Items[idx-1].LV
		}

		originRight := LV(-1)
		for i := idx; i < len(doc.Items); i++ {
			item2 := doc.Items[i]
			if item2.CurState != StateNotYetInserted {
				originRight = item2.LV
				break
			}
		}

		item := &CRDTItem{
			LV:          opLv,
			OriginLeft:  originLeft,
			OriginRight: originRight,
			Deleted:     false,
			CurState:    StateInserted,
		}
		doc.ItemsByLV[opLv] = item

		Integrate(doc, log, item, idx, endPos, snapshot)
	}
}

func Do1Operation[T any](doc *CRDTDoc, log *OpLog[T], lv LV, snapshot *[]T) {
	op := log.Ops[lv]
	diffRes := Diff(log, doc.CurrentVersion, op.Parents)

	for _, i := range diffRes.AOnly {
		Retreat(doc, log, i)
	}
	for _, i := range diffRes.BOnly {
		Advance(doc, log, i)
	}

	Apply(doc, log, snapshot, lv)
	doc.CurrentVersion = []LV{lv}
}

func Checkout[T any](log *OpLog[T]) []T {
	doc := &CRDTDoc{
		Items:          []*CRDTItem{},
		CurrentVersion: []LV{},
		DelTargets:     make(map[LV]LV),
		ItemsByLV:      make(map[LV]*CRDTItem),
	}

	snapshot := []T{}

	for lv := 0; lv < len(log.Ops); lv++ {
		Do1Operation(doc, log, LV(lv), &snapshot)
	}
	return snapshot
}

// ==========================================
// Advanced Checkout (Fancy)
// ==========================================

// CompareArrays compares two sorted (descending) slices of LV.
// Returns >0 if a > b, <0 if a < b, 0 if equal.
func CompareArrays(a, b []LV) int {
	for i := range len(a) {
		if len(b) <= i {
			return 1
		}
		delta := int(a[i] - b[i])
		if delta != 0 {
			return delta
		}
	}
	if len(a) < len(b) {
		return -1
	}
	return 0
}

type OpsToVisit struct {
	CommonVersion []LV
	SharedOps     []LV
	BOnlyOps      []LV
}

type MergePoint struct {
	V     []LV // Sorted in inverse order
	IsInA bool
}

// MergePointQueue is a priority queue for MergePoints.
// We want to dequeue the "largest" array (newest version).
type MergePointQueue []MergePoint

func (pq MergePointQueue) Len() int { return len(pq) }
func (pq MergePointQueue) Less(i, j int) bool {
	// We want Max Heap behavior based on CompareArrays.
	// standard heap is MinHeap (Less returns true for smaller).
	// So we return true if pq[i] > pq[j].
	return CompareArrays(pq[i].V, pq[j].V) > 0
}
func (pq MergePointQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }
func (pq *MergePointQueue) Push(x any)   { *pq = append(*pq, x.(MergePoint)) }
func (pq *MergePointQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

func FindOpsToVisit[T any](log *OpLog[T], a []LV, b []LV) OpsToVisit {
	pq := &MergePointQueue{}
	heap.Init(pq)

	enq := func(lv []LV, isInA bool) {
		// Sort copy in descending order
		v := make([]LV, len(lv))
		copy(v, lv)
		sort.Slice(v, func(i, j int) bool {
			return v[i] > v[j]
		})

		mp := MergePoint{
			V:     v,
			IsInA: isInA,
		}
		heap.Push(pq, mp)
	}

	enq(a, true)
	enq(b, false)

	var commonVersion []LV
	var sharedOps, bOnlyOps []LV

	for {
		item := heap.Pop(pq).(MergePoint)
		v := item.V
		isInA := item.IsInA

		if len(v) == 0 {
			commonVersion = []LV{}
			break
		}

		for pq.Len() > 0 {
			peekItem := (*pq)[0]
			if CompareArrays(v, peekItem.V) != 0 {
				break
			}
			heap.Pop(pq)
			if peekItem.IsInA {
				isInA = true
			}
		}

		if pq.Len() == 0 {
			// Reverse v for commonVersion
			commonVersion = make([]LV, len(v))
			for i, val := range v {
				commonVersion[len(v)-1-i] = val
			}
			break
		}

		if len(v) >= 2 {
			for _, vv := range v {
				enq([]LV{vv}, isInA)
			}
		} else {
			lv := v[0]
			if isInA {
				sharedOps = append(sharedOps, lv)
			} else {
				bOnlyOps = append(bOnlyOps, lv)
			}

			op := log.Ops[lv]
			enq(op.Parents, isInA)
		}
	}

	// Reverse results to get chronological order
	rev := func(s []LV) []LV {
		r := make([]LV, len(s))
		for i, v := range s {
			r[len(s)-1-i] = v
		}
		return r
	}

	return OpsToVisit{
		CommonVersion: commonVersion,
		SharedOps:     rev(sharedOps),
		BOnlyOps:      rev(bOnlyOps),
	}
}

type Branch[T any] struct {
	Snapshot []T
	Frontier []LV
}

func NewBranch[T any]() *Branch[T] {
	return &Branch[T]{
		Snapshot: []T{},
		Frontier: []LV{},
	}
}

func CheckoutFancy[T any](log *OpLog[T], branch *Branch[T], mergeFrontier []LV) {
	if mergeFrontier == nil {
		mergeFrontier = log.Frontier
	}

	visit := FindOpsToVisit(log, branch.Frontier, mergeFrontier)

	doc := &CRDTDoc{
		Items:          []*CRDTItem{},
		CurrentVersion: visit.CommonVersion,
		DelTargets:     make(map[LV]LV),
		ItemsByLV:      make(map[LV]*CRDTItem),
	}

	// Create placeholders
	maxFrontier := -1
	for _, v := range branch.Frontier {
		if int(v) > maxFrontier {
			maxFrontier = int(v)
		}
	}
	placeholderLength := max(0, maxFrontier+1)

	for i := range placeholderLength {
		item := &CRDTItem{
			LV:          LV(i) + 1e12, // Hack from original TS
			CurState:    StateInserted,
			Deleted:     false,
			OriginLeft:  -1,
			OriginRight: -1,
		}
		doc.Items = append(doc.Items, item)
		doc.ItemsByLV[item.LV] = item
	}

	// Process shared ops (modify doc state only, ignore snapshot)
	for _, lv := range visit.SharedOps {
		Do1Operation(doc, log, lv, nil)
	}

	// Process B-only ops (modify doc state and branch snapshot)
	for _, lv := range visit.BOnlyOps {
		Do1Operation(doc, log, lv, &branch.Snapshot)
		op := log.Ops[lv]
		branch.Frontier = AdvanceFrontier(branch.Frontier, lv, op.Parents)
	}
}

// ==========================================
// Main Wrapper Class
// ==========================================

type CRDTDocument struct {
	OpLog  *OpLog[string]
	Agent  int
	Branch *Branch[string]
}

func NewCRDTDocument(agent int) *CRDTDocument {
	return &CRDTDocument{
		OpLog:  NewOpLog[string](),
		Agent:  agent,
		Branch: NewBranch[string](),
	}
}

func (doc *CRDTDocument) Check() {
	actualDoc := Checkout(doc.OpLog)
	s1 := fmt.Sprint(actualDoc)
	s2 := fmt.Sprint(doc.Branch.Snapshot)
	if s1 != s2 {
		panic("Document out of sync: " + s1 + " vs " + s2)
	}
}

func (doc *CRDTDocument) Ins(pos int, text string) {
	chars := []string{}
	for _, r := range text {
		chars = append(chars, string(r))
	}

	LocalInsert(doc.OpLog, doc.Agent, pos, chars)

	// Splice snapshot
	// snapshot.splice(pos, 0, ...inserted)
	doc.Branch.Snapshot = append(doc.Branch.Snapshot, chars...)
	copy(doc.Branch.Snapshot[pos+len(chars):], doc.Branch.Snapshot[pos:len(doc.Branch.Snapshot)-len(chars)])
	copy(doc.Branch.Snapshot[pos:], chars)

	// Copy frontier
	doc.Branch.Frontier = make([]LV, len(doc.OpLog.Frontier))
	copy(doc.Branch.Frontier, doc.OpLog.Frontier)
}

func (doc *CRDTDocument) Del(pos int, delLen int) {
	LocalDelete(doc.OpLog, doc.Agent, pos, delLen)

	// Splice snapshot remove
	// snapshot.splice(pos, delLen)
	doc.Branch.Snapshot = append(doc.Branch.Snapshot[:pos], doc.Branch.Snapshot[pos+delLen:]...)

	doc.Branch.Frontier = make([]LV, len(doc.OpLog.Frontier))
	copy(doc.Branch.Frontier, doc.OpLog.Frontier)
}

func (doc *CRDTDocument) GetString() string {
	res := ""
	for _, s := range doc.Branch.Snapshot {
		res += s
	}
	return res
}

func (doc *CRDTDocument) MergeFrom(other *CRDTDocument) {
	MergeInto(doc.OpLog, other.OpLog)
	CheckoutFancy(doc.OpLog, doc.Branch, doc.OpLog.Frontier)
}

func (doc *CRDTDocument) Reset() {
	doc.OpLog = NewOpLog[string]()
	doc.Branch = NewBranch[string]()
}
