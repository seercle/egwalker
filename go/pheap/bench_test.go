package pheap

import (
	"fmt"
	"math/rand"
	"testing"
)

var benchSizes = []int{1_000, 10_000, 100_000}

func BenchmarkPush(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(fmt.Sprintf("Size/%d", size), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				h := New[int]()
				b.StartTimer()
				for i := range size {
					h.Push(i)
				}
			}
		})
	}
}

func BenchmarkPop(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(fmt.Sprintf("Size/%d", size), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				h := New[int]()
				r := rand.New(rand.NewSource(42))
				for range size {
					h.Push(r.Int())
				}
				b.StartTimer()
				for range size {
					h.Pop()
				}
			}
		})
	}
}
