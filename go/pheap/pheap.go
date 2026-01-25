package pheap

import "cmp"

type node[T cmp.Ordered] struct {
	value    T
	subtrees []*node[T]
}

type PairingHeap[T cmp.Ordered] struct {
	root *node[T]
	size int
}

//

func (h *PairingHeap[T]) meld(a, b *node[T]) *node[T] {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.value < b.value {
		a.subtrees = append(a.subtrees, b)
		return a
	} else {
		b.subtrees = append(b.subtrees, a)
		return b
	}
}

func (h *PairingHeap[T]) mergePair(l []*node[T]) *node[T] {
	if (len(l)) == 0 {
		return nil
	} else if len(l) == 1 {
		return l[0]
	}
	return h.meld(h.meld(l[0], l[1]), h.mergePair(l[2:]))
}

//

func New[T cmp.Ordered]() *PairingHeap[T] {
	return &PairingHeap[T]{root: nil, size: 0}
}
func (h *PairingHeap[T]) Push(value T) {
	h.root = h.meld(&node[T]{value: value}, h.root)
	h.size++
}
func (h *PairingHeap[T]) Pop() (T, bool) {
	if h.root == nil {
		var zero T
		return zero, false
	}
	max := h.root.value
	h.root = h.mergePair(h.root.subtrees)
	h.size--
	return max, true
}
func (h *PairingHeap[T]) Size() int {
	return h.size
}
