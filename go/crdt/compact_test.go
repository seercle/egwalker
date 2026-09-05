package crdt

import (
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestCompactFullSync compacts a fully-synchronized rune document and pins
// the core contract: one anchor op, content and version preserved, coverage
// equal to the pre-compaction version, Check() green.
func TestCompactFullSync(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "hello world")
	a.Del(5, 6) // "hello" — tombstones must be physically dropped
	before := a.GetString()
	beforeLen := a.Len()
	a.Check()

	want := map[int]int{}
	for agent, seq := range a.doc.opLog.version {
		want[agent] = seq
	}

	a.Compact()

	log := a.doc.opLog
	if len(log.ops) != 1 {
		t.Fatalf("after Compact: %d ops, want 1 anchor", len(log.ops))
	}
	anchor := &log.ops[0]
	if anchor.id.agent != anchorAgent {
		t.Errorf("anchor agent = %d, want %d", anchor.id.agent, anchorAgent)
	}
	if anchor.coverage == nil {
		t.Fatal("anchor.coverage is nil")
	}
	if len(anchor.coverage) != len(want) {
		t.Errorf("anchor.coverage has %d agents, want %d", len(anchor.coverage), len(want))
	}
	for agent, seq := range want {
		if got := anchor.coverage[agent]; got != seq {
			t.Errorf("anchor.coverage[%d] = %d, want %d (pre-compaction version)", agent, got, seq)
		}
	}
	if got := a.GetString(); got != before {
		t.Errorf("content changed: %q -> %q", before, got)
	}
	if a.Len() != beforeLen {
		t.Errorf("Len changed: %d -> %d", beforeLen, a.Len())
	}
	if a.doc.opLog.anchorCoverage == nil {
		t.Error("opLog.anchorCoverage not set")
	}
	a.Check()
}

// TestCompactIdempotent pins that compacting a compacted document is a no-op.
func TestCompactIdempotent(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "abc")
	a.Compact()
	first := a.GetString()
	a.Compact()
	if a.GetString() != first {
		t.Errorf("second Compact changed content: %q", a.GetString())
	}
	if len(a.doc.opLog.ops) != 1 {
		t.Errorf("second Compact changed op count: %d", len(a.doc.opLog.ops))
	}
	a.Check()
}

// TestCompactThenLocalEdits pins that local editing continues normally on the
// compacted log (fresh lv space, frontier = anchor end).
func TestCompactThenLocalEdits(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "abc")
	a.Compact()
	a.Ins(3, "def")
	a.Ins(0, "x")
	a.Del(1, 3)
	if got, want := a.GetString(), "xdef"; got != want {
		t.Errorf("GetString() = %q, want %q", got, want)
	}
	a.Check()
}

// TestCompactTombstoneOnly pins the empty-content anchor path: a document
// whose content was fully deleted compacts to a zero-op log (tombstones
// physically dropped) that still carries the coverage table, and local
// editing continues normally from the empty state.
func TestCompactTombstoneOnly(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "abc")
	a.Del(0, 3)
	a.Compact()

	if len(a.doc.opLog.ops) != 0 {
		t.Fatalf("after Compact: %d ops, want 0 (empty-content anchor)", len(a.doc.opLog.ops))
	}
	if a.doc.opLog.anchorCoverage == nil {
		t.Error("opLog.anchorCoverage not set")
	}
	if got := a.GetString(); got != "" {
		t.Errorf("GetString() = %q, want empty", got)
	}
	a.Check()

	a.Ins(0, "xyz")
	if got := a.GetString(); got != "xyz" {
		t.Errorf("GetString() = %q, want %q after post-compaction Ins", got, "xyz")
	}
	a.Check()
}

// TestCompactPreservesVersion pins the invariant that version is untouched by
// compaction (skip-delivery depends on it).
func TestCompactPreservesVersion(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "abc")
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	a.MergeFrom(b)
	before := map[int]int{}
	for k, v := range a.doc.opLog.version {
		before[k] = v
	}
	a.Compact()
	for k, v := range before {
		if got := a.doc.opLog.version[k]; got != v {
			t.Errorf("version[%d]: %d -> %d", k, v, got)
		}
	}
	if len(a.doc.opLog.version) != len(before) {
		t.Errorf("version gained/lost agents: %v -> %v", before, a.doc.opLog.version)
	}
	a.Check()
}

// TestCompactArray pins the same contract for the generic array document:
// content preserved, one anchor op, local editing continues.
func TestCompactArray(t *testing.T) {
	a := NewArrayDocument[int](1)
	a.Ins(0, []int{1, 2, 3})
	a.Del(0, 1)
	before := a.GetItems()
	a.Check()

	a.Compact()

	if len(a.doc.opLog.ops) != 1 {
		t.Fatalf("after Compact: %d ops, want 1 anchor", len(a.doc.opLog.ops))
	}
	if got := a.GetItems(); !reflect.DeepEqual(got, before) {
		t.Errorf("items changed: %v -> %v", before, got)
	}
	a.Check()

	a.Ins(0, []int{9})
	if got, want := a.GetItems(), []int{9, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Errorf("GetItems() = %v, want %v", got, want)
	}
	a.Check()
}

// TestCompactMapMergeableValues pins that MapDocument compaction preserves
// the live bindings — including Mergeable values, which are carried by
// reference — and that a compacted map merges a fully-synchronized peer
// cleanly: pre-critical ops are skipped, so the recursion pass must not try
// to resolve their (no longer existing) ids in the compacted log.
func TestCompactMapMergeableValues(t *testing.T) {
	m1 := NewMapDocument[string, *RuneDocument](1)
	childA := NewRuneDocument(10)
	childA.Ins(0, "A1")
	childB := NewRuneDocument(11)
	childB.Ins(0, "B1")
	m1.Set("a", childA)
	m1.Set("b", childB)

	m2 := NewMapDocument[string, *RuneDocument](2)
	m2.MergeFrom(m1)

	m1.Compact()

	if len(m1.opLog.ops) != 1 {
		t.Fatalf("after Compact: %d ops, want 1 anchor", len(m1.opLog.ops))
	}
	if got, want := m1.opLog.ops[0].content.Len(), 2; got != want {
		t.Errorf("anchor holds %d bindings, want %d", got, want)
	}
	if v, ok := m1.Get("a"); !ok || v != childA {
		t.Errorf("Get(%q) = (_, %v), want pointer-identical childA after compaction", "a", ok)
	}
	if v, ok := m1.Get("b"); !ok || v != childB {
		t.Errorf("Get(%q) = (_, %v), want pointer-identical childB after compaction", "b", ok)
	}
	if got, _ := m1.Get("a"); got.GetString() != "A1" {
		t.Errorf("Get(%q).GetString() = %q, want %q", "a", got.GetString(), "A1")
	}
	if got, _ := m1.Get("b"); got.GetString() != "B1" {
		t.Errorf("Get(%q).GetString() = %q, want %q", "b", got.GetString(), "B1")
	}
	if keys := m1.Keys(); len(keys) != 2 {
		t.Errorf("Keys() = %v, want 2 keys", keys)
	}

	// Merging the synced peer is a clean no-op (everything is pre-critical).
	m1.MergeFrom(m2)
	if got, _ := m1.Get("a"); got.GetString() != "A1" {
		t.Errorf("Get(%q).GetString() = %q after covered merge, want %q", "a", got.GetString(), "A1")
	}
	if got, _ := m1.Get("b"); got.GetString() != "B1" {
		t.Errorf("Get(%q).GetString() = %q after covered merge, want %q", "b", got.GetString(), "B1")
	}

	// Local editing continues on the compacted map.
	childC := NewRuneDocument(12)
	childC.Ins(0, "C1")
	m1.Set("c", childC)
	if got, ok := m1.Get("c"); !ok || got.GetString() != "C1" {
		t.Errorf("Get(%q) = (%q, %v) after post-compaction Set", "c", got.GetString(), ok)
	}
	if got, _ := m1.Get("a"); got.GetString() != "A1" {
		t.Errorf("Get(%q).GetString() = %q after post-compaction Set, want %q", "a", got.GetString(), "A1")
	}
}

// TestMergeIntoCompactedFromFullState pins the §2 case: a compacted replica
// merging a non-compacted peer that has made NEW edits since the compaction
// point. The peer's new ops parent into its own pre-critical frontier; those
// parents must resolve to the anchor via coverage.
func TestMergeIntoCompactedFromFullState(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "hello")
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	a.MergeFrom(b)
	a.Compact() // a: [anchor], coverage {1:4, 2:4}

	b.Ins(5, " world") // b's new op parents into b's pre-critical frontier
	a.MergeFrom(b)
	if got, want := a.GetString(), "hello world"; got != want {
		t.Errorf("GetString() = %q, want %q", got, want)
	}
	a.Check()

	// Reverse direction: b (full history) merges a (anchor) — a has nothing
	// new, so b must converge without corruption.
	b.MergeFrom(a)
	if got, want := b.GetString(), "hello world"; got != want {
		t.Errorf("reverse merge: GetString() = %q, want %q", got, want)
	}
	b.Check()
}

// TestMergeCompactedPeers pins two compacted replicas exchanging new edits.
func TestMergeCompactedPeers(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)
	a.Ins(0, "hi")
	b.MergeFrom(a)
	a.MergeFrom(b)
	a.Compact()
	b.Compact()
	a.Ins(2, "!")
	b.Ins(0, ">")
	a.MergeFrom(b)
	b.MergeFrom(a)
	if a.GetString() != b.GetString() {
		t.Errorf("divergence: a=%q b=%q", a.GetString(), b.GetString())
	}
	a.Check()
	b.Check()
}

// TestFreshReplicaAdoptsAnchor pins bootstrap: an empty replica receives a
// compacted peer's anchor and ends up with the content AND the coverage table,
// so subsequent re-delivery of pre-critical ops is skipped.
func TestFreshReplicaAdoptsAnchor(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "seed")
	a.Compact()
	fresh := NewRuneDocument(3)
	fresh.MergeFrom(a)
	if got, want := fresh.GetString(), "seed"; got != want {
		t.Errorf("GetString() = %q, want %q", got, want)
	}
	if !fresh.doc.opLog.isCompacted() {
		t.Error("fresh replica did not adopt anchorCoverage")
	}
	// Pre-critical re-delivery must be skipped (version covers it): merging a
	// full-history replica must not duplicate content.
	full := NewRuneDocument(1)
	full.Ins(0, "seed")
	fresh.MergeFrom(full)
	if got, want := fresh.GetString(), "seed"; got != want {
		t.Errorf("after re-delivery: GetString() = %q, want %q", got, want)
	}
	// The anchor sentinel must never survive adoption in version (it would
	// leak agent -1 into skip-delivery, seq continuation, and serialization).
	if _, ok := fresh.doc.opLog.version[anchorAgent]; ok {
		t.Error("version carries the anchor sentinel after adoption")
	}
	fresh.Check()
}

// TestMapFreshReplicaAdoptsAnchor pins map adoption of a multi-entry anchor:
// every binding of the anchor must become reachable through keyIndex (not
// just content[0]), values must survive, the log must satisfy the compacted
// invariants, and the adopting replica must re-compact idempotently.
// MapDocument has no Check() (see Task 1 report F2), so the shared
// checkCompacted helper stands in for the log-level invariants.
func TestMapFreshReplicaAdoptsAnchor(t *testing.T) {
	src := NewMapDocument[string, int](1)
	src.Set("a", 1)
	src.Set("b", 2)
	src.Set("c", 3)
	src.Compact()

	fresh := NewMapDocument[string, int](2)
	fresh.MergeFrom(src)

	if !fresh.opLog.isCompacted() {
		t.Fatal("fresh map did not adopt anchorCoverage")
	}
	if _, ok := fresh.opLog.version[anchorAgent]; ok {
		t.Error("version carries the anchor sentinel after adoption")
	}
	if _, ok := fresh.opLog.anchorCoverage[anchorAgent]; ok {
		t.Error("anchorCoverage carries the anchor sentinel after adoption")
	}
	for k, want := range map[string]int{"a": 1, "b": 2, "c": 3} {
		if got, ok := fresh.Get(k); !ok || got != want {
			t.Errorf("Get(%q) = (%v, %v), want (%v, true)", k, got, ok, want)
		}
	}
	if keys := fresh.Keys(); len(keys) != 3 {
		t.Errorf("Keys() = %v, want 3 keys", keys)
	}
	checkCompacted(fresh.opLog)

	// The adopting replica can re-compact idempotently and stays intact.
	fresh.Compact()
	for k, want := range map[string]int{"a": 1, "b": 2, "c": 3} {
		if got, ok := fresh.Get(k); !ok || got != want {
			t.Errorf("after re-Compact: Get(%q) = (%v, %v), want (%v, true)", k, got, ok, want)
		}
	}
	if keys := fresh.Keys(); len(keys) != 3 {
		t.Errorf("after re-Compact: Keys() = %v, want 3 keys", keys)
	}
	if _, ok := fresh.opLog.version[anchorAgent]; ok {
		t.Error("re-Compact reintroduced the anchor sentinel into version")
	}
	checkCompacted(fresh.opLog)
}

// TestEmptyAnchorCoveredParentPanics pins the empty-anchor fall-through: a
// zero-op compacted log and an edited empty-anchor log hold no anchor lv, so
// a pre-critical parent query falls through the coverage interception and
// panics in runIdxForSeq — there is no lv it could resolve to. The failed
// merge must leave the destination untouched.
func TestEmptyAnchorCoveredParentPanics(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "abc")
	c := NewRuneDocument(3)
	c.MergeFrom(a)
	c.Ins(3, "!") // {3,0} parents the pre-critical run end (1, 2)

	// Edited empty-anchor shape: tombstone-only compaction followed by a
	// local edit — ops[0] is a real-agent root, no anchor op exists.
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	b.Del(0, 3)
	b.Compact()
	b.Ins(0, "x")
	if b.doc.opLog.ops[0].id.agent == anchorAgent {
		t.Fatal("expected an edited empty-anchor log without an anchor op")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic resolving a pre-critical parent on an edited empty-anchor log")
			}
		}()
		b.MergeFrom(c)
	}()
	if got, want := b.GetString(), "x"; got != want {
		t.Errorf("dest mutated by failed merge: GetString() = %q, want %q", got, want)
	}
	if len(b.doc.opLog.ops) != 1 {
		t.Errorf("dest log mutated by failed merge: %d ops, want 1", len(b.doc.opLog.ops))
	}

	// Zero-op shape: the same query before any local edit.
	d := NewRuneDocument(4)
	d.MergeFrom(a)
	d.Del(0, 3)
	d.Compact() // zero ops, coverage set
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic resolving a pre-critical parent on a zero-op compacted log")
			}
		}()
		d.MergeFrom(c)
	}()
	if got := d.GetString(); got != "" {
		t.Errorf("dest mutated by failed merge: GetString() = %q, want empty", got)
	}
}

// TestMergeIntoPartialStatePanics pins the unsupported-topology guard: merging
// from a compacted peer into a partially-converged replica panics.
func TestMergeIntoPartialStatePanics(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "abc")
	a.Compact()
	partial := NewRuneDocument(2)
	partial.Ins(0, "zz") // holds unrelated content: neither empty nor converged
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic merging compacted replica into partial state")
			}
		}()
		partial.MergeFrom(a)
	}()
}

// TestMergeStraddlingExtendedRun pins contract item (3): an extended run
// re-arriving at a compacted replica whose held prefix is folded into the
// anchor. The re-arrival path must split off the unknown suffix and parent it
// at the anchor's end via coverage, not demand the (discarded) prefix op id.
func TestMergeStraddlingExtendedRun(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "hello")
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	a.MergeFrom(b)
	a.Compact() // a: [anchor "hello"], coverage {1:4}

	// full re-derives agent 1's history as one extended run: {1,0} len 11.
	full := NewRuneDocument(1)
	full.Ins(0, "hello world")
	a.MergeFrom(full)
	if got, want := a.GetString(), "hello world"; got != want {
		t.Errorf("GetString() = %q, want %q", got, want)
	}
	a.Check()

	// The suffix op a derived must round-trip to the full replica as a no-op.
	full.MergeFrom(a)
	if got, want := full.GetString(), "hello world"; got != want {
		t.Errorf("reverse merge: GetString() = %q, want %q", got, want)
	}
	full.Check()
}

// TestCompactZeroOpIdempotent pins that Compact on an already-compacted
// zero-op log (empty frontier — the tombstone-only shape) is a no-op rather
// than a precondition panic: there is no history left to collapse.
func TestCompactZeroOpIdempotent(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "abc")
	a.Del(0, 3)
	a.Compact() // zero-op compacted log: ops empty, frontier empty

	before := map[int]int{}
	for k, v := range a.doc.opLog.version {
		before[k] = v
	}

	a.Compact() // must be a no-op, not a panic

	if len(a.doc.opLog.ops) != 0 {
		t.Errorf("after re-Compact: %d ops, want 0", len(a.doc.opLog.ops))
	}
	if a.doc.opLog.anchorCoverage == nil {
		t.Error("re-Compact lost anchorCoverage")
	}
	for k, v := range before {
		if got := a.doc.opLog.version[k]; got != v {
			t.Errorf("version[%d]: %d -> %d", k, v, got)
		}
	}
	if got := a.GetString(); got != "" {
		t.Errorf("GetString() = %q, want empty", got)
	}
	a.Check()
}

// TestCompactTraceScale measures compaction at real editing-trace scale: the
// full trace is replayed into a single replica, then Compact() must collapse
// the log to the anchor while the visible content stays byte-identical and
// Check() stays green. Heap numbers come from runtime.ReadMemStats around an
// explicit GC on both sides, so the dead pre-compaction log is not counted
// in the "after" reading; the printed lines are quoted verbatim in
// CONTEXT.md.
func TestCompactTraceScale(t *testing.T) {
	if testing.Short() {
		t.Skip("trace replay is slow")
	}
	raw, err := os.ReadFile("../../resources/editing-trace.json")
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	a := NewRuneDocument(1)
	if err := replayTrace(a, raw); err != nil {
		t.Fatalf("replay trace: %v", err)
	}
	before := a.GetString()

	ms := runtime.MemStats{}
	runtime.GC()
	runtime.ReadMemStats(&ms)
	t.Logf("before: ops=%d heap=%dMB (%d bytes)", len(a.doc.opLog.ops), ms.HeapAlloc>>20, ms.HeapAlloc)

	a.Compact()

	runtime.GC()
	runtime.ReadMemStats(&ms)
	t.Logf("after: ops=%d heap=%dMB (%d bytes)", len(a.doc.opLog.ops), ms.HeapAlloc>>20, ms.HeapAlloc)

	if a.GetString() != before {
		t.Fatal("content changed across compaction")
	}
	a.Check()
}

// TestCompactArrayMergeableValues pins the array counterpart of
// TestCompactMapMergeableValues: a compacted array of Mergeable values (maps)
// merging a full-history peer must skip the peer's pre-critical Mergeable ops
// via the coverage table instead of resolving their folded ids with idToLV
// (which panics "Could not find id in oplog"), and both directions must
// converge with the element values intact.
func TestCompactArrayMergeableValues(t *testing.T) {
	dest := NewArrayDocument[*MapDocument[string, int]](1)
	childA := NewMapDocument[string, int](10)
	childA.Set("k", 1)
	childB := NewMapDocument[string, int](11)
	childB.Set("k", 2)
	dest.Ins(0, []*MapDocument[string, int]{childA, childB})
	dest.Check()

	full := NewArrayDocument[*MapDocument[string, int]](2)
	full.MergeFrom(dest)
	full.Check()

	dest.Compact() // folds the two Mergeable element ops into the anchor
	dest.Check()

	// Post-compaction element append on the full replica: its op parents
	// into full's pre-critical frontier, which the compacted dest resolves
	// via coverage. The recursion pass over full's pre-critical Mergeable
	// ops is the C1 regression: without the coverage guard it panics.
	childC := NewMapDocument[string, int](12)
	childC.Set("k", 3)
	full.Ins(2, []*MapDocument[string, int]{childC})
	dest.MergeFrom(full)
	if got, want := dest.Len(), 3; got != want {
		t.Errorf("dest.Len() = %d, want %d", got, want)
	}
	items := dest.GetItems()
	if len(items) != 3 || items[0] != childA || items[1] != childB || items[2] != childC {
		t.Errorf("dest elements = %v, want [childA childB childC] pointer-identical", items)
	}
	if v, ok := items[2].Get("k"); !ok || v != 3 {
		t.Errorf("childC.Get(%q) = (%v, %v), want (3, true)", "k", v, ok)
	}
	dest.Check()

	// Reverse direction: the full replica takes dest's post-compaction edit
	// and both converge without corruption.
	childD := NewMapDocument[string, int](13)
	childD.Set("k", 4)
	dest.Ins(3, []*MapDocument[string, int]{childD})
	full.MergeFrom(dest)
	fullItems := full.GetItems()
	if len(fullItems) != 4 || fullItems[2] != childC || fullItems[3] != childD {
		t.Errorf("full elements after reverse merge = %v, want childC+childD appended", fullItems)
	}
	if v, ok := fullItems[3].Get("k"); !ok || v != 4 {
		t.Errorf("childD.Get(%q) = (%v, %v), want (4, true)", "k", v, ok)
	}
	full.Check()
	dest.Check()
}

// TestNonAlignedCompactionPointsPanic pins the loud boundary for replicas
// compacted at different points: when src's post-compaction op references
// src's (shorter) anchor end, dest's own longer anchor matches the (-1, seq)
// query — resolving it would split dest's anchor and silently poison the log
// (a second agent-(-1) op without coverage, two frontier tips, later panics
// far from the cause). The merge must panic with the documented-topology
// message and leave dest untouched.
func TestNonAlignedCompactionPointsPanic(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "hi")
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	a.MergeFrom(b)
	b.Compact()   // b: anchor "hi", coverage {1:1}
	a.Ins(2, "!") // independent edit on a
	a.Compact()   // a: anchor "hi!" (longer), coverage {1:2}

	b.Ins(0, ">") // b's post-compaction op parents b's anchor end (-1, 1)
	content := a.GetString()
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("expected panic merging replicas compacted at different points")
				return
			}
			msg, ok := r.(string)
			if !ok || !strings.Contains(msg, "compacted at different points") {
				t.Errorf("panic %v does not name the non-aligned-compaction topology", r)
			}
		}()
		a.MergeFrom(b)
	}()
	if got := a.GetString(); got != content {
		t.Errorf("dest content corrupted by failed merge: %q -> %q", content, got)
	}
	a.Check()
}
