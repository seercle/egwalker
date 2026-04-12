package crdt

import (
	"reflect"
	"testing"
)

func TestSerializationNonContiguous(t *testing.T) {
	log := newOpLog[rune]()

	// Manually add non-contiguous ops for the same agent
	// Op 1: Agent 1, Seq 0, Len 5
	op1 := op[rune]{
		opType:  opTypeIns,
		content: []rune("Hello"),
		length:  5,
		id:      id{agent: 1, seq: 0},
	}
	log.ops = append(log.ops, op1)
	log.opStartLVs = append(log.opStartLVs, 0)
	log.agentOps[1] = append(log.agentOps[1], 0)
	log.version[1] = 4

	// Op 2: Agent 1, Seq 10, Len 5 (GAP of 5)
	op2 := op[rune]{
		opType:  opTypeIns,
		content: []rune("World"),
		length:  5,
		id:      id{agent: 1, seq: 10},
	}
	log.ops = append(log.ops, op2)
	log.opStartLVs = append(log.opStartLVs, 5)
	log.agentOps[1] = append(log.agentOps[1], 1)
	log.version[1] = 14

	// Marshal and Unmarshal
	data := log.Marshal()
	newLog := Unmarshal(data)

	// Check if Op 2 has the correct Seq
	if newLog.ops[1].id.seq != 10 {
		t.Errorf("Op 2 Seq mismatch. Expected 10, got %d", newLog.ops[1].id.seq)
	}

	if !reflect.DeepEqual(log.ops, newLog.ops) {
		t.Errorf("Ops mismatch.\nOriginal: %+v\nNew:      %+v", log.ops, newLog.ops)
	}
}
