package bxtree

import (
	"math/rand"
	"reflect"
	"slices"
	"testing"
)

// Helpers
func setupCountSummary() *SummaryConfig[int, int] {
	return &SummaryConfig[int, int]{
		FromItem: func(item int) int { return 1 },
		Add:      func(a, b int) int { return a + b },
		Sub:      func(a, b int) int { return a - b },
	}
}

func TestNew(t *testing.T) {
	tree := New[int, struct{}]()
	if tree.Size() != 0 {
		t.Errorf("New tree should have size 0, got %d", tree.Size())
	}
	if tree.Root != nil || tree.First != nil || tree.Last != nil {
		t.Error("New tree should have nil pointers")
	}
}

func TestIndexOutOfBounds(t *testing.T) {
	tree := New[int, struct{}]()

	t.Run("EmptyTree", func(t *testing.T) {
		if _, err := tree.GetAt(0); err != ErrIndexOutOfBounds {
			t.Errorf("GetAt(0) expected index out of bounds, got %v", err)
		}
		if err := tree.DeleteAt(0); err != ErrIndexOutOfBounds {
			t.Errorf("DeleteAt(0) expected index out of bounds, got %v", err)
		}
	})

	tree.InsertAt(0, 10) // Size 1

	t.Run("NonEmptyTree", func(t *testing.T) {
		tests := []int{-1, 1, 10}
		for _, idx := range tests {
			if _, err := tree.GetAt(idx); err != ErrIndexOutOfBounds {
				t.Errorf("GetAt(%d) expected index out of bounds, got %v", idx, err)
			}
			if err := tree.DeleteAt(idx); err != ErrIndexOutOfBounds {
				t.Errorf("DeleteAt(%d) expected index out of bounds, got %v", idx, err)
			}
		}
	})
}

func TestInsert(t *testing.T) {
	tree := New[int, struct{}]()

	t.Run("Sequential", func(t *testing.T) {
		for i := range 100 {
			if err := tree.InsertAt(i, i); err != nil {
				t.Fatalf("InsertAt(%d) failed: %v", i, err)
			}
		}
		if tree.Size() != 100 {
			t.Errorf("Expected size 100, got %d", tree.Size())
		}
	})

	t.Run("Prepend", func(t *testing.T) {
		tree := New[int, struct{}]()
		for i := range 100 {
			tree.InsertAt(0, i)
		}
		val, _ := tree.GetAt(0)
		if *val != 99 {
			t.Errorf("Prepend failed, expected 99 at index 0, got %d", *val)
		}
	})

	t.Run("Range", func(t *testing.T) {
		tree := New[int, struct{}]()
		items := []int{1, 2, 3, 4, 5}
		tree.InsertRange(0, items)
		if tree.Size() != 5 {
			t.Errorf("InsertRange size mismatch: %d", tree.Size())
		}
		tree.InsertRange(2, []int{10, 20}) // [1, 2, 10, 20, 3, 4, 5]
		val, _ := tree.GetAt(2)
		if *val != 10 {
			t.Errorf("InsertRange middle failed, expected 10, got %d", *val)
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("Single", func(t *testing.T) {
		tree := New[int, struct{}]()
		for i := range 10 {
			tree.InsertAt(i, i)
		}
		tree.DeleteAt(5) // Remove 5
		if tree.Size() != 9 {
			t.Errorf("Size mismatch after delete: %d", tree.Size())
		}
		val, _ := tree.GetAt(5)
		if *val != 6 {
			t.Errorf("Value mismatch after delete: expected 6, got %d", *val)
		}
	})

	t.Run("Range", func(t *testing.T) {
		tree := New[int, struct{}]()
		for i := range 100 {
			tree.InsertAt(i, i)
		}
		tree.DeleteRange(10, 80) // Keep [0-9] and [90-99]
		if tree.Size() != 20 {
			t.Errorf("DeleteRange size mismatch: %d", tree.Size())
		}
		val, _ := tree.GetAt(10)
		if *val != 90 {
			t.Errorf("Value mismatch after DeleteRange: expected 90, got %d", *val)
		}
	})

	t.Run("Emptying", func(t *testing.T) {
		tree := New[int, struct{}]()
		tree.InsertAt(0, 1)
		tree.DeleteAt(0)
		if tree.Size() != 0 || tree.Root != nil {
			t.Error("Tree should be completely empty after deleting last item")
		}
	})
}

func TestPointers(t *testing.T) {
	tree := New[int, struct{}]()
	count := 1000 // Enough to cause multiple splits

	for i := range count {
		tree.InsertAt(tree.Size(), i)
	}

	if tree.First == nil || tree.Last == nil {
		t.Fatal("First/Last pointers should not be nil")
	}

	// Forward traversal
	curr := tree.First
	visited := 0
	for curr != nil {
		if !curr.isLeaf {
			t.Error("Leaf chain contains internal node")
		}
		visited += len(curr.Items)
		if curr.next == nil && curr != tree.Last {
			t.Error("Last node in chain is not tree.Last")
		}
		curr = curr.next
	}
	if visited != count {
		t.Errorf("Forward traversal visited %d items, want %d", visited, count)
	}

	// Boundary values
	firstVal := tree.First.Items[0]
	lastVal := tree.Last.Items[len(tree.Last.Items)-1]
	if firstVal != 0 || lastVal != count-1 {
		t.Errorf("Boundary values mismatch: first=%d, last=%d", firstVal, lastVal)
	}
}

func TestSummary(t *testing.T) {
	tree := New[int, int]()
	tree.SummaryConfig = setupCountSummary()

	// Initial inserts
	for i := range 100 {
		tree.InsertAt(i, i*10)
	}

	if tree.Root.Summary != 100 {
		t.Errorf("Root summary mismatch after inserts: got %d, want 100", tree.Root.Summary)
	}

	// Deletions
	tree.DeleteRange(0, 10)
	if tree.Root.Summary != 90 {
		t.Errorf("Root summary mismatch after delete: got %d, want 90", tree.Root.Summary)
	}

	// Check internal nodes (recursive)
	var verifyNodeSummary func(*Node[int, int])
	verifyNodeSummary = func(n *Node[int, int]) {
		if n.isLeaf {
			expected := len(n.Items)
			if n.Summary != expected {
				t.Errorf("Leaf node summary mismatch: got %d, want %d", n.Summary, expected)
			}
		} else {
			sum := 0
			for _, child := range n.children {
				verifyNodeSummary(child)
				sum += child.Summary
			}
			if n.Summary != sum {
				t.Errorf("Internal node summary mismatch: got %d, want %d", n.Summary, sum)
			}
		}
	}
	verifyNodeSummary(tree.Root)
}

func TestOnItemMoved(t *testing.T) {
	type item struct {
		id   int
		node *Node[*item, struct{}]
	}

	tree := New[*item, struct{}]()
	tree.OnItemMoved = func(it *item, n *Node[*item, struct{}]) {
		it.node = n
	}

	items := make([]*item, 200)
	for i := range items {
		items[i] = &item{id: i}
		tree.InsertAt(i, items[i])
	}

	// Verify all items know their nodes
	for i, it := range items {
		if it.node == nil {
			t.Fatalf("Item %d has nil node pointer", i)
		}
		if !slices.Contains(it.node.Items, it) {
			t.Fatalf("Item %d pointer to node %p is incorrect; item not found in node", i, it.node)
		}
	}

	// Delete some to trigger merges/rebalancing
	tree.DeleteRange(0, 100)

	// Verify again
	tree.ForEach(func(it *item) {
		if !slices.Contains(it.node.Items, it) {
			t.Errorf("Item %d pointer to node %p is incorrect after rebalancing", it.id, it.node)
		}
	})
}

func TestStressRandom(t *testing.T) {
	seed := int64(42)
	rng := rand.New(rand.NewSource(seed))
	tree := New[int, struct{}]()
	var mirror []int

	for range 5000 {
		op := rng.Intn(3)
		if len(mirror) == 0 {
			op = 0
		}

		switch op {
		case 0: // Insert
			val := rng.Intn(10000)
			pos := rng.Intn(len(mirror) + 1)
			tree.InsertAt(pos, val)
			mirror = append(mirror[:pos], append([]int{val}, mirror[pos:]...)...)
		case 1: // Delete
			pos := rng.Intn(len(mirror))
			tree.DeleteAt(pos)
			mirror = append(mirror[:pos], mirror[pos+1:]...)
		case 2: // Verify
			pos := rng.Intn(len(mirror))
			val, _ := tree.GetAt(pos)
			if *val != mirror[pos] {
				t.Fatalf("Value mismatch at %d: expected %d, got %d", pos, mirror[pos], *val)
			}
		}
	}

	// Final verification
	idx := 0
	tree.ForEach(func(item int) {
		if item != mirror[idx] {
			t.Fatalf("ForEach mismatch at %d", idx)
		}
		idx++
	})
}

func TestForEach(t *testing.T) {
	tree := New[int, struct{}]()

	t.Run("Empty", func(t *testing.T) {
		count := 0
		tree.ForEach(func(int) { count++ })
		if count != 0 {
			t.Error("ForEach on empty tree should not run")
		}
	})

	t.Run("SingleNode", func(t *testing.T) {
		tree.InsertAt(0, 1)
		tree.InsertAt(1, 2)
		var items []int
		tree.ForEach(func(i int) { items = append(items, i) })
		if !reflect.DeepEqual(items, []int{1, 2}) {
			t.Errorf("ForEach failed: %v", items)
		}
	})
}
