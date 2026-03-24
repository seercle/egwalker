package bxtree

import (
	"fmt"
	"math/rand"
	"testing"
)

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
