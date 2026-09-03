package crdt

import (
	"testing"
)

// TestOpContentSmoke checks the run-content seam: a 3-rune insert still fans
// out to one op per character (behavior preserved), each carrying a length-1
// run. The RLE optimization lands later and collapses this to a single run op,
// at which point this test is updated.
func TestOpContentSmoke(t *testing.T) {
	log := newOpLog[rune, runeText]()
	localInsert(log, 1, 0, runeText("abc"))
	if got := len(log.ops); got != 3 { // behavior preserved: one op per char, for now
		t.Fatalf("seam smoke: want 3 single-char ops, got %d", got)
	}
}
