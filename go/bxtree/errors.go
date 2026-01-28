package bxtree

import "errors"

var (
	ErrIndexOutOfBounds       = errors.New("index out of bounds")
	ErrNotRootAndOneChild     = errors.New("node is not root and has only one child")
	ErrParentDoesNotHaveChild = errors.New("parent does not have this node as child")
)
