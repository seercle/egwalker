package pheap

import (
	"slices"
	"testing"
)

func expectPanic(t *testing.T, name string, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("%s: expected panic, but it did not", name)
			return
		}
		if r != want {
			t.Errorf("%s: expected panic message %q, got %q", name, want, r)
		}
	}()
	f()
}

func TestPairingHeap(t *testing.T) {
	// Use as a max-heap (default)
	h := New[int]()

	if h.Size() != 0 {
		t.Errorf("New heap should have size 0, got %d", h.Size())
	}

	values := []int{5, 3, 8, 1, 9, 2, 4, 7, 6}
	for _, v := range values {
		h.Push(v)
	}

	if h.Size() != len(values) {
		t.Errorf("Heap size mismatch: got %d, want %d", h.Size(), len(values))
	}

	slices.Sort(values)
	slices.Reverse(values)

	for _, expected := range values {
		peek, ok := h.Peek()
		if !ok || peek != expected {
			t.Errorf("Peek() mismatch: got %d, want %d (ok: %v)", peek, expected, ok)
		}

		val, ok := h.Pop()
		if !ok || val != expected {
			t.Errorf("Pop() mismatch: got %d, want %d (ok: %v)", val, expected, ok)
		}
	}

	if h.Size() != 0 {
		t.Errorf("Heap size after pops mismatch: got %d, want 0", h.Size())
	}

	if _, ok := h.Pop(); ok {
		t.Error("Pop on empty heap should return ok=false")
	}
}

func TestMinHeap(t *testing.T) {
	// Use as a min-heap
	h := New(WithLess(func(a, b int) bool { return a > b }))

	values := []int{5, 3, 8, 1, 9, 2, 4, 7, 6}
	for _, v := range values {
		h.Push(v)
	}

	slices.Sort(values)

	for _, expected := range values {
		val, ok := h.Pop()
		if !ok || val != expected {
			t.Errorf("Pop() mismatch: got %d, want %d", val, expected)
		}
	}
}

func TestNilHeap(t *testing.T) {
	var h *PairingHeap[int]

	expectPanic(t, "Size", "pheap: Size called on nil heap", func() { h.Size() })
	expectPanic(t, "Push", "pheap: Push called on nil heap", func() { h.Push(1) })
	expectPanic(t, "Pop", "pheap: Pop called on nil heap", func() { h.Pop() })
	expectPanic(t, "Peek", "pheap: Peek called on nil heap", func() { h.Peek() })
}

func TestNewAny(t *testing.T) {
	type person struct {
		name string
		age  int
	}

	// Use as a max-heap by age
	h := NewAny(WithLess(func(a, b person) bool {
		return a.age < b.age
	}))

	h.Push(person{"Alice", 25})
	h.Push(person{"Bob", 30})
	h.Push(person{"Charlie", 20})

	val, _ := h.Pop()
	if val.name != "Bob" {
		t.Errorf("expected Bob, got %s", val.name)
	}

	expectPanic(t, "NewAnyWithoutLess", "pheap: less function must be provided for NewAny", func() {
		NewAny[int]()
	})
}

func TestPeekEmpty(t *testing.T) {
	h := New[int]()
	if v, ok := h.Peek(); ok || v != 0 {
		t.Errorf("Peek on empty heap = (%d, %v), want (0, false)", v, ok)
	}
}

func TestDuplicateValues(t *testing.T) {
	h := New[int]()
	for range 5 {
		h.Push(7)
		h.Push(3)
	}
	if h.Size() != 10 {
		t.Fatalf("size = %d, want 10", h.Size())
	}
	sevens := 0
	for h.Size() > 0 {
		v, ok := h.Pop()
		if !ok {
			t.Fatal("Pop on non-empty heap returned ok=false")
		}
		if v == 7 {
			sevens++
		}
	}
	if sevens != 5 {
		t.Errorf("popped %d sevens, want 5", sevens)
	}
}

func TestSingleElement(t *testing.T) {
	h := New[int]()
	h.Push(42)
	if v, _ := h.Pop(); v != 42 {
		t.Errorf("Pop = %d, want 42", v)
	}
	if _, ok := h.Pop(); ok {
		t.Error("Pop on empty heap should return ok=false")
	}
}
