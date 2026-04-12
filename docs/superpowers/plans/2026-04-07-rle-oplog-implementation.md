# RLE OpLog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Run-Length Encoding (RLE) in the `opLog` to store contiguous operations as single units, improving memory efficiency and performance.

**Architecture:** Use "Virtual LVs" where each logical version (LV) represents a character/item index. `opLog` stores `op` runs with a `length`. Mapping between LVs and `op` indexes is done via binary search on an `opStartLVs` array.

**Tech Stack:** Go (Generics)

---

### Task 1: Update Data Structures in `types.go`

**Files:**
- Modify: `go/crdt/types.go`

- [ ] **Step 1: Update `id`, `op`, and `opLog` structures**

```go
type id struct {
	agent int
	seq   int // Starting sequence number for this run
}

type op[T any] struct {
	opType  opType
	content []T   // Slice of items for insertions (empty for deletions)
	length  int   // Total number of items in this run
	pos     int   // Starting document position for local ops
	id      id    // ID of the FIRST item in the run
	parents []lv  // LVs that the START of this run directly depends on
}

type opLog[T any] struct {
	ops        []op[T]
	opStartLVs []lv            // Starting LV for each op in 'ops' (sorted)
	agentOps   map[int][]int   // Maps agent ID -> list of indexes into 'ops', sorted by seq
	frontier   []lv            // Current maxima (ends of runs)
	version    remoteVersion   // Max seq seen per agent
}
```

- [ ] **Step 2: Update `newOpLog` in `op_log.go`**

```go
func newOpLog[T any]() *opLog[T] {
	return &opLog[T]{
		ops:        []op[T]{},
		opStartLVs: []lv{},
		agentOps:   make(map[int][]int),
		frontier:   []lv{},
		version:    make(remoteVersion),
	}
}
```

- [ ] **Step 3: Commit**

```bash
git add go/crdt/types.go go/crdt/op_log.go
git commit -m "refactor: update opLog data structures for RLE"
```

### Task 2: Implement LV and ID Resolution

**Files:**
- Modify: `go/crdt/op_log.go`

- [ ] **Step 1: Implement `getOpByLV`**

```go
func (log *opLog[T]) getOpByLV(v lv) (opIdx int, offset int) {
	opIdx = sort.Search(len(log.opStartLVs), func(i int) bool {
		return log.opStartLVs[i] > v
	}) - 1
	if opIdx < 0 {
		panic("LV not found")
	}
	offset = int(v - log.opStartLVs[opIdx])
	return opIdx, offset
}
```

- [ ] **Step 2: Implement `resolveID` (formerly `idToLV`)**

```go
func (log *opLog[T]) resolveID(target id) lv {
	idxs := log.agentOps[target.agent]
	i := sort.Search(len(idxs), func(i int) bool {
		return log.ops[idxs[i]].id.seq > target.seq
	}) - 1
	if i < 0 {
		panic("Sequence number not found")
	}
	opIdx := idxs[i]
	o := log.ops[opIdx]
	offset := target.seq - o.id.seq
	return log.opStartLVs[opIdx] + lv(offset)
}
```

- [ ] **Step 3: Update `idToLV` wrapper for backward compatibility (temporary)**

```go
func idToLV[T any](log *opLog[T], target_id id) lv {
	return log.resolveID(target_id)
}
```

- [ ] **Step 4: Commit**

```bash
git add go/crdt/op_log.go
git commit -m "feat: implement LV and ID resolution for RLE opLog"
```

### Task 3: Update Operation Insertion Logic

**Files:**
- Modify: `go/crdt/op_log.go`

- [ ] **Step 1: Update `pushLocalOp`**

```go
func (log *opLog[T]) pushLocalOp(agent int, o op[T]) {
	last_seq, ok := log.version[agent]
	if !ok {
		last_seq = -1
	}
	seq := last_seq + 1

	var cur_lv lv
	if len(log.ops) == 0 {
		cur_lv = 0
	} else {
		lastIdx := len(log.ops) - 1
		cur_lv = log.opStartLVs[lastIdx] + lv(log.ops[lastIdx].length)
	}
	
	o.id = id{agent: agent, seq: seq}
	o.parents = make([]lv, len(log.frontier))
	copy(o.parents, log.frontier)

	opIdx := len(log.ops)
	log.ops = append(log.ops, o)
	log.opStartLVs = append(log.opStartLVs, cur_lv)
	log.agentOps[agent] = append(log.agentOps[agent], opIdx)
	
	lastLV := cur_lv + lv(o.length-1)
	log.frontier = []lv{lastLV}
	log.version[agent] = seq + o.length - 1
}
```

- [ ] **Step 2: Update `localInsert` and `localDelete` to use runs**

```go
func localInsert[T any](log *opLog[T], agent int, pos int, content []T) {
	log.pushLocalOp(agent, op[T]{
		opType:  opTypeIns,
		content: content,
		length:  len(content),
		pos:     pos,
	})
}

func localDelete[T any](log *opLog[T], agent int, pos int, del_len int) {
	log.pushLocalOp(agent, op[T]{
		opType: opTypeDel,
		length: del_len,
		pos:    pos,
	})
}
```

- [ ] **Step 3: Update `pushRemoteOp`**

```go
func pushRemoteOp[T any](log *opLog[T], o op[T], parent_ids []id) {
	agent := o.id.agent
	seq := o.id.seq

	last_known_seq, ok := log.version[agent]
	if !ok {
		last_known_seq = -1
	}

	if last_known_seq >= seq+o.length-1 {
		return // Already have the whole run
	}

	var cur_lv lv
	if len(log.ops) == 0 {
		cur_lv = 0
	} else {
		lastIdx := len(log.ops) - 1
		cur_lv = log.opStartLVs[lastIdx] + lv(log.ops[lastIdx].length)
	}

	parents := make([]lv, len(parent_ids))
	for i, pid := range parent_ids {
		parents[i] = log.resolveID(pid)
	}
	o.parents = sortLVs(parents)

	opIdx := len(log.ops)
	log.ops = append(log.ops, o)
	log.opStartLVs = append(log.opStartLVs, cur_lv)
	log.agentOps[agent] = append(log.agentOps[agent], opIdx)
	
	lastLV := cur_lv + lv(o.length-1)
	log.frontier = advanceFrontier(log.frontier, lastLV, o.parents)

	log.version[agent] = max(log.version[agent], seq+o.length-1)
}
```

- [ ] **Step 4: Commit**

```bash
git add go/crdt/op_log.go
git commit -m "feat: update local and remote op insertion for RLE"
```

### Task 4: Update CRDT Integration Logic (BxTree)

**Files:**
- Modify: `go/crdt/crdt.go`

- [ ] **Step 1: Update `diff` to handle RLE**
Actually, `diff` works on LVs, but when it enqueues parents, it should enqueue the **last** LV of each parent run.

```go
// Inside diff:
o := log.ops[curLV] // Wait, curLV is the virtual LV. Need opIdx.
opIdx, _ := log.getOpByLV(curLV)
o := log.ops[opIdx]
for _, p := range o.parents {
    enq(p, flag)
}
```

- [ ] **Step 2: Update `apply` and `integrate`**
`apply` for insertion should use `o.length`.

```go
// Inside apply:
item := &crdtItem{
    lv:          opLV,
    originLeft:  originLeft,
    originRight: originRight,
    deleted:     false,
    curState:    stateInserted,
    length:      o.length, // Use run length
}
```

`integrate` needs to handle `o.content` as a slice.

```go
// Inside integrate:
if snapshot != nil {
    err := snapshot.InsertRange(endPos, o.content) // Use InsertRange
    if err != nil {
        panic("Snapshot insert failed")
    }
}
```

- [ ] **Step 3: Update `retreat` and `advance`**
They need to work on the entire run or atomize if needed. For simplicity, we'll atomize.

```go
func retreat[T any](doc *crdtDoc, log *opLog[T], opLV lv) {
    opIdx, _ := log.getOpByLV(opLV)
    o := log.ops[opIdx]
    for i := range o.length {
        targetLV := opLV + lv(i)
        if o.opType == opTypeDel {
            targetLV = doc.delTargets[opLV + lv(i)]
        }
        item := ensureAtomized(doc, targetLV)
        // ... rest of retreat logic ...
    }
}
```

Wait, `delTargets` needs to be updated to map `lv -> lv`.

- [ ] **Step 4: Commit**

```bash
git add go/crdt/crdt.go
git commit -m "feat: update CRDT integration logic for RLE runs"
```

### Task 5: Update Serialization

**Files:**
- Modify: `go/crdt/serialization.go`

- [ ] **Step 1: Update `Marshal` and `Unmarshal`**
The columnar format should now iterate over `log.ops` (runs) instead of virtual LVs.

```go
// Inside Marshal:
for i, o := range log.ops {
    // ... RLE logic ...
    res.Content = append(res.Content, o.content...) // Expand content slice
    res.Parents = append(res.Parents, o.parents)
}
```

Wait, `res.Content` should probably be a slice of slices or we need to track run lengths in `ColumnarData`. Actually, `ColumnarData` already has `TypeRuns` and `AgentRuns`. We should add `OpLengths`.

- [ ] **Step 2: Commit**

```bash
git add go/crdt/serialization.go
git commit -m "feat: update serialization for RLE opLog"
```

### Task 6: Verification and Bug Fixing

**Files:**
- Modify: `go/crdt/crdt_unit_test.go`, `go/crdt/fuzzer_test.go`

- [ ] **Step 1: Run existing tests**

Run: `go test -v -C go ./crdt`
Expected: Many failures initially.

- [ ] **Step 2: Fix tests iteratively**
Update test cases that assume `lv` is a simple index.

- [ ] **Step 3: Run fuzzer**

Run: `go test -v -run TestFuzzerMerge -C go ./crdt`
Run: `go test -v -run TestFuzzerSlice -C go ./crdt`

- [ ] **Step 4: Commit fixes**

```bash
git commit -m "test: fix tests and verify RLE implementation"
```
