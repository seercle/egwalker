package crdt

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
)

// buildBinaryTestLog assembles a log through real mutation paths covering
// every column: ins runs (multibyte + invalid UTF-8), deletes, concurrent
// branches (parents), and a non-trivial frontier.
func buildBinaryTestLog(t *testing.T) *opLog[runeText] {
	t.Helper()
	return buildBinaryTestLogT()
}

// buildBinaryTestLogT is buildBinaryTestLog without a testing.T, for callers
// outside tests proper (fuzz targets).
func buildBinaryTestLogT() *opLog[runeText] {
	d1, d2 := NewRuneDocument(1), NewRuneDocument(2)
	d1.Ins(0, "Hello")
	d2.MergeFrom(d1)
	d2.Del(1, 2)
	d2.Ins(d2.Len(), "wörld ✓\xff")
	d1.Ins(d1.Len(), "!")
	d1.MergeFrom(d2)
	d1.Del(0, 1)
	d1.Check() // branch must match a full checkout after the edits and merges
	d2.Check()
	return d1.doc.opLog
}

// runeDocFromLog wraps a decoded opLog in a RuneDocument, rebuilding the tree
// layer through checkout — the same from-log construction path doc.Compact
// uses for its fresh branch.
func runeDocFromLog(log *opLog[runeText], agent int) *RuneDocument {
	d := &doc[runeText]{opLog: log, agent: agent, branch: newBranch[runeText]()}
	d.branch.snapshot = checkout(log)
	d.branch.frontier = make([]lv, len(log.frontier))
	copy(d.branch.frontier, log.frontier)
	return &RuneDocument{doc: d}
}

func TestBinaryRoundTrip(t *testing.T) {
	log := buildBinaryTestLog(t)
	want := log.Marshal()

	blob, err := MarshalBinary(log, RuneTextCodec{})
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	round, err := UnmarshalBinary[runeText](blob, RuneTextCodec{})
	if err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	got := round.Marshal()

	if !reflect.DeepEqual(got.Types, want.Types) || !reflect.DeepEqual(got.TypeRuns, want.TypeRuns) ||
		!reflect.DeepEqual(got.Agents, want.Agents) || !reflect.DeepEqual(got.AgentRuns, want.AgentRuns) ||
		!reflect.DeepEqual(got.Seqs, want.Seqs) || !reflect.DeepEqual(got.Positions, want.Positions) ||
		!reflect.DeepEqual(got.Lengths, want.Lengths) || !reflect.DeepEqual(got.Content, want.Content) ||
		!reflect.DeepEqual(got.Parents, want.Parents) || !reflect.DeepEqual(got.Frontier, want.Frontier) {
		t.Fatalf("round-trip columns differ:\nwant %+v\ngot  %+v", want, got)
	}
	if !reflect.DeepEqual(round.version, log.version) {
		t.Fatalf("version maps differ: want %v, got %v", log.version, round.version)
	}
}

func TestBinaryRoundTripEmpty(t *testing.T) {
	log := newOpLog[runeText]()
	blob, err := MarshalBinary(log, RuneTextCodec{})
	if err != nil {
		t.Fatalf("MarshalBinary empty log: %v", err)
	}
	round, err := UnmarshalBinary[runeText](blob, RuneTextCodec{})
	if err != nil {
		t.Fatalf("empty log: %v", err)
	}
	if len(round.ops) != 0 || len(round.frontier) != 0 {
		t.Fatalf("empty log round-tripped to %d ops, frontier %v", len(round.ops), round.frontier)
	}
}

// TestBinaryRoundTripCompacted pins that a compacted log round-trips: the
// frame must carry the anchor coverage table so the decoded log is still
// compacted, keeps its version vector (the per-op columns alone cannot
// re-derive the pre-critical high-water marks), and merges like the original.
func TestBinaryRoundTripCompacted(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "persist me")
	a.Del(0, 4)
	a.Compact()

	data, err := MarshalBinary(a.doc.opLog, RuneTextCodec{})
	if err != nil {
		t.Fatal(err)
	}
	log, err := UnmarshalBinary[runeText](data, RuneTextCodec{})
	if err != nil {
		t.Fatal(err)
	}
	b := runeDocFromLog(log, 1)

	if got, want := b.GetString(), a.GetString(); got != want {
		t.Errorf("content: %q != %q", got, want)
	}
	if !b.doc.opLog.isCompacted() {
		t.Error("unmarshaled compacted log lost anchorCoverage")
	}
	for agent, seq := range a.doc.opLog.anchorCoverage {
		if got := b.doc.opLog.anchorCoverage[agent]; got != seq {
			t.Errorf("coverage[%d]: %d != %d", agent, seq, got)
		}
	}
	if !reflect.DeepEqual(b.doc.opLog.version, a.doc.opLog.version) {
		t.Errorf("version maps differ: want %v, got %v", a.doc.opLog.version, b.doc.opLog.version)
	}
	if !reflect.DeepEqual(b.doc.opLog.ops[0].coverage, a.doc.opLog.ops[0].coverage) {
		t.Errorf("anchor op coverage differs: want %v, got %v", a.doc.opLog.ops[0].coverage, b.doc.opLog.ops[0].coverage)
	}
	// The round-tripped replica must still merge like the original: merge the
	// same concurrent peer into both and require identical results. (c re-does
	// the history concurrently, never having synced with a, so the union is
	// not c's own state — the contract is that b behaves exactly like a.)
	c := NewRuneDocument(2)
	c.Ins(0, "persist me")
	c.Del(0, 4)
	c.Ins(0, "more ")
	a.MergeFrom(c)
	b.MergeFrom(c)
	if got, want := b.GetString(), a.GetString(); got != want {
		t.Errorf("post-roundtrip merge: %q != %q", got, want)
	}
	a.Check()
	// Re-delivering the peer's ops must be skipped entirely — the
	// reconstructed version vector claims the peer's history.
	before := b.GetString()
	b.MergeFrom(c)
	if got := b.GetString(); got != before {
		t.Errorf("re-delivered ops changed content: %q != %q", got, before)
	}
	b.Check()
}

// TestBinaryRoundTripCompactedAnchorless pins the two compacted shapes that
// hold no anchor op: the zero-op log left by tombstone-only compaction, and
// the edited empty-anchor log a local edit produces from it. Both are
// compacted (anchorCoverage set) and must stay that way across the frame.
func TestBinaryRoundTripCompactedAnchorless(t *testing.T) {
	a := NewRuneDocument(1)
	a.Ins(0, "abc")
	a.Del(0, 3)
	a.Compact()
	if len(a.doc.opLog.ops) != 0 || !a.doc.opLog.isCompacted() {
		t.Fatalf("expected a zero-op compacted log, got %d ops, compacted %v",
			len(a.doc.opLog.ops), a.doc.opLog.isCompacted())
	}

	data, err := MarshalBinary(a.doc.opLog, RuneTextCodec{})
	if err != nil {
		t.Fatal(err)
	}
	round, err := UnmarshalBinary[runeText](data, RuneTextCodec{})
	if err != nil {
		t.Fatal(err)
	}
	if !round.isCompacted() {
		t.Error("zero-op compacted log lost anchorCoverage")
	}
	if !reflect.DeepEqual(round.anchorCoverage, a.doc.opLog.anchorCoverage) {
		t.Errorf("anchorCoverage: want %v, got %v", a.doc.opLog.anchorCoverage, round.anchorCoverage)
	}
	if !reflect.DeepEqual(round.version, a.doc.opLog.version) {
		t.Errorf("version: want %v, got %v", a.doc.opLog.version, round.version)
	}

	// Edited empty-anchor shape: a local edit lands on the zero-op log, so
	// the log holds real ops but still no anchor op.
	a.Ins(0, "x")
	data, err = MarshalBinary(a.doc.opLog, RuneTextCodec{})
	if err != nil {
		t.Fatal(err)
	}
	round, err = UnmarshalBinary[runeText](data, RuneTextCodec{})
	if err != nil {
		t.Fatal(err)
	}
	if len(round.ops) != 1 || round.ops[0].id.agent == anchorAgent {
		t.Fatalf("expected an edited empty-anchor log, got %d ops, ops[0] agent %d",
			len(round.ops), round.ops[0].id.agent)
	}
	if !round.isCompacted() {
		t.Error("edited empty-anchor log lost anchorCoverage")
	}
	if !reflect.DeepEqual(round.anchorCoverage, a.doc.opLog.anchorCoverage) {
		t.Errorf("anchorCoverage: want %v, got %v", a.doc.opLog.anchorCoverage, round.anchorCoverage)
	}
	if !reflect.DeepEqual(round.version, a.doc.opLog.version) {
		t.Errorf("version: want %v, got %v", a.doc.opLog.version, round.version)
	}
	b := runeDocFromLog(round, 1)
	if got, want := b.GetString(), a.GetString(); got != want {
		t.Errorf("content: %q != %q", got, want)
	}
	b.Check()
}

// TestBinaryV1StillDecodes pins v1 frame compatibility: v1 predates the
// coverage column and can only describe uncompacted logs, so decode must
// accept it and produce an uncompacted log.
func TestBinaryV1StillDecodes(t *testing.T) {
	// A hand-built v1 frame of the empty log — the exact encoding v1 writers
	// produced (ten zero-count columns, no Coverage column).
	log, err := UnmarshalBinary[runeText](
		binaryZstdEncoder.EncodeAll(emptyColumns(frameHeader(1)), nil), RuneTextCodec{})
	if err != nil {
		t.Fatal(err)
	}
	if log.isCompacted() {
		t.Error("v1 frame decoded as compacted")
	}
	if len(log.ops) != 0 || len(log.frontier) != 0 {
		t.Errorf("v1 empty frame decoded to %d ops, frontier %v", len(log.ops), log.frontier)
	}
}

// TestBinaryRoundTripCompactedZeroSeq pins a subtle version-fold case: a
// version entry of exactly 0 (a single length-1 op at seq 0) has no surviving
// op to re-derive it from — the anchor op re-derives only the -1 sentinel —
// so the coverage fold must create the key, not just raise existing values.
func TestBinaryRoundTripCompactedZeroSeq(t *testing.T) {
	a := NewRuneDocument(5)
	a.Ins(0, "x")
	a.Compact()
	if !reflect.DeepEqual(a.doc.opLog.version, remoteVersion{5: 0}) {
		t.Fatalf("setup: version = %v, want map[5:0]", a.doc.opLog.version)
	}

	data, err := MarshalBinary(a.doc.opLog, RuneTextCodec{})
	if err != nil {
		t.Fatal(err)
	}
	round, err := UnmarshalBinary[runeText](data, RuneTextCodec{})
	if err != nil {
		t.Fatal(err)
	}
	if !round.isCompacted() {
		t.Error("lost anchorCoverage")
	}
	if !reflect.DeepEqual(round.version, a.doc.opLog.version) {
		t.Errorf("version: want %v, got %v", a.doc.opLog.version, round.version)
	}
}

// frameHeader starts a hand-crafted frame: magic + uvarint version.
func frameHeader(version uint64) []byte {
	return binary.AppendUvarint(append([]byte{}, binaryMagic...), version)
}

// emptyColumns appends the ten zero-count columns of an empty log. A v2 frame
// additionally carries the (empty) Coverage column, appended by callers that
// need a complete v2 frame — see TestBinaryV1StillDecodes and the trailing
// bytes garbage case.
func emptyColumns(frame []byte) []byte {
	zero := binary.AppendUvarint(nil, 0)
	for range 10 {
		frame = appendBinaryColumn(frame, zero)
	}
	return frame
}

// TestBinaryRejectsGarbage feeds hostile blobs to UnmarshalBinary. The frame
// cases are hand-crafted and compressed with the same encoder MarshalBinary
// uses, so decompression succeeds and each case's intended rejection fires
// inside the frame parser itself, pinned to its specific error. The raw
// non-zstd case pins the decompress wrapper; empty input decodes to an empty
// frame and dies at the magic check.
func TestBinaryRejectsGarbage(t *testing.T) {
	compress := func(frame []byte) []byte {
		return binaryZstdEncoder.EncodeAll(frame, nil)
	}

	// hostileCols builds the magic, version, and ten op columns of a frame
	// describing `ops` length-1 ins runs by a single agent at seq 0. The
	// Coverage column is left for the caller so coverage cases can shape,
	// truncate, or omit it.
	hostileCols := func(agent int64, ops int) []byte {
		f := frameHeader(binaryVersion)
		f = appendBinaryColumn(f, binary.AppendUvarint(binary.AppendUvarint(nil, 1), 0)) // Types: one ins run
		f = appendBinaryColumn(f, binary.AppendUvarint(binary.AppendUvarint(nil, 1), uint64(ops)))
		f = appendBinaryColumn(f, append(binary.AppendUvarint(nil, 1), binary.AppendVarint(nil, agent)...))
		f = appendBinaryColumn(f, binary.AppendUvarint(binary.AppendUvarint(nil, 1), uint64(ops)))
		f = appendBinaryColumn(f, append(binary.AppendUvarint(nil, 1), binary.AppendVarint(nil, 0)...))
		positions := binary.AppendUvarint(nil, uint64(ops))
		lengths := binary.AppendUvarint(nil, uint64(ops))
		content := binary.AppendUvarint(nil, uint64(ops))
		parents := binary.AppendUvarint(nil, uint64(ops))
		for range ops {
			positions = append(positions, 0) // delta 0
			lengths = append(lengths, 1)     // run of 1
			content = append(content, 0)     // empty blob
			parents = append(parents, 0)     // no parents
		}
		f = appendBinaryColumn(f, positions)
		f = appendBinaryColumn(f, lengths)
		f = appendBinaryColumn(f, content)
		f = appendBinaryColumn(f, parents)
		return appendBinaryColumn(f, binary.AppendUvarint(nil, 0)) // Frontier: empty
	}

	// coverageRows renders a Coverage column body: uvarint rowCount, then
	// each row as a length-prefixed body.
	coverageRows := func(rows ...[]byte) []byte {
		body := binary.AppendUvarint(nil, uint64(len(rows)))
		for _, row := range rows {
			body = appendBinaryColumn(body, row)
		}
		return body
	}
	// tableRow12 is a coverage table with the single entry {agent 1: seq 2}.
	tableRow12 := func() []byte {
		body := binary.AppendUvarint(nil, 1) // one entry
		body = binary.AppendVarint(body, 1)  // agent delta 1 → agent 1
		return binary.AppendUvarint(body, 2) // seq 2
	}
	emptyRow := []byte{}
	fiveEntryHeader := []byte{0x05} // table claiming 5 entries, no bytes follow

	cases := []struct {
		name string
		in   []byte
		want string // error substring naming the intended failure path
	}{
		{
			name: "empty input",
			in:   nil,
			want: "bad magic", // zstd decodes empty input to an empty frame; the magic check rejects it
		},
		{
			name: "raw non-zstd bytes",
			in:   []byte("NOPE"),
			want: "binary: decompress:", // not a zstd frame; parser never reached
		},
		{
			name: "bad magic",
			in:   compress(append([]byte("NOPE"), emptyColumns(frameHeader(binaryVersion))...)),
			want: "bad magic",
		},
		{
			name: "version varint truncated",
			in:   compress(append(append([]byte{}, binaryMagic...), 0x80)), // continuation bit, then end of frame
			want: "truncated varint in version",
		},
		{
			name: "version varint overflow",
			in:   compress(append(append(append([]byte{}, binaryMagic...), 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff), 0x02)),
			want: "varint overflow in version",
		},
		{
			name: "unsupported version",
			// binaryVersion is 2; the frame claims 3.
			in:   compress(emptyColumns(frameHeader(binaryVersion + 1))),
			want: "unsupported version 3",
		},
		{
			name: "column bodyLen exceeds frame",
			in:   compress(append(frameHeader(binaryVersion), 0x0f)), // Types bodyLen 15, zero bytes follow
			want: "Types column truncated",
		},
		{
			name: "column count exceeds body",
			in:   compress(appendBinaryColumn(frameHeader(binaryVersion), binary.AppendUvarint(nil, 200))), // count 200, no entries in body
			want: "Types count 200 exceeds",
		},
		{
			name: "frame ends before Types bodyLen",
			in:   compress(frameHeader(binaryVersion)),
			want: "truncated varint in Types",
		},
		{
			name: "trailing bytes after Frontier",
			// A complete v2 frame (empty Coverage column) plus one stray byte.
			in:   compress(append(append(append([]byte{}, emptyColumns(frameHeader(binaryVersion))...), binary.AppendUvarint(nil, 0)...), 0x00)),
			want: "trailing bytes after Frontier",
		},
		{
			name: "TypeRuns run length 0",
			// Valid header, a Types column with one ins entry, then a
			// TypeRuns column declaring one run of length 0 — a shape
			// MarshalBinary never emits (a run exists only when at least
			// one op of that kind exists). All later columns are empty so
			// parsing reaches TypeRuns before any other invariant fires.
			in: compress(func() []byte {
				f := frameHeader(binaryVersion)
				f = appendBinaryColumn(f, binary.AppendUvarint(binary.AppendUvarint(nil, 1), 0)) // Types: count 1, one opTypeIns code
				f = appendBinaryColumn(f, binary.AppendUvarint(binary.AppendUvarint(nil, 1), 0)) // TypeRuns: count 1, run 0
				zero := binary.AppendUvarint(nil, 0)
				for range 8 {
					f = appendBinaryColumn(f, zero)
				}
				return f
			}()),
			want: "run length 0",
		},
		{
			name: "coverage row length exceeds remaining bytes",
			// One-op anchor frame whose single coverage row claims 15 bytes
			// that the frame does not hold.
			in:   compress(appendBinaryColumn(hostileCols(anchorAgent, 1), binary.AppendUvarint(binary.AppendUvarint(nil, 1), 0x0f))),
			want: "Coverage row 0 truncated",
		},
		{
			name: "coverage row on non-anchor op",
			// op 0 belongs to a real agent but carries the coverage row.
			in:   compress(appendBinaryColumn(hostileCols(5, 1), coverageRows(tableRow12()))),
			want: "coverage on non-anchor op",
		},
		{
			name: "coverage row on a later op",
			// The anchor op's row is legitimate; op 1 (non-anchor) carries
			// one too.
			in:   compress(appendBinaryColumn(hostileCols(anchorAgent, 2), coverageRows(tableRow12(), tableRow12()))),
			want: "coverage on non-anchor op",
		},
		{
			name: "anchor op without coverage",
			// The frame claims an anchor op but ships an empty coverage row
			// where the table should be.
			in:   compress(appendBinaryColumn(hostileCols(anchorAgent, 1), coverageRows(emptyRow))),
			want: "anchor op without coverage",
		},
		{
			name: "anchor op in anchorless coverage layout",
			// rowCount == ops+1 declares the anchorless layout, which has no
			// per-op row an anchor op could claim its coverage from.
			in:   compress(appendBinaryColumn(hostileCols(anchorAgent, 1), coverageRows(tableRow12(), emptyRow))),
			want: "anchor op without coverage",
		},
		{
			name: "compacted coverage without anchor",
			// A non-empty Coverage column means compacted, but with a real
			// agent at op 0 and an empty row there is no table anywhere.
			in:   compress(appendBinaryColumn(hostileCols(5, 1), coverageRows(emptyRow))),
			want: "coverage column without anchor coverage",
		},
		{
			name: "coverage row count mismatches op count",
			in:   compress(appendBinaryColumn(hostileCols(anchorAgent, 1), coverageRows(emptyRow, emptyRow, emptyRow, emptyRow, emptyRow))),
			want: "Coverage row count 5 != op count 1",
		},
		{
			name: "coverage entries exceed row body",
			// The anchor's table claims 5 entries; the row ends immediately.
			in:   compress(appendBinaryColumn(hostileCols(anchorAgent, 1), coverageRows(fiveEntryHeader))),
			want: "Coverage entries count 5 exceeds",
		},
		{
			name: "coverage column truncated",
			// Coverage bodyLen claims 32 bytes; the frame ends first.
			in:   compress(append(hostileCols(anchorAgent, 1), 0x20)),
			want: "Coverage column truncated",
		},
		{
			name: "v2 frame without coverage column",
			// Ten columns then end of frame: the Coverage bodyLen read hits
			// nothing. (Writers always append the column, empty or not.)
			in:   compress(hostileCols(anchorAgent, 1)),
			want: "truncated varint in Coverage",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := UnmarshalBinary[runeText](tc.in, RuneTextCodec{})
			if err == nil {
				t.Fatalf("expected error containing %q, got none", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestBinaryCompressionShrinks(t *testing.T) {
	// A repetitive corpus is required here: zstd's fixed frame overhead
	// exceeds the content bytes of a tiny log like buildBinaryTestLog's, so
	// the "smaller than content" pin needs a body worth compressing.
	d := NewRuneDocument(1)
	for range 100 {
		d.Ins(d.Len(), "Hello world ")
	}
	log := d.doc.opLog

	blob, err := MarshalBinary(log, RuneTextCodec{})
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if bytes.HasPrefix(blob, []byte(binaryMagic)) {
		t.Fatalf("blob is a bare EGW1 frame, not compressed")
	}
	if len(blob) >= contentBytesOf(t, log) {
		t.Fatalf("compressed blob %d bytes not smaller than content %d", len(blob), contentBytesOf(t, log))
	}
}

func contentBytesOf(tb testing.TB, log *opLog[runeText]) int {
	tb.Helper()
	n := 0
	for _, o := range log.ops {
		n += len([]byte(o.content))
	}
	return n
}
