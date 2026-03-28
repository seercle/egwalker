package bxtree

import "errors"

var (
	// ErrIndexOutOfBounds is returned when an operation is performed on an index
	// that is outside the range [0, tree.Size()).
	ErrIndexOutOfBounds = errors.New("index out of bounds")
)
