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
	Summary  S
	Items    []T           // only for leaf nodes
	next     *Node[T, S]   // only for leaf nodes
	children []*Node[T, S] // only for internal nodes
}

type SummaryConfig[T any, S any] struct {
	FromItem func(item T) S
	Add      func(a, b S) S
	Sub      func(a, b S) S
}

type BxTree[T any, S any] struct {
	Root            *Node[T, S]
	First           *Node[T, S]
	Last            *Node[T, S]
	internalMinSize int
	internalMaxSize int
	leafMinSize     int
	leafMaxSize     int
	OnItemMoved     func(item T, node *Node[T, S])
	SummaryConfig   *SummaryConfig[T, S]
}
