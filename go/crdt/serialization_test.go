package crdt

import (
	"reflect"
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
	rowSize, colSize := estimateSize(log, data)

	t.Logf("Compression Results for %d consecutive inserts:", N)
	t.Logf("  Row-based (In-Memory): ~%d bytes", rowSize)
	t.Logf("  Columnar (Unpacked):   ~%d bytes", colSize)
	t.Logf("  Reduction:             %.2fx", float64(rowSize)/float64(colSize))

	// Even without bit-packing, the reduction is massive because we replaced
	// 10,000 "Type" and "Agent" fields with just 1 entry + 1 count.
}

func TestMixedCompression(t *testing.T) {
	log := newOpLog[rune]()

	// Scenario: Two users alternating every 10 characters
	for i := 0; i < 100; i++ {
		agent := (i % 2) + 1
		localInsert(log, agent, i*10, make([]rune, 10))
	}

	data := log.Marshal()
	rowSize, colSize := estimateSize(log, data)

	t.Logf("Compression Results for alternating users (1,000 total ops):")
	t.Logf("  Row-based (In-Memory): ~%d bytes", rowSize)
	t.Logf("  Columnar (Unpacked):   ~%d bytes", colSize)
	t.Logf("  Reduction:             %.2fx", float64(rowSize)/float64(colSize))
}
