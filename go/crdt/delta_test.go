package crdt

import (
	"encoding/binary"
	"reflect"
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

// === Hostile frames ===

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
