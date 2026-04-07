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
	last_seq, ok := log.version[agent]
	if !ok {
		last_seq = -1
	}
	seq := last_seq + 1

	cur_lv := lv(len(log.ops))
	o.id = id{agent: agent, seq: seq}

	parents_copy := make([]lv, len(log.frontier))
	copy(parents_copy, log.frontier)
	o.parents = parents_copy

	log.ops = append(log.ops, o)
	log.frontier = []lv{cur_lv}
	log.version[agent] = seq
}

func localInsert[T any](log *opLog[T], agent int, pos int, content []T) {
	current_pos := pos
	for _, c := range content {
		localInsertOne(log, agent, current_pos, c)
		current_pos++
	}
}

func localInsertOne[T any](log *opLog[T], agent int, pos int, content T) {
	log.pushLocalOp(agent, op[T]{
		opType:  opTypeIns,
		content: []T{content},
		length:  1,
		pos:     pos,
	})
}

func localDelete[T any](log *opLog[T], agent int, pos int, del_len int) {
	for del_len > 0 {
		log.pushLocalOp(agent, op[T]{
			opType: opTypeDel,
			pos:    pos,
		})
		del_len--
	}
}

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

		for _, p := range log.ops[curr].parents {
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

func pushRemoteOp[T any](log *opLog[T], o op[T], parent_ids []id) {
	agent := o.id.agent
	seq := o.id.seq

	last_known_seq, ok := log.version[agent]
	if !ok {
		last_known_seq = -1
	}

	if last_known_seq >= seq {
		return // Already have the op
	}

	cur_lv := lv(len(log.ops))

	// Resolve parents
	parents := make([]lv, len(parent_ids))
	for i, pid := range parent_ids {
		parents[i] = idToLV(log, pid)
	}
	o.parents = sortLVs(parents)

	log.ops = append(log.ops, o)
	log.frontier = advanceFrontier(log.frontier, cur_lv, o.parents)

	if seq != last_known_seq+1 {
		panic("Seq numbers out of order")
	}
	log.version[agent] = seq
}

func mergeInto[T any](dest *opLog[T], src *opLog[T]) {
	for _, o := range src.ops {
		parent_ids := make([]id, len(o.parents))
		for i, p_lv := range o.parents {
			parent_ids[i] = src.ops[p_lv].id
		}
		new_op := o
		pushRemoteOp(dest, new_op, parent_ids)
	}
}
