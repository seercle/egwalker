package pheap

type node[T any] struct {
	value    T
	subtrees []*node[T]
}

type PairingHeap[T any] struct {
	root *node[T]
	size int
	less func(a, b T) bool
}

func (h *PairingHeap[T]) meld(a, b *node[T]) *node[T] {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	// If less(b.value, a.value) is true, a is "greater" or "higher priority"
	// So for a max-heap, less should be <, but we want the root to be the MAX.
	// Actually, let's define 'less' such that it returns true if 'a' should be a child of 'b'.
	// Or more standard: less(a, b) returns true if a < b.
	// In a max-heap, we want the root to be the largest.
	// So we keep the one that is NOT less.
	if h.less(b.value, a.value) {
		a.subtrees = append(a.subtrees, b)
		return a
	} else {
		b.subtrees = append(b.subtrees, a)
		return b
	}
}

func (h *PairingHeap[T]) mergePair(l []*node[T]) *node[T] {
	if len(l) == 0 {
		return nil
	}
	if len(l) == 1 {
		return l[0]
	}

	// Two-pass merging
	var pairs []*node[T]
	for i := 0; i < len(l); i += 2 {
		if i+1 < len(l) {
			pairs = append(pairs, h.meld(l[i], l[i+1]))
		} else {
			pairs = append(pairs, l[i])
		}
	}

	root := pairs[len(pairs)-1]
	for i := len(pairs) - 2; i >= 0; i-- {
		root = h.meld(root, pairs[i])
	}
	return root
}

func NewWithLess[T any](less func(a, b T) bool) *PairingHeap[T] {
	return &PairingHeap[T]{root: nil, size: 0, less: less}
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
	val := h.root.value
	h.root = h.mergePair(h.root.subtrees)
	h.size--
	return val, true
}

func (h *PairingHeap[T]) Size() int {
	return h.size
}

func (h *PairingHeap[T]) Peek() (T, bool) {
	if h.root == nil {
		var zero T
		return zero, false
	}
	return h.root.value, true
}
