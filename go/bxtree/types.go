package bxtree

const (
	DefaultInternalMinSize = 16
	DefaultInternalMaxSize = 32
	DefaultLeafMinSize     = 64
	DefaultLeafMaxSize     = 128
)

type Node[T any, S any] struct {
	isLeaf   bool
	parent   *Node[T, S]
	size     int
	summary  S
	items    []T           // only for leaf nodes
	next     *Node[T, S]   // only for leaf nodes
	prev     *Node[T, S]   // only for leaf nodes
	children []*Node[T, S] // only for internal nodes
}

func (n *Node[T, S]) Summary() S {
	if n == nil {
		panic("bxtree: Summary called on nil node")
	}
	return n.summary
}
func (n *Node[T, S]) Items() []T {
	if n == nil {
		panic("bxtree: Items called on nil node")
	}
	return n.items
}

type Summary[T any, S any] struct {
	FromItem func(item T) S
	Add      func(a, b S) S
	Sub      func(a, b S) S
}

type Config struct {
	InternalMinSize int
	InternalMaxSize int
	LeafMinSize     int
	LeafMaxSize     int
}

type BxTree[T any, S any] struct {
	root            *Node[T, S]
	first           *Node[T, S]
	last            *Node[T, S]
	internalMinSize int
	internalMaxSize int
	leafMinSize     int
	leafMaxSize     int
	OnItemMoved     func(item T, node *Node[T, S])
	Summary         *Summary[T, S]
}

func (tree *BxTree[T, S]) Root() *Node[T, S] {
	if tree == nil {
		panic("bxtree: Root called on nil tree")
	}
	return tree.root
}
func (tree *BxTree[T, S]) First() *Node[T, S] {
	if tree == nil {
		panic("bxtree: First called on nil tree")
	}
	return tree.first
}
func (tree *BxTree[T, S]) Last() *Node[T, S] {
	if tree == nil {
		panic("bxtree: Last called on nil tree")
	}
	return tree.last
}
