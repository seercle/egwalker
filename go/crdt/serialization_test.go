package crdt

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func TestSerializationLossless(t *testing.T) {
	// Build a real concurrent history (no interior-character branches can exist
	// once runs collapse). Agent 1 types "Hello", which is a single run op.
	// Agent 2 syncs, deletes "lo" (a delete run) and types "p"; agent 1, which
	// never merged, appends "!" at its own end, so its run extends to "Hello!".
	// Merging produces a log holding a run op, a delete run, a single-char op
	// and a genuine run-boundary frontier.
	d1 := NewRuneDocument(1)
	d2 := NewRuneDocument(2)

	d1.Ins(0, "Hello")
	d2.MergeFrom(d1)
	d2.Del(3, 2) // delete "lo"
	d2.Ins(3, "p")
	d1.Ins(5, "!") // concurrent to d2's edits; extends d1's run
	d1.MergeFrom(d2)

	log := d1.opLog

	// Save original state for comparison
	originalOps := make([]op[rune, runeText], len(log.ops))
	copy(originalOps, log.ops)
	originalFrontier := make([]lv, len(log.frontier))
	copy(originalFrontier, log.frontier)

	// Marshal and Unmarshal
	data := log.Marshal()
	newLog := Unmarshal[rune, runeText](data)

	// 1. Check Ops
	if len(log.ops) != len(newLog.ops) {
		t.Errorf("Expected %d ops, got %d", len(log.ops), len(newLog.ops))
	}
	for i := range log.ops {
		if !reflect.DeepEqual(log.ops[i], newLog.ops[i]) {
			t.Errorf("Op %d mismatch.\nOriginal: %+v\nNew:      %+v", i, log.ops[i], newLog.ops[i])
		}
	}

	// 2. Check Frontier
	if !reflect.DeepEqual(log.frontier, newLog.frontier) {
		t.Errorf("Frontier mismatch. Expected %v, got %v", log.frontier, newLog.frontier)
	}

	// 3. Check Version (Vector Clock)
	if !reflect.DeepEqual(log.version, newLog.version) {
		t.Errorf("Version map mismatch. Expected %v, got %v", log.version, newLog.version)
	}

	// 4. Check ID Map
	if !reflect.DeepEqual(log.idToLV, newLog.idToLV) {
		t.Errorf("ID Map mismatch")
	}

	// The saved originals guard against accidental mutation during round-trip.
	if !reflect.DeepEqual(originalOps, log.ops) || !reflect.DeepEqual(originalFrontier, log.frontier) {
		t.Errorf("marshal mutated the source log")
	}
}

// estimateSize provides a rough byte count for comparison. Rows are run ops: a
// per-op fixed cost plus the run content bytes each op holds.
func estimateSize[E any, C content[E]](log *opLog[E, C], data *ColumnarData[E, C]) (int, int) {
	elemSize := int(unsafe.Sizeof(*new(E)))
	opSize := int(unsafe.Sizeof(op[E, C]{}))
	contentHeader := int(unsafe.Sizeof(*new(C)))

	// Row-based: ops slice overhead + each run-op struct + its opLV entry +
	// parents slice + run content bytes.
	rowSize := 0
	for _, o := range log.ops {
		rowSize += opSize
		rowSize += 8  // opLV entry
		rowSize += 24 // parents slice header
		rowSize += len(o.parents) * 8
		rowSize += contentHeader
		rowSize += len(o.content.Elems()) * elemSize
	}

	// Columnar-based: sum of all slices plus the content bytes.
	colSize := 0
	colSize += len(data.Types) * 8     // opType is string, approx 8-16 bytes
	colSize += len(data.TypeRuns) * 8  // int
	colSize += len(data.Agents) * 8    // int
	colSize += len(data.AgentRuns) * 8 // int
	colSize += len(data.Seqs) * 8      // int
	colSize += len(data.Positions) * 8 // int
	colSize += len(data.Lengths) * 8   // int
	colSize += len(data.Content) * contentHeader
	for _, c := range data.Content {
		colSize += len(c.Elems()) * elemSize
	}
	for _, p := range data.Parents {
		colSize += len(p) * 8
	}
	colSize += len(data.Frontier) * 8

	return rowSize, colSize
}

func TestCompressionRatio(t *testing.T) {
	log := newOpLog[rune, runeText]()

	// Scenario: One user typing a 10,000 character document collapses into a
	// single run op, so the columnar form holds one row plus the run content.
	const N = 10000
	localInsert(log, 1, 0, []rune(string(make([]rune, N))))

	if len(log.ops) != 1 {
		t.Fatalf("10k-char insert made %d ops, want 1 run op", len(log.ops))
	}

	data := log.Marshal()

	if len(data.Types) != 1 || len(data.TypeRuns) != 1 || data.TypeRuns[0] != 1 {
		t.Errorf("type RLE: got %d run(s) with lengths %v, want 1 run of 1 op", len(data.Types), data.TypeRuns)
	}
	if len(data.Agents) != 1 || len(data.AgentRuns) != 1 || data.AgentRuns[0] != 1 {
		t.Errorf("agent RLE: got %d run(s) with lengths %v, want 1 run of 1 op", len(data.Agents), data.AgentRuns)
	}
	if len(data.Lengths) != 1 || data.Lengths[0] != N {
		t.Errorf("lengths = %v, want a single run of %d chars", data.Lengths, N)
	}
	if len(data.Content) != 1 {
		t.Errorf("content column has %d entries, want 1 run", len(data.Content))
	}

	rowSize, colSize := estimateSize(log, data)

	t.Logf("Compression Results for a single %d-char run:", N)
	t.Logf("  Row-based (In-Memory): ~%d bytes", rowSize)
	t.Logf("  Columnar (Unpacked):   ~%d bytes", colSize)
	t.Logf("  Reduction:             %.2fx", float64(rowSize)/float64(colSize))

	if colSize >= rowSize {
		t.Errorf("columnar representation not smaller than row-based: col=%d, row=%d", colSize, rowSize)
	}
}

func TestMixedCompression(t *testing.T) {
	log := newOpLog[rune, runeText]()

	// Scenario: Two users alternating every 10 characters -> 100 agent runs of
	// one 10-char run op each (positions are non-adjacent, so no collapse).
	for i := range 100 {
		agent := (i % 2) + 1
		localInsert(log, agent, i*10, []rune(string(make([]rune, 10))))
	}

	data := log.Marshal()

	// 100 groups of alternating agents => exactly 100 agent runs of length 1.
	if len(data.Agents) != 100 || len(data.AgentRuns) != 100 {
		t.Fatalf("agent RLE: got %d agent run(s) / %d lengths, want 100 each", len(data.Agents), len(data.AgentRuns))
	}
	for i, r := range data.AgentRuns {
		if r != 1 {
			t.Errorf("agent run %d has length %d, want 1", i, r)
		}
	}
	// All ops are inserts, so type RLE stays a single run of 100 run ops.
	if len(data.Types) != 1 || len(data.TypeRuns) != 1 || data.TypeRuns[0] != 100 {
		t.Errorf("type RLE: got %v, want single run of 100 ops", data.TypeRuns)
	}
	for i, l := range data.Lengths {
		if l != 10 {
			t.Errorf("op %d length = %d, want 10", i, l)
		}
	}

	rowSize, colSize := estimateSize(log, data)

	t.Logf("Compression Results for alternating users (100 run ops):")
	t.Logf("  Row-based (In-Memory): ~%d bytes", rowSize)
	t.Logf("  Columnar (Unpacked):   ~%d bytes", colSize)
	t.Logf("  Reduction:             %.2fx", float64(rowSize)/float64(colSize))

	if colSize >= rowSize {
		t.Errorf("columnar representation not smaller than row-based: col=%d, row=%d", colSize, rowSize)
	}
}

func TestSerialization_RLEStructure(t *testing.T) {
	// One agent inserting 50 runes => a single run op, so every per-op column
	// has exactly one entry whose Lengths[0] is the full run length.
	log := newOpLog[rune, runeText]()
	const n = 50
	localInsert(log, 1, 0, []rune(string(make([]rune, n))))
	data := log.Marshal()
	if len(data.Types) != 1 || data.TypeRuns[0] != 1 {
		t.Errorf("type RLE: %v (len %d), want one run of 1 op", data.TypeRuns, len(data.TypeRuns))
	}
	if len(data.Agents) != 1 || data.AgentRuns[0] != 1 {
		t.Errorf("agent RLE: %v (len %d), want one run of 1 op", data.AgentRuns, len(data.AgentRuns))
	}
	if len(data.Positions) != 1 || len(data.Content) != 1 || len(data.Lengths) != 1 {
		t.Errorf("positions/content/lengths = %d/%d/%d, want 1 each", len(data.Positions), len(data.Content), len(data.Lengths))
	}
	if data.Lengths[0] != n {
		t.Errorf("run length = %d, want %d", data.Lengths[0], n)
	}
}

func TestSerialization_RoundTripCheckout(t *testing.T) {
	// Build a real document with inserts, deletes, and a concurrent branch.
	d1 := NewRuneDocument(1)
	d2 := NewRuneDocument(2)

	d1.Ins(0, "Hello")
	d2.MergeFrom(d1)
	d2.Del(1, 2) // remove 'e' and 'l' -> "Hlo"
	d2.Ins(d2.Len(), "y")
	d1.Ins(d1.Len(), "!") // concurrent to d2's edits
	d1.MergeFrom(d2)

	want := d1.GetString()

	round := Unmarshal[rune, runeText](d1.opLog.Marshal())
	if len(round.ops) != len(d1.opLog.ops) {
		t.Fatalf("round-trip op count = %d, want %d", len(round.ops), len(d1.opLog.ops))
	}

	var sb strings.Builder
	checkout(round).ForEach(func(r rune) { sb.WriteRune(r) })
	if got := sb.String(); got != want {
		t.Errorf("checkout after round-trip = %q, want %q", got, want)
	}
}
