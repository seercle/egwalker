package crdt

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func TestSerializationLossless(t *testing.T) {
	log := newOpLog[rune]()

	// Create a complex history
	// User 1 types "Hello"
	localInsert(log, 1, 0, []rune("Hello"))

	// User 2 deletes "lo" and types "p"
	localDelete(log, 2, 3, 2)
	localInsert(log, 2, 3, []rune("p"))

	// User 1 types "!" at the end (concurrent to User 2's edits)
	// We manually simulate a branch by resetting the frontier
	log.frontier = []lv{4} // Pointing to the 'o' in Hello
	localInsert(log, 1, 5, []rune("!"))

	// Save original state for comparison
	originalOps := make([]op[rune], len(log.ops))
	copy(originalOps, log.ops)
	originalFrontier := make([]lv, len(log.frontier))
	copy(originalFrontier, log.frontier)

	// Marshal and Unmarshal
	data := log.Marshal()
	newLog := Unmarshal(data)

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
}

// estimateSize provides a rough byte count for comparison.
func estimateSize[T any](log *opLog[T], data *ColumnarData[T]) (int, int) {
	// Row-based: ops slice overhead + each op struct
	opSize := int(unsafe.Sizeof(op[T]{}))
	rowSize := len(log.ops) * opSize
	// Add overhead for parents slices (rough estimate)
	for _, o := range log.ops {
		rowSize += len(o.parents) * 8
	}

	// Columnar-based: sum of all slices
	colSize := 0
	colSize += len(data.Types) * 8     // opType is string, approx 8-16 bytes
	colSize += len(data.TypeRuns) * 8  // int
	colSize += len(data.Agents) * 8    // int
	colSize += len(data.AgentRuns) * 8 // int
	colSize += len(data.Seqs) * 8      // int
	colSize += len(data.Positions) * 8 // int
	colSize += len(data.Content) * int(unsafe.Sizeof(*new(T)))
	for _, p := range data.Parents {
		colSize += len(p) * 8
	}
	colSize += len(data.Frontier) * 8

	return rowSize, colSize
}

func TestCompressionRatio(t *testing.T) {
	log := newOpLog[rune]()

	// Scenario: One user typing a 10,000 character document
	const N = 10000
	localInsert(log, 1, 0, make([]rune, N))

	data := log.Marshal()

	// The implemented RLE must collapse all N identical types and agents
	// into a single run each.
	if len(data.Types) != 1 || len(data.TypeRuns) != 1 || data.TypeRuns[0] != N {
		t.Errorf("type RLE: got %d run(s) with lengths %v, want 1 run of length %d", len(data.Types), data.TypeRuns, N)
	}
	if len(data.Agents) != 1 || len(data.AgentRuns) != 1 || data.AgentRuns[0] != N {
		t.Errorf("agent RLE: got %d run(s) with lengths %v, want 1 run of length %d", len(data.Agents), data.AgentRuns, N)
	}

	rowSize, colSize := estimateSize(log, data)

	t.Logf("Compression Results for %d consecutive inserts:", N)
	t.Logf("  Row-based (In-Memory): ~%d bytes", rowSize)
	t.Logf("  Columnar (Unpacked):   ~%d bytes", colSize)
	t.Logf("  Reduction:             %.2fx", float64(rowSize)/float64(colSize))

	if colSize >= rowSize {
		t.Errorf("columnar representation not smaller than row-based: col=%d, row=%d", colSize, rowSize)
	}
}

func TestMixedCompression(t *testing.T) {
	log := newOpLog[rune]()

	// Scenario: Two users alternating every 10 characters -> 100 agent runs.
	for i := range 100 {
		agent := (i % 2) + 1
		localInsert(log, agent, i*10, make([]rune, 10))
	}

	data := log.Marshal()

	// 100 groups of 10 same-agent ops => exactly 100 agent runs of length 10.
	if len(data.Agents) != 100 || len(data.AgentRuns) != 100 {
		t.Fatalf("agent RLE: got %d agent run(s) / %d lengths, want 100 each", len(data.Agents), len(data.AgentRuns))
	}
	for i, r := range data.AgentRuns {
		if r != 10 {
			t.Errorf("agent run %d has length %d, want 10", i, r)
		}
	}
	// All ops are inserts, so type RLE stays a single run.
	if len(data.Types) != 1 || len(data.TypeRuns) != 1 || data.TypeRuns[0] != 1000 {
		t.Errorf("type RLE: got %v, want single run of 1000", data.TypeRuns)
	}

	rowSize, colSize := estimateSize(log, data)

	t.Logf("Compression Results for alternating users (1,000 total ops):")
	t.Logf("  Row-based (In-Memory): ~%d bytes", rowSize)
	t.Logf("  Columnar (Unpacked):   ~%d bytes", colSize)
	t.Logf("  Reduction:             %.2fx", float64(rowSize)/float64(colSize))

	if colSize >= rowSize {
		t.Errorf("columnar representation not smaller than row-based: col=%d, row=%d", colSize, rowSize)
	}
}

func TestSerialization_RLEStructure(t *testing.T) {
	// One agent inserting 50 runes => single type run and single agent run.
	log := newOpLog[rune]()
	const n = 50
	localInsert(log, 1, 0, make([]rune, n))
	data := log.Marshal()
	if len(data.Types) != 1 || data.TypeRuns[0] != n {
		t.Errorf("type RLE: %v (len %d), want one run of %d", data.TypeRuns, len(data.TypeRuns), n)
	}
	if len(data.Agents) != 1 || data.AgentRuns[0] != n {
		t.Errorf("agent RLE: %v (len %d), want one run of %d", data.AgentRuns, len(data.AgentRuns), n)
	}
	if len(data.Positions) != n || len(data.Content) != n {
		t.Errorf("positions/content length = %d/%d, want %d", len(data.Positions), len(data.Content), n)
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

	round := Unmarshal(d1.opLog.Marshal())
	if len(round.ops) != len(d1.opLog.ops) {
		t.Fatalf("round-trip op count = %d, want %d", len(round.ops), len(d1.opLog.ops))
	}

	var sb strings.Builder
	checkout(round).ForEach(func(r rune) { sb.WriteRune(r) })
	if got := sb.String(); got != want {
		t.Errorf("checkout after round-trip = %q, want %q", got, want)
	}
}
