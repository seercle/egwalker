package pheap_test

import (
	"egwalker/pheap"
	"fmt"
)

func ExampleNew() {
	// Create a max-heap of integers (default)
	h := pheap.New[int]()

	h.Push(10)
	h.Push(30)
	h.Push(20)

	val, _ := h.Pop()
	fmt.Println(val)
	// Output: 30
}

func ExampleWithLess() {
	// Create a min-heap of integers using WithLess
	h := pheap.New(pheap.WithLess(func(a, b int) bool {
		return a > b
	}))

	h.Push(10)
	h.Push(30)
	h.Push(20)

	val, _ := h.Pop()
	fmt.Println(val)
	// Output: 10
}

func ExampleNewAny() {
	type item struct {
		priority int
		data     string
	}

	// NewAny is used for non-ordered types. WithLess is mandatory.
	h := pheap.NewAny(pheap.WithLess(func(a, b item) bool {
		return a.priority < b.priority
	}))

	h.Push(item{1, "low"})
	h.Push(item{10, "high"})
	h.Push(item{5, "medium"})

	val, _ := h.Pop()
	fmt.Println(val.data)
	// Output: high
}
