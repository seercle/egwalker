// Package-wide test utilities for the bxtree package.

package bxtree

import (
	"reflect"
	"testing"
)

type countSummarizer struct{}

func (s countSummarizer) FromItem(item int) int { return 1 }
func (s countSummarizer) Add(a, b int) int      { return a + b }
func (s countSummarizer) Sub(a, b int) int      { return a - b }

func mustNew[T any, S any](t *testing.T, opts ...Option[T, S]) *BxTree[T, S] {
	t.Helper()
	tree, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tree
}

func mustNewFromSlice[T any, S any](t *testing.T, items []T, opts ...Option[T, S]) *BxTree[T, S] {
	t.Helper()
	tree, err := NewFromSlice(items, opts...)
	if err != nil {
		t.Fatalf("NewFromSlice: %v", err)
	}
	return tree
}

func mustNewB[T any, S any](b *testing.B, opts ...Option[T, S]) *BxTree[T, S] {
	b.Helper()
	tree, err := New(opts...)
	if err != nil {
		b.Fatal(err)
	}
	return tree
}

func expectPanic(t *testing.T, name string, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("%s: expected panic, but it did not", name)
			return
		}
		if r != want {
			t.Errorf("%s: expected panic message %q, got %q", name, want, r)
		}
	}()
	f()
}

func verifyTree[T any, S any](t *testing.T, tree *BxTree[T, S], expected []T) {
	t.Helper()

	// 1. Check size
	if tree.Size() != len(expected) {
		t.Errorf("Tree size mismatch: got %d, want %d", tree.Size(), len(expected))
	}

	// 2. Check content via ForEach
	var actual []T
	tree.ForEach(func(item T) {
		actual = append(actual, item)
	})

	if len(actual) != len(expected) {
		t.Errorf("ForEach length mismatch: got %d, want %d", len(actual), len(expected))
	} else {
		for i := range actual {
			if !reflect.DeepEqual(actual[i], expected[i]) {
				t.Errorf("Content mismatch at index %d: got %v, want %v", i, actual[i], expected[i])
			}
		}
	}

	// 3. Check Point Lookups
	for i := range expected {
		val, err := tree.GetAt(i)
		if err != nil {
			t.Errorf("GetAt(%d) failed: %v", i, err)
			continue
		}
		if !reflect.DeepEqual(*val, expected[i]) {
			t.Errorf("GetAt(%d) mismatch: got %v, want %v", i, *val, expected[i])
		}
	}

	// 4. Verify tree structure (internal consistency)
	if tree.Root() != nil {
		verifyNode(t, tree.Root(), tree)
	}

	// 5. Verify Leaf pointers (First -> next -> ... -> Last)
	if len(expected) == 0 {
		if tree.First() != nil || tree.Last() != nil {
			t.Error("Empty tree should have nil First/Last")
		}
	} else {
		curr := tree.First()
		count := 0
		var lastSeen *Node[T, S]
		for curr != nil {
			if !curr.IsLeaf() {
				t.Error("Leaf chain contains non-leaf node")
			}
			count += len(curr.Items())
			lastSeen = curr
			curr = curr.Next()
		}
		if count != len(expected) {
			t.Errorf("Leaf chain total size mismatch: got %d, want %d", count, len(expected))
		}
		if lastSeen != tree.Last() {
			t.Error("Leaf chain end does not match tree.Last")
		}
	}
}

func verifyNode[T any, S any](t *testing.T, n *Node[T, S], tree *BxTree[T, S]) int {
	t.Helper()

	size := 0
	var summary S
	first := true

	if n.IsLeaf() {
		items := n.Items()
		size = len(items)
		if tree.summarizer != nil {
			for i, item := range items {
				m := tree.summarizer.FromItem(item)
				if i == 0 {
					summary = m
				} else {
					summary = tree.summarizer.Add(summary, m)
				}
			}
		}
	} else {
		for _, child := range n.Children() {
			childSize := verifyNode(t, child, tree)
			size += childSize
			if tree.summarizer != nil {
				if first {
					summary = child.Summary()
					first = false
				} else {
					summary = tree.summarizer.Add(summary, child.Summary())
				}
			}
		}
	}

	if n.size != size {
		t.Errorf("Node size mismatch: got %d, want %d", n.size, size)
	}

	if tree.summarizer != nil && !reflect.DeepEqual(n.Summary(), summary) {
		t.Errorf("Node summary mismatch: got %v, want %v", n.Summary(), summary)
	}

	// Occupancy invariants: no node may exceed its configured maximum, and
	// every non-root node must meet its configured minimum. The root is exempt
	// from the minimum (it may legitimately hold fewer items/children).
	isRoot := n == tree.Root()
	if n.IsLeaf() {
		if len(n.Items()) > tree.leafMaxSize {
			t.Errorf("Leaf occupancy violation: %d items exceeds leafMaxSize %d", len(n.Items()), tree.leafMaxSize)
		}
		if !isRoot && len(n.Items()) < tree.leafMinSize {
			t.Errorf("Leaf occupancy violation: %d items below leafMinSize %d (non-root)", len(n.Items()), tree.leafMinSize)
		}
	} else {
		if len(n.Children()) > tree.internalMaxSize {
			t.Errorf("Internal occupancy violation: %d children exceeds internalMaxSize %d", len(n.Children()), tree.internalMaxSize)
		}
		if !isRoot && len(n.Children()) < tree.internalMinSize {
			t.Errorf("Internal occupancy violation: %d children below internalMinSize %d (non-root)", len(n.Children()), tree.internalMinSize)
		}
	}

	return size
}

// checkNodeBounds verifies every node respects the configured min/max sizes,
// except the root, which is allowed to underflow.
func checkNodeBounds[T any](t *testing.T, tree *BxTree[T, int]) {
	t.Helper()
	root := tree.Root()
	if root == nil {
		return
	}
	// Note: `var check func(...); check = ...` — a plain `:=` closure cannot
	// reference itself in Go.
	var check func(n *Node[T, int], path string)
	check = func(n *Node[T, int], path string) {
		if n.isLeaf {
			lo, hi := tree.leafMinSize, tree.leafMaxSize
			if n == root && len(n.items) > hi {
				t.Errorf("%s: root leaf has %d items, max %d", path, len(n.items), hi)
			}
			if n != root && (len(n.items) < lo || len(n.items) > hi) {
				t.Errorf("%s: leaf has %d items, want [%d, %d]", path, len(n.items), lo, hi)
			}
			return
		}
		if n != root && (len(n.children) < tree.internalMinSize || len(n.children) > tree.internalMaxSize) {
			t.Errorf("%s: internal node has %d children, want [%d, %d]", path, len(n.children), tree.internalMinSize, tree.internalMaxSize)
		}
		for i, c := range n.children {
			if c.parent != n {
				t.Errorf("%s: child %d has wrong parent", path, i)
			}
			check(c, path+"->child")
		}
	}
	check(root, "root")
}
