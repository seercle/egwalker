# Pairing heap (throwaway self-check fixture)

Insertion into the heap:

```go include go/pheap/pheap.go L114-L121
// Push adds a new value to the heap.
func (h *PairingHeap[T]) Push(value T) {
	if h == nil {
		panic("pheap: Push called on nil heap")
	}
	h.root = h.meld(&node[T]{value: value}, h.root)
	h.size++
}
```
