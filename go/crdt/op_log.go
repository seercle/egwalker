package crdt

import (
	"fmt"
	"sort"
)

// ==========================================
// OpLog Functions (Internal)
// ==========================================

func newOpLog[C content[C]]() *opLog[C] {
	return &opLog[C]{
		ops:      []op[C]{},
		opLV:     []lv{},
		totalLV:  0,
		frontier: []lv{},
		version:  make(remoteVersion),
		idToLV:   make(map[id]lv),
	}
}

// covers reports whether the character lv x lies inside some run op's span.
func (log *opLog[C]) covers(x lv) bool {
	i := sort.Search(len(log.opLV), func(i int) bool { return log.opLV[i] > x })
	return i > 0 && x < log.opLV[i-1]+lv(log.ops[i-1].length)
}

// opIdxAt returns the index of the run op whose [opLV[i], opLV[i]+length) span
// contains the character lv x. Every character lv belongs to exactly one run op
// (insert runs and delete runs both occupy length character slots), so the
// first-LV table partitions [0, totalLV).
func (log *opLog[C]) opIdxAt(x lv) int {
	i := sort.Search(len(log.opLV), func(i int) bool { return log.opLV[i] > x })
	if !(i > 0 && x < log.opLV[i-1]+lv(log.ops[i-1].length)) {
		panic(fmt.Sprintf("oplog: lv %d not covered by any op", x))
	}
	return i - 1
}

// opAt resolves an arbitrary character lv to its containing run op.
func (log *opLog[C]) opAt(x lv) *op[C] {
	return &log.ops[log.opIdxAt(x)]
}

// seqAt returns the per-agent sequence number of the character at lv x.
func (log *opLog[C]) seqAt(x lv) int {
	i := log.opIdxAt(x)
	return log.ops[i].id.seq + int(x-log.opLV[i])
}

// endLV returns the last character lv covered by run op i (its causal head).
func (log *opLog[C]) endLV(i int) lv {
	return log.opLV[i] + lv(log.ops[i].length) - 1
}

// pushLocalOp appends (or extends) a local op and returns its first LV. A new
// insert op merges into log.ops[len-1] iff the tail op is the sole causal head,
// both are inserts from the same agent, the seq/pos are character-adjacent, and
// neither content holds Mergeable elements.
func (log *opLog[C]) pushLocalOp(agent int, o op[C]) lv {
	lastSeq, ok := log.version[agent]
	if !ok {
		lastSeq = -1
	}
	o.id = id{agent: agent, seq: lastSeq + 1}
	if o.opType != opTypeDel {
		o.length = o.content.Len()
	}

	last := len(log.ops) - 1
	lastEnd := lv(-1)
	if last >= 0 {
		lastEnd = log.endLV(last)
	}
	if last >= 0 && o.opType == opTypeIns && log.ops[last].opType == opTypeIns &&
		len(log.frontier) == 1 && log.frontier[0] == lastEnd &&
		log.ops[last].id.agent == agent &&
		o.id.seq == log.ops[last].id.seq+log.ops[last].length &&
		o.pos == log.ops[last].pos+log.ops[last].length &&
		o.content.Collapsible() && log.ops[last].content.Collapsible() {

		log.ops[last].content = log.ops[last].content.Concat(o.content)
		log.ops[last].length = log.ops[last].content.Len()
		first := log.opLV[last]
		end := first + lv(log.ops[last].length) - 1
		log.frontier = []lv{end}
		log.idToLV[log.ops[last].id] = end
		log.version[agent] = o.id.seq + o.length - 1
		log.totalLV += lv(o.length)
		return first
	}

	o.parents = make([]lv, len(log.frontier))
	copy(o.parents, log.frontier)
	first := log.totalLV
	log.ops = append(log.ops, o)
	log.opLV = append(log.opLV, first)
	log.totalLV += lv(o.length)
	log.idToLV[o.id] = first + lv(o.length) - 1
	log.frontier = []lv{first + lv(o.length) - 1}
	log.version[agent] = o.id.seq + o.length - 1
	return first
}

// localDelete pushes a contiguous range delete as a single run op whose length
// is the number of characters deleted. Delete runs carry no content; length is
// authoritative.
func localDelete[C content[C]](log *opLog[C], agent int, pos int, delLen int) {
	if delLen <= 0 {
		return
	}
	log.pushLocalOp(agent, op[C]{
		opType: opTypeDel,
		length: delLen,
		pos:    pos,
	})
}

func idToLV[C content[C]](log *opLog[C], target_id id) lv {
	if lv, ok := log.idToLV[target_id]; ok {
		return lv
	}
	panic("Could not find id in oplog")
}

func (log *opLog[C]) isAncestor(ancestorLV, descendantLV lv) bool {
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

		for _, p := range log.opAt(curr).parents {
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

// opEndLVForSeq returns the end lv of the run op from `agent` whose seq range
// contains `seq`. Used to parent a split-off extension op correctly when a run
// op re-arrives in extended form, and to re-encode parent references across
// replicas whose run boundaries differ.
func (log *opLog[C]) opEndLVForSeq(agent, seq int) lv {
	return log.endLV(log.runIdxForSeq(agent, seq))
}

// runIdxForSeq returns the index of the run op from `agent` whose seq range
// contains `seq`.
func (log *opLog[C]) runIdxForSeq(agent, seq int) int {
	for i := len(log.ops) - 1; i >= 0; i-- {
		o := &log.ops[i]
		if o.id.agent == agent && seq >= o.id.seq && seq < o.id.seq+o.length {
			return i
		}
	}
	panic(fmt.Sprintf("oplog: no op from agent %d covers seq %d", agent, seq))
}

// splitRunOp splits run op j so that its first k characters form the prefix op
// (kept at index j) and the rest become a new suffix op inserted at j+1. The
// split preserves the lv span, so no later opLV/parent/frontier renumbering is
// needed; only the idToLV entries for the two halves change. It returns the
// prefix op's end lv.
func (log *opLog[C]) splitRunOp(j, k int) lv {
	o := log.ops[j]
	if o.opType != opTypeIns || k <= 0 || k >= o.length {
		panic("oplog: invalid run split")
	}
	prefix, suffix := o.content.SplitAt(k)
	prefixEnd := log.opLV[j] + lv(k) - 1

	suffixOp := op[C]{
		opType:  opTypeIns,
		content: suffix,
		length:  o.length - k,
		pos:     o.pos + k,
		id:      id{agent: o.id.agent, seq: o.id.seq + k},
		parents: []lv{prefixEnd},
	}

	o.content = prefix
	o.length = k
	log.ops[j] = o

	log.ops = append(log.ops, op[C]{})
	copy(log.ops[j+2:], log.ops[j+1:])
	log.ops[j+1] = suffixOp

	log.opLV = append(log.opLV, 0)
	copy(log.opLV[j+2:], log.opLV[j+1:])
	log.opLV[j+1] = prefixEnd + 1

	log.idToLV[o.id] = prefixEnd
	log.idToLV[suffixOp.id] = log.endLV(j + 1)
	return prefixEnd
}

// resolveParentLV returns the run-node lv in log for the character (agent, seq).
// If that character is interior to one of our run ops (a replica observed a
// boundary inside a run we hold fused), the run is split at the character so the
// reference resolves to a real boundary. This keeps ancestry, and therefore the
// winding-based origin computation, identical across replicas whose run
// boundaries differ.
func (log *opLog[C]) resolveParentLV(agent, seq int) lv {
	j := log.runIdxForSeq(agent, seq)
	o := &log.ops[j]
	if seq == o.id.seq+o.length-1 {
		return log.endLV(j)
	}
	return log.splitRunOp(j, seq-o.id.seq+1)
}

// pushRemoteOp appends a run op received from another replica, resolving its
// parent op ids to destination character lvs. Ops are pushed whole and never
// collapse on the receiving side, and an applied op's lv span is immutable: it
// is never mutated in place. When the owner extended a run after a prior sync,
// the op re-arrives carrying a strict prefix we already hold; we validate that
// prefix against the held copy, derive the unknown suffix, and append it as a
// NEW op at the log tail with the synthetic id {agent, last_known_seq+1} and a
// single parent edge to the end lv of the op covering the last known seq.
func pushRemoteOp[C content[C]](log *opLog[C], o op[C], parent_ids []id) {
	parents := make([]lv, len(parent_ids))
	for i, pid := range parent_ids {
		parents[i] = idToLV(log, pid)
	}
	pushRemoteOpLV(log, o, parents)
}

// pushRemoteOpLV appends a run op received from another replica whose parent
// references are already resolved to destination character lvs.
func pushRemoteOpLV[C content[C]](log *opLog[C], o op[C], parents []lv) {
	agent := o.id.agent
	seq := o.id.seq

	if o.opType != opTypeDel {
		o.length = o.content.Len()
	}

	last_known_seq, ok := log.version[agent]
	if !ok {
		last_known_seq = -1
	}

	// Already hold the full seq range this op covers.
	if last_known_seq >= seq+o.length-1 {
		return
	}

	// A re-arrival must not create a gap.
	if seq > last_known_seq+1 {
		panic("Seq numbers out of order")
	}

	// Re-arrival of an op we hold as a strict prefix (the owner extended the
	// run after a prior sync). Op lv spans are immutable once applied, so we
	// never grow our existing copy in place (that would move its end LV out
	// from under branch frontiers and op ids that already reference it).
	// Instead we keep our prefix op untouched and append the not-yet-known
	// suffix as a NEW op at the log tail, so no later opLV shifts.
	if seq <= last_known_seq {
		if _, exists := log.idToLV[o.id]; !exists {
			panic("overlapping seq range without a matching op id")
		}
		offset := last_known_seq + 1 - seq
		if o.opType == opTypeDel || offset >= o.content.Len() {
			panic("inconsistent extended op prefix")
		}
		_, suffix := o.content.SplitAt(offset)
		o = op[C]{
			opType:  opTypeIns,
			content: suffix,
			length:  suffix.Len(),
			pos:     o.pos + offset,
			id:      id{agent: agent, seq: last_known_seq + 1},
			parents: []lv{log.opEndLVForSeq(agent, last_known_seq)},
		}
	} else {
		// Whole op is new to us: parents were resolved by the caller.
		o.parents = sortLVs(parents)
	}

	first := log.totalLV
	log.ops = append(log.ops, o)
	log.opLV = append(log.opLV, first)
	log.totalLV += lv(o.length)
	log.idToLV[o.id] = first + lv(o.length) - 1
	log.frontier = advanceFrontier(log.frontier, first+lv(o.length)-1, o.parents)
	log.version[agent] = o.id.seq + o.length - 1
}

// mergeInto copies src's ops into dest. Parent references are re-encoded by
// (agent, character-seq) rather than by op id: replicas may hold the same run
// content under different op boundaries (an extended run that arrived via the
// split path), so the character a parent edge points at must resolve to
// whatever run op covers that exact character in dest. Ops dest fully holds
// are skipped before any resolution, so parent resolution happens only for ops
// that will actually append; run-boundary convergence driven by already-held
// ops' references becomes lazy — a future merge that references those seqs
// splits then.
func mergeInto[C content[C]](dest *opLog[C], src *opLog[C]) {
	for _, o := range src.ops {
		// Ops dest fully holds are discarded by pushRemoteOpLV; resolving
		// their parents first is pure wasted scan (profiled 2026-09-05:
		// 91% of map-merge CPU at 50k ops, 48% of rune's). Skip them.
		// src ops always carry a set length (pushLocalOp/pushRemoteOpLV
		// normalize before append), so this matches the effective range
		// pushRemoteOpLV's skip check uses.
		if last, ok := dest.version[o.id.agent]; ok && last >= o.id.seq+o.length-1 {
			continue
		}
		parents := make([]lv, len(o.parents))
		for i, p_lv := range o.parents {
			pa := src.opAt(p_lv)
			parents[i] = dest.resolveParentLV(pa.id.agent, src.seqAt(p_lv))
		}
		pushRemoteOpLV(dest, o, parents)
	}
}

// ==========================================
// Snapshot-Anchor Compaction
// ==========================================

// isCompacted reports whether the log has been collapsed to a snapshot anchor.
func (log *opLog[C]) isCompacted() bool { return log.anchorCoverage != nil }

// cloneRemoteVersion shallow-copies a version vector (small: one entry per
// agent).
func cloneRemoteVersion(m remoteVersion) remoteVersion {
	if m == nil {
		return nil
	}
	out := make(remoteVersion, len(m))
	for agent, seq := range m {
		out[agent] = seq
	}
	return out
}

// Compact collapses the entire log into a single anchor op holding the current
// content, discarding all history including tombstones. Precondition (v1): the
// document is fully synchronized — no unconverged concurrency — validated by
// requiring a single-tip frontier. version is preserved; a coverage table
// (clone of version) lets future (agent, seq) parent references below the
// compaction point resolve to the anchor. The content snapshot is supplied by
// the document layer (the log must not duplicate checkout logic); an empty
// snapshot compacts to a zero-op log that still carries the coverage table, so
// tombstone-only documents compact too.
func (log *opLog[C]) Compact(content C) error {
	if len(log.frontier) != 1 {
		return fmt.Errorf("oplog: Compact requires a fully synchronized document (frontier has %d tips)", len(log.frontier))
	}
	for i := range log.ops {
		if log.ops[i].id.agent != anchorAgent {
			continue
		}
		if log.anchorCoverage != nil && i == 0 {
			continue // our own anchor from a previous Compact
		}
		return fmt.Errorf("oplog: Compact: agent %d is reserved for the compaction anchor", anchorAgent)
	}
	fresh := newOpLog[C]()
	if content.Len() > 0 {
		// Anchor ops must not fold into later ops and must survive idToLV
		// rebuilds; pushLocalOp already recorded idToLV[{anchorAgent, 0}].
		fresh.pushLocalOp(anchorAgent, op[C]{opType: opTypeIns, pos: 0, content: content})
		fresh.ops[0].coverage = cloneRemoteVersion(log.version)
	}
	fresh.anchorCoverage = cloneRemoteVersion(log.version)
	log.replaceWith(fresh)
	return nil
}

// replaceWith swaps in the fresh log's structural state after a rebuild.
// version is deliberately NOT copied: compaction never rewrites the version
// vector (skip-delivery depends on it), and the fresh log's own vector only
// holds the anchor sentinel that pushLocalOp recorded.
func (log *opLog[C]) replaceWith(fresh *opLog[C]) {
	log.ops = fresh.ops
	log.opLV = fresh.opLV
	log.totalLV = fresh.totalLV
	log.frontier = fresh.frontier
	log.idToLV = fresh.idToLV
	log.anchorCoverage = fresh.anchorCoverage
}

// checkCompacted validates the compacted-log invariants: the log starts with
// the anchor op carrying coverage (or, for an empty-content anchor, holds no
// ops at all), and coverage never exceeds version. Called from the document
// Check() implementations when the log is compacted.
func checkCompacted[C content[C]](log *opLog[C]) {
	if log.totalLV > 0 {
		if len(log.ops) == 0 || log.ops[0].id.agent != anchorAgent || log.ops[0].coverage == nil {
			panic("Check: compacted log must start with an anchor op carrying coverage")
		}
	} else if len(log.ops) != 0 {
		panic("Check: compacted log with zero lvs must hold no ops")
	}
	for agent, seq := range log.anchorCoverage {
		if log.version[agent] < seq {
			panic("Check: anchorCoverage exceeds version")
		}
	}
}
