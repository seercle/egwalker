package crdt

import (
	"sort"
)

// ==========================================
// OpLog Functions (Internal)
// ==========================================

func newOpLog[T any]() *opLog[T] {
	return &opLog[T]{
		ops:        []op[T]{},
		opStartLVs: []lv{},
		agentOps:   make(map[int][]int),
		frontier:   []lv{},
		version:    make(remoteVersion),
	}
}

func (log *opLog[T]) pushLocalOp(agent int, o op[T]) {
	lastSeq, ok := log.version[agent]
	if !ok {
		lastSeq = -1
	}
	seq := lastSeq + 1

	var curLV lv
	if len(log.ops) == 0 {
		curLV = 0
	} else {
		lastIdx := len(log.ops) - 1
		curLV = log.opStartLVs[lastIdx] + lv(log.ops[lastIdx].length)
	}
	
	o.id = id{agent: agent, seq: seq}
	o.parents = make([]lv, len(log.frontier))
	copy(o.parents, log.frontier)

	opIdx := len(log.ops)
	log.ops = append(log.ops, o)
	log.opStartLVs = append(log.opStartLVs, curLV)
	log.agentOps[agent] = append(log.agentOps[agent], opIdx)
	
	lastLVOfRun := curLV + lv(o.length-1)
	log.frontier = []lv{lastLVOfRun}
	log.version[agent] = seq + o.length - 1
}

func localInsert[T any](log *opLog[T], agent int, pos int, content []T) {
	log.pushLocalOp(agent, op[T]{
		opType:  opTypeIns,
		content: content,
		length:  len(content),
		pos:     pos,
	})
}

func localDelete[T any](log *opLog[T], agent int, pos int, delLen int) {
	log.pushLocalOp(agent, op[T]{
		opType: opTypeDel,
		length: delLen,
		pos:    pos,
	})
}

func (log *opLog[T]) getOpByLV(v lv) (opIdx int, offset int) {
	opIdx = sort.Search(len(log.opStartLVs), func(i int) bool {
		return log.opStartLVs[i] > v
	}) - 1
	if opIdx < 0 {
		panic("LV not found")
	}
	offset = int(v - log.opStartLVs[opIdx])
	if offset >= log.ops[opIdx].length {
		panic("LV offset out of bounds")
	}
	return opIdx, offset
}

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
	if offset >= o.length {
		panic("Sequence number out of bounds for run")
	}
	return log.opStartLVs[opIdx] + lv(offset)
}

func idToLV[T any](log *opLog[T], target_id id) lv {
	return log.resolveID(target_id)
}

func (log *opLog[T]) isAncestor(ancestorLV, descendantLV lv) bool {
	if ancestorLV == descendantLV {
		return true
	}
	if ancestorLV > descendantLV {
		return false
	}

	visited := make(map[lv]bool)
	stack := []lv{descendantLV}
	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if curr == ancestorLV {
			return true
		}
		if curr < ancestorLV {
			continue
		}

		opIdx, _ := log.getOpByLV(curr)
		for _, p := range log.ops[opIdx].parents {
			if !visited[p] {
				visited[p] = true
				stack = append(stack, p)
			}
		}
	}
	return false
}

func sortLVs(frontier []lv) []lv {
	sort.Slice(frontier, func(i, j int) bool {
		return frontier[i] < frontier[j]
	})
	return frontier
}

func advanceFrontier(frontier []lv, cur_lv lv, parents []lv) []lv {
	f := []lv{}
	parent_map := make(map[lv]bool)
	for _, p := range parents {
		parent_map[p] = true
	}

	for _, v := range frontier {
		if !parent_map[v] {
			f = append(f, v)
		}
	}
	f = append(f, cur_lv)
	return sortLVs(f)
}

func pushRemoteOp[T any](log *opLog[T], o op[T], parentIds []id) {
	agent := o.id.agent
	seq := o.id.seq

	lastKnownSeq, ok := log.version[agent]
	if !ok {
		lastKnownSeq = -1
	}

	if lastKnownSeq >= seq+o.length-1 {
		return // Already have the whole run
	}

	var curLV lv
	if len(log.ops) == 0 {
		curLV = 0
	} else {
		lastIdx := len(log.ops) - 1
		curLV = log.opStartLVs[lastIdx] + lv(log.ops[lastIdx].length)
	}

	parents := make([]lv, len(parentIds))
	for i, pid := range parentIds {
		parents[i] = log.resolveID(pid)
	}
	o.parents = sortLVs(parents)

	opIdx := len(log.ops)
	log.ops = append(log.ops, o)
	log.opStartLVs = append(log.opStartLVs, curLV)
	log.agentOps[agent] = append(log.agentOps[agent], opIdx)
	
	lastLVOfRun := curLV + lv(o.length-1)
	log.frontier = advanceFrontier(log.frontier, lastLVOfRun, o.parents)

	log.version[agent] = max(log.version[agent], seq+o.length-1)
}

func mergeInto[T any](dest *opLog[T], src *opLog[T]) {
	for _, o := range src.ops {
		parent_ids := make([]id, len(o.parents))
		for i, p_lv := range o.parents {
			opIdx, offset := src.getOpByLV(p_lv)
			parent_ids[i] = id{
				agent: src.ops[opIdx].id.agent,
				seq:   src.ops[opIdx].id.seq + offset,
			}
		}
		new_op := o
		pushRemoteOp(dest, new_op, parent_ids)
	}
}
