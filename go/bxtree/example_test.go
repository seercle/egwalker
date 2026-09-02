package bxtree

import (
	"fmt"
)

func ExampleNew() {
	// Create a simple tree of strings
	tree, err := New[string, struct{}]()
	if err != nil {
		panic(err)
	}

	// O(log N) operations
	tree.InsertAt(0, "World")
	tree.InsertAt(0, "Hello")
	tree.InsertAt(1, "Go")

	// Iterate over items
	for item := range tree.All() {
		fmt.Println(item)
	}
	// Output:
	// Hello
	// Go
	// World
}

type readmeSummarizer struct{}

func (s readmeSummarizer) FromItem(item int) int { return item }
func (s readmeSummarizer) Add(a, b int) int      { return a + b }
func (s readmeSummarizer) Sub(a, b int) int      { return a - b }

func ExampleSummarizer() {
	// Initialize tree with the summarizer
	tree, err := New[int, int](
		WithSummarizer[int, int](readmeSummarizer{}),
	)
	if err != nil {
		panic(err)
	}

	tree.InsertRange(0, []int{10, 20, 30, 40})

	// The root now contains the total sum of all items (100)
	fmt.Printf("Total Sum: %d\n", tree.Root().Summary())

	// Use FindPath to find the first position where the accumulated sum exceeds 45
	node, pos, acc := tree.FindPath(func(acc int, cur int) bool {
		return acc+cur > 45
	})

	if node != nil {
		fmt.Printf("Found item %d at index %d in node, with accumulated sum %d\n", node.Items()[pos], pos, acc)
	}
	// Output:
	// Total Sum: 100
	// Found item 30 at index 2 in node, with accumulated sum 30
}
