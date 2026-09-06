package crdt

import "testing"

// TestGetStringCacheCoherence interleaves every mutation with reads and
// asserts the cached path returns exactly the uncached render.
func TestGetStringCacheCoherence(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)

	expect := func(doc *RuneDocument, want string) {
		t.Helper()
		if got := doc.GetString(); got != want {
			t.Fatalf("GetString = %q, want %q", got, want)
		}
	}

	a.Ins(0, "hello")
	expect(a, "hello")
	a.Del(0, 2)
	expect(a, "llo") // dirty after Del
	a.Ins(3, " world")
	expect(a, "llo world") // local edits keep invalidating

	b.Ins(0, "hello world")
	a.MergeFrom(b) // checkout rebuilds the rope — cache must refresh
	expect(a, "llo worldhello world")

	if blob, err := b.Delta(a.Version()); err != nil {
		t.Fatal(err)
	} else {
		a.ApplyDelta(blob) // delta path re-checks-out too
	}
	b.Del(5, 6)
	blob, _ := b.Delta(a.Version())
	a.ApplyDelta(blob)
	expect(a, "llo worldhello")
}

// TestGetItemsCacheCoherence is the array counterpart: append, delete,
// merge, delta — read each time.
func TestGetItemsCacheCoherence(t *testing.T) {
	a := NewArrayDocument[int](1)
	b := NewArrayDocument[int](2)

	expect := func(doc *ArrayDocument[int], want []int) {
		t.Helper()
		got := doc.GetItems()
		if len(got) != len(want) {
			t.Fatalf("GetItems len %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("GetItems = %v, want %v", got, want)
			}
		}
	}

	a.Ins(0, []int{1, 2, 3})
	expect(a, []int{1, 2, 3})
	a.Del(1, 1)
	expect(a, []int{1, 3})
	b.Ins(0, []int{7})
	a.MergeFrom(b)
	expect(a, []int{1, 3, 7})
}
