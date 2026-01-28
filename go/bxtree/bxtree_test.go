package bxtree

import (
	"math/rand"
	"testing"
)

func TestFirstLastPointers(t *testing.T) {
	tree := New[int]()

	tree.InsertAt(0, 100)
	if tree.first == nil || tree.last == nil {
		t.Fatal("First or Last pointer is nil after first insert")
	}
	if tree.first != tree.last {
		t.Fatal("First and Last should point to the same node after single insert")
	}

	count := 500
	for i := 1; i < count; i++ {
		err := tree.InsertAt(tree.Size(), 100+i)
		if err != nil {
			t.Fatalf("Insert failed at %d: %v", i, err)
		}

		lastNode := tree.last
		if lastNode == nil {
			t.Fatalf("Last pointer is nil at iteration %d", i)
		}
		if !lastNode.isLeaf {
			t.Fatalf("Last pointer points to internal node at iteration %d", i)
		}

		if len(lastNode.items) == 0 {
			t.Fatalf("Last node is empty at iteration %d", i)
		}
		if lastNode.items[len(lastNode.items)-1] != 100+i {
			t.Errorf("Last node does not contain the last inserted item. Expected %d, got %d", 100+i, lastNode.items[len(lastNode.items)-1])
		}
	}

	for i := 1; i < count; i++ {
		val := 100 - i
		err := tree.InsertAt(0, val)
		if err != nil {
			t.Fatalf("Prepend failed at %d: %v", i, err)
		}

		firstNode := tree.first
		if firstNode == nil {
			t.Fatalf("First pointer is nil at prepend iteration %d", i)
		}
		if !firstNode.isLeaf {
			t.Fatalf("First pointer points to internal node at prepend iteration %d", i)
		}
		if len(firstNode.items) == 0 {
			t.Fatalf("First node is empty at prepend iteration %d", i)
		}
		if firstNode.items[0] != val {
			t.Errorf("First node does not contain the first inserted item. Expected %d, got %d", val, firstNode.items[0])
		}
	}

	curr := tree.root
	for !curr.isLeaf {
		curr = curr.children[0]
	}
	if curr != tree.first {
		t.Errorf("Root traversal to leftmost leaf does not match tree.first")
	}

	curr = tree.root
	for !curr.isLeaf {
		curr = curr.children[len(curr.children)-1]
	}
	if curr != tree.last {
		t.Errorf("Root traversal to rightmost leaf does not match tree.last")
	}

	val, err := tree.GetAt(0)
	if err != nil {
		t.Fatalf("GetAt(0) failed: %v", err)
	}
	if *val != 100-(count-1) {
		t.Errorf("First element incorrect. Expected %d, got %d", 100-(count-1), *val)
	}

	val, err = tree.GetAt(tree.Size() - 1)
	if err != nil {
		t.Fatalf("GetAt(last) failed: %v", err)
	}
	if *val != 100+(count-1) {
		t.Errorf("Last element incorrect. Expected %d, got %d", 100+(count-1), *val)
	}
}

func TestDeleteEmpty(t *testing.T) {
	tree := New[int]()
	err := tree.DeleteAt(0)
	if err != ErrIndexOutOfBounds {
		t.Errorf("Expected ErrIndexOutOfBounds when deleting from empty tree, got %v", err)
	}
}

func TestDeleteOutOfBounds(t *testing.T) {
	tree := New[int]()
	tree.InsertAt(0, 1)

	tests := []int{-1, 1, 5}
	for _, idx := range tests {
		err := tree.DeleteAt(idx)
		if err != ErrIndexOutOfBounds {
			t.Errorf("Expected ErrIndexOutOfBounds for index %d, got %v", idx, err)
		}
	}
}

func TestDeleteSingleItem(t *testing.T) {
	tree := New[int]()
	tree.InsertAt(0, 10)

	err := tree.DeleteAt(0)
	if err != nil {
		t.Fatalf("DeleteAt(0) failed: %v", err)
	}

	if tree.Size() != 0 {
		t.Errorf("Expected size 0, got %d", tree.Size())
	}

	err = tree.InsertAt(0, 20)
	if err != nil {
		t.Fatalf("Re-insertion after emptying failed: %v", err)
	}
	if val, _ := tree.GetAt(0); *val != 20 {
		t.Errorf("Re-insertion failed value check")
	}
}

func TestDeleteFromLeavesSimple(t *testing.T) {
	tree := New[int]()
	for i := 0; i < 5; i++ {
		tree.InsertAt(i, i)
	}

	tree.DeleteAt(2) // [0, 1, 3, 4]
	if tree.Size() != 4 {
		t.Errorf("Size incorrect after delete")
	}
	if v, _ := tree.GetAt(2); *v != 3 {
		t.Errorf("Index 2 is wrong after delete, expected 3, got %d", *v)
	}

	tree.DeleteAt(3) // [0, 1, 3]
	if v, _ := tree.GetAt(2); *v != 3 {
		t.Errorf("Last element wrong, expected 3, got %d", *v)
	}

	tree.DeleteAt(0) // [1, 3]
	if v, _ := tree.GetAt(0); *v != 1 {
		t.Errorf("First element wrong, expected 1, got %d", *v)
	}
}

func TestDeleteMergesAndBorrows(t *testing.T) {
	tree := New[int]()
	count := 50

	for i := 0; i < count; i++ {
		tree.InsertAt(i, i)
	}

	initialSize := tree.Size()

	for i := 0; i < count; i++ {
		err := tree.DeleteAt(0)
		if err != nil {
			t.Fatalf("DeleteAt(0) iteration %d failed: %v", i, err)
		}

		expectedSize := initialSize - 1 - i
		if tree.Size() != expectedSize {
			t.Fatalf("Size mismatch at iteration %d. Expected %d, got %d", i, expectedSize, tree.Size())
		}

		if expectedSize > 0 {
			val, _ := tree.GetAt(0)
			if *val != i+1 {
				t.Fatalf("Data corruption at iteration %d. Expected head to be %d, got %d", i, i+1, *val)
			}
		}
	}
}

func TestDeleteReverse(t *testing.T) {
	tree := New[int]()
	count := 50
	for i := 0; i < count; i++ {
		tree.InsertAt(i, i)
	}

	for i := 0; i < count; i++ {
		err := tree.DeleteAt(tree.Size() - 1)
		if err != nil {
			t.Fatalf("DeleteAt(last) iteration %d failed: %v", i, err)
		}

		if tree.Size() > 0 {
			val, _ := tree.GetAt(tree.Size() - 1)
			expected := count - i - 2
			if *val != expected {
				t.Errorf("Data corruption deleting from end. Expected last %d, got %d", expected, *val)
			}
		}
	}
}

func TestFirstLastPointersAfterDelete(t *testing.T) {
	tree := New[int]()
	count := 20
	for i := 0; i < count; i++ {
		tree.InsertAt(i, i)
	}

	for i := 0; i < 5; i++ {
		tree.DeleteAt(0)
	}

	if tree.first == nil {
		t.Fatal("tree.first is nil after deletions")
	}
	if tree.first.size == 0 {
		t.Fatal("tree.first is empty")
	}
	if tree.first.items[0] != 5 {
		t.Errorf("tree.first item mismatch. Expected 5, got %d", tree.first.items[0])
	}

	currentSize := tree.Size()
	for i := 0; i < 5; i++ {
		tree.DeleteAt(currentSize - 1 - i)
	}

	if tree.last == nil {
		t.Fatal("tree.last is nil after deletions")
	}
	if tree.last.size == 0 {
		t.Fatal("tree.last is empty")
	}
	expectedLast := count - 1 - 5
	actualLast := tree.last.items[tree.last.size-1]
	if actualLast != expectedLast {
		t.Errorf("tree.last item mismatch. Expected %d, got %d", expectedLast, actualLast)
	}
}

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
