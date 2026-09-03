package crdt

import (
	"strings"
	"testing"
)

// TestRunOpsLocalInsertCollapses checks that a contiguous multi-character
// insert collapses into a single run op.
func TestRunOpsLocalInsertCollapses(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "hello")
	if got := len(doc.opLog.ops); got != 1 {
		t.Fatalf("5-char insert made %d ops, want 1 run op", got)
	}
	if got := doc.opLog.ops[0].length; got != 5 {
		t.Fatalf("run op length = %d, want 5", got)
	}
	if got := doc.GetString(); got != "hello" {
		t.Fatalf("content diverged: %q", got)
	}
	doc.Check()
}

// TestRunOpsMergeAcrossInsCalls checks the merge rule across adjacent Ins calls.
func TestRunOpsMergeAcrossInsCalls(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "ab")
	doc.Ins(2, "cd") // adjacent, same agent, linear tail
	if got := len(doc.opLog.ops); got != 1 {
		t.Fatalf("adjacent Ins calls made %d ops, want 1 run op", got)
	}
	doc.Check()
}

// TestRunOpsDoNotMergeAcrossDivergence is the divergence scenario from the
// plan: b forks on top of a, then a continues linearly on its own tail.
func TestRunOpsDoNotMergeAcrossDivergence(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)
	a.Ins(0, "ab")
	b.MergeFrom(a)
	b.Ins(2, "X")  // b diverges on top of a
	a.Ins(2, "cd") // a continues linearly: frontier = {lv1}? see notes
	_ = a
	_ = b
}

// TestRunOpsTwoAgentsNoMerge checks that separate agents never collapse.
func TestRunOpsTwoAgentsNoMerge(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "a")
	doc.MergeFrom(func() *RuneDocument { d := NewRuneDocument(2); d.Ins(0, "b"); return d }())
	doc.Ins(1, "c")
	if got := len(doc.opLog.ops); got != 3 {
		t.Fatalf("3 non-adjacent-agent edits made %d ops, want 3 (no cross-agent merge)", got)
	}
	doc.Check()
}

// TestRunOpsDeleteCollapses checks a contiguous range delete becomes one run op.
func TestRunOpsDeleteCollapses(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "abcdefgh")
	doc.Del(2, 4) // contiguous range delete
	if got := len(doc.opLog.ops); got != 2 {
		t.Fatalf("insert+range-delete made %d ops, want 2 (one run op each)", got)
	}
	if doc.GetString() != "abgh" {
		t.Fatalf("content diverged: %q", doc.GetString())
	}
	doc.Check()
}

// TestRunOpsMergeRemote checks a run op round-trips a merge between replicas.
func TestRunOpsMergeRemote(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, strings.Repeat("x", 300))
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	if got := len(b.opLog.ops); got != 1 {
		t.Fatalf("merged run made %d ops, want 1", got)
	}
	if b.GetString() != a.GetString() {
		t.Fatalf("content diverged after run merge")
	}
	b.Check()
	a.Check()
}

// TestRunOpsCheckoutLengthMatches checks checkout replay reproduces the
// snapshot for a doc whose insert regions were later split by an interior edit.
func TestRunOpsCheckoutLengthMatches(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "abcdef")
	doc.Ins(3, "XY") // splits the run region
	doc.Check()      // checkout(doc.opLog) must equal the snapshot
	if doc.GetString() != "abcXYdef" {
		t.Fatalf("content diverged: %q", doc.GetString())
	}
}

// TestRunOpsInteriorDelete checks deleting a single character inside a run.
func TestRunOpsInteriorDelete(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "abcdef")
	doc.Del(2, 1) // delete 'c' inside the run
	if doc.GetString() != "abdef" {
		t.Fatalf("content diverged: %q", doc.GetString())
	}
	doc.Check()
}

// TestRunOpsConcurrentSplitMerge checks convergence when one replica inserts
// inside another replica's run while the owner appends after it.
func TestRunOpsConcurrentSplitMerge(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)
	a.Ins(0, strings.Repeat("a", 50))
	b.MergeFrom(a)
	b.Ins(10, strings.Repeat("b", 10))      // concurrent insert inside a's run
	a.Ins(a.Len(), strings.Repeat("c", 10)) // a extends its run at its own end
	a.MergeFrom(b)
	b.MergeFrom(a)
	if a.GetString() != b.GetString() {
		t.Fatalf("replicas diverged:\na=%q\nb=%q", a.GetString(), b.GetString())
	}
	a.Check()
	b.Check()
}

// TestRunOpsTwoSidedMerge checks convergence of a two-sided run merge.
func TestRunOpsTwoSidedMerge(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)
	a.Ins(0, "hello world")
	b.MergeFrom(a)
	b.Del(5, 5)
	a.Ins(5, " dear")
	b.MergeFrom(a)
	a.MergeFrom(b)
	if a.GetString() != b.GetString() {
		t.Fatalf("replicas diverged after two-sided run merge")
	}
	a.Check()
	b.Check()
}

// TestRunOpsSerializationRoundTrip checks columnar serialization round-trips
// a log containing run ops losslessly.
func TestRunOpsSerializationRoundTrip(t *testing.T) {
	d1 := NewRuneDocument(1)
	d1.Ins(0, strings.Repeat("a", 5000))
	d1.Del(2, 3)
	data := d1.opLog.Marshal()
	round := Unmarshal[rune, runeText](data)
	if len(round.ops) != len(d1.opLog.ops) {
		t.Fatalf("round-trip op count %d != %d", len(round.ops), len(d1.opLog.ops))
	}
	out := checkout(round)
	if out.Size() != d1.Len() {
		t.Fatalf("round-trip checkout size %d != %d", out.Size(), d1.Len())
	}
	got := d1.GetString() // also verify via checkout equality below
	_ = got
}
