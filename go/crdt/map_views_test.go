package crdt

import "testing"

// TestMapKeysFromKeyIndex pins Keys() semantics under the keyIndex
// implementation: every key ever Set is present, exactly once, regardless
// of how many re-Sets or merges happened.
func TestMapKeysFromKeyIndex(t *testing.T) {
	a := NewMapDocument[string, int](1)
	for i := 0; i < 100; i++ {
		a.Set("k", i) // 100 bindings of ONE key, concurrent winners too
	}
	b := NewMapDocument[string, int](2)
	b.Set("other", 42)
	a.MergeFrom(b)

	seen := map[string]bool{}
	keys := a.Keys()
	if len(keys) != 2 {
		t.Fatalf("Keys() = %v, want exactly 2 keys", keys)
	}
	for _, k := range keys {
		seen[k] = true
	}
	if !seen["k"] || !seen["other"] {
		t.Fatalf("Keys() missing keys: %v", keys)
	}
}

// TestMapWinnerCacheEquivalence for a key with a rich binding history:
// uncached Get (recompute from scratch) must equal cached Get, through
// re-Sets, merges, and racing concurrent writes. The oracle re-derives the
// winner on a FRESH map document merged from the populated one: its first
// Get populates the cache from the recompute path, so the fresh map's Get
// IS the recompute-path answer; the original document's Get (after prior
// Gets) takes the cached path. Both must agree.
func TestMapWinnerCacheEquivalence(t *testing.T) {
	a := NewMapDocument[string, int](1)
	b := NewMapDocument[string, int](2)
	for i := 0; i < 50; i++ {
		a.Set("k", i)
		a.Set("hot", i*2)
	}
	for i := 0; i < 50; i++ {
		b.Set("k", 1000+i)
	}
	a.MergeFrom(b) // concurrent bindings on "k"

	// Oracle: a fresh replica whose first Get takes the recompute path.
	oracle := NewMapDocument[string, int](1)
	oracle.MergeFrom(a)
	wantV, wantOK := oracle.Get("k")

	// cached path: several Gets in a row, interleaved with a new Set
	gotV, gotOK := a.Get("k")
	if gotOK != wantOK || gotV != wantV {
		t.Fatalf("cached Get = (%v,%v), want (%v,%v)", gotV, gotOK, wantV, wantOK)
	}
	a.Set("k", 7) // epoch bump
	if v, ok := a.Get("k"); !ok || v != 7 {
		t.Fatalf("post-Set Get = (%v,%v), want (7,true)", v, ok)
	}
	a.Set("k", 9)
	a.Set("k", 9)
	if v, _ := a.Get("k"); v != 9 {
		t.Fatalf("repeat cache hit wrong winner: %v", v)
	}
}
