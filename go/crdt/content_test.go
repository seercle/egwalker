package crdt

import (
	"testing"
	"unicode/utf8"
)

// TestRuneTextLenCountsRunes pins rune-count semantics across ASCII, 2-, 3-,
// and 4-byte runes: Len must count runes, not bytes, and must agree with the
// stdlib counter.
func TestRuneTextLenCountsRunes(t *testing.T) {
	s := runeText("aé中\U0001F600b") // 5 runes, 11 bytes
	if s.Len() != 5 {
		t.Fatalf("Len=%d, want 5", s.Len())
	}
	if s.Len() != utf8.RuneCountInString(string(s)) {
		t.Fatalf("Len disagrees with utf8.RuneCountInString")
	}
	if n := len(string(s)); n != 11 {
		t.Fatalf("sanity: byte length %d, want 11 (test data changed)", n)
	}
}

// TestRuneTextSplitAtBoundaries pins split semantics at every rune boundary of
// mixed-width content: halves must reassemble exactly, lengths must add up,
// and the edge cases k=0 and k=Len must be the identity splits.
func TestRuneTextSplitAtBoundaries(t *testing.T) {
	s := runeText("aé中\U0001F600b")
	for k := 0; k <= s.Len(); k++ {
		l, r := s.SplitAt(k)
		if l.Len()+r.Len() != s.Len() {
			t.Fatalf("SplitAt(%d): %d+%d != %d", k, l.Len(), r.Len(), s.Len())
		}
		if string(l)+string(r) != string(s) {
			t.Fatalf("SplitAt(%d): halves %q+%q != %q", k, l, r, s)
		}
	}
	if l, r := s.SplitAt(0); l != "" || r != s {
		t.Fatalf("SplitAt(0) not identity")
	}
	if l, r := s.SplitAt(s.Len()); l != s || r != "" {
		t.Fatalf("SplitAt(Len) not identity")
	}
}

// TestRuneTextSplitAtInvalidUTF8 pins the raw-byte-preservation semantics:
// each invalid byte counts as one rune and is preserved as a raw byte in its
// half (the rune sequence matches the old []rune round-trip, which re-encoded
// them as U+FFFD).
func TestRuneTextSplitAtInvalidUTF8(t *testing.T) {
	bad := runeText("\xffx\xfe")
	if bad.Len() != 3 {
		t.Fatalf("Len=%d, want 3 (invalid bytes count one rune each)", bad.Len())
	}
	l, r := bad.SplitAt(2)
	if string(l) != "\xffx" || string(r) != "\xfe" {
		t.Fatalf("SplitAt(2) = %q, %q; raw bytes must be preserved", l, r)
	}
}
