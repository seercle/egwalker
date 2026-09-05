package crdt

import (
	"strings"
	"testing"
)

// TestMergeLazyBoundaryConvergence pins that content convergence does not
// depend on eager run-boundary splitting: src holds the same edits as dest
// but with a finer op boundary; after merging (and the fix skipping the
// already-held ops' parent resolution), a third replica's merge must still
// converge to the same content.
//
// b's boundary is forced finer by building the target string one prepend at a
// time in reverse order: every insert lands at pos 0 while the tail run op
// already starts at pos 0, so pushLocalOp's fold condition
// (o.pos == tail.pos + tail.length) never holds and no ops collapse — unlike
// per-char appends, which fold into a single run op.
func TestMergeLazyBoundaryConvergence(t *testing.T) {
	s := strings.Repeat("abcdef", 30) // 180 chars

	// a: one contiguous append run (coarse boundary, folded by pushLocalOp).
	a := NewRuneDocument(1)
	a.Ins(0, s) // 180 chars in as few ops as folding allows

	// b: same final content, built per-char as reverse-order prepends so its
	// log holds the same seqs under a strictly finer boundary.
	b := NewRuneDocument(2)
	for i := len(s) - 1; i >= 0; i-- {
		b.Ins(0, string(s[i]))
	}

	// Precondition self-checks: the construction must actually produce the
	// boundary mismatch this test exists to exercise. A weak construction
	// must defeat itself, not pass silently.
	if got := b.GetString(); got != a.GetString() || got != s {
		t.Fatalf("precondition unmet: b's build does not reproduce a's content: a=%q b=%q", a.GetString(), got)
	}
	if na, nb := len(a.doc.opLog.ops), len(b.doc.opLog.ops); nb <= na {
		t.Fatalf("precondition unmet: b's op boundary is not finer than a's: b=%d ops, a=%d ops", nb, na)
	}

	b.MergeFrom(a) // shared ancestor: b now holds a's ops AND its own finer view

	// c: independent replica that only ever sees b's log.
	c := NewRuneDocument(3)
	c.MergeFrom(b)

	// a pulls everything; all three must agree on content.
	a.MergeFrom(b)
	if a.GetString() != b.GetString() || b.GetString() != c.GetString() {
		t.Fatalf("content divergence: a=%q b=%q c=%q",
			a.GetString(), b.GetString(), c.GetString())
	}
	a.Check()
	b.Check()
	c.Check()
}
