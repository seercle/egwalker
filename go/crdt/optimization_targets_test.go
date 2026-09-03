//go:build optimization_targets

package crdt

import (
	"bytes"
	"compress/flate"
	"fmt"
	"strings"
	"testing"
)

// targetRetentionSet computes the ops a critical-version compaction would keep:
// every live frontier op plus every op with >= 2 children (a branch point).
func targetRetentionSet(log *opLog[runeText]) map[lv]bool {
	keep := make(map[lv]bool)
	for _, f := range log.frontier {
		keep[f] = true
	}
	childCount := make(map[lv]int)
	for _, o := range log.ops {
		for _, p := range o.parents {
			childCount[p]++
		}
	}
	for p, c := range childCount {
		if c >= 2 {
			keep[p] = true
		}
	}
	return keep
}

// TestTargetCriticalVersionCompaction expects the missing critical-version
// truncation optimization (CONTEXT.md, Missing / Section 3.5). Once two
// replicas have fully synchronized (both hold the entire history and share a
// frontier), history below the frontier is acked and a compaction can discard
// every non-critical op. Today the opLog grows without bound and keeps all ops.
func TestTargetCriticalVersionCompaction(t *testing.T) {
	a := NewRuneDocument(1)
	b := NewRuneDocument(2)

	const total = 5000
	for i := 0; i < total; i += 100 {
		a.Ins(a.Len(), strings.Repeat("x", 100))
	}
	b.MergeFrom(a)
	a.MergeFrom(b) // full sync: identical logs, identical frontier

	keep := targetRetentionSet(a.doc.opLog)
	if got, want := len(a.doc.opLog.ops), len(keep); got != want {
		t.Errorf("critical-version truncation not implemented: opLog retains %d ops after full sync, compaction target is %d (frontier + critical versions)", got, want)
	}
}

// estimateCompressedSize approximates what a varint + LZ4/Zstd binary codec
// (CONTEXT.md, Missing / Section 3.8) would produce for this log: varints for
// type/agent/seq and delta positions, and flate-compressed content bytes.
func estimateCompressedSize(log *opLog[runeText]) int {
	var total int
	var lastType opType
	var lastAgent int
	var lastPos int
	var buf bytes.Buffer
	fw, _ := flate.NewWriter(&buf, flate.BestCompression)
	for i, o := range log.ops {
		if i == 0 || o.opType != lastType {
			total += 1 // varint for a type marker
			lastType = o.opType
		}
		if i == 0 || o.id.agent != lastAgent {
			total += 1 // varint for an agent marker
			lastAgent = o.id.agent
		}
		delta := o.pos - lastPos
		lastPos = o.pos
		if delta != 1 {
			total += 1 // varint delta (1 when consecutive)
		}
		if o.opType == opTypeIns {
			fw.Write([]byte(fmt.Sprintf("%c", []rune(o.content)[0])))
		}
	}
	fw.Close()
	total += buf.Len()
	return total
}

// TestTargetBinaryCompression expects a binary serialization codec applying
// varint encoding and content compression, bringing the wire/storage size of a
// highly compressible document orders of magnitude below the current raw
// columnar representation. No such codec exists yet, so the current in-memory
// columnar size dwarfs the compressed target.
func TestTargetBinaryCompression(t *testing.T) {
	log := newOpLog[runeText]()
	const total = 200000
	text := strings.Repeat("the quick brown fox jumps over the lazy dog ", (total+44)/45)
	if len(text) < total {
		text += strings.Repeat("a", total-len(text))
	}
	log.pushLocalOp(1, op[runeText]{opType: opTypeIns, content: runeText(text), pos: 0})

	data := log.Marshal()
	_, rawSize := estimateSize(log, data)
	compressed := estimateCompressedSize(log)
	if rawSize > compressed*10 {
		t.Errorf("binary compression not implemented: raw columnar ~%d bytes is >10x the ~%d byte varint+compressed target", rawSize, compressed)
	}
}
