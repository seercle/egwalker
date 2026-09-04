package crdt

import (
	"bytes"
	"reflect"
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

func TestBinaryRejectsGarbage(t *testing.T) {
	good, err := MarshalBinary(newOpLog[runeText](), RuneTextCodec{})
	if err != nil {
		t.Fatalf("MarshalBinary empty log: %v", err)
	}
	cases := [][]byte{
		{},                                      // empty
		[]byte("NOPE"),                          // bad magic
		[]byte("EGW1\xff"),                      // truncated after magic
		[]byte("EGW1\x01"),                      // version only, no columns
		append([]byte("EGW1"), 0x02),            // version 2 -> unsupported
		[]byte("EGW1\x01\x01\x00"),              // empty Types column, then truncated
		[]byte("EGW1\x01\x02\x02\x00"),          // Types count 2 exceeds remaining body
		append(append([]byte{}, good...), 0x00), // trailing bytes after Frontier
	}
	for i, in := range cases {
		if _, err := UnmarshalBinary[runeText](in, RuneTextCodec{}); err == nil {
			t.Errorf("case %d (%q): expected error, got none", i, in)
		}
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

func contentBytesOf(t *testing.T, log *opLog[runeText]) int {
	t.Helper()
	n := 0
	for _, o := range log.ops {
		n += len([]byte(o.content))
	}
	return n
}
