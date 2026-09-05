//go:build optimization_targets

package crdt

import (
	"testing"
)

// targetRetentionSet computes the ops a critical-version compaction would keep:
// every live frontier op plus every op with >= 2 children (a branch point).
func targetRetentionSet(log *opLog[runeText]) map[lv]bool {
	keep := make(map[lv]bool)
	for _, f := range log.frontier {
		keep[f] = true
	}
	childCount := make(map[lv]int)
	for _, o := range log.ops {
		for _, p := range o.parents {
			childCount[p]++
		}
	}
	for p, c := range childCount {
		if c >= 2 {
			keep[p] = true
		}
	}
	return keep
}

// TestTargetCriticalVersionCompaction pins the retention target of
// critical-version compaction (CONTEXT.md, Section 3.5 — implemented
// 2026-09-05 as snapshot-anchor compaction). Once two replicas have fully
// synchronized (both hold the entire history and share a frontier), history
// below the frontier is acked and compaction may discard every non-critical
// op: what must remain is the retention set computed below (every live
// frontier op plus every op with >= 2 children — for this linear history,
// the single anchor op).
//
// Compaction is an explicit API (design decision 2026-09-05): this test
// invokes it and asserts the retention target is met. Automatic
// watermark-driven compaction remains a future wrapper (see TODO).
//
// The original end-append workload was vacated by run-length coalescing: it
// collapsed to a single run op that already matched the retention target. The
// strided workload below produces ~total non-collapsing ops, so the assertion
// is meaningful: without compaction the log retains ~total ops.
func TestTargetCriticalVersionCompaction(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)

	const total = 5000
	for i := 0; i < total; i++ {
		a.Ins((i*7919)%(a.Len()+1), "x")
	}
	b.MergeFrom(a)
	a.MergeFrom(b) // full sync: identical logs, identical frontier

	a.Compact() // explicit API (design decision 2026-09-05): discard the acked history

	keep := targetRetentionSet(a.doc.opLog)
	if got, want := len(a.doc.opLog.ops), len(keep); got != want {
		t.Errorf("critical-version truncation not implemented: opLog retains %d ops after full sync, compaction target is %d (frontier + critical versions)", got, want)
	}
}
