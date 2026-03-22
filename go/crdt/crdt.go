package crdt

import (
	"container/heap"
	"egwalker/bxtree"
	"sort"
)

// ==========================================
// Priority Queue / Heap Helpers (Internal)
// ==========================================

type intMaxHeap []lv

func (h intMaxHeap) Len() int           { return len(h) }
func (h intMaxHeap) Less(i, j int) bool { return h[i] > h[j] } // Max Heap
func (h intMaxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *intMaxHeap) Push(x any)        { *h = append(*h, x.(lv)) }
func (h *intMaxHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// ==========================================
// Diff Algorithm (Internal)
// ==========================================

func diff[T any](log *opLog[T], a []lv, b []lv) diffResult {
	flags := make(map[lv]diffFlag)
	numShared := 0

	pq := &intMaxHeap{}
	heap.Init(pq)

	enq := func(v lv, flag diffFlag) {
		oldFlag, exists := flags[v]
		if !exists {
			heap.Push(pq, v)
			flags[v] = flag
			if flag == diffFlagShared {
				numShared++
			}
		} else if flag != oldFlag && oldFlag != diffFlagShared {
			flags[v] = diffFlagShared
			numShared++
		}
	}

	for _, aa := range a {
		enq(aa, diffFlagA)
	}
	for _, bb := range b {
		enq(bb, diffFlagB)
	}

	var aOnly, bOnly []lv

	for pq.Len() > numShared {
		curLV := heap.Pop(pq).(lv)
		flag := flags[curLV]

		switch flag {
		case diffFlagShared:
			numShared--
		case diffFlagA:
			aOnly = append(aOnly, curLV)
		case diffFlagB:
			bOnly = append(bOnly, curLV)
		}

		o := log.ops[curLV]
		for _, p := range o.parents {
			enq(p, flag)
		}
	}

	return diffResult{aOnly: aOnly, bOnly: bOnly}
}

// ==========================================
// CRDT Logic (Internal)
// ==========================================

func retreat[T any](doc *crdtDoc, log *opLog[T], opLV lv) {
	o := log.ops[opLV]
	var targetLV lv
	if o.opType == opTypeIns {
		targetLV = opLV
	} else {
		targetLV = doc.delTargets[opLV]
	}

	item := doc.itemsByLV[targetLV]
	item.curState--
}

func advance[T any](doc *crdtDoc, log *opLog[T], opLV lv) {
	o := log.ops[opLV]
	var targetLV lv
	if o.opType == opTypeIns {
		targetLV = opLV
	} else {
		targetLV = doc.delTargets[opLV]
	}

	item := doc.itemsByLV[targetLV]
	item.curState++
}

func findItemIdxAtLV(items []*crdtItem, targetLV lv) int {
	for i, item := range items {
		if item.lv == targetLV {
			return i
		}
	}
	panic("Could not find item")
}

func integrate[T any](doc *crdtDoc, log *opLog[T], newItem *crdtItem, idx int, endPos int, snapshot *bxtree.BxTree[T]) {
	scanIdx := idx
	scanEndPos := endPos

	left := scanIdx - 1
	right := len(doc.items)
	if newItem.originRight != -1 {
		right = findItemIdxAtLV(doc.items, newItem.originRight)
	}

	scanning := false

	for scanIdx < right {
		other := doc.items[scanIdx]

		if other.curState != stateNotYetInserted {
			break
		}

		oLeft := -1
		if other.originLeft != -1 {
			oLeft = findItemIdxAtLV(doc.items, other.originLeft)
		}

		oRight := len(doc.items)
		if other.originRight != -1 {
			oRight = findItemIdxAtLV(doc.items, other.originRight)
		}

		newItemAgent := log.ops[newItem.lv].id.agent
		otherAgent := log.ops[other.lv].id.agent

		if oLeft < left || (oLeft == left && oRight == right && newItemAgent < otherAgent) {
			break
		}

		if oLeft == left {
			scanning = oRight < right
		}

		if !other.deleted {
			scanEndPos++
		}
		scanIdx++

		if !scanning {
			idx = scanIdx
			endPos = scanEndPos
		}
	}

	doc.items = append(doc.items[:idx], append([]*crdtItem{newItem}, doc.items[idx:]...)...)

	o := log.ops[newItem.lv]
	if o.opType != opTypeIns {
		panic("Cannot insert a delete")
	}

	if snapshot != nil {
		err := snapshot.InsertAt(endPos, o.content)
		if err != nil {
			panic("Snapshot insert failed")
		}
	}
}

func findByCurrentPos(items []*crdtItem, targetPos int) (int, int) {
	curPos := 0
	endPos := 0
	idx := 0

	for ; curPos < targetPos; idx++ {
		if idx >= len(items) {
			panic("Past end of items list")
		}
		item := items[idx]
		if item.curState == stateInserted {
			curPos++
		}
		if !item.deleted {
			endPos++
		}
	}
	return idx, endPos
}

func apply[T any](doc *crdtDoc, log *opLog[T], snapshot *bxtree.BxTree[T], opLV lv) {
	o := log.ops[opLV]

	if o.opType == opTypeDel {
		idx, endPos := findByCurrentPos(doc.items, o.pos)

		for doc.items[idx].curState != stateInserted {
			if !doc.items[idx].deleted {
				endPos++
			}
			idx++
		}

		item := doc.items[idx]

		if !item.deleted {
			item.deleted = true
			if snapshot != nil {
				err := snapshot.DeleteAt(endPos)
				if err != nil {
					panic("Snapshot delete failed")
				}
			}
		}

		item.curState = 1 // Deleted(1)
		doc.delTargets[opLV] = item.lv

	} else {
		idx, endPos := findByCurrentPos(doc.items, o.pos)

		if idx >= 1 && doc.items[idx-1].curState != stateInserted {
			panic("Item to the left is not inserted!")
		}

		originLeft := lv(-1)
		if idx > 0 {
			originLeft = doc.items[idx-1].lv
		}

		originRight := lv(-1)
		for i := idx; i < len(doc.items); i++ {
			item2 := doc.items[i]
			if item2.curState != stateNotYetInserted {
				originRight = item2.lv
				break
			}
		}

		item := &crdtItem{
			lv:          opLV,
			originLeft:  originLeft,
			originRight: originRight,
			deleted:     false,
			curState:    stateInserted,
		}
		doc.itemsByLV[opLV] = item

		integrate(doc, log, item, idx, endPos, snapshot)
	}
}

func do1Operation[T any](doc *crdtDoc, log *opLog[T], opLV lv, snapshot *bxtree.BxTree[T]) {
	o := log.ops[opLV]
	diffRes := diff(log, doc.currentVersion, o.parents)

	for _, i := range diffRes.aOnly {
		retreat(doc, log, i)
	}
	for _, i := range diffRes.bOnly {
		advance(doc, log, i)
	}

	apply(doc, log, snapshot, opLV)
	doc.currentVersion = []lv{opLV}
}

func checkout[T any](log *opLog[T]) *bxtree.BxTree[T] {
	doc := &crdtDoc{
		items:          []*crdtItem{},
		currentVersion: []lv{},
		delTargets:     make(map[lv]lv),
		itemsByLV:      make(map[lv]*crdtItem),
	}

	snapshot := bxtree.New[T]()

	for i := 0; i < len(log.ops); i++ {
		do1Operation(doc, log, lv(i), snapshot)
	}
	return snapshot
}

// ==========================================
// Advanced Checkout (Internal)
// ==========================================

func compareArrays(a, b []lv) int {
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

type mergePointQueue []mergePoint

func (pq mergePointQueue) Len() int { return len(pq) }
func (pq mergePointQueue) Less(i, j int) bool {
	return compareArrays(pq[i].v, pq[j].v) > 0
}
func (pq mergePointQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }
func (pq *mergePointQueue) Push(x any)   { *pq = append(*pq, x.(mergePoint)) }
func (pq *mergePointQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

func findOpsToVisit[T any](log *opLog[T], a []lv, b []lv) opsToVisit {
	pq := &mergePointQueue{}
	heap.Init(pq)

	enq := func(lvs []lv, isInA bool) {
		v := make([]lv, len(lvs))
		copy(v, lvs)
		sort.Slice(v, func(i, j int) bool {
			return v[i] > v[j]
		})

		mp := mergePoint{
			v:     v,
			isInA: isInA,
		}
		heap.Push(pq, mp)
	}

	enq(a, true)
	enq(b, false)

	var commonVersion []lv
	var sharedOps, bOnlyOps []lv

	for {
		item := heap.Pop(pq).(mergePoint)
		v := item.v
		isInA := item.isInA

		if len(v) == 0 {
			commonVersion = []lv{}
			break
		}

		for pq.Len() > 0 {
			peekItem := (*pq)[0]
			if compareArrays(v, peekItem.v) != 0 {
				break
			}
			heap.Pop(pq)
			if peekItem.isInA {
				isInA = true
			}
		}

		if pq.Len() == 0 {
			commonVersion = make([]lv, len(v))
			for i, val := range v {
				commonVersion[len(v)-1-i] = val
			}
			break
		}

		if len(v) >= 2 {
			for _, vv := range v {
				enq([]lv{vv}, isInA)
			}
		} else {
			curLV := v[0]
			if isInA {
				sharedOps = append(sharedOps, curLV)
			} else {
				bOnlyOps = append(bOnlyOps, curLV)
			}

			o := log.ops[curLV]
			enq(o.parents, isInA)
		}
	}

	rev := func(s []lv) []lv {
		r := make([]lv, len(s))
		for i, v := range s {
			r[len(s)-1-i] = v
		}
		return r
	}

	return opsToVisit{
		commonVersion: commonVersion,
		sharedOps:     rev(sharedOps),
		bOnlyOps:      rev(bOnlyOps),
	}
}

func newBranch[T any]() *branch[T] {
	return &branch[T]{
		snapshot: bxtree.New[T](),
		frontier: []lv{},
	}
}

func checkoutFancy[T any](log *opLog[T], b *branch[T], mergeFrontier []lv) {
	if mergeFrontier == nil {
		mergeFrontier = log.frontier
	}

	visit := findOpsToVisit(log, b.frontier, mergeFrontier)

	doc := &crdtDoc{
		items:          []*crdtItem{},
		currentVersion: visit.commonVersion,
		delTargets:     make(map[lv]lv),
		itemsByLV:      make(map[lv]*crdtItem),
	}

	maxFrontier := -1
	for _, v := range b.frontier {
		if int(v) > maxFrontier {
			maxFrontier = int(v)
		}
	}
	placeholderLength := max(0, maxFrontier+1)

	for i := range placeholderLength {
		item := &crdtItem{
			lv:          lv(i) + 1e12,
			curState:    stateInserted,
			deleted:     false,
			originLeft:  -1,
			originRight: -1,
		}
		doc.items = append(doc.items, item)
		doc.itemsByLV[item.lv] = item
	}

	for _, curLV := range visit.sharedOps {
		do1Operation(doc, log, curLV, nil)
	}

	for _, curLV := range visit.bOnlyOps {
		do1Operation(doc, log, curLV, b.snapshot)
		o := log.ops[curLV]
		b.frontier = advanceFrontier(b.frontier, curLV, o.parents)
	}
}
