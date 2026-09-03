package crdt

import (
	"sort"
)

// ==========================================
// OpLog Functions (Internal)
// ==========================================

func newOpLog[E any, C content[E]]() *opLog[E, C] {
	return &opLog[E, C]{
		ops:      []op[E, C]{},
		opLV:     []lv{},
		totalLV:  0,
		frontier: []lv{},
		version:  make(remoteVersion),
		idToLV:   make(map[id]lv),
	}
}

func (log *opLog[E, C]) pushLocalOp(agent int, o op[E, C]) lv {
	last_seq, ok := log.version[agent]
	if !ok {
		last_seq = -1
	}
	seq := last_seq + 1

	cur_lv := lv(len(log.ops))
	o.id = id{agent: agent, seq: seq}

	// An op's length is derived from its run content. Delete ops carry no
	// content in this model, so their length stays as set by the caller
	// (zero today; a later task sets delete-run lengths explicitly).
	if o.opType != opTypeDel {
		o.length = o.content.Len()
	}

	parents_copy := make([]lv, len(log.frontier))
	copy(parents_copy, log.frontier)
	o.parents = parents_copy

	firstLV := log.totalLV
	log.ops = append(log.ops, o)
	log.opLV = append(log.opLV, firstLV)
	log.totalLV += lv(o.length)
	log.idToLV[o.id] = cur_lv
	log.frontier = []lv{cur_lv}
	log.version[agent] = seq
	return firstLV
}

// localInsert fans a run of content out into per-element single ops: each
// element becomes one length-1 op, preserving the per-character model. When
// the RLE optimization lands, whole runs collapse here instead.
func localInsert[E any, C content[E]](log *opLog[E, C], agent int, pos int, content C) {
	current_pos := pos
	for _, c := range content.Elems() {
		localInsertOne(log, agent, current_pos, c)
		current_pos++
	}
}

func localInsertOne[E any, C content[E]](log *opLog[E, C], agent int, pos int, content E) {
	log.pushLocalOp(agent, op[E, C]{
		opType:  opTypeIns,
		content: oneRun[E, C](content),
		pos:     pos,
	})
}

func localDelete[E any, C content[E]](log *opLog[E, C], agent int, pos int, del_len int) {
	for del_len > 0 {
		log.pushLocalOp(agent, op[E, C]{
			opType: opTypeDel,
			pos:    pos,
		})
		del_len--
	}
}

func idToLV[E any, C content[E]](log *opLog[E, C], target_id id) lv {
	if lv, ok := log.idToLV[target_id]; ok {
		return lv
	}
	panic("Could not find id in oplog")
}

func (log *opLog[E, C]) isAncestor(ancestorLV, descendantLV lv) bool {
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

func pushRemoteOp[E any, C content[E]](log *opLog[E, C], o op[E, C], parent_ids []id) {
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

	if o.opType != opTypeDel {
		o.length = o.content.Len()
	}

	// Resolve parents
	parents := make([]lv, len(parent_ids))
	for i, pid := range parent_ids {
		parents[i] = idToLV(log, pid)
	}
	o.parents = sortLVs(parents)

	firstLV := log.totalLV
	log.ops = append(log.ops, o)
	log.opLV = append(log.opLV, firstLV)
	log.totalLV += lv(o.length)
	log.idToLV[o.id] = cur_lv
	log.frontier = advanceFrontier(log.frontier, cur_lv, o.parents)

	if seq != last_known_seq+1 {
		panic("Seq numbers out of order")
	}
	log.version[agent] = seq
}

func mergeInto[E any, C content[E]](dest *opLog[E, C], src *opLog[E, C]) {
	for _, o := range src.ops {
		parent_ids := make([]id, len(o.parents))
		for i, p_lv := range o.parents {
			parent_ids[i] = src.ops[p_lv].id
		}
		new_op := o
		pushRemoteOp(dest, new_op, parent_ids)
	}
}
