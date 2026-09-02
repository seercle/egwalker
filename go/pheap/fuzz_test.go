package pheap

import (
	"math/rand"
	"testing"
)

func randBytes(seed int64, n int) []byte {
	r := rand.New(rand.NewSource(seed))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	return b
}

// FuzzHeap checks Push/Pop/Peek against a reference slice, asserting the
// heap always yields its maximum element.
func FuzzHeap(f *testing.F) {
	for _, s := range [][]byte{
		{},
		{5, 3, 8, 1, 9, 2, 4, 7, 6},
		{0, 9, 0, 8, 0, 7, 0, 6},
		randBytes(1, 64),
		randBytes(2, 128),
		randBytes(3, 256),
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		h := New[int]()
		var ref []int
		i := 0

		for i < len(data) {
			if len(ref) == 0 {
				// Must push.
				h.Push(int(data[i]))
				ref = append(ref, int(data[i]))
				i++
			} else if data[i]%2 == 0 {
				// Push.
				h.Push(int(data[i]))
				ref = append(ref, int(data[i]))
				i++
			} else {
				// Pop the maximum element.
				maxIdx := 0
				for j := 1; j < len(ref); j++ {
					if ref[j] > ref[maxIdx] {
						maxIdx = j
					}
				}
				want := ref[maxIdx]
				ref = append(ref[:maxIdx], ref[maxIdx+1:]...)

				got, ok := h.Pop()
				if !ok {
					t.Fatalf("Pop() on non-empty heap returned ok=false")
				}
				if got != want {
					t.Fatalf("Pop() = %d, want %d", got, want)
				}
				i++
			}

			if h.Size() != len(ref) {
				t.Fatalf("size mismatch after op %d: heap=%d, ref=%d", i, h.Size(), len(ref))
			}

			if len(ref) > 0 {
				peek, ok := h.Peek()
				if !ok {
					t.Fatalf("Peek() on non-empty heap returned ok=false")
				}
				maxIdx := 0
				for j := 1; j < len(ref); j++ {
					if ref[j] > ref[maxIdx] {
						maxIdx = j
					}
				}
				if peek != ref[maxIdx] {
					t.Fatalf("Peek() = %d, want %d", peek, ref[maxIdx])
				}
			}
		}
	})
}
