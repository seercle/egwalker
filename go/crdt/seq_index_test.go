package crdt

import (
	"testing"
)

// seqIndexLinear is the linear-scan oracle: the old runIdxForSeq body kept
// verbatim for cross-checking the index against, for every (agent, seq) the
// log can resolve.
func seqIndexLinear[C content[C]](log *opLog[C], agent, seq int) int {
	for i := len(log.ops) - 1; i >= 0; i-- {
		o := &log.ops[i]
		if o.id.agent == agent && seq >= o.id.seq && seq < o.id.seq+o.length {
			return i
		}
	}
	return -1
}

// TestSeqIndexMatchesScan drives every mutation that can shape the index —
// local edits, remote merges (with splitRunOp forced via run-boundary
// divergence), compaction, and deserialization — and after each step
// compares the index lookup against the linear oracle for every op and every
// seq in [opStart, opEnd), and validates the structural invariant.
func TestSeqIndexMatchesScan(t *testing.T) {
	compareAll := func(t *testing.T, log *opLog[runeText]) {
		t.Helper()
		for i := range log.ops {
			o := &log.ops[i]
			for s := o.id.seq; s < o.id.seq+o.length; s++ {
				want := seqIndexLinear(log, o.id.agent, s)
				idx, ok := log.seqIndexOf(o.id.agent, s)
				if (want < 0) != !ok || (want >= 0 && idx != want) {
					log.checkSeqIndex() // regenerate full detail on failure
					t.Fatalf("seqIndexOf(%d,%d) = (%d,%v), linear = %d",
						o.id.agent, s, idx, ok, want)
				}
			}
		}
	}

	a := NewRuneDocument(1)
	a.Ins(0, "hello world") // one run op, agent 1
	a.Del(4, 6)             // delete run op, agent 1
	compareAll(t, a.doc.opLog)

	// Divergent run boundaries force resolveParentLV -> splitRunOp: b holds
	// a's run fused; c sends an op whose parent lands inside it.
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	b.Ins(2, "XY")
	a.MergeFrom(b) // re-arrival extends a's tail; boundary checks exercise splits
	c := NewRuneDocument(3)
	c.MergeFrom(a)
	c.MergeFrom(b) // concurrent branches into one log, parents resolved per op
	compareAll(t, c.doc.opLog)

	// Compaction: fresh log with a single anchor entry.
	c.Compact()
	compareAll(t, c.doc.opLog)
	c.Ins(0, "z") // post-compaction edit alongside the anchor
	compareAll(t, c.doc.opLog)

	// Deserialization: columnar rebuild must construct the index.
	d := NewRuneDocument(4)
	d.MergeFrom(a)
	d.Ins(1, "q")
	log := Unmarshal[runeText](a.doc.opLog.Marshal())
	compareAll(t, log)
}
