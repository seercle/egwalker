package bxtree

var (
	InternalMinSize = 16
	InternalMaxSize = 32
	LeafMinSize     = 64
	LeafMaxSize     = 128
)

type node[T any] struct {
	isLeaf   bool
	parent   *node[T]
	size     int
	items    []T        // only for leaf nodes
	next     *node[T]   // only for leaf nodes
	children []*node[T] // only for internal nodes
}

type BxTree[T any] struct {
	root  *node[T]
	first *node[T]
	last  *node[T]
}
