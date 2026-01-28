package bxtree

const (
	INTERNAL_MIN_SIZE = 16
	INTERNAL_MAX_SIZE = INTERNAL_MIN_SIZE * 2
	LEAF_MIN_SIZE     = 2
	LEAF_MAX_SIZE     = LEAF_MIN_SIZE * 2
)

type node[T any] struct {
	isLeaf   bool
	parent   *node[T]
	size     int
	items    []T        // only for leaf nodes
	children []*node[T] // only for internal nodes
}

type BxTree[T any] struct {
	root  *node[T]
	first *node[T]
	last  *node[T]
}
