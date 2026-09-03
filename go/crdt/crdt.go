package crdt

import (
	"egwalker/bxtree"
	"egwalker/pheap"
	"sort"
)

// ==========================================
// Diff Algorithm (Internal)
// ==========================================

func diff[E any, C content[E]](log *opLog[E, C], a []lv, b []lv) diffResult {
	flags := make(map[lv]diffFlag)
	numShared := 0

	// Max-heap of lv
	pq := pheap.New[lv]()

	enq := func(v lv, flag diffFlag) {
		oldFlag, exists := flags[v]
		if !exists {
			pq.Push(v)
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

	for pq.Size() > numShared {
		curLV, ok := pq.Pop()
		if !ok {
			break
		}
		flag := flags[curLV]

		switch flag {
		case diffFlagShared:
			numShared--
		case diffFlagA:
			aOnly = append(aOnly, curLV)
		case diffFlagB:
			bOnly = append(bOnly, curLV)
		}

		o := log.opAt(curLV)
		for _, p := range o.parents {
			enq(p, flag)
		}
	}

	return diffResult{aOnly: aOnly, bOnly: bOnly}
}

// ==========================================
// CRDT Logic (Internal)
// ==========================================

type crdtSummarizer struct{}

func (s crdtSummarizer) FromItem(item *crdtItem) crdtSummary {
	m := crdtSummary{0, 0}
	if item.curState == stateInserted {
		m[0] = item.length
	}
	if !item.deleted {
		m[1] = item.length
	}
	return m
}

func (s crdtSummarizer) Add(a, b crdtSummary) crdtSummary {
	return crdtSummary{a[0] + b[0], a[1] + b[1]}
}

func (s crdtSummarizer) Sub(a, b crdtSummary) crdtSummary {
	return crdtSummary{a[0] - b[0], a[1] - b[1]}
}

var crdtSummaryConfig = crdtSummarizer{}

func addItemLV(doc *crdtDoc, item *crdtItem) {
	idx := sort.Search(len(doc.sortedItems), func(i int) bool { return doc.sortedItems[i].lv >= item.lv })
	if idx < len(doc.sortedItems) && doc.sortedItems[idx].lv == item.lv {
		doc.sortedItems[idx] = item // Update existing entry
		return
	}
	// Insert at idx
	doc.sortedItems = append(doc.sortedItems, nil)
	copy(doc.sortedItems[idx+1:], doc.sortedItems[idx:])
	doc.sortedItems[idx] = item
}

func removeItemLV(doc *crdtDoc, targetLV lv) {
	idx := sort.Search(len(doc.sortedItems), func(i int) bool { return doc.sortedItems[i].lv >= targetLV })
	if idx < len(doc.sortedItems) && doc.sortedItems[idx].lv == targetLV {
		doc.sortedItems = append(doc.sortedItems[:idx], doc.sortedItems[idx+1:]...)
	}
}

func findItemAtLV(doc *crdtDoc, targetLV lv) *crdtItem {
	if len(doc.sortedItems) == 0 {
		return nil
	}

	idx := sort.Search(len(doc.sortedItems), func(i int) bool { return doc.sortedItems[i].lv > targetLV })
	if idx == 0 {
		return nil
	}

	item := doc.sortedItems[idx-1]
	if targetLV < item.lv+lv(item.length) {
		return item
	}
	return nil
}

func ensureAtomized(doc *crdtDoc, targetLV lv) *crdtItem {
	item := findItemAtLV(doc, targetLV)
	if item == nil {
		return nil
	}
	if item.length == 1 {
		return item
	}

	idx := findItemIdxAtLVInternal(doc, item.lv)
	offset := int(targetLV - item.lv)
	if offset > 0 {
		split(doc, idx, offset)
		idx++
	}
	item = findItemAtLV(doc, targetLV)
	if item.length > 1 {
		split(doc, idx, 1)
	}
	return findItemAtLV(doc, targetLV)
}

// toggleRunChar toggles the staged insertion state (delta = -1 retreat, +1
// advance) of the single character at opLV. For an insert op the character
// itself is the target; for a delete op the target is the character the delete
// removed (looked up in delTargets). Items are atomized so each character is
// toggled independently.
func toggleRunChar[E any, C content[E]](doc *crdtDoc, log *opLog[E, C], opLV lv, delta int) {
	o := log.opAt(opLV)
	var targetLV lv
	if o.opType == opTypeIns {
		targetLV = opLV
	} else {
		var ok bool
		targetLV, ok = doc.delTargets[opLV]
		if !ok {
			return
		}
	}

	item := ensureAtomized(doc, targetLV)
	if item == nil {
		return
	}

	oldM0 := 0
	if item.curState == stateInserted {
		oldM0 = 1
	}
	item.curState += delta
	newM0 := 0
	if item.curState == stateInserted {
		newM0 = 1
	}
	if oldM0 != newM0 {
		item.node.SummaryAddUpward(crdtSummary{newM0 - oldM0, 0}, doc.items)
	}
}

// retreat un-applies (winds back) the run op containing opLV. A run op is
// atomic: every character it covers is toggled, one at a time, so per-character
// item/split state is preserved.
func retreat[E any, C content[E]](doc *crdtDoc, log *opLog[E, C], opLV lv) {
	i := log.opIdxAt(opLV)
	for k := 0; k < log.ops[i].length; k++ {
		toggleRunChar(doc, log, log.opLV[i]+lv(k), -1)
	}
}

// advance re-applies (winds forward) the run op containing opLV, character by
// character.
func advance[E any, C content[E]](doc *crdtDoc, log *opLog[E, C], opLV lv) {
	i := log.opIdxAt(opLV)
	for k := 0; k < log.ops[i].length; k++ {
		toggleRunChar(doc, log, log.opLV[i]+lv(k), +1)
	}
}

func findItemIdxAtLVInternal(doc *crdtDoc, targetLV lv) int {
	item := findItemAtLV(doc, targetLV)
	if item == nil {
		panic("Could not find item")
	}
	if item.node == nil {
		panic("Item node is nil")
	}

	nodeIdx := item.node.Index()
	posInNode := -1
	for i, it := range item.node.Items() {
		if it == item {
			posInNode = i
			break
		}
	}
	if posInNode == -1 {
		panic("Could not find item in node items")
	}
	return nodeIdx + posInNode
}

func canMerge[E any, C content[E]](log *opLog[E, C], left *crdtItem, right *crdtItem) bool {
	if left == nil || right == nil {
		return false
	}
	if left.deleted != right.deleted || left.curState != right.curState {
		return false
	}

	// Items whose lv is not a real op coordinate (e.g. the checkoutFancy
	// placeholder sentinel) never merge.
	if !log.covers(left.lv) || !log.covers(right.lv) {
		return false
	}

	opL := log.opAt(left.lv)
	opR := log.opAt(right.lv)

	if opL.id.agent != opR.id.agent {
		return false
	}

	// Contiguous in seq and LV. Items may start at a run-interior lv (after a
	// split), so the op sequence at the item start lv is derived via seqAt.
	if log.seqAt(left.lv)+left.length != log.seqAt(right.lv) {
		return false
	}
	if left.lv+lv(left.length) != right.lv {
		return false
	}

	// Origin check
	if right.originLeft != left.lv+lv(left.length-1) {
		return false
	}
	if left.originRight != right.originRight {
		return false
	}

	return true
}

func mergeLeft(doc *crdtDoc, idx int) {
	leftPtr, _ := doc.items.GetAt(idx - 1)
	rightPtr, _ := doc.items.GetAt(idx)
	left := *leftPtr
	right := *rightPtr

	// Update left item
	left.length += right.length
	left.originRight = right.originRight

	// Update sortedItems index
	removeItemLV(doc, right.lv)
	addItemLV(doc, left)

	// Delete right item
	doc.items.DeleteAt(idx)

	// Update summary
	left.node.UpdateSummaryUpward(doc.items)
}

func tryMergeAt[E any, C content[E]](doc *crdtDoc, log *opLog[E, C], idx int) {
	if idx > 0 {
		leftPtr, _ := doc.items.GetAt(idx - 1)
		rightPtr, _ := doc.items.GetAt(idx)
		if canMerge(log, *leftPtr, *rightPtr) {
			mergeLeft(doc, idx)
			idx--
		}
	}
	if idx < doc.items.Size()-1 {
		leftPtr, _ := doc.items.GetAt(idx)
		rightPtr, _ := doc.items.GetAt(idx + 1)
		if canMerge(log, *leftPtr, *rightPtr) {
			mergeLeft(doc, idx+1)
		}
	}
}

func getLogicalPos(doc *crdtDoc, targetLV lv) (int, int) {
	if targetLV == -1 {
		return -1, 0
	}
	item := findItemAtLV(doc, targetLV)
	if item == nil {
		panic("Could not find item")
	}
	return findItemIdxAtLVInternal(doc, item.lv), int(targetLV - item.lv)
}

func integrate[E any, C content[E]](doc *crdtDoc, log *opLog[E, C], newItem *crdtItem, idx int, endPos int, snapshot *bxtree.BxTree[E, struct{}]) int {
	scanIdx := idx
	scanEndPos := endPos

	leftIdx, leftOffset := getLogicalPos(doc, newItem.originLeft)
	rightIdx, rightOffset := doc.items.Size(), 0
	if newItem.originRight != -1 {
		rightIdx, rightOffset = getLogicalPos(doc, newItem.originRight)
	}

	scanning := false

	if scanIdx < doc.items.Size() {
		node, pos, err := doc.items.GetAtNode(scanIdx)
		if err != nil {
			panic("GetAtNode failed")
		}

		for scanIdx < doc.items.Size() {
			other := node.Items()[pos]

			if other.curState != stateNotYetInserted {
				break
			}

			oLeftIdx, oLeftOffset := getLogicalPos(doc, other.originLeft)
			oRightIdx, oRightOffset := doc.items.Size(), 0
			if other.originRight != -1 {
				oRightIdx, oRightOffset = getLogicalPos(doc, other.originRight)
			}

			newItemAgent := log.opAt(newItem.lv).id.agent
			otherAgent := log.opAt(other.lv).id.agent

			if oLeftIdx < leftIdx || (oLeftIdx == leftIdx && oLeftOffset < leftOffset) ||
				(oLeftIdx == leftIdx && oLeftOffset == leftOffset && oRightIdx == rightIdx && oRightOffset == rightOffset && newItemAgent < otherAgent) {
				break
			}

			if oLeftIdx == leftIdx && oLeftOffset == leftOffset {
				scanning = oRightIdx < rightIdx || (oRightIdx == rightIdx && oRightOffset < rightOffset)
			}

			if !other.deleted {
				scanEndPos += other.length
			}
			scanIdx++
			pos++
			if pos >= len(node.Items()) {
				node = node.Next()
				pos = 0
			}

			if !scanning {
				idx = scanIdx
				endPos = scanEndPos
			}
		}
	}

	doc.items.InsertAt(idx, newItem)

	o := log.opAt(newItem.lv)
	if o.opType != opTypeIns {
		panic("Cannot insert a delete")
	}

	if snapshot != nil {
		err := snapshot.InsertRange(endPos, o.content.Elems())
		if err != nil {
			panic("Snapshot insert failed")
		}
	}
	return idx
}

func findByCurrentPos(doc *crdtDoc, targetPos int) (int, int) {
	if targetPos == 0 {
		return 0, 0
	}

	node, posInNode, acc := doc.items.FindPath(func(acc crdtSummary, cur crdtSummary) bool {
		return acc[0]+cur[0] >= targetPos
	})

	if node == nil {
		if doc.items.Root() == nil {
			return 0, 0
		}
		return doc.items.Size(), doc.items.Root().Summary()[1]
	}

	item := node.Items()[posInNode]
	m := crdtSummaryConfig.FromItem(item)

	if targetPos > acc[0] && targetPos < acc[0]+m[0] {
		idx := node.Index() + posInNode
		offset := targetPos - acc[0]
		split(doc, idx, offset)
		return findByCurrentPos(doc, targetPos)
	}

	return node.Index() + posInNode + 1, acc[1] + m[1]
}

func split(doc *crdtDoc, idx int, offset int) {
	if offset <= 0 {
		return
	}

	itemPtr, err := doc.items.GetAt(idx)
	if err != nil || itemPtr == nil {
		return
	}
	item := *itemPtr

	if offset >= item.length {
		return
	}

	// Create second part
	second := &crdtItem{
		lv:          item.lv + lv(offset),
		originLeft:  item.lv + lv(offset-1),
		originRight: item.originRight,
		deleted:     item.deleted,
		curState:    item.curState,
		length:      item.length - offset,
	}

	// Update first part
	item.length = offset
	item.node.UpdateSummaryUpward(doc.items)

	// Insert second part
	doc.items.InsertAt(idx+1, second)

	// Update the mapping for the second part
	addItemLV(doc, second)
}

// deleteOne deletes the single visible character at doc position pos of the
// staged state, recording the deletion target for the delete-run character lv
// opLV so retreat/advance can toggle it back independently.
func deleteOne[E any, C content[E]](doc *crdtDoc, log *opLog[E, C], snapshot *bxtree.BxTree[E, struct{}], opLV lv, pos int) {
	idx, endPos := findByCurrentPos(doc, pos)

	node, nodePos, err := doc.items.GetAtNode(idx)
	if err == nil {
		for {
			item := node.Items()[nodePos]
			if item.curState == stateInserted {
				break
			}
			if !item.deleted {
				endPos++
			}
			idx++
			nodePos++
			if nodePos >= len(node.Items()) {
				node = node.Next()
				nodePos = 0
				if node == nil {
					break
				}
			}
		}
	}

	itemPtr, _ := doc.items.GetAt(idx)
	item := *itemPtr

	if item.length > 1 {
		split(doc, idx, 1)
		itemPtr, _ = doc.items.GetAt(idx)
		item = *itemPtr
	}

	if !item.deleted {
		item.deleted = true
		if snapshot != nil {
			err := snapshot.DeleteAt(endPos)
			if err != nil {
				panic("Snapshot delete failed")
			}
		}
		item.node.SummaryAddUpward(crdtSummary{0, -1}, doc.items)
	}

	item.curState = 1 // Deleted(1)
	item.node.SummaryAddUpward(crdtSummary{-1, 0}, doc.items)

	doc.delTargets[opLV] = item.lv
	tryMergeAt(doc, log, idx)
}

func apply[E any, C content[E]](doc *crdtDoc, log *opLog[E, C], snapshot *bxtree.BxTree[E, struct{}], opLV lv) {
	idx := log.opIdxAt(opLV)
	o := &log.ops[idx]
	first := log.opLV[idx]

	if o.opType == opTypeDel {
		// A delete run removes o.length visible characters at positions
		// pos..pos+o.length-1 of the staged state, one character at a time.
		for i := 0; i < o.length; i++ {
			deleteOne(doc, log, snapshot, first+lv(i), o.pos)
		}
		return
	}

	posIdx, endPos := findByCurrentPos(doc, o.pos)

	if posIdx >= 1 {
		prevPtr, _ := doc.items.GetAt(posIdx - 1)
		if (*prevPtr).curState != stateInserted {
			panic("Item to the left is not inserted!")
		}
	}

	originLeft := lv(-1)
	if posIdx > 0 {
		prevPtr, _ := doc.items.GetAt(posIdx - 1)
		originLeft = (*prevPtr).lv + lv((*prevPtr).length-1)
	}

	originRight := lv(-1)
	for i := posIdx; i < doc.items.Size(); i++ {
		item2Ptr, _ := doc.items.GetAt(i)
		item2 := *item2Ptr
		if item2.curState != stateNotYetInserted {
			originRight = item2.lv
			break
		}
	}

	item := &crdtItem{
		lv:          first,
		originLeft:  originLeft,
		originRight: originRight,
		deleted:     false,
		curState:    stateInserted,
		length:      o.length,
	}
	addItemLV(doc, item)

	posIdx = integrate(doc, log, item, posIdx, endPos, snapshot)
	tryMergeAt(doc, log, posIdx)
}

func do1Operation[E any, C content[E]](doc *crdtDoc, log *opLog[E, C], opLV lv, snapshot *bxtree.BxTree[E, struct{}]) {
	idx := log.opIdxAt(opLV)
	o := &log.ops[idx]
	first := log.opLV[idx]
	end := log.endLV(idx)

	diffRes := diff(log, doc.currentVersion, o.parents)

	for _, i := range diffRes.aOnly {
		retreat(doc, log, i)
	}
	for _, i := range diffRes.bOnly {
		advance(doc, log, i)
	}

	apply(doc, log, snapshot, first)
	doc.currentVersion = []lv{end}
}

func checkout[E any, C content[E]](log *opLog[E, C]) *bxtree.BxTree[E, struct{}] {
	doc := &crdtDoc{
		items: newBxTree(
			bxtree.WithSummarizer(crdtSummaryConfig),
			bxtree.WithOnItemMoved(func(item *crdtItem, node *bxtree.Node[*crdtItem, crdtSummary]) {
				item.node = node
			}),
		),
		currentVersion: []lv{},
		delTargets:     make(map[lv]lv),
		sortedItems:    []*crdtItem{},
	}

	snapshot := newBxTree[E, struct{}]()

	for i := 0; i < len(log.ops); i++ {
		do1Operation(doc, log, log.opLV[i], snapshot)
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

func findOpsToVisit[E any, C content[E]](log *opLog[E, C], a []lv, b []lv) opsToVisit {
	// Phase 1: Find Common Ancestor (CCA) using the original Priority Queue approach
	pq := pheap.NewAny(pheap.WithLess(func(a, b mergePoint) bool {
		return compareArrays(a.v, b.v) < 0
	}))

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
		pq.Push(mp)
	}

	enq(a, true)
	enq(b, false)

	var commonVersion []lv
	var sharedOpsSet = make(map[lv]bool)
	var bOnlyOpsSet = make(map[lv]bool)
	var allDeltaOps = make(map[lv]bool)

	// We need to keep track of which ops belong to which side in the delta
	for {
		item, ok := pq.Pop()
		if !ok {
			break
		}
		v := item.v
		isInA := item.isInA

		if len(v) == 0 {
			commonVersion = []lv{}
			break
		}

		for {
			peekItem, ok := pq.Peek()
			if !ok || compareArrays(v, peekItem.v) != 0 {
				break
			}
			pq.Pop()
			if peekItem.isInA {
				isInA = true
			}
		}

		if pq.Size() == 0 {
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
			allDeltaOps[curLV] = true
			if isInA {
				sharedOpsSet[curLV] = true
			} else {
				bOnlyOpsSet[curLV] = true
			}

			o := log.opAt(curLV)
			enq(o.parents, isInA)
		}
	}

	// Phase 2: Build Child Mapping for the Delta Subgraph
	children := make(map[lv][]lv)
	for curLV := range allDeltaOps {
		o := log.opAt(curLV)
		for _, p := range o.parents {
			if allDeltaOps[p] {
				children[p] = append(children[p], curLV)
			}
		}
	}

	// Also add children for the commonVersion ops that are in the delta's direct ancestry
	// Actually, easier: iterate all delta ops and if a parent is in commonVersion, add it.
	ccaSet := make(map[lv]bool)
	for _, v := range commonVersion {
		ccaSet[v] = true
	}
	
	roots := []lv{}
	for curLV := range allDeltaOps {
		o := log.opAt(curLV)
		isRoot := true
		for _, p := range o.parents {
			if allDeltaOps[p] {
				isRoot = false
			}
			if ccaSet[p] {
				children[p] = append(children[p], curLV)
			}
		}
		if isRoot {
			roots = append(roots, curLV)
		}
	}

	// Phase 3: Calculate Weights (Iterative descendant count within Delta)
	weights := make(map[lv]int)
	
	// To calculate weights iteratively, we need to process nodes in reverse topological order (bottom-up)
	// We can use the fact that larger LVs are generally descendants.
	// But to be sure, we'll use a Kahn-like approach but for weights.
	inDegree := make(map[lv]int)
	for curLV := range allDeltaOps {
		o := log.opAt(curLV)
		for _, p := range o.parents {
			if allDeltaOps[p] {
				inDegree[p]++
			}
		}
	}

	// Leaf nodes in the Delta subgraph (no children in Delta)
	queue := []lv{}
	for curLV := range allDeltaOps {
		if inDegree[curLV] == 0 {
			queue = append(queue, curLV)
		}
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		
		w := 1
		for _, child := range children[cur] {
			w += weights[child]
		}
		weights[cur] = w

		// Move up to parents
		o := log.opAt(cur)
		for _, p := range o.parents {
			if allDeltaOps[p] {
				inDegree[p]--
				if inDegree[p] == 0 {
					queue = append(queue, p)
				}
			}
		}
	}

	// Phase 4: Heuristic DFS Topological Sort (Iterative using a stack)
	var sharedOps, bOnlyOps []lv
	visited := make(map[lv]bool)
	
	// Sort roots by weight (ascending) initially
	sort.Slice(roots, func(i, j int) bool {
		return weights[roots[i]] < weights[roots[j]]
	})

	stack := make([]lv, 0, len(roots))
	// Push roots in reverse order so lightest is popped first
	for i := len(roots) - 1; i >= 0; i-- {
		stack = append(stack, roots[i])
	}

	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[cur] {
			continue
		}

		// Check if all parents in Delta are visited
		ready := true
		o := log.opAt(cur)
		for _, p := range o.parents {
			if allDeltaOps[p] && !visited[p] {
				ready = false
				break
			}
		}

		if !ready {
			continue
		}

		visited[cur] = true
		if sharedOpsSet[cur] {
			sharedOps = append(sharedOps, cur)
		} else {
			bOnlyOps = append(bOnlyOps, cur)
		}

		// Add children to stack
		childList := children[cur]
		// Sort children by weight DESCENDING so that when we push them onto the stack,
		// the ASCENDING order (lightest first) is preserved for popping.
		sort.Slice(childList, func(i, j int) bool {
			return weights[childList[i]] > weights[childList[j]]
		})

		for _, child := range childList {
			stack = append(stack, child)
		}
	}

	return opsToVisit{
		commonVersion: commonVersion,
		sharedOps:     sharedOps,
		bOnlyOps:      bOnlyOps,
	}
}

func newBranch[T any]() *branch[T] {
	return &branch[T]{
		snapshot: newBxTree[T, struct{}](),
		frontier: []lv{},
	}
}

func checkoutFancy[E any, C content[E]](log *opLog[E, C], b *branch[E], mergeFrontier []lv) {
	if mergeFrontier == nil {
		mergeFrontier = log.frontier
	}

	visit := findOpsToVisit(log, b.frontier, mergeFrontier)

	doc := &crdtDoc{
		items: newBxTree(
			bxtree.WithSummarizer(crdtSummaryConfig),
			bxtree.WithOnItemMoved(func(item *crdtItem, node *bxtree.Node[*crdtItem, crdtSummary]) {
				item.node = node
			}),
		),
		currentVersion: visit.commonVersion,
		delTargets:     make(map[lv]lv),
		sortedItems:    []*crdtItem{},
	}

	maxFrontier := -1
	for _, v := range b.frontier {
		if int(v) > maxFrontier {
			maxFrontier = int(v)
		}
	}
	placeholderLength := max(0, maxFrontier+1)

	if placeholderLength > 0 {
		item := &crdtItem{
			lv:          1e12,
			curState:    stateInserted,
			deleted:     false,
			originLeft:  -1,
			originRight: -1,
			length:      placeholderLength,
		}
		doc.items.InsertAt(doc.items.Size(), item)
		addItemLV(doc, item)
	}

	for _, curLV := range visit.sharedOps {
		do1Operation(doc, log, curLV, nil)
	}

	for _, curLV := range visit.bOnlyOps {
		do1Operation(doc, log, curLV, b.snapshot)
		o := log.opAt(curLV)
		b.frontier = advanceFrontier(b.frontier, curLV, o.parents)
	}
}
