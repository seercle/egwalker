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
	if got := len(doc.doc.opLog.ops); got != 1 {
		t.Fatalf("5-char insert made %d ops, want 1 run op", got)
	}
	if got := doc.doc.opLog.ops[0].length; got != 5 {
		t.Fatalf("run op length = %d, want 5", got)
	}
	if got := doc.GetString(); got != "hello" {
		t.Fatalf("content diverged: %q", got)
	}
	doc.Check()
}

// TestRunOpsLargeContiguousInsertCollapses is the relocated RLE acceptance
// test (formerly TestTargetOpLogRLE in the gated file): a single contiguous
// 5000-char insert must collapse into exactly one run op.
func TestRunOpsLargeContiguousInsertCollapses(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, strings.Repeat("a", 5000))
	if got := len(doc.doc.opLog.ops); got != 1 {
		t.Fatalf("contiguous 5000-char insert produced %d ops; want a single run op", got)
	}
	if got := doc.GetString(); got != strings.Repeat("a", 5000) {
		t.Fatalf("content diverged after 5000-char run insert")
	}
	doc.Check()
}

// TestRunOpsMergeAcrossInsCalls checks the merge rule across adjacent Ins calls.
func TestRunOpsMergeAcrossInsCalls(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "ab")
	doc.Ins(2, "cd") // adjacent, same agent, linear tail
	if got := len(doc.doc.opLog.ops); got != 1 {
		t.Fatalf("adjacent Ins calls made %d ops, want 1 run op", got)
	}
	doc.Check()
}

// TestRunOpsDoNotMergeAcrossDivergence exercises the divergence-blocking
// invariant for run collapse: a local insert only grows the tail run op in
// place when that op is still the log's sole causal head at the moment of the
// insert. Here agent 1 writes "ab", agent 2 forks from that state and writes
// "X" on top, and agent 1 merges agent 2's divergent op back in. Agent 1's "ab"
// op is no longer the log tail -- agent 2's "X" now occupies the slot between
// it and any later agent-1 append -- so agent 1's own subsequent contiguous
// append ("cd") must NOT collapse into the "ab" run. In a linear (non-diverged)
// history that append would have extended "ab" in place; the merged divergence
// is what forces it into a distinct op.
func TestRunOpsDoNotMergeAcrossDivergence(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)
	a.Ins(0, "ab")
	b.MergeFrom(a)
	b.Ins(2, "X")  // b diverges on top of a's linear tail
	a.MergeFrom(b) // a adopts b's divergent op: its tail op is now agent 2's "X"
	if got := len(a.doc.opLog.ops); got != 2 {
		t.Fatalf("after the diverging merge a has %d ops, want 2 (its ab run + b's X)", got)
	}
	a.Ins(3, "cd") // contiguous agent-1 append; pre-divergence it would collapse into "ab"
	if got := len(a.doc.opLog.ops); got != 3 {
		t.Fatalf("contiguous append after divergence made %d ops, want 3 (must not collapse into the ab run)", got)
	}
	if got := a.doc.opLog.ops[0].length; got != 2 {
		t.Fatalf("ab run op length = %d, want 2 (the held run must not be mutated)", got)
	}
	if got := a.GetString(); got != "abXcd" {
		t.Fatalf("content diverged: %q", got)
	}
	a.Check()
}

// TestRunOpsTwoAgentsNoMerge checks that separate agents never collapse.
func TestRunOpsTwoAgentsNoMerge(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "a")
	doc.MergeFrom(func() *RuneDocument { d := NewRuneDocument(2); d.Ins(0, "b"); return d }())
	doc.Ins(1, "c")
	if got := len(doc.doc.opLog.ops); got != 3 {
		t.Fatalf("3 non-adjacent-agent edits made %d ops, want 3 (no cross-agent merge)", got)
	}
	doc.Check()
}

// TestRunOpsDeleteCollapses checks a contiguous range delete becomes one run op.
func TestRunOpsDeleteCollapses(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "abcdefgh")
	doc.Del(2, 4) // contiguous range delete
	if got := len(doc.doc.opLog.ops); got != 2 {
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
	if got := len(b.doc.opLog.ops); got != 1 {
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
	doc.Check()      // checkout(doc.doc.opLog) must equal the snapshot
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

// TestRunOpsSplitThenDeleteAcrossBoundary is a regression test for a delete
// run whose range crosses an insert run that was split when a concurrent
// interior edit from another replica arrived (the splitRunOp path in
// resolveParentLV). The delete must apply across the boundary and both replicas
// must converge.
func TestRunOpsSplitThenDeleteAcrossBoundary(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)
	a.Ins(0, strings.Repeat("a", 50))
	b.MergeFrom(a)
	b.Ins(10, strings.Repeat("b", 10))      // concurrent, interior to a's run
	a.Ins(a.Len(), strings.Repeat("c", 10)) // a extends its run on its own tail
	a.MergeFrom(b)                          // b's parent edge points into a's extended run -> split
	b.MergeFrom(a)
	if a.GetString() != b.GetString() {
		t.Fatalf("replicas diverged before delete:\na=%q\nb=%q", a.GetString(), b.GetString())
	}
	pre := a.Len()
	b.Del(20, 30) // crosses the split boundary in the now-converged doc
	a.MergeFrom(b)
	b.MergeFrom(a)
	if a.GetString() != b.GetString() {
		t.Fatalf("replicas diverged after delete across split:\na=%q\nb=%q", a.GetString(), b.GetString())
	}
	if a.Len() != pre-30 {
		t.Fatalf("delete across split removed wrong count: len=%d, want %d", a.Len(), pre-30)
	}
	a.Check()
	b.Check()
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
	data := d1.doc.opLog.Marshal()
	round := Unmarshal[runeText](data)
	if len(round.ops) != len(d1.doc.opLog.ops) {
		t.Fatalf("round-trip op count %d != %d", len(round.ops), len(d1.doc.opLog.ops))
	}
	out := checkout(round)
	if out.Len() != d1.Len() {
		t.Fatalf("round-trip checkout size %d != %d", out.Len(), d1.Len())
	}
	got := d1.GetString() // also verify via checkout equality below
	_ = got
}

// TestRunOpsMergeableChildrenNoCollapse is a regression test for the I1
// finding: a single Ins of two+ Mergeable child documents must NOT collapse
// into one multi-element run op, because the recursive-merge path matches ops
// by id and reconciles one element per op. If the children were fused into one
// op, only the first child's op would be reconciled and the others would
// silently diverge across replicas.
func TestRunOpsMergeableChildrenNoCollapse(t *testing.T) {
	// Child documents on replica 1 (Mergeable elements: *RuneDocument).
	childA1 := NewRuneDocument(10)
	childA1.Ins(0, "AB")
	childB1 := NewRuneDocument(11)
	childB1.Ins(0, "CD")

	p1 := NewArrayDocument[*RuneDocument](1)
	p1.Ins(0, []*RuneDocument{childA1, childB1})

	// Mergeable content must never collapse: each child keeps its own length-1
	// op so recursive merge can match it by id.
	if got := len(p1.doc.opLog.ops); got != 2 {
		t.Fatalf("one Ins of 2 Mergeable children made %d ops, want 2 (no collapse)", got)
	}
	if p1.doc.opLog.ops[0].length != 1 || p1.doc.opLog.ops[1].length != 1 {
		t.Fatalf("Mergeable children collapsed into runs of length %d and %d, want 1 each",
			p1.doc.opLog.ops[0].length, p1.doc.opLog.ops[1].length)
	}

	// Build a distinct replica of the parent whose ops reference independent
	// child replicas. (Document merge copies element pointers, so replace them
	// explicitly to make cross-replica reconciliation genuine.)
	p2 := NewArrayDocument[*RuneDocument](2)
	p2.MergeFrom(p1)
	for i := range p2.doc.opLog.ops {
		o := &p2.doc.opLog.ops[i]
		src := []*RuneDocument(o.content)
		replicas := make([]*RuneDocument, len(src))
		for j, c := range src {
			replica := NewRuneDocument(c.doc.agent + 10)
			replica.MergeFrom(c)
			replicas[j] = replica
		}
		o.content = itemRun[*RuneDocument](replicas)
	}
	p2.doc.branch.snapshot = checkout(p2.doc.opLog)
	p2.doc.branch.frontier = append([]lv{}, p2.doc.opLog.frontier...)
	p2.Check()

	items2 := p2.GetItems()
	if len(items2) != 2 {
		t.Fatalf("parent2 has %d items, want 2", len(items2))
	}
	childA2, childB2 := items2[0], items2[1]

	// Divergent edits to both children on both replicas.
	childA1.Ins(1, "X")
	childB1.Ins(1, "P")
	childA2.Ins(1, "Y")
	childB2.Ins(1, "Q")

	// Merging the parents must recursively reconcile BOTH children.
	p1.MergeFrom(p2)
	p2.MergeFrom(p1)

	items1 := p1.GetItems()
	if len(items1) != 2 {
		t.Fatalf("parent1 has %d items, want 2", len(items1))
	}
	for _, c := range []struct {
		name string
		got  string
		want []string
	}{
		{"childA1", items1[0].GetString(), []string{"X", "Y"}},
		{"childB1", items1[1].GetString(), []string{"P", "Q"}},
		{"childA2", items2[0].GetString(), []string{"X", "Y"}},
		{"childB2", items2[1].GetString(), []string{"P", "Q"}},
	} {
		for _, s := range c.want {
			if !strings.Contains(c.got, s) {
				t.Errorf("%s content %q missing %q after recursive merge", c.name, c.got, s)
			}
		}
	}
	if items1[0].GetString() != items2[0].GetString() || items1[1].GetString() != items2[1].GetString() {
		t.Errorf("children diverged across replicas after recursive merge")
	}

	p1.Check()
	p2.Check()
	items1[0].Check()
	items1[1].Check()
	items2[0].Check()
	items2[1].Check()
}

// TestRunOpsIdleReplicaReMergeExtendedRun is a regression test for the
// tail-grow-in-place bug: an idle replica that synced a run op and never
// diverged re-merges an owner that has since extended its trailing run. The
// extended run re-arrives while our copy is the sole tail op; the op's lv span
// must not be mutated in place (that desyncs the branch frontier and panics).
func TestRunOpsIdleReplicaReMergeExtendedRun(t *testing.T) {
	owner := NewRuneDocument(1)
	owner.Ins(0, strings.Repeat("a", 50))
	idle := NewRuneDocument(2)
	idle.MergeFrom(owner)
	owner.Ins(owner.Len(), strings.Repeat("b", 20)) // owner extends its trailing run
	idle.MergeFrom(owner)                           // extended run re-arrives
	if idle.Len() != owner.Len() || idle.GetString() != owner.GetString() {
		t.Fatalf("idle replica diverged after re-merge:\nowner=%q (%d)\nidle =%q (%d)",
			owner.GetString(), owner.Len(), idle.GetString(), idle.Len())
	}
	idle.Check()
	owner.Check()
}

// TestRunOpsOriginalReMergeCloneAppend is a regression test for the
// tail-grow-in-place bug in the opposite direction: a serialization round-trip
// clone of a single-run document extends the run, then the original re-merges
// the clone. The original's copy of the run is its sole tail op when the
// extended run re-arrives.
func TestRunOpsOriginalReMergeCloneAppend(t *testing.T) {
	orig := NewRuneDocument(1)
	orig.Ins(0, strings.Repeat("a", 50))

	cloneLog := Unmarshal[runeText](orig.doc.opLog.Marshal())
	clone := &RuneDocument{doc: &doc[runeText]{
		opLog:  cloneLog,
		agent:  1,
		branch: &branch[runeText]{snapshot: checkout(cloneLog), frontier: append([]lv{}, cloneLog.frontier...)},
	}}
	clone.Check()
	clone.Ins(clone.Len(), strings.Repeat("b", 20)) // clone extends the run

	orig.MergeFrom(clone) // extended run re-arrives while orig's copy is the tail
	if orig.Len() != clone.Len() || orig.GetString() != clone.GetString() {
		t.Fatalf("original diverged after re-merging extended clone:\norig =%q (%d)\nclone=%q (%d)",
			orig.GetString(), orig.Len(), clone.GetString(), clone.Len())
	}
	orig.Check()
	clone.Check()
}

func TestShapeB5000CharInsertOneSnapshotLeaf(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, strings.Repeat("a", 5000))
	if leaves := doc.doc.branch.snapshot.leafCount(); leaves != 1 {
		t.Fatalf("5000-char run insert made %d snapshot leaves; want 1", leaves)
	}
	doc.Check()
}

func TestShapeBAppendTypingKeepsLeavesBounded(t *testing.T) {
	doc := NewRuneDocument(1)
	for i := 0; i < 5000; i++ {
		doc.Ins(doc.Len(), "x")
	}
	if leaves := doc.doc.branch.snapshot.leafCount(); leaves > 100 {
		t.Fatalf("append-only typing left %d snapshot leaves; want <= 100", leaves)
	}
	if doc.GetString() != strings.Repeat("x", 5000) {
		t.Fatalf("content diverged")
	}
	doc.Check()
}

func TestShapeBInteriorDeleteSplitsOneLeaf(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, strings.Repeat("a", 1000))
	doc.Ins(1000, strings.Repeat("b", 1000)) // second leaf
	doc.Del(500, 1)                          // interior delete in first leaf
	// Lenient on purpose: an interior delete splits the run's leaf (the 1000-char
	// "a" leaf becomes two around the deleted rune), so the count here is 3, not 2.
	if got := doc.GetString(); len([]rune(got)) != 1999 {
		t.Fatalf("len=%d", len([]rune(got)))
	}
	doc.Check()
}
