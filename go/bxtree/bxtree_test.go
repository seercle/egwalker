package bxtree

import (
	"math/rand"
	"testing"
)

const (
	SmallSize  = 100
	MediumSize = 1_000
	LargeSize  = 10_000
)

type List[T any] struct {
	items []T
}

func NewList[T any]() *List[T] {
	return &List[T]{items: make([]T, 0)}
}

// Slice InsertAt: Requires shifting all elements after index
func (l *List[T]) InsertAt(index int, item T) {
	if index == len(l.items) {
		l.items = append(l.items, item)
		return
	}
	l.items = append(l.items[:index+1], l.items[index:]...)
	l.items[index] = item
}

func (l *List[T]) GetAt(index int) T {
	return l.items[index]
}

// --- Benchmarks ---

// 1. Insert at Random Positions (The B-Tree Killer Feature)
// Slices are O(N) for insert-at-middle. B-Trees are O(log N).

func BenchmarkInsertRandom_Slice_Medium(b *testing.B) {
	for n := 0; n < b.N; n++ {
		list := NewList[int]()
		for i := 0; i < MediumSize; i++ {
			// Insert at random position between 0 and current length
			pos := 0
			if len(list.items) > 0 {
				pos = rand.Intn(len(list.items))
			}
			list.InsertAt(pos, i)
		}
	}
}

func BenchmarkInsertRandom_BxTree_Medium(b *testing.B) {
	for n := 0; n < b.N; n++ {
		tree := New[int]()
		for i := 0; i < MediumSize; i++ {
			pos := 0
			if tree.size > 0 {
				pos = rand.Intn(tree.size)
			}
			tree.InsertAt(pos, i)
		}
	}
}

func BenchmarkInsertRandom_Slice_Large(b *testing.B) {
	for n := 0; n < b.N; n++ {
		list := NewList[int]()
		for i := 0; i < LargeSize; i++ {
			pos := 0
			if len(list.items) > 0 {
				pos = rand.Intn(len(list.items))
			}
			list.InsertAt(pos, i)
		}
	}
}

func BenchmarkInsertRandom_BxTree_Large(b *testing.B) {
	for n := 0; n < b.N; n++ {
		tree := New[int]()
		for i := 0; i < LargeSize; i++ {
			pos := 0
			if tree.size > 0 {
				pos = rand.Intn(tree.size)
			}
			tree.InsertAt(pos, i)
		}
	}
}

// 2. Append (Insert at End)
// Slices are extremely fast at this (amortized O(1)). B-Trees are slower.

func BenchmarkAppend_Slice_Large(b *testing.B) {
	for n := 0; n < b.N; n++ {
		list := NewList[int]()
		for i := 0; i < LargeSize; i++ {
			list.InsertAt(len(list.items), i)
		}
	}
}

func BenchmarkAppend_BxTree_Large(b *testing.B) {
	for n := 0; n < b.N; n++ {
		tree := New[int]()
		for i := 0; i < LargeSize; i++ {
			tree.InsertAt(tree.size, i)
		}
	}
}

// 3. Random Read Access
// Slices are O(1). B-Trees are O(log N).

func BenchmarkReadRandom_Slice_Large(b *testing.B) {
	// Setup
	list := NewList[int]()
	for i := 0; i < LargeSize; i++ {
		list.InsertAt(len(list.items), i)
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		pos := rand.Intn(LargeSize)
		_ = list.GetAt(pos)
	}
}

func BenchmarkReadRandom_BxTree_Large(b *testing.B) {
	// Setup
	tree := New[int]()
	for i := 0; i < LargeSize; i++ {
		tree.InsertAt(tree.size, i)
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		pos := rand.Intn(LargeSize)
		_, _ = tree.GetAt(pos)
	}
}
