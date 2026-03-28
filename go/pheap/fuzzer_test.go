package pheap

import (
	"math/rand"
	"slices"
	"testing"
)

func TestFuzzHeap(t *testing.T) {
	seedCount := 50
	for seed := range seedCount {
		r := rand.New(rand.NewSource(int64(seed)))
		h := New[int]()
		var reference []int

		for op := range 200 {
			if len(reference) == 0 || r.Float64() < 0.7 {
				// Push
				val := r.Intn(1000)
				h.Push(val)
				reference = append(reference, val)
			} else {
				// Pop
				slices.Sort(reference)
				maxVal := reference[len(reference)-1]
				reference = reference[:len(reference)-1]

				val, ok := h.Pop()
				if !ok {
					t.Fatalf("Seed %d, Op %d: expected ok=true", seed, op)
				}
				if val != maxVal {
					t.Fatalf("Seed %d, Op %d: expected %d, got %d", seed, op, maxVal, val)
				}
			}

			if h.Size() != len(reference) {
				t.Fatalf("Seed %d, Op %d: size mismatch: heap=%d, reference=%d", seed, op, h.Size(), len(reference))
			}

			if len(reference) > 0 {
				peek, ok := h.Peek()
				if !ok {
					t.Fatalf("Seed %d, Op %d: expected peek ok=true", seed, op)
				}
				slices.Sort(reference)
				if peek != reference[len(reference)-1] {
					t.Fatalf("Seed %d, Op %d: peek mismatch: got %d, want %d", seed, op, peek, reference[len(reference)-1])
				}
			}
		}
	}
}
