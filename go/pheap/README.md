# pheap

`pheap` is a fast, generic pairing heap implementation for Go.

## Features

- **Generic**: Works with any type `T` using functional options.
- **Option Pattern**: Clean API for configuration using `WithLess`.
- **Efficient**: Pairing heaps are known for excellent performance in practice.
- **Simple API**: Easy to use `Push`, `Pop`, `Peek`, and `Size` methods.

## Installation

```bash
go get github.com/your-username/egwalker/go/pheap
```

## Usage

### Max-Heap Example (Ordered types)

```go
package main

import (
	"fmt"
	"egwalker/pheap"
)

func main() {
	// Create a max-heap of integers (default)
	h := pheap.New[int]()

	h.Push(10)
	h.Push(30)
	h.Push(20)

	val, _ := h.Pop()
	fmt.Println(val) // Output: 30
}
```

### Min-Heap Example (Ordered types)

```go
package main

import (
	"fmt"
	"egwalker/pheap"
)

func main() {
	// Create a min-heap using WithLess
	h := pheap.New(pheap.WithLess(func(a, b int) bool {
		return a > b
	}))

	h.Push(10)
	h.Push(30)
	h.Push(20)

	val, _ := h.Pop()
	fmt.Println(val) // Output: 10
}
```

### Any Type Example

```go
package main

import (
	"fmt"
	"egwalker/pheap"
)

type Task struct {
	Priority int
	Name     string
}

func main() {
	// Use NewAny for types that are not cmp.Ordered
	h := pheap.NewAny(pheap.WithLess(func(a, b Task) bool {
		return a.Priority < b.Priority
	}))

	h.Push(Task{1, "low priority"})
	h.Push(Task{10, "high priority"})

	task, _ := h.Pop()
	fmt.Println(task.Name) // Output: high priority
}
```

## Performance

Pairing heaps offer very efficient amortized time complexity:
- `Push`: O(1)
- `Pop`: O(log n)
- `Peek`: O(1)

## License

This project is licensed under the MIT License - see the [LICENSE](./LICENSE) file for details.
