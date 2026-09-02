package bxtree

import (
	"testing"
)

// Simple summary that counts items
var countSummary = countSummarizer{}

func FuzzBxTree(f *testing.F) {
	for _, s := range [][]byte{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{2, 10, 20, 30, 40, 50, 60, 70, 80, 90},
		{1, 5, 3, 9, 7, 2, 8, 4, 6, 0, 1, 2, 3, 4, 5},
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}

		withSummary := data[0]%2 == 0
		data = data[1:]

		var tree *BxTree[int, int]
		if withSummary {
			tr, err := New(WithSummarizer(countSummary))
			if err != nil {
				t.Fatal(err)
			}
			tree = tr
		} else {
			tr, err := New[int, int]()
			if err != nil {
				t.Fatal(err)
			}
			tree = tr
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

			if len(reference)%10 == 0 {
				verifyTree(t, tree, reference)
			}
		}

		verifyTree(t, tree, reference)
	})
}
