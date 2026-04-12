package crdt

import (
	"testing"
)

func TestLVResolution(t *testing.T) {
	log := &opLog[rune]{
		ops: []op[rune]{
			{id: id{agent: 0, seq: 0}, length: 5},
			{id: id{agent: 0, seq: 5}, length: 3},
			{id: id{agent: 1, seq: 0}, length: 10},
		},
		opStartLVs: []lv{0, 5, 8},
		agentOps: map[int][]int{
			0: {0, 1},
			1: {2},
		},
	}

	// Test getOpByLV
	tests := []struct {
		v       lv
		wantIdx int
		wantOff int
	}{
		{0, 0, 0},
		{4, 0, 4},
		{5, 1, 0},
		{7, 1, 2},
		{8, 2, 0},
		{17, 2, 9},
	}

	for _, tt := range tests {
		idx, off := log.getOpByLV(tt.v)
		if idx != tt.wantIdx || off != tt.wantOff {
			t.Errorf("getOpByLV(%d) = (%d, %d), want (%d, %d)", tt.v, idx, off, tt.wantIdx, tt.wantOff)
		}
	}

	// Test resolveID
	idTests := []struct {
		target id
		wantLV lv
	}{
		{id{0, 0}, 0},
		{id{0, 4}, 4},
		{id{0, 5}, 5},
		{id{0, 7}, 7},
		{id{1, 0}, 8},
		{id{1, 9}, 17},
	}

	for _, tt := range idTests {
		got := log.resolveID(tt.target)
		if got != tt.wantLV {
			t.Errorf("resolveID(%v) = %d, want %d", tt.target, got, tt.wantLV)
		}
	}
}

func TestLVResolutionBounds(t *testing.T) {
	log := &opLog[rune]{
		ops: []op[rune]{
			{id: id{agent: 0, seq: 0}, length: 5},
		},
		opStartLVs: []lv{0},
		agentOps: map[int][]int{
			0: {0},
		},
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("getOpByLV(10) did not panic")
		}
	}()

	// This should ideally panic but currently doesn't
	idx, off := log.getOpByLV(10)
	t.Logf("Unexpectedly got idx=%d, off=%d for LV=10", idx, off)

	// To make the test fail if it doesn't panic, we can check the length
	if off >= log.ops[idx].length {
		t.Errorf("getOpByLV(10) returned offset %d >= length %d", off, log.ops[idx].length)
	}
}
