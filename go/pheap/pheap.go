// Package pheap provides a generic implementation of a pairing heap.
//
// A pairing heap is a type of self-adjusting heap that is known for its
// simplicity and excellent performance in practice. It supports basic
// priority queue operations like Push, Pop, and Peek.
package pheap

import "cmp"

// WithLess sets the comparison function for the heap.
// The less function determines the priority: for a max-heap, use 'a < b';
// for a min-heap, use 'a > b'.
func WithLess[T any](less func(a, b T) bool) Option[T] {
	return func(h *PairingHeap[T]) {
		h.less = less
	}
}

func (h *PairingHeap[T]) meld(a, b *node[T]) *node[T] {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	// For a max-heap, less(a, b) should be true if a < b.
	// If less(b.value, a.value) is true, then b < a, so a becomes the parent.
	if h.less(b.value, a.value) {
		b.sibling = a.child
		a.child = b
		return a
	} else {
		a.sibling = b.child
		b.child = a
		return b
	}
}

// mergePairs implements a two-pass merge algorithm.
func (h *PairingHeap[T]) mergePairs(n *node[T]) *node[T] {
	if n == nil {
		return nil
	}

	// Pass 1: Merge pairs from left to right
	var pairs *node[T] // using sibling pointer to link the results of the first pass
	curr := n
	for curr != nil && curr.sibling != nil {
		a := curr
		b := curr.sibling
		next := b.sibling

		a.sibling = nil
		b.sibling = nil

		res := h.meld(a, b)
		res.sibling = pairs
		pairs = res

		curr = next
	}

	if curr != nil {
		curr.sibling = pairs
		pairs = curr
	}

	// Pass 2: Merge the results from right to left
	// Since we prepended to 'pairs', we actually just need to merge them one by one.
	// (The last pair of Pass 1 is at the head of the 'pairs' list)
	if pairs == nil {
		return nil
	}

	root := pairs
	curr = pairs.sibling
	for curr != nil {
		next := curr.sibling
		curr.sibling = nil
		root = h.meld(root, curr)
		curr = next
	}

	return root
}

// New creates a new PairingHeap for types that are ordered.
// By default, it uses 'a < b' as the less function (max-heap).
func New[T cmp.Ordered](opts ...Option[T]) *PairingHeap[T] {
	h := &PairingHeap[T]{
		less: func(a, b T) bool { return a < b },
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// NewAny creates a new PairingHeap for any type with the provided options.
// Note: You MUST provide a comparison function via WithLess if the type is not ordered
// or if you want to override the default behavior.
func NewAny[T any](opts ...Option[T]) *PairingHeap[T] {
	h := &PairingHeap[T]{}
	for _, opt := range opts {
		opt(h)
	}
	if h.less == nil {
		panic("pheap: less function must be provided for NewAny")
	}
	return h
}

// Push adds a new value to the heap.
func (h *PairingHeap[T]) Push(value T) {
	if h == nil {
		panic("pheap: Push called on nil heap")
	}
	h.root = h.meld(&node[T]{value: value}, h.root)
	h.size++
}

// Pop removes and returns the highest priority element from the heap.
// It returns (zero value, false) if the heap is empty.
func (h *PairingHeap[T]) Pop() (T, bool) {
	if h == nil {
		panic("pheap: Pop called on nil heap")
	}
	if h.root == nil {
		var zero T
		return zero, false
	}
	val := h.root.value
	h.root = h.mergePairs(h.root.child)
	h.size--
	return val, true
}

// Size returns the number of elements currently in the heap.
func (h *PairingHeap[T]) Size() int {
	if h == nil {
		panic("pheap: Size called on nil heap")
	}
	return h.size
}

// Peek returns the highest priority element without removing it.
// It returns (zero value, false) if the heap is empty.
func (h *PairingHeap[T]) Peek() (T, bool) {
	if h == nil {
		panic("pheap: Peek called on nil heap")
	}
	if h.root == nil {
		var zero T
		return zero, false
	}
	return h.root.value, true
}
