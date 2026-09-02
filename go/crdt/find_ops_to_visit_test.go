package crdt

import (
	"slices"
	"testing"
)

// buildForkLog builds a log with lv0 = 'a', and two concurrent children
// lv1 = 'b' (agent 2) and lv2 = 'c' (agent 1) of lv0.
func buildForkLog(t *testing.T) *opLog[rune] {
	t.Helper()
	log := newOpLog[rune]()
	log.pushLocalOp(1, op[rune]{opType: opTypeIns, content: 'a', pos: 0})
	pushRemoteOp(log, op[rune]{id: id{agent: 2, seq: 0}, opType: opTypeIns, content: 'b', pos: 1}, []id{{agent: 1, seq: 0}})
	pushRemoteOp(log, op[rune]{id: id{agent: 1, seq: 1}, opType: opTypeIns, content: 'c', pos: 2}, []id{{agent: 1, seq: 0}})
	if len(log.ops) != 3 {
		t.Fatalf("setup: want 3 ops, got %d", len(log.ops))
	}
	return log
}

// assertTopoOrder verifies each op's parents appear in commonVersion or earlier
// in the same list (parents precede children).
func assertTopoOrder(t *testing.T, log *opLog[rune], common []lv, ops []lv) {
	t.Helper()
	commonSet := make(map[lv]bool, len(common))
	for _, c := range common {
		commonSet[c] = true
	}
	seen := make(map[lv]bool)
	for _, cur := range ops {
		for _, p := range log.ops[cur].parents {
			if commonSet[p] || seen[p] {
				continue
			}
			t.Errorf("op %d: parent %d not in commonVersion or earlier in list", cur, p)
		}
		seen[cur] = true
	}
}

func TestFindOpsToVisit_DivergentSiblings(t *testing.T) {
	log := buildForkLog(t)
	// Move from frontier {lv2} to frontier {lv1}: CCA is lv0; lv2 is on the
	// current side (shared), lv1 is the remote-only op to advance.
	res := findOpsToVisit(log, []lv{2}, []lv{1})
	if !slices.Equal(res.commonVersion, []lv{0}) {
		t.Errorf("commonVersion = %v, want [0]", res.commonVersion)
	}
	if !slices.Equal(res.sharedOps, []lv{2}) {
		t.Errorf("sharedOps = %v, want [2]", res.sharedOps)
	}
	if !slices.Equal(res.bOnlyOps, []lv{1}) {
		t.Errorf("bOnlyOps = %v, want [1]", res.bOnlyOps)
	}
	assertTopoOrder(t, log, res.commonVersion, res.sharedOps)
	assertTopoOrder(t, log, res.commonVersion, res.bOnlyOps)
}

func TestFindOpsToVisit_EmptyStartFirstMerge(t *testing.T) {
	// Linear history 0,1,2 from a single agent; merging from an empty branch.
	log := newOpLog[rune]()
	log.pushLocalOp(1, op[rune]{opType: opTypeIns, content: 'a', pos: 0})
	log.pushLocalOp(1, op[rune]{opType: opTypeIns, content: 'b', pos: 1})
	log.pushLocalOp(1, op[rune]{opType: opTypeIns, content: 'c', pos: 2})

	res := findOpsToVisit(log, []lv{}, []lv{2})
	if len(res.commonVersion) != 0 {
		t.Errorf("commonVersion = %v, want empty", res.commonVersion)
	}
	if len(res.sharedOps) != 0 {
		t.Errorf("sharedOps = %v, want empty", res.sharedOps)
	}
	got := slices.Clone(res.bOnlyOps)
	slices.Sort(got)
	if !slices.Equal(got, []lv{0, 1, 2}) {
		t.Errorf("bOnlyOps sorted = %v, want [0 1 2]", got)
	}
	assertTopoOrder(t, log, res.commonVersion, res.bOnlyOps)
}

func TestFindOpsToVisit_AOnlyOp(t *testing.T) {
	log := buildForkLog(t)
	// a={lv1} has a local-only op; b={lv2} is the merge target.
	res := findOpsToVisit(log, []lv{1}, []lv{2})
	if !slices.Equal(res.commonVersion, []lv{0}) {
		t.Errorf("commonVersion = %v, want [0]", res.commonVersion)
	}
	if !slices.Equal(res.sharedOps, []lv{1}) {
		t.Errorf("sharedOps = %v, want [1]", res.sharedOps)
	}
	if !slices.Equal(res.bOnlyOps, []lv{2}) {
		t.Errorf("bOnlyOps = %v, want [2]", res.bOnlyOps)
	}
}
