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

// TestTargetCriticalVersionCompaction expects the missing critical-version
// truncation optimization (CONTEXT.md, Missing / Section 3.5). Once two
// replicas have fully synchronized (both hold the entire history and share a
// frontier), history below the frontier is acked and a compaction can discard
// every non-critical op. Today the opLog grows without bound and keeps all ops.
//
// The original end-append workload was vacated by run-length coalescing: it
// collapsed to a single run op that already matched the retention target, so
// the test passed without any compaction existing. The strided workload below
// produces ~total non-collapsing ops, so the test fails again until
// critical-version truncation (Section 3.5) lands.
func TestTargetCriticalVersionCompaction(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)

	const total = 5000
	for i := 0; i < total; i++ {
		a.Ins((i*7919)%(a.Len()+1), "x")
	}
	b.MergeFrom(a)
	a.MergeFrom(b) // full sync: identical logs, identical frontier

	keep := targetRetentionSet(a.doc.opLog)
	if got, want := len(a.doc.opLog.ops), len(keep); got != want {
		t.Errorf("critical-version truncation not implemented: opLog retains %d ops after full sync, compaction target is %d (frontier + critical versions)", got, want)
	}
}
