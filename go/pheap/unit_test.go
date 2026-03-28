package pheap

import (
	"slices"
	"testing"
)

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

	expectPanic := func(name string, expectedMessage string, f func()) {
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("%s: expected panic, but it did not", name)
				return
			}
			if r != expectedMessage {
				t.Errorf("%s: expected panic message %q, got %q", name, expectedMessage, r)
			}
		}()
		f()
	}

	expectPanic("Size", "pheap: Size called on nil heap", func() { h.Size() })
	expectPanic("Push", "pheap: Push called on nil heap", func() { h.Push(1) })
	expectPanic("Pop", "pheap: Pop called on nil heap", func() { h.Pop() })
	expectPanic("Peek", "pheap: Peek called on nil heap", func() { h.Peek() })
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

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for NewAny without WithLess")
		}
	}()
	NewAny[int]()
}
