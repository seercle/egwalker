package bxtree

import (
	"fmt"
	"math/rand"
	"testing"
)

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

func TestBxTree_RandomOperations(t *testing.T) {
	seed := int64(42)
	rng := rand.New(rand.NewSource(seed))
	tree := New[int, struct{}]()
	var mirror []int

	for i := 0; i < 5000; i++ {
		op := rng.Intn(3)
		if len(mirror) == 0 {
			op = 0 // Must insert if empty
		}

		switch op {
		case 0: // Insert
			val := rng.Intn(10000)
			pos := rng.Intn(len(mirror) + 1)
			err := tree.InsertAt(pos, val)
			if err != nil {
				t.Fatalf("InsertAt(%d, %d) failed: %v", pos, val, err)
			}
			mirror = append(mirror[:pos], append([]int{val}, mirror[pos:]...)...)

		case 1: // Delete
			pos := rng.Intn(len(mirror))
			err := tree.DeleteAt(pos)
			if err != nil {
				t.Fatalf("DeleteAt(%d) failed: %v", pos, err)
			}
			mirror = append(mirror[:pos], mirror[pos+1:]...)

		case 2: // Get/Verify
			pos := rng.Intn(len(mirror))
			val, err := tree.GetAt(pos)
			if err != nil {
				t.Fatalf("GetAt(%d) failed: %v", pos, err)
			}
			if *val != mirror[pos] {
				t.Fatalf("Value mismatch at %d: expected %d, got %d", pos, mirror[pos], *val)
			}
		}

		if tree.Size() != len(mirror) {
			t.Fatalf("Size mismatch: tree=%d, mirror=%d", tree.Size(), len(mirror))
		}
	}

	// Final verification via ForEach
	idx := 0
	tree.ForEach(func(item int) {
		if idx >= len(mirror) {
			t.Fatalf("ForEach went out of bounds at index %d", idx)
		}
		if item != mirror[idx] {
			t.Fatalf("ForEach mismatch at index %d: expected %d, got %d", idx, mirror[idx], item)
		}
		idx++
	})
}

func TestBxTree_BulkInsertDelete(t *testing.T) {
	tree := New[int, struct{}]()
	count := 1000
	items := make([]int, count)
	for i := 0; i < count; i++ {
		items[i] = i
	}

	// Bulk insert
	err := tree.InsertRange(0, items)
	if err != nil {
		t.Fatalf("InsertRange failed: %v", err)
	}

	if tree.Size() != count {
		t.Fatalf("Expected size %d, got %d", count, tree.Size())
	}

	// Verify all
	tree.ForEach(func(item int) {
		if item < 0 || item >= count {
			t.Errorf("Unexpected item in tree: %d", item)
		}
	})

	// Bulk delete from middle
	delLen := 200
	delStart := 400
	err = tree.DeleteRange(delStart, delLen)
	if err != nil {
		t.Fatalf("DeleteRange failed: %v", err)
	}

	expectedSize := count - delLen
	if tree.Size() != expectedSize {
		t.Fatalf("Expected size %d, got %d", expectedSize, tree.Size())
	}

	// Verify items after deletion
	for i := 0; i < tree.Size(); i++ {
		val, _ := tree.GetAt(i)
		if i < delStart {
			if *val != i {
				t.Errorf("Mismatch before deletion range at %d: expected %d, got %d", i, i, *val)
			}
		} else {
			if *val != i+delLen {
				t.Errorf("Mismatch after deletion range at %d: expected %d, got %d", i, i+delLen, *val)
			}
		}
	}
}

func TestBxTree_BoundaryStability(t *testing.T) {
	// Test very small max sizes to trigger splits/merges frequently
	// Note: This assumes we could configure LeafMaxSize/InternalMaxSize,
	// but since they are constants in types.go, we just push enough data
	// to ensure depth > 1.

	tree := New[string, struct{}]()

	// Prepend many items
	for i := 0; i < 1000; i++ {
		tree.InsertAt(0, fmt.Sprintf("val-%d", i))
	}

	// Append many items
	for i := 0; i < 1000; i++ {
		tree.InsertAt(tree.Size(), fmt.Sprintf("end-%d", i))
	}

	// Delete from both ends
	for i := 0; i < 500; i++ {
		tree.DeleteAt(0)
		tree.DeleteAt(tree.Size() - 1)
	}

	expectedSize := 1000 // (1000 + 1000) - (500 + 500)
	if tree.Size() != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, tree.Size())
	}

	// Verify linked list integrity
	curr := tree.First
	visitedCount := 0
	for curr != nil {
		visitedCount += len(curr.Items)
		if curr.next == nil && curr != tree.Last {
			t.Error("Reached end of list but tree.Last is not set to this node")
		}
		curr = curr.next
	}

	if visitedCount != tree.Size() {
		t.Errorf("Linked list traversal count %d does not match tree size %d", visitedCount, tree.Size())
	}
}
