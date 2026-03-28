package bxtree

const (
	DefaultInternalMinSize = 16
	DefaultInternalMaxSize = 32
	DefaultLeafMinSize     = 64
	DefaultLeafMaxSize     = 128
)

// Node represents a node in the BxTree. It can be either a leaf node containing items
// or an internal node containing child nodes.
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

// Summary returns the accumulated summary value for this node.
func (n *Node[T, S]) Summary() S {
	if n == nil {
		panic("bxtree: Summary called on nil node")
	}
	return n.summary
}

// Items returns the items stored in this node. Only valid for leaf nodes.
func (n *Node[T, S]) Items() []T {
	if n == nil {
		panic("bxtree: Items called on nil node")
	}
	return n.items
}

// Summarizer defines the interface for maintaining tree-wide summaries.
// T is the type of items in the tree, and S is the type of the summary metadata.
//
// Summaries are updated automatically during insertions and deletions.
type Summarizer[T any, S any] interface {
	// FromItem converts a single item into its summary representation.
	FromItem(item T) S
	// Add combines two summaries. This is used when moving up the tree.
	Add(a, b S) S
	// Sub subtracts a summary from another. This is used during deletions.
	Sub(a, b S) S
}

// BxTree is a positional B+Tree that supports efficient indexed operations
// and tree-wide metadata summaries.
type BxTree[T any, S any] struct {
	root            *Node[T, S]
	first           *Node[T, S]
	last            *Node[T, S]
	internalMinSize int
	internalMaxSize int
	leafMinSize     int
	leafMaxSize     int
	onItemMoved     func(item T, node *Node[T, S])
	summarizer      Summarizer[T, S]
}

// Option is a functional option for configuring a BxTree during initialization.
type Option[T any, S any] func(*BxTree[T, S])

// Root returns the root node of the tree.
func (tree *BxTree[T, S]) Root() *Node[T, S] {
	if tree == nil {
		panic("bxtree: Root called on nil tree")
	}
	return tree.root
}

// First returns the first leaf node in the tree.
func (tree *BxTree[T, S]) First() *Node[T, S] {
	if tree == nil {
		panic("bxtree: First called on nil tree")
	}
	return tree.first
}

// Last returns the last leaf node in the tree.
func (tree *BxTree[T, S]) Last() *Node[T, S] {
	if tree == nil {
		panic("bxtree: Last called on nil tree")
	}
	return tree.last
}
