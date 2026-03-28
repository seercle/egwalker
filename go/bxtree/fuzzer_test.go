package bxtree

import (
	"math/rand"
	"reflect"
	"testing"
)

// Simple summary that counts items
var countSummary = countSummarizer{}

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

	return size
}

func TestFuzzTree(t *testing.T) {
	seedCount := 50
	for _, withSummary := range []bool{true, false} {
		for seed := range seedCount {
			src := rand.NewSource(int64(seed))
			r := rand.New(src)

			var tree *BxTree[int, int]
			if withSummary {
				tree = New(WithSummarizer(countSummary))
			} else {
				tree = New[int, int]()
			}
			var reference []int

			for op := range 200 {
				length := len(reference)

				if length == 0 || r.Float64() < 0.7 {
					val := r.Intn(1000)
					pos := 0
					if length > 0 {
						pos = r.Intn(length + 1)
					}

					err := tree.InsertAt(pos, val)
					if err != nil {
						t.Fatalf("Seed %d, Op %d: InsertAt(%d) failed: %v", seed, op, pos, err)
					}

					reference = append(reference, 0)
					copy(reference[pos+1:], reference[pos:])
					reference[pos] = val
				} else {
					pos := r.Intn(length)
					delLen := r.Intn(5) + 1
					if pos+delLen > length {
						delLen = length - pos
					}

					err := tree.DeleteRange(pos, delLen)
					if err != nil {
						t.Fatalf("Seed %d, Op %d: DeleteRange(%d, %d) failed: %v", seed, op, pos, delLen, err)
					}

					reference = append(reference[:pos], reference[pos+delLen:]...)
				}

				if op%20 == 0 {
					verifyTree(t, tree, reference)
				}
			}
			verifyTree(t, tree, reference)
		}
	}
}

func FuzzBxTree(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}

		withSummary := data[0]%2 == 0
		data = data[1:]

		var tree *BxTree[int, int]
		if withSummary {
			tree = New(WithSummarizer(countSummary))
		} else {
			tree = New[int, int]()
		}
		var reference []int

		for i := 0; i < len(data); {
			op := data[i] % 2
			i++
			if i >= len(data) {
				break
			}

			length := len(reference)

			if op == 0 || length == 0 {
				val := int(data[i])
				i++
				pos := 0
				if length > 0 {
					if i >= len(data) {
						break
					}
					pos = int(data[i]) % (length + 1)
					i++
				}

				err := tree.InsertAt(pos, val)
				if err != nil {
					t.Fatalf("InsertAt(%d) failed: %v", pos, err)
				}

				reference = append(reference, 0)
				copy(reference[pos+1:], reference[pos:])
				reference[pos] = val
			} else {
				pos := int(data[i]) % length
				i++
				if i >= len(data) {
					break
				}
				delLen := (int(data[i]) % 5) + 1
				i++
				if pos+delLen > length {
					delLen = length - pos
				}

				err := tree.DeleteRange(pos, delLen)
				if err != nil {
					t.Fatalf("DeleteRange(%d, %d) failed: %v", pos, delLen, err)
				}

				reference = append(reference[:pos], reference[pos+delLen:]...)
			}
		}

		verifyTree(t, tree, reference)
	})
}
