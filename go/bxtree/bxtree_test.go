package bxtree

import (
	"testing"
)

func TestTest(t *testing.T) {
	tree := New[int, struct{}]()
	for i := 0; i < 129; i++ {
		tree.InsertAt(i, i)
	}
	tree.DeleteRange(1, 5)
	tree.Print()
	tree.InsertRange(1, []int{100, 101, 102, 103, 104})
	tree.Print()
}

func TestFirstLastPointers(t *testing.T) {
	tree := New[int, struct{}]()

	tree.InsertAt(0, 100)
	if tree.First == nil || tree.Last == nil {
		t.Fatal("First or Last pointer is nil after first insert")
	}
	if tree.First != tree.Last {
		t.Fatal("First and Last should point to the same node after single insert")
	}

	count := 500
	for i := 1; i < count; i++ {
		err := tree.InsertAt(tree.Size(), 100+i)
		if err != nil {
			t.Fatalf("Insert failed at %d: %v", i, err)
		}

		lastNode := tree.Last
		if lastNode == nil {
			t.Fatalf("Last pointer is nil at iteration %d", i)
		}
		if !lastNode.isLeaf {
			t.Fatalf("Last pointer points to internal node at iteration %d", i)
		}

		if len(lastNode.Items) == 0 {
			t.Fatalf("Last node is empty at iteration %d", i)
		}
		if lastNode.Items[len(lastNode.Items)-1] != 100+i {
			t.Errorf("Last node does not contain the last inserted item. Expected %d, got %d", 100+i, lastNode.Items[len(lastNode.Items)-1])
		}
	}

	for i := 1; i < count; i++ {
		val := 100 - i
		err := tree.InsertAt(0, val)
		if err != nil {
			t.Fatalf("Prepend failed at %d: %v", i, err)
		}

		firstNode := tree.First
		if firstNode == nil {
			t.Fatalf("First pointer is nil at prepend iteration %d", i)
		}
		if !firstNode.isLeaf {
			t.Fatalf("First pointer points to internal node at prepend iteration %d", i)
		}
		if len(firstNode.Items) == 0 {
			t.Fatalf("First node is empty at prepend iteration %d", i)
		}
		if firstNode.Items[0] != val {
			t.Errorf("First node does not contain the first inserted item. Expected %d, got %d", val, firstNode.Items[0])
		}
	}

	curr := tree.Root
	for !curr.isLeaf {
		curr = curr.children[0]
	}
	if curr != tree.First {
		t.Errorf("Root traversal to leftmost leaf does not match tree.First")
	}

	curr = tree.Root
	for !curr.isLeaf {
		curr = curr.children[len(curr.children)-1]
	}
	if curr != tree.Last {
		t.Errorf("Root traversal to rightmost leaf does not match tree.Last")
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
	tree := New[int, struct{}]()
	err := tree.DeleteAt(0)
	if err != ErrIndexOutOfBounds {
		t.Errorf("Expected ErrIndexOutOfBounds when deleting from empty tree, got %v", err)
	}
}

func TestDeleteOutOfBounds(t *testing.T) {
	tree := New[int, struct{}]()
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
	tree := New[int, struct{}]()
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
	tree := New[int, struct{}]()
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
	tree := New[int, struct{}]()
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
	tree := New[int, struct{}]()
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
	tree := New[int, struct{}]()
	count := 20
	for i := 0; i < count; i++ {
		tree.InsertAt(i, i)
	}

	for i := 0; i < 5; i++ {
		tree.DeleteAt(0)
	}

	if tree.First == nil {
		t.Fatal("tree.First is nil after deletions")
	}
	if tree.First.size == 0 {
		t.Fatal("tree.First is empty")
	}
	if tree.First.Items[0] != 5 {
		t.Errorf("tree.First item mismatch. Expected 5, got %d", tree.First.Items[0])
	}

	currentSize := tree.Size()
	for i := 0; i < 5; i++ {
		tree.DeleteAt(currentSize - 1 - i)
	}

	if tree.Last == nil {
		t.Fatal("tree.Last is nil after deletions")
	}
	if tree.Last.size == 0 {
		t.Fatal("tree.Last is empty")
	}
	expectedLast := count - 1 - 5
	actualLast := tree.Last.Items[tree.Last.size-1]
	if actualLast != expectedLast {
		t.Errorf("tree.Last item mismatch. Expected %d, got %d", expectedLast, actualLast)
	}
}
