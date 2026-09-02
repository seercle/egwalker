package bxtree

import (
	"errors"
	"fmt"
)

var (
	// ErrIndexOutOfBounds is returned when an operation is performed on an index
	// that is outside the range [0, tree.Size()).
	ErrIndexOutOfBounds = errors.New("index out of bounds")
)

// InvalidNodeSizeError is returned by New/NewFromSlice when the configured leaf
// or internal node sizes cannot be honored by the tree's split, borrow, and
// merge operations. Such configurations would let nodes fall outside their
// [min, max] occupancy bounds during mutation, so they are rejected at
// construction instead.
type InvalidNodeSizeError struct {
	// Kind is either "leaf" or "internal".
	Kind string
	Min  int
	Max  int
	// Reason names the violated constraint.
	Reason string
}

func (e *InvalidNodeSizeError) Error() string {
	return fmt.Sprintf("bxtree: unsupported %s node sizes (min=%d max=%d): %s", e.Kind, e.Min, e.Max, e.Reason)
}
