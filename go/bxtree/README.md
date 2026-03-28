# bxtree

`bxtree` is a high-performance, positional B+Tree implementation for Go. It provides an efficient way to manage ordered sequences of items while maintaining tree-wide "summaries" (metadata) that enable O(log N) searches based on custom criteria.

This data structure is particularly useful for any application requiring efficient indexed inserts/deletes and range-based queries.

## Features

- **Positional Indexing**: O(log N) insertion, deletion, and point lookups by index.
- **Customizable Summaries**: Maintain arbitrary metadata (weights, counts, offsets) across the tree with the `Summarizer` interface.
- **Efficient Search**: Use `FindPath` to navigate the tree based on accumulated summaries in O(log N) time.
- **Functional Options**: Clean API for configuring node sizes, summarizers, and move callbacks.
- **Standard Iterators**: Native support for Go iterators (`All`, `Reverse`) for idiomatic range loops.
- **Optimized for Performance**: Built with cache-friendly internal node structures and high-coverage testing (including fuzzing).

## Installation

```bash
go get github.com/seercle/egwalker/go/bxtree
```

## Usage

### Basic List Usage

If you just need an efficient list that supports fast inserts and deletes at any position:

```go
package main

import (
	"fmt"
	"egwalker/bxtree"
)

func main() {
	// Create a simple tree of strings
	tree := bxtree.New[string, struct{}]()

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
```

### Using a Summarizer

The power of `bxtree` lies in the `Summarizer` interface. You can use it to maintain properties like "sum of weights" or "count of non-deleted items."

```go
type MySummarizer struct{}

// FromItem converts an item to its initial summary value
func (s MySummarizer) FromItem(item int) int { return item }

// Add combines two summaries (moving up the tree)
func (s MySummarizer) Add(a, b int) int { return a + b }

// Sub removes a value from a summary (during deletions)
func (s MySummarizer) Sub(a, b int) int { return a - b }

func main() {
	// Initialize tree with the summarizer
	tree := bxtree.New[int, int](
		bxtree.WithSummarizer[int, int](MySummarizer{}),
	)

	tree.InsertRange(0, []int{10, 20, 30, 40})

	// The root now contains the total sum of all items (100)
	fmt.Printf("Total Sum: %d\n", tree.Root().Summary())

	// Use FindPath to find the first position where the accumulated sum exceeds 45
	node, pos, acc := tree.FindPath(func(acc int, cur int) bool {
		return acc + cur > 45
	})
    // ...
}
```

## Configuration

You can tune the tree performance using functional options:

```go
tree := bxtree.New[T, S](
	bxtree.WithLeafNodeSize(64, 128),     // Min/Max items in leaf nodes
	bxtree.WithInternalNodeSize(16, 32),  // Min/Max children in internal nodes
	bxtree.WithOnItemMoved(myCallback),    // Useful for tracking item locations
)
```

## Performance

`bxtree` is significantly faster than standard Go slices for large collections when frequent insertions or deletions occur in the middle of the sequence. While slices are O(N) for these operations, `bxtree` remains O(log N).

## License

MIT
