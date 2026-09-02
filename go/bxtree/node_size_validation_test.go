package bxtree

import (
	"errors"
	"testing"
)

// TestNewRejectsUnsupportedNodeSizes locks in the supported configuration
// envelope: split/borrow/merge can only keep every non-root node within
// [min, max] when max >= 2*min - 1 (and min makes sense), so New must refuse
// anything else.
func TestNewRejectsUnsupportedNodeSizes(t *testing.T) {
	cases := []struct {
		name     string
		opts     []Option[int, int]
		wantKind string
	}{
		{"leaf min below 1", []Option[int, int]{WithLeafNodeSize[int, int](0, 8)}, "leaf"},
		{"leaf min above max", []Option[int, int]{WithLeafNodeSize[int, int](10, 4)}, "leaf"},
		{"leaf max below 2*min-1", []Option[int, int]{WithLeafNodeSize[int, int](7, 8)}, "leaf"},
		{"leaf boundary max below 2*min-1", []Option[int, int]{WithLeafNodeSize[int, int](3, 4)}, "leaf"},
		{"internal min below 2", []Option[int, int]{WithInternalNodeSize[int, int](1, 4)}, "internal"},
		{"internal min above max", []Option[int, int]{WithInternalNodeSize[int, int](4, 2)}, "internal"},
		{"internal max below 2*min-1", []Option[int, int]{WithInternalNodeSize[int, int](2, 2)}, "internal"},
		{"internal boundary max below 2*min-1", []Option[int, int]{WithInternalNodeSize[int, int](4, 6)}, "internal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := New(tc.opts...)
			if err == nil {
				t.Fatalf("New(%s): expected error, got tree", tc.name)
			}
			if tree != nil {
				t.Fatalf("New(%s): expected nil tree on error, got %v", tc.name, tree)
			}
			var ierr *InvalidNodeSizeError
			if !errors.As(err, &ierr) {
				t.Fatalf("New(%s): error is %T, want *InvalidNodeSizeError (%v)", tc.name, err, err)
			}
			if ierr.Kind != tc.wantKind {
				t.Fatalf("New(%s): error kind = %q, want %q", tc.name, ierr.Kind, tc.wantKind)
			}
		})
	}
}

// TestNewAcceptsSupportedNodeSizes verifies the valid envelope is accepted,
// including the exact max == 2*min - 1 boundary.
func TestNewAcceptsSupportedNodeSizes(t *testing.T) {
	cases := []struct {
		name string
		opts []Option[int, int]
	}{
		{"defaults", nil},
		{"leaf half-size", []Option[int, int]{WithLeafNodeSize[int, int](4, 8)}},
		{"internal half-size", []Option[int, int]{WithInternalNodeSize[int, int](2, 4)}},
		{"leaf min==max==1", []Option[int, int]{WithLeafNodeSize[int, int](1, 1)}},
		{"leaf boundary 2*min-1", []Option[int, int]{WithLeafNodeSize[int, int](3, 5)}},
		{"internal boundary 2*min-1", []Option[int, int]{WithInternalNodeSize[int, int](2, 3)}},
		{"combined small config", []Option[int, int]{
			WithLeafNodeSize[int, int](4, 8),
			WithInternalNodeSize[int, int](2, 4),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := New(tc.opts...)
			if err != nil {
				t.Fatalf("New(%s): unexpected error: %v", tc.name, err)
			}
			if tree == nil {
				t.Fatalf("New(%s): tree is nil", tc.name)
			}
			if tree.Size() != 0 {
				t.Fatalf("New(%s): Size = %d, want 0", tc.name, tree.Size())
			}
		})
	}
}

func TestNewFromSlicePropagatesNodeSizeError(t *testing.T) {
	items := make([]int, 10)
	for i := range items {
		items[i] = i
	}

	tree, err := NewFromSlice(items, WithLeafNodeSize[int, int](7, 8))
	if err == nil {
		t.Fatal("NewFromSlice: expected error for unsupported leaf sizes")
	}
	if tree != nil {
		t.Fatal("NewFromSlice: expected nil tree on error")
	}
	var ierr *InvalidNodeSizeError
	if !errors.As(err, &ierr) {
		t.Fatalf("NewFromSlice: error is %T, want *InvalidNodeSizeError (%v)", err, err)
	}

	ok, err := NewFromSlice[int, struct{}](items)
	if err != nil {
		t.Fatalf("NewFromSlice defaults: unexpected error: %v", err)
	}
	if ok == nil || ok.Size() != len(items) {
		t.Fatalf("NewFromSlice defaults: tree = %v, Size = %d, want %d", ok, ok.Size(), len(items))
	}
}
