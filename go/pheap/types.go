package pheap

type node[T any] struct {
	value   T
	child   *node[T]
	sibling *node[T]
}

// PairingHeap is a generic implementation of a pairing heap data structure.
// It can be used as a priority queue where the order of elements is
// determined by a user-provided comparison function.
type PairingHeap[T any] struct {
	root *node[T]
	size int
	less func(a, b T) bool
}

// Option is a functional option for configuring a PairingHeap during initialization.
type Option[T any] func(*PairingHeap[T])
