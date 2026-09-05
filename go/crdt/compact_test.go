package crdt

import (
	"reflect"
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
