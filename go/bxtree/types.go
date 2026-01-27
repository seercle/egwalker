package bxtree

const (
	INTERNAL_MIN_SIZE = 16
	INTERNAL_MAX_SIZE = INTERNAL_MIN_SIZE * 2
	LEAF_MIN_SIZE     = 64
	LEAF_MAX_SIZE     = LEAF_MIN_SIZE * 2
)

type Node[T any] interface {
	isLeaf() bool
	parent() *InternalNode[T]
	setParent(*InternalNode[T])
	size() int
}

type InternalNode[T any] struct {
	_parent  *InternalNode[T]
	_size    int
	len      int
	children [INTERNAL_MAX_SIZE]Node[T]
}

func (n *InternalNode[T]) isLeaf() bool                 { return false }
func (n *InternalNode[T]) parent() *InternalNode[T]     { return n._parent }
func (n *InternalNode[T]) setParent(p *InternalNode[T]) { n._parent = p }
func (n *InternalNode[T]) size() int                    { return n._size }

type LeafNode[T any] struct {
	_parent *InternalNode[T]
	_len    int
	items   [LEAF_MAX_SIZE * 2]T
	next    *LeafNode[T]
}

func (n *LeafNode[T]) isLeaf() bool                 { return true }
func (n *LeafNode[T]) parent() *InternalNode[T]     { return n._parent }
func (n *LeafNode[T]) setParent(p *InternalNode[T]) { n._parent = p }
func (n *LeafNode[T]) size() int                    { return n._len }

type BxTree[T any] struct {
	root Node[T]
	size int
}
