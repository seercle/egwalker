package bxtree

import (
	"math/rand"
	"testing"
)

func TestFirstLastPointers(t *testing.T) {
	tree := New[int]()

	// 1. Initial Insert
	tree.InsertAt(0, 100)
	if tree.first == nil || tree.last == nil {
		t.Fatal("First or Last pointer is nil after first insert")
	}
	if tree.first != tree.last {
		t.Fatal("First and Last should point to the same node after single insert")
	}

	// 2. Append elements to trigger splits (updates Last)
	// LEAF_MAX_SIZE is 128 (64*2). Let's insert enough to split a few times.
	count := 500
	for i := 1; i < count; i++ {
		err := tree.InsertAt(tree.Size(), 100+i)
		if err != nil {
			t.Fatalf("Insert failed at %d: %v", i, err)
		}

		// Verify Last pointer invariant
		// The last node should contain the last element
		lastNode := tree.last
		if lastNode == nil {
			t.Fatalf("Last pointer is nil at iteration %d", i)
		}
		if !lastNode.isLeaf {
			t.Fatalf("Last pointer points to internal node at iteration %d", i)
		}
		// The last item in the last node should be the item we just inserted
		if len(lastNode.items) == 0 {
			t.Fatalf("Last node is empty at iteration %d", i)
		}
		if lastNode.items[len(lastNode.items)-1] != 100+i {
			t.Errorf("Last node does not contain the last inserted item. Expected %d, got %d", 100+i, lastNode.items[len(lastNode.items)-1])
		}
	}

	// 3. Prepend elements (updates/verifies First)
	// We are inserting at 0.
	for i := 1; i < count; i++ {
		val := 100 - i
		err := tree.InsertAt(0, val)
		if err != nil {
			t.Fatalf("Prepend failed at %d: %v", i, err)
		}

		// Verify First pointer invariant
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

	// 4. Verify structural integrity by traversing to edges
	// Check Leftmost
	curr := tree.root
	for !curr.isLeaf {
		curr = curr.children[0]
	}
	if curr != tree.first {
		t.Errorf("Root traversal to leftmost leaf does not match tree.first")
	}

	// Check Rightmost
	curr = tree.root
	for !curr.isLeaf {
		curr = curr.children[len(curr.children)-1]
	}
	if curr != tree.last {
		t.Errorf("Root traversal to rightmost leaf does not match tree.last")
	}

	// 5. Verify data integrity
	// We inserted 100..599 (append) and 99..-399 (prepend)
	// Range is 100 to 599 (500 items)
	// Range is 99 down to 100-499 = -399 (499 items)
	// Total items = 1 + 499 + 499 = 999 items?
	// Initial: 100.
	// Append loop: 1 to 499. Inserted 101, 102... 100+499=599.
	// Prepend loop: 1 to 499. Inserted 99, 98... 100-499=-399.
	// Expected min: -399. Expected max: 599.

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

//

const (
	SmallSize  = 100
	MediumSize = 1_000
	LargeSize  = 100_000
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
			tree.InsertAt(tree.Size(), i)
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
		tree.InsertAt(tree.Size(), i)
	}
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		pos := rand.Intn(LargeSize)
		_, _ = tree.GetAt(pos)
	}
}
