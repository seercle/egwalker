package crdt

import (
	"cmp"
	"encoding/binary"
	"maps"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// mustDelta encodes src's delta vs since and fails t on error.
func mustDelta(t *testing.T, src *RuneDocument, since remoteVersion) []byte {
	t.Helper()
	blob, err := MarshalDelta(src.doc.opLog, RuneTextCodec{}, since)
	if err != nil {
		t.Fatalf("MarshalDelta: %v", err)
	}
	return blob
}

// mustUnmarshalDelta decodes src's delta frame and fails t on error.
func mustUnmarshalDelta(t *testing.T, blob []byte) *deltaFrame[runeText] {
	t.Helper()
	frame, err := UnmarshalDelta[runeText](blob, RuneTextCodec{})
	if err != nil {
		t.Fatalf("UnmarshalDelta: %v", err)
	}
	return frame
}

// TestDeltaInclusionRules pins the sender-side filter: ops the receiver fully
// holds are excluded, straddling ops are included whole, the anchor is
// included iff not redundant vs since.
func TestDeltaInclusionRules(t *testing.T) {
	// The first insert must not collapse into a later adjacent run (which
	// would fuse the ops; the brief's original shape collapses for real), so
	// space the edits apart.
	a := NewRuneDocument(1)
	a.Ins(0, "hello")
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	a.Ins(0, "H")
	a.Ins(5, " world")

	// No Version() yet (Task 2): use the sender log's raw version vector.
	since := cloneRemoteVersion(b.doc.opLog.version)
	frame := mustUnmarshalDelta(t, mustDelta(t, a, since))
	if len(frame.ops) != 2 {
		t.Fatalf("delta has %d ops, want 2 (a's post-sync ins)", len(frame.ops))
	}
	if got := frame.ops[0].op.id.agent; got != 1 {
		t.Errorf("delta op agent = %d, want 1", got)
	}

	// Fresh receiver (empty version): everything, in order.
	log := a.doc.opLog
	fresh := mustUnmarshalDelta(t, mustDelta(t, a, remoteVersion{}))
	if len(fresh.ops) != len(log.ops) {
		t.Errorf("fresh delta has %d ops, want %d", len(fresh.ops), len(log.ops))
	}
	for i, o := range log.ops {
		if d := fresh.ops[i]; d.op.id != o.id || d.op.opType != o.opType || d.op.pos != o.pos || d.op.length != o.length || d.op.content != o.content {
			t.Errorf("fresh record %d: %+v != log op %+v", i, d.op, o)
		}
	}
}

// TestDeltaFrameLevelRoundTrip pins the frame-level round trip: decoding
// mustDelta(...) against an EMPTY since yields, per record, exactly the
// sender's op (id, opType, length, pos, codec-decoded content, parents as
// (agent, endSeq) pairs) and the sender's version vector.
func TestDeltaFrameLevelRoundTrip(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "the quick brown fox")
	a.Del(4, 6)
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	b.Ins(3, "ZZ")
	a.MergeFrom(b)

	log := a.doc.opLog
	frame := mustUnmarshalDelta(t, mustDelta(t, a, remoteVersion{}))

	if len(frame.ops) != len(log.ops) {
		t.Fatalf("frame has %d ops, log has %d", len(frame.ops), len(log.ops))
	}
	for i, o := range log.ops {
		d := frame.ops[i]
		if d.op.id != o.id {
			t.Errorf("op %d id: %v != %v", i, d.op.id, o.id)
		}
		if d.op.opType != o.opType {
			t.Errorf("op %d opType: %q != %q", i, d.op.opType, o.opType)
		}
		if d.op.length != o.length {
			t.Errorf("op %d length: %d != %d", i, d.op.length, o.length)
		}
		if d.op.pos != o.pos {
			t.Errorf("op %d pos: %d != %d", i, d.op.pos, o.pos)
		}
		if d.op.content != o.content {
			t.Errorf("op %d content: %q != %q", i, string(d.op.content), string(o.content))
		}
		if len(d.op.parents) != 0 {
			t.Errorf("op %d wire record must not carry lv parents", i)
		}
		if len(d.parents) != len(o.parents) {
			t.Errorf("op %d parents: %d pairs != %d", i, len(d.parents), len(o.parents))
			continue
		}
		for j, p := range o.parents {
			pa := log.opAt(p)
			want := id{agent: pa.id.agent, seq: log.seqAt(p)}
			if d.parents[j] != want {
				t.Errorf("op %d parent %d: %+v != (agent %d, endSeq %d)", i, j, d.parents[j], want.agent, want.seq)
			}
		}
		if i > 0 && d.coverage != nil {
			t.Errorf("op %d is not an anchor but carries coverage %v", i, d.coverage)
		}
	}
	if !reflect.DeepEqual(frame.senderVersion, log.version) {
		t.Errorf("senderVersion: got %v, want %v", frame.senderVersion, log.version)
	}
	if frame.anchorCoverage != nil {
		t.Errorf("uncompacted log's delta carries an anchor coverage table: %v", frame.anchorCoverage)
	}

	// A since that fully covers the receiver yields a valid zero-op frame.
	saturating := remoteVersion{}
	for agent, seq := range log.version {
		saturating[agent] = seq
	}
	empty := mustUnmarshalDelta(t, mustDelta(t, a, saturating))
	if len(empty.ops) != 0 {
		t.Errorf("fully-sinceed delta has %d ops, want 0", len(empty.ops))
	}
	if !reflect.DeepEqual(empty.senderVersion, log.version) {
		t.Errorf("zero-op delta lost senderVersion: %v != %v", empty.senderVersion, log.version)
	}
}

// TestDeltaFrameCompacted pins the anchor-record path: a full delta of a
// compacted log carries the anchor record (agent sentinel, snapshot content,
// coverage), and a delta of a compacted log whose anchor the receiver holds
// decodes with the log-level anchor coverage table riding the extra row.
func TestDeltaFrameCompacted(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "persist")
	a.Compact()

	full := mustUnmarshalDelta(t, mustDelta(t, a, remoteVersion{}))
	if len(full.ops) != 1 {
		t.Fatalf("anchor delta has %d ops, want 1", len(full.ops))
	}
	if got := full.ops[0].op.id.agent; got != anchorAgent {
		t.Fatalf("first record agent = %d, want %d", got, anchorAgent)
	}
	if !reflect.DeepEqual(full.ops[0].coverage, a.doc.opLog.anchorCoverage) {
		t.Errorf("anchor record coverage: %v != %v", full.ops[0].coverage, a.doc.opLog.anchorCoverage)
	}
	if full.anchorCoverage != nil {
		t.Errorf("kept-anchor delta carries a log-level table: %v", full.anchorCoverage)
	}
	if !reflect.DeepEqual(full.senderVersion, a.doc.opLog.version) {
		t.Errorf("senderVersion: %v != %v", full.senderVersion, a.doc.opLog.version)
	}

	// Receiver holds the anchor: since covers everything the anchor covers.
	since := cloneRemoteVersion(a.doc.opLog.anchorCoverage)
	a.Ins(2, "x")
	log := a.doc.opLog
	rest := mustUnmarshalDelta(t, mustDelta(t, a, since))
	// Replicate the sender filter: the anchor is excluded, the rest kept.
	kept := log.ops[1:]
	if len(rest.ops) != len(kept) {
		t.Fatalf("kept ops: %d records, want %d", len(rest.ops), len(kept))
	}
	for i, d := range rest.ops {
		if d.op.id.agent == anchorAgent {
			t.Errorf("record %d is a redundant anchor", i)
		}
		o := log.ops[i+1] // kept = log.ops[1:]
		if d.op.id != o.id || d.op.opType != o.opType || d.op.pos != o.pos || d.op.length != o.length {
			t.Errorf("record %d: %+v != log op %+v", i, d.op, o)
		}
		if d.op.content != o.content {
			t.Errorf("record %d content: %q != %q", i, string(d.op.content), string(o.content))
		}
		for j, p := range o.parents {
			pa := log.opAt(p)
			want := id{agent: pa.id.agent, seq: log.seqAt(p)}
			if d.parents[j] != want {
				t.Errorf("record %d parent %d: %+v != (agent %d, endSeq %d)", i, j, d.parents[j], want.agent, want.seq)
			}
		}
	}
	if !reflect.DeepEqual(rest.anchorCoverage, log.anchorCoverage) {
		t.Errorf("anchorless delta's log-level coverage: %v != %v", rest.anchorCoverage, log.anchorCoverage)
	}
}

// TestDeltaFrameEmptyLog pins that an empty delta (empty log, empty kept) is
// a valid frame decoding to a zero-op frame.
func TestDeltaFrameEmptyLog(t *testing.T) {
	frame := mustUnmarshalDelta(t, mustDelta(t, NewRuneDocument(9), remoteVersion{}))
	if len(frame.ops) != 0 {
		t.Fatalf("empty log delta has %d ops", len(frame.ops))
	}
	if frame.senderVersion == nil {
		t.Error("senderVersion must decode to a table (possibly empty), not nil")
	}
}

// TestDeltaRoundTripFull pins the encoder-equivalence guard: applying a
// sender's full delta (empty-since) to an empty replica through the public
// API reproduces the sender exactly.
func TestDeltaRoundTripFull(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "the quick brown fox")
	a.Del(4, 6)
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	b.Ins(3, "ZZ")
	a.MergeFrom(b)

	fresh := NewRuneDocument(3)
	blob, err := a.Delta(fresh.Version())
	if err != nil {
		t.Fatalf("Delta: %v", err)
	}
	fresh.ApplyDelta(blob)
	if got, want := fresh.GetString(), a.GetString(); got != want {
		t.Errorf("content: %q != %q", got, want)
	}
	if len(fresh.doc.opLog.ops) != len(a.doc.opLog.ops) {
		t.Errorf("op count: %d != %d", len(fresh.doc.opLog.ops), len(a.doc.opLog.ops))
	}
	if !maps.Equal(fresh.Version(), a.Version()) {
		t.Errorf("version: %v != %v", fresh.Version(), a.Version())
	}
	fresh.Check()
}

// mustApply encodes src's delta against dst's version and applies it.
func mustApply(t *testing.T, dst *RuneDocument, src *RuneDocument) []byte {
	t.Helper()
	blob, err := src.Delta(dst.Version())
	if err != nil {
		t.Fatalf("Delta: %v", err)
	}
	return blob
}

// mergeableCell is a gob-encodable Mergeable element standing in for the
// matrix entry's element maps: racing Set on its inner map reconciles through
// the element-0 MergeFromAny pass (same semantics as merge, F4 carried).
type mergeableCell struct{ Entries map[string]int }

func (c *mergeableCell) MergeFromAny(other any) {
	o, ok := other.(*mergeableCell)
	if !ok {
		return
	}
	for k, v := range o.Entries {
		c.Entries[k] = v
	}
}

// cellsText renders a cell array's content as a stable comparable form
// (elements may arrive unordered across components, so sort by the "a" key).
func cellsText(items []*mergeableCell) string {
	var sb strings.Builder
	for _, c := range items {
		keys := make([]string, 0, len(c.Entries))
		for k := range c.Entries {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(strconv.Itoa(c.Entries[k]))
			sb.WriteString(" ")
		}
		sb.WriteString("|")
	}
	return sb.String()
}

// mapEntries renders a map doc's state deterministically (Keys order is not).
func mapEntries[K cmp.Ordered, V comparable](keys []K, get func(K) (V, bool)) map[K]V {
	out := make(map[K]V, len(keys))
	for _, k := range keys {
		v, ok := get(k)
		if ok {
			out[k] = v
		}
	}
	return out
}

// TestDeltaEqualsMerge is the core equivalence matrix: for each family and
// sync shape, ApplyDelta(Delta(since)) must land exactly where MergeFrom
// lands — content, version, and Check.
func TestDeltaEqualsMerge(t *testing.T) {
	t.Run("rune", func(t *testing.T) {
		a := NewRuneDocument(1)
		a.Ins(0, "hello")
		b := NewRuneDocument(2)
		b.MergeFrom(a)
		a.Ins(5, " world")
		a.Del(0, 1)

		viaMerge := NewRuneDocument(3)
		viaMerge.MergeFrom(a)

		viaDelta := NewRuneDocument(3)
		viaDelta.ApplyDelta(mustApply(t, viaDelta, a))

		if viaDelta.GetString() != viaMerge.GetString() {
			t.Errorf("content diverged: %q vs %q", viaDelta.GetString(), viaMerge.GetString())
		}
		if !maps.Equal(viaDelta.Version(), viaMerge.Version()) {
			t.Errorf("version diverged: %v vs %v", viaDelta.Version(), viaMerge.Version())
		}
		viaDelta.Check()
	})

	t.Run("rune full log round trip", func(t *testing.T) {
		a := baseDeltaDoc()
		b := NewRuneDocument(4)
		b.MergeFrom(a)
		a.Ins(1, "Q")
		a.Ins(4, "R")
		a.Del(6, 2)

		viaMerge := NewRuneDocument(5)
		viaMerge.MergeFrom(a)

		viaDelta := NewRuneDocument(6)
		viaDelta.ApplyDelta(mustApply(t, viaDelta, a))

		if viaDelta.GetString() != viaMerge.GetString() {
			t.Errorf("content diverged: %q vs %q", viaDelta.GetString(), viaMerge.GetString())
		}
		if !maps.Equal(viaDelta.Version(), viaMerge.Version()) {
			t.Errorf("version diverged: %v vs %v", viaDelta.Version(), viaMerge.Version())
		}
		if len(viaDelta.doc.opLog.ops) != len(viaMerge.doc.opLog.ops) {
			t.Errorf("op count diverged: %d vs %d", len(viaDelta.doc.opLog.ops), len(viaMerge.doc.opLog.ops))
		}
		viaDelta.Check()
	})

	t.Run("map", func(t *testing.T) {
		m1 := NewMapDocument[string, string](1)
		m2 := NewMapDocument[string, string](2)
		m1.Set("k", "x1")
		m2.MergeFrom(m1)
		m1.MergeFrom(m2)
		m1.Set("k", "a")
		m2.Set("k", "b") // racing Set, both unresolved in one place:
		hub := NewMapDocument[string, string](3)
		hub.MergeFrom(m1)
		hub.MergeFrom(m2) // hub holds the racing pair: agent 2 wins LWW
		hub.Set("onlyHub", "x")

		viaMerge := NewMapDocument[string, string](4)
		viaMerge.MergeFrom(hub)

		viaDelta := NewMapDocument[string, string](3)
		blob, err := hub.Delta(viaDelta.Version())
		if err != nil {
			t.Fatalf("Delta: %v", err)
		}
		viaDelta.ApplyDelta(blob)

		if v, ok := viaDelta.Get("k"); !ok || v != "b" {
			t.Errorf("viaDelta.Get(k) = (%q, %v), want (%q, true) — LWW winner agent 2", v, ok, "b")
		}
		if !maps.Equal(mapEntries[string, string](viaDelta.Keys(), viaDelta.Get),
			mapEntries[string, string](viaMerge.Keys(), viaMerge.Get)) {
			t.Errorf("maps diverged: %v vs %v",
				mapEntries[string, string](viaDelta.Keys(), viaDelta.Get),
				mapEntries[string, string](viaMerge.Keys(), viaMerge.Get))
		}
		if !maps.Equal(viaDelta.Version(), viaMerge.Version()) {
			t.Errorf("version diverged: %v vs %v", viaDelta.Version(), viaMerge.Version())
		}
	})

	t.Run("map mergeable values", func(t *testing.T) {
		m1 := NewMapDocument[string, *mergeableCell](1)
		cell := &mergeableCell{Entries: map[string]int{"a": 1}}
		m1.Set("k", cell)

		m2 := NewMapDocument[string, *mergeableCell](2)
		m2.MergeFrom(m1)
		m1.MergeFrom(m2)
		cell.Entries["z"] = 9 // shared-reference mutation: no new ops

		viaMerge := NewMapDocument[string, *mergeableCell](3)
		viaMerge.MergeFrom(m1)
		viaMerge.MergeFrom(m2)

		viaDelta := NewMapDocument[string, *mergeableCell](3)
		blob, err := m2.Delta(viaDelta.Version())
		if err != nil {
			t.Fatalf("Delta: %v", err)
		}
		viaDelta.ApplyDelta(blob)

		g1, _ := viaMerge.Get("k")
		g2, ok := viaDelta.Get("k")
		if got, want := cellsText([]*mergeableCell{g2}), cellsText([]*mergeableCell{g1}); !ok || got != want {
			t.Errorf("cell diverged: %q vs %q (present %v)", got, want, ok)
		}
		if !maps.Equal(viaDelta.Version(), viaMerge.Version()) {
			t.Errorf("version diverged: %v vs %v", viaDelta.Version(), viaMerge.Version())
		}
	})

	t.Run("array mergeable elements", func(t *testing.T) {
		arr1 := NewArrayDocument[*mergeableCell](1)
		cellA := &mergeableCell{Entries: map[string]int{"a": 1}}
		arr1.Ins(0, []*mergeableCell{cellA})
		arr2 := NewArrayDocument[*mergeableCell](2)
		arr2.MergeFrom(arr1)

		// New elements both sides plus racing Set on the shared cell.
		arr1.Ins(1, []*mergeableCell{{Entries: map[string]int{"b": 2}}})
		arr2.Ins(0, []*mergeableCell{{Entries: map[string]int{"c": 0}}})
		cellA.Entries["z"] = 9

		viaMerge := NewArrayDocument[*mergeableCell](3)
		viaMerge.MergeFrom(arr1)
		viaMerge.MergeFrom(arr2)

		viaDelta := NewArrayDocument[*mergeableCell](3)
		viaDelta.MergeFrom(arr1)
		blob, err := arr2.Delta(viaDelta.Version())
		if err != nil {
			t.Fatalf("Delta: %v", err)
		}
		viaDelta.ApplyDelta(blob)

		if got, want := cellsText(viaDelta.GetItems()), cellsText(viaMerge.GetItems()); got != want {
			t.Errorf("array diverged: %q vs %q", got, want)
		}
		if !maps.Equal(viaDelta.Version(), viaMerge.Version()) {
			t.Errorf("version diverged: %v vs %v", viaDelta.Version(), viaMerge.Version())
		}
		viaDelta.Check()
		viaMerge.Check()
	})

	t.Run("straddle", func(t *testing.T) {
		a := NewRuneDocument(1)
		a.Ins(0, "hello")
		b := NewRuneDocument(2)
		b.MergeFrom(a)
		a.Ins(5, " world") // extends a's tail run past what b holds

		before := len(b.doc.opLog.ops)
		b.ApplyDelta(mustApply(t, b, a))

		// The delta rides pushRemoteOpLV's suffix path: one new suffix op.
		if got := len(b.doc.opLog.ops); got != before+1 {
			t.Errorf("op count grew by %d, want 1", got-before)
		}
		suffix := b.doc.opLog.ops[len(b.doc.opLog.ops)-1]
		if string(suffix.content) != " world" || suffix.id != (id{agent: 1, seq: 5}) {
			t.Errorf("suffix op = %+v, want (world, {1 5})", suffix)
		}
		if got, want := b.GetString(), "hello world"; got != want {
			t.Errorf("GetString() = %q, want %q", got, want)
		}
		if !maps.Equal(b.Version(), a.Version()) {
			t.Errorf("version diverged: %v vs %v", b.Version(), a.Version())
		}
		b.Check()
	})

	t.Run("compacted dest, full src", func(t *testing.T) {
		a := NewRuneDocument(1)
		a.Ins(0, "hello")
		b := NewRuneDocument(2)
		b.MergeFrom(a)
		a.MergeFrom(b)
		a.Compact()
		b.Ins(5, " world")

		viaMerge := NewRuneDocument(3)
		viaMerge.MergeFrom(a)
		viaMerge.MergeFrom(b)

		viaDelta := NewRuneDocument(3)
		viaDelta.MergeFrom(a) // adopts a's anchor: compacted dest
		if !viaDelta.doc.opLog.isCompacted() {
			t.Fatal("viaDelta should be compacted before the delta")
		}
		viaDelta.ApplyDelta(mustApply(t, viaDelta, b))

		if got, want := viaDelta.GetString(), viaMerge.GetString(); got != want {
			t.Errorf("content diverged: %q vs %q", got, want)
		}
		if !maps.Equal(viaDelta.Version(), viaMerge.Version()) {
			t.Errorf("version diverged: %v vs %v", viaDelta.Version(), viaMerge.Version())
		}
		viaDelta.Check()
		viaMerge.Check()
	})

	t.Run("compacted src, fresh dest", func(t *testing.T) {
		a := NewRuneDocument(1)
		a.Ins(0, "hello")
		b := NewRuneDocument(2)
		b.MergeFrom(a)
		b.Ins(5, " world")
		a.MergeFrom(b)
		a.Compact()

		fresh := NewRuneDocument(3)
		fresh.ApplyDelta(mustApply(t, fresh, a))

		if !fresh.doc.opLog.isCompacted() {
			t.Error("fresh dest did not adopt the anchor: isCompacted() false")
		}
		if !maps.Equal(fresh.doc.opLog.anchorCoverage, a.doc.opLog.anchorCoverage) {
			t.Errorf("coverage not restored: %v vs %v", fresh.doc.opLog.anchorCoverage, a.doc.opLog.anchorCoverage)
		}
		if got, want := fresh.GetString(), "hello world"; got != want {
			t.Errorf("GetString() = %q, want %q", got, want)
		}
		if got := len(fresh.doc.opLog.ops); got != 1 {
			t.Errorf("fresh dest holds %d ops, want 1 (the adopted anchor)", got)
		}

		// Pre-critical re-delivery from the full-history replica: no dup.
		since := cloneRemoteVersion(b.doc.opLog.version)
		redelivery := mustUnmarshalDelta(t, mustDelta(t, b, since))
		if len(redelivery.ops) != 0 {
			t.Errorf("b's delta vs the adoption-raised vector has %d ops, want 0", len(redelivery.ops))
		}
		opsBefore := len(fresh.doc.opLog.ops)
		fresh.ApplyDelta(mustApply(t, fresh, b))
		if got := len(fresh.doc.opLog.ops); got != opsBefore {
			t.Errorf("re-delivery duplicated ops: %d -> %d", opsBefore, got)
		}
		fresh.Check()
	})

	t.Run("both compacted", func(t *testing.T) {
		a := NewRuneDocument(1)
		b := NewRuneDocument(2)
		a.Ins(0, "hi")
		b.MergeFrom(a)
		a.MergeFrom(b)
		a.Compact()
		b.Compact()
		a.Ins(2, "!")
		b.Ins(0, ">")

		aDelta := mustApply(t, a, b) // b -> a, computed before either applies
		bDelta := mustApply(t, b, a)
		a.ApplyDelta(aDelta)
		b.ApplyDelta(bDelta)

		if a.GetString() != b.GetString() {
			t.Errorf("divergence: a=%q b=%q", a.GetString(), b.GetString())
		}
		if !maps.Equal(a.Version(), b.Version()) {
			t.Errorf("version diverged: %v vs %v", a.Version(), b.Version())
		}
		a.Check()
		b.Check()
	})

	t.Run("boundary pin", func(t *testing.T) {
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
		opsBefore := len(a.doc.opLog.ops)
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Error("expected panic applying delta across non-aligned compaction points")
					return
				}
				msg, ok := r.(string)
				if !ok || !strings.Contains(msg, "compacted at different points") {
					t.Errorf("panic %v does not name the non-aligned-compaction topology", r)
				}
			}()
			a.ApplyDelta(mustApply(t, a, b))
		}()
		if got := a.GetString(); got != content {
			t.Errorf("dest content corrupted by failed apply: %q -> %q", content, got)
		}
		if got := len(a.doc.opLog.ops); got != opsBefore {
			t.Errorf("dest log mutated by failed apply: %d ops -> %d", opsBefore, got)
		}
		a.Check()
	})

	t.Run("zero-op delta", func(t *testing.T) {
		a := NewRuneDocument(1)
		a.Ins(0, "hello")
		a.Del(0, 2)
		b := NewRuneDocument(2)
		b.MergeFrom(a)

		opsBefore := len(b.doc.opLog.ops)
		b.ApplyDelta(mustApply(t, b, a))

		if got := len(b.doc.opLog.ops); got != opsBefore {
			t.Errorf("op count changed: %d -> %d", opsBefore, got)
		}
		if got, want := b.GetString(), a.GetString(); got != want {
			t.Errorf("content changed: %q vs %q", got, want)
		}
		if !maps.Equal(b.Version(), a.Version()) {
			t.Errorf("version changed: %v vs %v", b.Version(), a.Version())
		}
	})

	t.Run("zero-op compacted sender", func(t *testing.T) {
		a := NewRuneDocument(1)
		a.Ins(0, "abc")
		a.Del(0, 3) // tombstone-only content: compaction leaves zero ops
		a.Compact()
		if !a.doc.opLog.isCompacted() || len(a.doc.opLog.ops) != 0 {
			t.Fatal("sender fixture: expected a zero-op compacted log")
		}

		fresh := NewRuneDocument(3)
		blob, err := a.Delta(fresh.Version())
		if err != nil {
			t.Fatalf("Delta: %v", err)
		}
		frame := mustUnmarshalDelta(t, blob)
		if len(frame.ops) != 0 {
			t.Fatalf("tombstone-only delta has %d ops, want 0", len(frame.ops))
		}
		if frame.anchorCoverage == nil {
			t.Fatal("tombstone-only delta must ride the log-level coverage table")
		}
		fresh.ApplyDelta(blob)

		if !fresh.doc.opLog.isCompacted() {
			t.Error("fresh dest did not adopt the log-level coverage: isCompacted() false")
		}
		if !maps.Equal(fresh.doc.opLog.anchorCoverage, a.doc.opLog.anchorCoverage) {
			t.Errorf("coverage: %v vs %v", fresh.doc.opLog.anchorCoverage, a.doc.opLog.anchorCoverage)
		}
		if !maps.Equal(fresh.Version(), a.Version()) {
			t.Errorf("version: %v vs %v", fresh.Version(), a.Version())
		}
		fresh.Check()
	})
}

// baseDeltaDoc builds a valid doc whose delta exercises every column: mixed
// types, multi-byte content, and cross-agent parents.
func baseDeltaDoc() *RuneDocument {
	a := NewRuneDocument(1)
	a.Ins(0, "the quick brown fox")
	a.Del(4, 6)
	b := NewRuneDocument(2)
	b.MergeFrom(a)
	b.Ins(3, "ZZ\n\xff")
	a.MergeFrom(b)
	return a
}

// deltaRawFrame decompresses a marshaled delta into its raw columnar frame.
func deltaRawFrame(t *testing.T, blob []byte) []byte {
	t.Helper()
	raw, err := binaryZstdDecoder.DecodeAll(blob, nil)
	if err != nil {
		t.Fatalf("delta does not decompress: %v", err)
	}
	return raw
}

// deltaFrameSplit splits a raw frame into its header (magic + version) and
// the ordered column bodies.
func deltaFrameSplit(t *testing.T, raw []byte) (header []byte, bodies [][]byte) {
	t.Helper()
	r := &binaryReader{buf: raw, off: len(deltaMagic)}
	if _, err := r.uvarint("version"); err != nil {
		t.Fatalf("delta version: %v", err)
	}
	header = append([]byte(nil), raw[:r.off]...)
	for r.off < len(raw) {
		b, err := r.column("seed")
		if err != nil {
			t.Fatalf("delta column split: %v", err)
		}
		bodies = append(bodies, b)
	}
	return header, bodies
}

// deltaFrameJoin re-encodes (possibly corrupted) columns into a raw frame
// (uncompressed; hostiledeltaCase wraps it).
func deltaFrameJoin(header []byte, bodies [][]byte) []byte {
	f := append([]byte(nil), header...)
	for _, b := range bodies {
		f = appendBinaryColumn(f, b)
	}
	return f
}

// hostiledeltaCase asserts UnmarshalDelta rejects raw with an error
// containing want.
func hostiledeltaCase(t *testing.T, raw []byte, want string) {
	t.Helper()
	blob := binaryZstdEncoder.EncodeAll(raw, nil)
	_, err := UnmarshalDelta[runeText](blob, RuneTextCodec{})
	if err == nil {
		t.Fatalf("expected error containing %q, got none", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}

// hostiledeltaCaseAlt is hostiledeltaCase but tolerates either substring
// (used where a truncation may surface as a bodyLen-overrun error instead).
func hostiledeltaCaseAlt(t *testing.T, raw []byte, alt string) {
	t.Helper()
	blob := binaryZstdEncoder.EncodeAll(raw, nil)
	_, err := UnmarshalDelta[runeText](blob, RuneTextCodec{})
	if err == nil {
		t.Fatal("expected a rejection, got none")
	}
	msg := err.Error()
	if !strings.Contains(msg, "truncated") && !strings.Contains(msg, alt) {
		t.Fatalf("error %q is neither a truncation nor contains %q", msg, alt)
	}
}

var deltaColNames = [...]string{"Types", "TypeRuns", "Agents", "AgentRuns", "Seqs",
	"Positions", "Lengths", "Content", "Parents", "Coverage", "SenderVersion"}

// deltaCoverageRows parses a Coverage column body into rowCount and rows.
func deltaCoverageRows(t *testing.T, body []byte) (uint64, [][]byte) {
	t.Helper()
	br := &binaryReader{buf: body}
	rowCount, err := br.uvarint("Coverage row count")
	if err != nil {
		t.Fatalf("coverage row count: %v", err)
	}
	rows := make([][]byte, rowCount)
	for i := range rows {
		rowLen, err := br.uvarint("Coverage row length")
		if err != nil {
			t.Fatalf("coverage row length: %v", err)
		}
		rows[i] = br.buf[br.off : br.off+int(rowLen)]
		br.off += int(rowLen)
	}
	if err := br.exact("Coverage"); err != nil {
		t.Fatalf("coverage rows: %v", err)
	}
	return rowCount, rows
}

// reencodeCoverageBody rebuilds a Coverage column body from rows.
func reencodeCoverageBody(rows [][]byte) []byte {
	body := binary.AppendUvarint(nil, uint64(len(rows)))
	for _, row := range rows {
		body = appendCoverageRow(body, row)
	}
	return body
}

// The column order of a delta frame: Types, TypeRuns, Agents, AgentRuns,
// Seqs, Positions, Lengths, Content, Parents, Coverage, SenderVersion.
const (
	colDTypes = iota
	colDTypeRuns
	colDAgents
	colDAgentRuns
	colDSeqs
	colDPositions
	colDLengths
	colDContent
	colDParents
	colDCoverage
	colDSenderVersion
	numDCols
)

func TestDeltaFrameHostileColumnTruncation(t *testing.T) {
	blob := mustDelta(t, baseDeltaDoc(), remoteVersion{})
	_, bodies := deltaFrameSplit(t, deltaRawFrame(t, blob))
	if len(bodies) != numDCols {
		t.Fatalf("delta frame has %d columns, want %d", len(bodies), numDCols)
	}
	for i := 0; i < numDCols; i++ {
		if len(bodies[i]) == 0 {
			continue // uncompacted Coverage body: nothing to truncate
		}
		truncated := append([]byte(nil), bodies[i][:len(bodies[i])-1]...)
		corrupt := make([][]byte, numDCols)
		copy(corrupt, bodies)
		corrupt[i] = truncated
		header := binary.AppendUvarint(append([]byte(nil), deltaMagic...), deltaVersion)
		t.Run(deltaColNames[i], func(t *testing.T) {
			hostiledeltaCaseAlt(t, deltaFrameJoin(header, corrupt), "exceeds")
		})
	}
}

func TestDeltaFrameHostileFrameHeader(t *testing.T) {
	raw := deltaRawFrame(t, mustDelta(t, baseDeltaDoc(), remoteVersion{}))

	// Magic corruption.
	badMagic := append([]byte(nil), raw...)
	badMagic[0] = 'X'
	hostiledeltaCase(t, badMagic, "binary: bad magic")

	// Version corruption: 0x02 where the uvarint version 1 sat.
	versionBad := append([]byte(nil), raw...)
	versionBad[len(deltaMagic)] = 0x02
	hostiledeltaCase(t, versionBad, "binary: unsupported version 2")
}

func TestDeltaFrameHostileColumns(t *testing.T) {
	blob := mustDelta(t, baseDeltaDoc(), remoteVersion{})
	_, bodies := deltaFrameSplit(t, deltaRawFrame(t, blob))
	header := binary.AppendUvarint(append([]byte(nil), deltaMagic...), deltaVersion)

	mkBodies := func(mutate func([][]byte) [][]byte) [][]byte {
		out := make([][]byte, numDCols)
		for i := range bodies {
			out[i] = append([]byte(nil), bodies[i]...)
		}
		return mutate(out)
	}

	t.Run("unknown type code", func(t *testing.T) {
		// The Types body ends with a type code; flip it to an unassigned
		// code.
		corrupt := mkBodies(func(bs [][]byte) [][]byte {
			bs[colDTypes][len(bs[colDTypes])-1] = 0x07
			return bs
		})
		hostiledeltaCase(t, deltaFrameJoin(header, corrupt), "binary: unknown op type code")
	})
	t.Run("TypeRuns truncated", func(t *testing.T) {
		corrupt := mkBodies(func(bs [][]byte) [][]byte {
			bs[colDTypeRuns] = bs[colDTypeRuns][:len(bs[colDTypeRuns])-1]
			return bs
		})
		hostiledeltaCase(t, deltaFrameJoin(header, corrupt), "binary: TypeRuns")
	})
	t.Run("AgentRuns truncated", func(t *testing.T) {
		corrupt := mkBodies(func(bs [][]byte) [][]byte {
			bs[colDAgentRuns] = bs[colDAgentRuns][:len(bs[colDAgentRuns])-1]
			return bs
		})
		hostiledeltaCase(t, deltaFrameJoin(header, corrupt), "binary: AgentRuns")
	})
	t.Run("content blob truncated", func(t *testing.T) {
		corrupt := mkBodies(func(bs [][]byte) [][]byte {
			bs[colDContent] = bs[colDContent][:len(bs[colDContent])-1]
			return bs
		})
		hostiledeltaCase(t, deltaFrameJoin(header, corrupt), "content")
	})
	t.Run("Parents pairs truncated", func(t *testing.T) {
		corrupt := mkBodies(func(bs [][]byte) [][]byte {
			bs[colDParents] = bs[colDParents][:len(bs[colDParents])-1]
			return bs
		})
		hostiledeltaCase(t, deltaFrameJoin(header, corrupt), "Parents")
	})
	t.Run("SenderVersion truncated", func(t *testing.T) {
		corrupt := mkBodies(func(bs [][]byte) [][]byte {
			bs[colDSenderVersion] = bs[colDSenderVersion][:len(bs[colDSenderVersion])-1]
			return bs
		})
		hostiledeltaCase(t, deltaFrameJoin(header, corrupt), "binary: SenderVersion column truncated")
	})
}

// TestDeltaFrameHostileCoverage corrupts the Coverage column of a compacted
// log's delta (anchor kept plus a post-anchor edit: rowCount 2, rows
// [table, empty]).
func TestDeltaFrameHostileCoverage(t *testing.T) {
	c := NewRuneDocument(1)
	c.Ins(0, "persist")
	c.Compact()
	c.Ins(2, "x")

	blob := mustDelta(t, c, remoteVersion{})
	_, bodies := deltaFrameSplit(t, deltaRawFrame(t, blob))
	if len(bodies) != numDCols {
		t.Fatalf("delta frame has %d columns, want %d", len(bodies), numDCols)
	}
	header := binary.AppendUvarint(append([]byte(nil), deltaMagic...), deltaVersion)

	rowCount, rows := deltaCoverageRows(t, bodies[colDCoverage])
	if rowCount != 2 {
		t.Fatalf("seed coverage rowCount = %d, want 2", rowCount)
	}

	t.Run("rowCount opCount+2", func(t *testing.T) {
		// Declare two extra rows beyond the anchorless shape.
		rows2 := append([][]byte{}, rows...)
		rows2 = append(rows2, nil, nil)
		corrupted := append([][]byte{}, bodies...)
		corrupted[colDCoverage] = reencodeCoverageBody(rows2)
		hostiledeltaCase(t, deltaFrameJoin(header, corrupted), "binary: coverage rowCount 4 != 2 or 3")
	})
	t.Run("coverage on non-anchor", func(t *testing.T) {
		// Row 1 (per-op row of a real agent) carries a table instead of
		// being empty.
		rows2 := [][]byte{rows[0], rows[0]}
		corrupted := append([][]byte{}, bodies...)
		corrupted[colDCoverage] = reencodeCoverageBody(rows2)
		hostiledeltaCase(t, deltaFrameJoin(header, corrupted), "binary: coverage on non-anchor op")
	})
	t.Run("anchor without coverage", func(t *testing.T) {
		rows2 := [][]byte{nil, nil}
		corrupted := append([][]byte{}, bodies...)
		corrupted[colDCoverage] = reencodeCoverageBody(rows2)
		hostiledeltaCase(t, deltaFrameJoin(header, corrupted), "binary: anchor record without coverage")
	})
}

// TestDeltaTraceScale applies the real editing trace through the delta
// transport at trace scale: replay the trace into one replica, then hand the
// whole content to a fresh replica as one Delta frame. Content must be
// byte-identical, Check() green; the logged sizes are quoted verbatim in
// CONTEXT.md.
func TestDeltaTraceScale(t *testing.T) {
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

	fresh := NewRuneDocument(2)
	blob, err := a.Delta(fresh.Version())
	if err != nil {
		t.Fatalf("Delta: %v", err)
	}
	fresh.ApplyDelta(blob)

	if got, want := fresh.GetString(), a.GetString(); got != want {
		t.Fatalf("content diverged at trace scale")
	}
	if !maps.Equal(fresh.Version(), a.Version()) {
		t.Fatalf("version diverged at trace scale")
	}
	fullBlob, err := MarshalBinary(a.doc.opLog, RuneTextCodec{})
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	t.Logf("trace delta (empty since): %d bytes vs full binary frame %d bytes (%d log ops)", len(blob), len(fullBlob), len(a.doc.opLog.ops))
	fresh.Check()
}
