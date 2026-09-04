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

// frameHeader starts a hand-crafted frame: magic + uvarint version.
func frameHeader(version uint64) []byte {
	return binary.AppendUvarint(append([]byte{}, binaryMagic...), version)
}

// emptyColumns appends ten zero-count columns — the exact encoding of an
// empty log — completing a valid frame after frameHeader.
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
			in:   compress(emptyColumns(frameHeader(binaryVersion + 1))),
			want: "unsupported version 2",
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
			in:   compress(append(append([]byte{}, emptyColumns(frameHeader(binaryVersion))...), 0x00)),
			want: "trailing bytes after Frontier",
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
