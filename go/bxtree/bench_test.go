package bxtree

import (
	"math/rand"
	"testing"
)

const (
	SmallSize  = 1_000
	MediumSize = 10_000
	LargeSize  = 100_000
)

type List[T any] struct {
	items []T
}

func NewList[T any]() *List[T] {
	return &List[T]{items: make([]T, 0)}
}

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

func BenchmarkInsertRandom_Slice_Medium(b *testing.B) {
	for n := 0; n < b.N; n++ {
		list := NewList[int]()
		for i := 0; i < MediumSize; i++ {
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
			if tree.Size() > 0 {
				pos = rand.Intn(tree.Size())
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
			if tree.Size() > 0 {
				pos = rand.Intn(tree.Size())
			}
			tree.InsertAt(pos, i)
		}
	}
}

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
			tree.InsertAt(tree.Size(), i)
		}
	}
}

func BenchmarkReadRandom_Slice_Large(b *testing.B) {
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
	tree := New[int]()
	for i := 0; i < LargeSize; i++ {
		tree.InsertAt(tree.Size(), i)
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		pos := rand.Intn(LargeSize)
		_, _ = tree.GetAt(pos)
	}
}
