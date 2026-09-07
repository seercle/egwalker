package crdt

import (
	"reflect"
	"strings"
	"testing"

	"egwalker/bxtree"
)

// streamDeleteRun replays a delete-run op's per-character staging against doc
// WITHOUT touching any snapshot, returning the per-character endPos stream
// (-1 entries kept). Diagnostic oracle for the batching rule.
func streamDeleteRun[C content[C]](doc *crdtDoc, log *opLog[C], opLV lv) []int {
	idx := log.opIdxAt(opLV)
	o := &log.ops[idx]
	first := log.opLV[idx]
	out := make([]int, 0, o.length)
	for i := 0; i < o.length; i++ {
		out = append(out, deleteOne(doc, log, first+lv(i), o.pos))
	}
	return out
}

// newStreamDoc builds a bare crdtDoc like checkout does (snapshot-free).
func newStreamDoc() *crdtDoc {
	return &crdtDoc{
		items: newBxTree(
			bxtree.WithSummarizer(crdtSummaryConfig),
			bxtree.WithOnItemMoved(func(item *crdtItem, node *bxtree.Node[*crdtItem, crdtSummary]) {
				item.node = node
			}),
		),
		currentVersion: []lv{},
		delTargets:     make(map[lv]lv),
		sortedItems:    []*crdtItem{},
	}
}

// streamAllDeleteRuns replays the log topologically like checkout: every op
// goes through the real do1Operation path (nil snapshot), except delete runs,
// whose per-character endPos stream is captured via streamDeleteRun instead of
// apply's branch (-1 entries kept). Returns one stream per delete op in log
// order.
func streamAllDeleteRuns[C content[C]](log *opLog[C]) [][]int {
	doc := newStreamDoc()
	var out [][]int
	for i := 0; i < len(log.ops); i++ {
		first := log.opLV[i]
		if log.ops[i].opType == opTypeDel {
			end := log.endLV(i)
			res := diff(log, doc.currentVersion, log.ops[i].parents)
			for _, x := range res.aOnly {
				retreat(doc, log, x)
			}
			for _, x := range res.bOnly {
				advance(doc, log, x)
			}
			out = append(out, streamDeleteRun(doc, log, first))
			doc.currentVersion = []lv{end}
		} else {
			do1Operation(doc, log, first, nil)
			doc.currentVersion = []lv{log.endLV(i)}
		}
	}
	return out
}

func TestDeleteRunEndPosStreams(t *testing.T) {
	// Every case builds state so the delete run is authored by an OTHER
	// replica and replayed remotely; streams are the observed endPos
	// values deleteOne reports during the staged replay (snapshot == nil;
	// batchDeleteRuns flag irrelevant — pure observation).
	cases := []struct {
		name      string
		build     func() *opLog[runeText]
		streamIdx []int
		want      [][]int
		desc      string
	}{
		{
			// Remote replay of a plain contiguous 3-char run: b staged
			// "abcde"; the run removes b,c,d at pos 1.
			name: "plain contiguous",
			build: func() *opLog[runeText] {
				a := NewRuneDocument(1)
				a.Ins(0, "abcde")
				b := NewRuneDocument(2)
				b.MergeFrom(a)
				a.Del(1, 3)
				b.MergeFrom(a)
				return b.doc.opLog
			},
			streamIdx: []int{0},
			want:      [][]int{{1, 1, 1}},
			desc:      "3-char interior run: consecutive chars shift into the vacated index; chain expected",
		},
		{
			// a "hello"; b deletes one 'l' locally (concurrent); c deletes
			// the whole "hello" run; b merges c remotely: c's 5-char run
			// crosses b's already-deleted char -> one -1.
			name: "hole via concurrent del",
			build: func() *opLog[runeText] {
				a := NewRuneDocument(1)
				a.Ins(0, "hello")
				b := NewRuneDocument(2)
				b.MergeFrom(a)
				b.Del(2, 1)
				c := NewRuneDocument(3)
				c.MergeFrom(a)
				c.Del(0, 5)
				b.MergeFrom(c)
				return b.doc.opLog
			},
			streamIdx: []int{0, 1},
			want:      [][]int{{2}, {0, 0, -1, 0, 0}},
			desc:      "one char concurrent-deleted: -1 mid-run; chain keeps running through it",
		},
		{
			// Both halves delete the identical full run concurrently.
			name: "whole run already deleted",
			build: func() *opLog[runeText] {
				a := NewRuneDocument(1)
				a.Ins(0, "hello")
				b := NewRuneDocument(2)
				b.MergeFrom(a)
				b.Del(0, 5)
				c := NewRuneDocument(3)
				c.MergeFrom(a)
				c.Del(0, 5)
				b.MergeFrom(c)
				return b.doc.opLog
			},
			streamIdx: []int{0, 1},
			want:      [][]int{{0, 0, 0, 0, 0}, {-1, -1, -1, -1, -1}},
			desc:      "entire run already deleted: all -1; chain never opens; no snapshot delete",
		},
		{
			// The delete run is already known to z from its first merge
			// (via x); the second merge (via y) replays it through
			// checkoutFancy's sharedOps path with snapshot == nil. The
			// streaming helper itself is a nil-snapshot replay, so this
			// case is structurally the plain-contiguous shape, pinning
			// that the batcher's nil guard equals the per-char guard.
			name: "snapshot nil (shared op replay)",
			build: func() *opLog[runeText] {
				a := NewRuneDocument(1)
				a.Ins(0, "abcde")
				x := NewRuneDocument(2)
				x.MergeFrom(a)
				a.Del(1, 3)
				x.MergeFrom(a)
				y := NewRuneDocument(3)
				y.MergeFrom(a)
				y.MergeFrom(x)
				z := NewRuneDocument(4)
				z.MergeFrom(x)
				z.Ins(z.Len(), "Z")
				z.MergeFrom(y)
				return z.doc.opLog
			},
			streamIdx: []int{0},
			want:      [][]int{{1, 1, 1}},
			desc:      "delete run replayed with snapshot nil; stream shape identical to plain contiguous",
		},
		{
			// Remote replay of a single-character run: chain of one group.
			name: "length-1 run",
			build: func() *opLog[runeText] {
				a := NewRuneDocument(1)
				a.Ins(0, "abc")
				b := NewRuneDocument(2)
				b.MergeFrom(a)
				a.Del(1, 1)
				b.MergeFrom(a)
				return b.doc.opLog
			},
			streamIdx: []int{0},
			want:      [][]int{{1}},
			desc:      "single char run: exactly one chain of length 1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := streamAllDeleteRuns(tc.build())
			if len(got) != len(tc.streamIdx) {
				t.Fatalf("expected %d delete-run streams, got %d", len(tc.streamIdx), len(got))
			}
			for k, si := range tc.streamIdx {
				if !reflect.DeepEqual(got[si], tc.want[k]) {
					t.Fatalf("case %q stream[%d] = %v, want %v (%s)", tc.name, si, got[si], tc.want[k], tc.desc)
				}
			}
		})
	}
	_ = reflect.DeepEqual
}

// TestDeleteRunBatchEquivalence drives the delete-run branch end-to-end
// through RuneDocument.MergeFrom: for each case the same merge is applied
// twice — once with batchDeleteRuns=false (per-character, today's behavior)
// and once with =true (batched) — on identical rebuilt fixtures, and the full
// application results are compared: non-empty content bytes (GetString),
// length, doc.Check, and version equality. Any deviation anywhere in the
// batcher, including multi-leaf seam behavior of contentTree.Delete, fails.
func TestDeleteRunBatchEquivalence(t *testing.T) {
	cases := []struct {
		name      string
		buildDst  func() *RuneDocument // pre-merge replica (takes the deletes remotely)
		buildSrc  func() *RuneDocument // replica whose del ops dst receives
		postMerge func(dst, src *RuneDocument)
	}{
		{
			name: "plain contiguous",
			buildDst: func() *RuneDocument {
				master := NewRuneDocument(1)
				master.Ins(0, "abcde")
				dst := NewRuneDocument(2)
				dst.MergeFrom(master)
				return dst
			},
			buildSrc: func() *RuneDocument {
				src := NewRuneDocument(1)
				src.Ins(0, "abcde")
				src.Del(1, 3)
				return src
			},
		},
		{
			name: "hole via concurrent del",
			buildDst: func() *RuneDocument {
				master := NewRuneDocument(1)
				master.Ins(0, "hello")
				dst := NewRuneDocument(2)
				dst.MergeFrom(master)
				dst.Del(2, 1) // concurrent local delete turns one run char into a hole
				return dst
			},
			buildSrc: func() *RuneDocument {
				src := NewRuneDocument(3)
				master := NewRuneDocument(1)
				master.Ins(0, "hello")
				src.MergeFrom(master)
				src.Del(0, 5)
				return src
			},
		},
		{
			name: "whole run already deleted",
			buildDst: func() *RuneDocument {
				master := NewRuneDocument(1)
				master.Ins(0, "hello")
				dst := NewRuneDocument(2)
				dst.MergeFrom(master)
				dst.Del(0, 5)
				return dst
			},
			buildSrc: func() *RuneDocument {
				src := NewRuneDocument(3)
				master := NewRuneDocument(1)
				master.Ins(0, "hello")
				src.MergeFrom(master)
				src.Del(0, 5)
				return src
			},
		},
		{
			name: "snapshot nil (shared op replay)",
			buildDst: func() *RuneDocument {
				master := NewRuneDocument(1)
				master.Ins(0, "abcde")
				x := NewRuneDocument(2)
				x.MergeFrom(master)
				master.Del(1, 3)
				x.MergeFrom(master)
				z := NewRuneDocument(4)
				z.MergeFrom(x)
				z.Ins(z.Len(), "Z") // second merge must carry fresh ops
				return z
			},
			buildSrc: func() *RuneDocument {
				master := NewRuneDocument(1)
				master.Ins(0, "abcde")
				x := NewRuneDocument(2)
				x.MergeFrom(master)
				master.Del(1, 3)
				x.MergeFrom(master)
				y := NewRuneDocument(3)
				y.MergeFrom(master)
				y.MergeFrom(x)
				return y
			},
		},
		{
			name: "two overlapping del runs from different agents",
			buildDst: func() *RuneDocument {
				dst := NewRuneDocument(4)
				return dst
			},
			buildSrc: func() *RuneDocument {
				master := NewRuneDocument(1)
				master.Ins(0, "abcdefgh")
				x := NewRuneDocument(2)
				x.MergeFrom(master)
				x.Del(1, 4) // deletes bcde
				y := NewRuneDocument(3)
				y.MergeFrom(master)
				y.Del(3, 4) // deletes defg (overlaps x's run)
				y.MergeFrom(x)
				return y
			},
		},
		{
			name: "long runs cross multi-leaf seams",
			buildDst: func() *RuneDocument {
				dst := NewRuneDocument(4)
				return dst
			},
			buildSrc: func() *RuneDocument {
				master := NewRuneDocument(1)
				master.Ins(0, strings.Repeat("a", 3000))
				x := NewRuneDocument(2)
				x.MergeFrom(master)
				x.Ins(1000, strings.Repeat("b", 500))
				x.Del(900, 900) // spans the seam
				y := NewRuneDocument(3)
				y.MergeFrom(master)
				y.MergeFrom(x)
				y.Del(1500, 1000) // spans the seam and x's not-yet-integrated region boundary
				y.MergeFrom(x)
				return y
			},
		},
		{
			name: "multibyte content",
			buildDst: func() *RuneDocument {
				dst := NewRuneDocument(4)
				return dst
			},
			buildSrc: func() *RuneDocument {
				master := NewRuneDocument(1)
				master.Ins(0, "字é字日月é字")
				master.Ins(5, "ßü")
				src := NewRuneDocument(2)
				src.MergeFrom(master)
				src.Del(1, 6) // crosses multibyte runes on both sides
				return src
			},
		},
		{
			name: "length-1 run",
			buildDst: func() *RuneDocument {
				master := NewRuneDocument(1)
				master.Ins(0, "abc")
				dst := NewRuneDocument(2)
				dst.MergeFrom(master)
				return dst
			},
			buildSrc: func() *RuneDocument {
				src := NewRuneDocument(1)
				src.Ins(0, "abc")
				src.Del(1, 1)
				return src
			},
		},
	}
	runWithFlag := func(flag bool, tc struct {
		name      string
		buildDst  func() *RuneDocument
		buildSrc  func() *RuneDocument
		postMerge func(dst, src *RuneDocument)
	}) *RuneDocument {
		old := batchDeleteRuns
		batchDeleteRuns = flag
		defer func() { batchDeleteRuns = old }()
		dst, src := tc.buildDst(), tc.buildSrc()
		dst.MergeFrom(src)
		if tc.postMerge != nil {
			tc.postMerge(dst, src)
		}
		dst.Check()
		if src != nil {
			src.Check()
		}
		return dst
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			perChar := runWithFlag(false, tc)
			batched := runWithFlag(true, tc)
			if perChar.GetString() != batched.GetString() {
				t.Fatalf("flag=%v → %d chars %q, flag=%v → %d chars %q",
					false, perChar.Len(), perChar.GetString(), true, batched.Len(), batched.GetString())
			}
			if perChar.Len() != batched.Len() {
				t.Fatalf("length diverged: %d vs %d", perChar.Len(), batched.Len())
			}
			if !reflect.DeepEqual(perChar.Version(), batched.Version()) {
				t.Fatalf("version diverged: %v vs %v", perChar.Version(), batched.Version())
			}
		})
	}
}
