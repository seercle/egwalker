package bxtree

import (
	"fmt"
	"math/rand"
	"testing"
)

// Sizes for benchmarks
var benchSizes = []int{1_000, 10_000, 100_000}

func BenchmarkInsertRandom(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(fmt.Sprintf("Slice/%d", size), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				s := make([]int, 0, size)
				r := rand.New(rand.NewSource(42))
				b.StartTimer()
				for i := range size {
					pos := 0
					if len(s) > 0 {
						pos = r.Intn(len(s))
					}
					s = append(s[:pos], append([]int{i}, s[pos:]...)...)
				}
			}
		})

		b.Run(fmt.Sprintf("BxTree/%d", size), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				tree := New[int, struct{}]()
				r := rand.New(rand.NewSource(42))
				b.StartTimer()
				for i := range size {
					pos := 0
					if tree.Size() > 0 {
						pos = r.Intn(tree.Size())
					}
					tree.InsertAt(pos, i)
				}
			}
		})
	}
}

func BenchmarkDeleteRandom(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(fmt.Sprintf("Slice/%d", size), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				s := make([]int, size)
				for i := range s {
					s[i] = i
				}
				r := rand.New(rand.NewSource(42))
				b.StartTimer()
				for len(s) > 0 {
					pos := r.Intn(len(s))
					s = append(s[:pos], s[pos+1:]...)
				}
			}
		})

		b.Run(fmt.Sprintf("BxTree/%d", size), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				tree := New[int, struct{}]()
				for i := range size {
					tree.InsertAt(i, i)
				}
				r := rand.New(rand.NewSource(42))
				b.StartTimer()
				for tree.Size() > 0 {
					pos := r.Intn(tree.Size())
					tree.DeleteAt(pos)
				}
			}
		})
	}
}

func BenchmarkReadRandom(b *testing.B) {
	size := 100_000
	b.Run("Slice", func(b *testing.B) {
		s := make([]int, size)
		r := rand.New(rand.NewSource(42))
		b.ResetTimer()
		for b.Loop() {
			_ = s[r.Intn(size)]
		}
	})

	b.Run("BxTree", func(b *testing.B) {
		tree := New[int, struct{}]()
		for i := range size {
			tree.InsertAt(i, i)
		}
		r := rand.New(rand.NewSource(42))
		b.ResetTimer()
		for b.Loop() {
			_, _ = tree.GetAt(r.Intn(size))
		}
	})
}

func BenchmarkIteration(b *testing.B) {
	size := 100_000
	b.Run("Slice", func(b *testing.B) {
		s := make([]int, size)
		b.ResetTimer()
		for b.Loop() {
			sum := 0
			for _, v := range s {
				sum += v
			}
			_ = sum
		}
	})

	b.Run("BxTree", func(b *testing.B) {
		tree := New[int, struct{}]()
		for i := range size {
			tree.InsertAt(i, i)
		}
		b.ResetTimer()
		for b.Loop() {
			sum := 0
			tree.ForEach(func(i int) {
				sum += i
			})
			_ = sum
		}
	})
}

func BenchmarkSummaryOverhead(b *testing.B) {
	size := 50_000
	b.Run("NoSummary", func(b *testing.B) {
		for b.Loop() {
			tree := New[int, struct{}]()
			for i := range size {
				tree.InsertAt(i, i)
			}
		}
	})

	b.Run("WithSummary", func(b *testing.B) {
		config := &SummaryConfig[int, int]{
			FromItem: func(i int) int { return 1 },
			Add:      func(a, b int) int { return a + b },
			Sub:      func(a, b int) int { return a - b },
		}
		for b.Loop() {
			tree := New[int, int]()
			tree.SummaryConfig = config
			for i := range size {
				tree.InsertAt(i, i)
			}
		}
	})
}

func BenchmarkRangeOperations(b *testing.B) {
	size := 100_000
	chunkSize := 500

	b.Run("InsertRange", func(b *testing.B) {
		items := make([]int, chunkSize)
		for b.Loop() {
			tree := New[int, struct{}]()
			for i := 0; i < size/chunkSize; i++ {
				tree.InsertRange(tree.Size(), items)
			}
		}
	})

	b.Run("DeleteRange", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			tree := New[int, struct{}]()
			for i := range size {
				tree.InsertAt(i, i)
			}
			b.StartTimer()
			for tree.Size() > 0 {
				toDel := min(chunkSize, tree.Size())
				tree.DeleteRange(0, toDel)
			}
		}
	})
}
