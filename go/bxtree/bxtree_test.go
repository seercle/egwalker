package bxtree

import "testing"

func Test(t *testing.T) {
	tree := New[int]()
	for i := 0; i < 64+16+1; i++ {
		tree.InsertAt(i, i)
	}
	tree.Print()
}
