package crdt

import (
	"slices"
	"testing"
	"unicode/utf8"
)

// runeOpsToString renders the model string from runes so multibyte content and
// character positions stay rune-accurate (the rope indexes characters, not
// bytes).
func runeOpsToString(ct *contentTree[runeText]) string {
	s := ""
	ct.ForEachContent(func(r runeText) { s += string(r) })
	return s
}

func TestRopeMatchesNaive(t *testing.T) {
	type opT struct {
		kind string // "ins" | "del"
		pos  int
		text string
		n    int
	}
	cases := []struct {
		name string
		ops  []opT
	}{
		{"append", []opT{{"ins", 0, "a", 0}, {"ins", 1, "b", 0}, {"ins", 2, "c", 0}}},
		{"prepend", []opT{{"ins", 0, "c", 0}, {"ins", 0, "b", 0}, {"ins", 0, "a", 0}}},
		{"interior", []opT{{"ins", 0, "ac", 0}, {"ins", 1, "b", 0}}},
		{"multibyte interior", []opT{{"ins", 0, "a€z", 0}, {"ins", 2, "ÿ", 0}}},
		{"delete middle of one leaf", []opT{{"ins", 0, "abcdef", 0}, {"del", 2, "", 1}}},
		{"delete across leaves", []opT{
			{"ins", 0, "abcdef", 0}, {"del", 1, "", 1}, // forces split; then append
			{"ins", 0, "0123", 0}, {"del", 0, "", 2}, {"del", 5, "", 2},
		}},
		{"delete whole doc", []opT{{"ins", 0, "abcdefghij", 0}, {"del", 0, "", 10}}},
		{"many small appends", func() []opT {
			var ops []opT
			for i := 0; i < 3000; i++ {
				ops = append(ops, opT{"ins", i, "x", 0})
			}
			for i := 0; i < 1000; i++ {
				ops = append(ops, opT{"del", 1000, "", 1})
			}
			return ops
		}(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ct := newContentTree[runeText]()
			var want []rune
			for _, o := range tc.ops {
				switch o.kind {
				case "ins":
					ct.Insert(o.pos, runeText(o.text))
					ins := []rune(o.text)
					want = append(want, make([]rune, len(ins))...)
					copy(want[o.pos+len(ins):], want[o.pos:])
					copy(want[o.pos:], ins)
				case "del":
					ct.Delete(o.pos, o.n)
					want = append(want[:o.pos], want[o.pos+o.n:]...)
				}
				if ct.Len() != len(want) {
					t.Fatalf("after %+v: Len=%d want %d", o, ct.Len(), len(want))
				}
			}
			if got := runeOpsToString(ct); got != string(want) {
				t.Fatalf("content %q != model %q", got, string(want))
			}
		})
	}
}

// TestRopeArrayMatchesNaive mirrors TestRopeMatchesNaive against a []int
// model, exercising the slice-backed itemRun content.
func TestRopeArrayMatchesNaive(t *testing.T) {
	type opT struct {
		kind string // "ins" | "del"
		pos  int
		vals []int
		n    int
	}
	cases := []struct {
		name string
		ops  []opT
	}{
		{"append", []opT{{"ins", 0, []int{1}, 0}, {"ins", 1, []int{2}, 0}, {"ins", 2, []int{3}, 0}}},
		{"prepend", []opT{{"ins", 0, []int{3}, 0}, {"ins", 0, []int{2}, 0}, {"ins", 0, []int{1}, 0}}},
		{"interior", []opT{{"ins", 0, []int{1, 3}, 0}, {"ins", 1, []int{2}, 0}}},
		{"multi-element insert", []opT{{"ins", 0, []int{1, 2, 3, 4}, 0}, {"ins", 2, []int{8, 9}, 0}}},
		{"delete middle of one leaf", []opT{{"ins", 0, ints(300), 0}, {"del", 100, nil, 50}}},
		{"delete across leaves", []opT{
			{"ins", 0, ints(200), 0},
			{"del", 50, nil, 1},  // splits the single leaf into [0,49] and [51,199]
			{"del", 40, nil, 60}, // crosses the boundary, both leaves partially removed
			{"del", 0, nil, 40},  // removes the whole left leaf
			{"del", 50, nil, 20}, // interior delete inside the surviving leaf
			{"ins", 5, []int{7, 7}, 0},
		}},
		{"delete whole doc", []opT{{"ins", 0, ints(10), 0}, {"del", 0, nil, 10}}},
		{"scatter deletes", []opT{
			{"ins", 0, ints(400), 0},
			{"del", 10, nil, 5},
			{"del", 200, nil, 60},
			{"del", 0, nil, 3},
			{"del", 300, nil, 30},
			{"ins", 150, []int{4, 4, 4}, 0},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ct := newContentTree[itemRun[int]]()
			var want []int
			for _, o := range tc.ops {
				switch o.kind {
				case "ins":
					ct.Insert(o.pos, itemRun[int](o.vals))
					want = append(want, make([]int, len(o.vals))...)
					copy(want[o.pos+len(o.vals):], want[o.pos:])
					copy(want[o.pos:], o.vals)
				case "del":
					ct.Delete(o.pos, o.n)
					want = append(want[:o.pos], want[o.pos+o.n:]...)
				}
				if ct.Len() != len(want) {
					t.Fatalf("after %+v: Len=%d want %d", o, ct.Len(), len(want))
				}
			}
			var got []int
			ct.ForEachContent(func(r itemRun[int]) { got = append(got, r...) })
			if !slices.Equal(got, want) {
				t.Fatalf("content %v != model %v", got, want)
			}
		})
	}
}

func ints(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func TestRopeLeafCountSmallAfterAppends(t *testing.T) {
	ct := newContentTree[runeText]()
	for i := 0; i < 5000; i++ {
		ct.Insert(i, runeText("x"))
	}
	if leaves := ct.leafCount(); leaves > 100 {
		t.Fatalf("append-only 5000 chars left %d leaves; want << 5000 (coalesced)", leaves)
	}
	if ct.Len() != 5000 {
		t.Fatalf("Len = %d, want 5000", ct.Len())
	}
}

// TestRopeMultibyteDeleteStorm deletes single characters from a rope of mixed
// ASCII and multibyte content (2-, 3-, and 4-byte runes) at deterministic
// positions, including positions computed to land on multibyte runes, until
// leaf boundaries fall inside or adjacent to multibyte runes. Final content
// must match a naive []rune model exactly and the leaf count must stay
// bounded (coalescing intact).
func TestRopeMultibyteDeleteStorm(t *testing.T) {
	const (
		insRuns = 30 // 16 runes per copy -> 480 runes, ~2 leaves at ropeLeafCap
		dels    = 200
	)
	text := "héllo wörld 你好 \U0001F600"
	ct := newContentTree[runeText]()
	var want []rune
	for i := 0; i < insRuns; i++ {
		ct.Insert(ct.Len(), runeText(text))
		want = append(want, []rune(text)...)
	}
	if ct.Len() != len(want) {
		t.Fatalf("build: Len=%d want %d", ct.Len(), len(want))
	}

	delAt := func(pos int, phase string, i int) {
		ct.Delete(pos, 1)
		want = append(want[:pos], want[pos+1:]...)
		if ct.Len() != len(want) {
			t.Fatalf("%s del %d @%d: Len=%d want %d", phase, i, pos, ct.Len(), len(want))
		}
	}

	// Scatter: pseudorandom positions across the whole document.
	for i := 0; i < dels; i++ {
		delAt((i*7919)%ct.Len(), "scatter", i)
	}
	// Targeted: always the first surviving multibyte rune, so deletes land
	// inside multibyte runs next to the leaf boundaries the scatter created.
	for i := 0; len(want) > 0 && i < 40; i++ {
		pos := -1
		for j, r := range want {
			if r >= utf8.RuneSelf {
				pos = j
				break
			}
		}
		if pos < 0 {
			break
		}
		delAt(pos, "multibyte", i)
	}

	if leaves := ct.leafCount(); leaves > insRuns {
		t.Fatalf("storm left %d leaves; want bounded (coalescing) count <= %d", leaves, insRuns)
	}
	if got := runeOpsToString(ct); got != string(want) {
		t.Fatalf("content %q != model %q", got, string(want))
	}
}

// TestRuneTextSplitAt checks that SplitAt splits on rune boundaries, not byte
// boundaries, for multibyte content.
func TestRuneTextSplitAt(t *testing.T) {
	cases := []struct {
		s    string
		k    int
		a, b string
	}{
		{"a€z", 0, "", "a€z"},
		{"a€z", 1, "a", "€z"},
		{"a€z", 2, "a€", "z"},
		{"a€z", 3, "a€z", ""},
		{"hello", 2, "he", "llo"},
	}
	for _, tc := range cases {
		a, b := runeText(tc.s).SplitAt(tc.k)
		if string(a) != tc.a || string(b) != tc.b {
			t.Errorf("%q.SplitAt(%d) = (%q, %q), want (%q, %q)", tc.s, tc.k, string(a), string(b), tc.a, tc.b)
		}
	}
}

// BenchmarkRopeDeleteStorm measures the rope under a delete-storm workload:
// build by appending (cap-sized leaves), then scattered single-char deletes —
// which fragment leaves toward single characters when seams are not
// coalesced — then further interior edits whose per-op cost depends on the
// tree shape the storm left behind. The custom "leaves-after-storm" metric
// makes fragmentation directly visible; without seam coalescing it grows by
// roughly one leaf per splitting delete (~6655 leaves observed without
// coalescing), with coalescing it stays ~buildChars/ropeLeafCap.
func BenchmarkRopeDeleteStorm(b *testing.B) {
	const (
		buildChars   = 30000
		stormDeletes = 10000
		postInserts  = 3000
	)

	b.ReportAllocs()
	var ct *contentTree[runeText]
	var leavesAfterStorm int
	for b.Loop() {
		b.StopTimer()
		ct = newContentTree[runeText]()
		for i := 0; i < buildChars; i++ {
			ct.Insert(i, runeText("x"))
		}
		b.StartTimer()

		for i := 0; i < stormDeletes; i++ {
			ct.Delete((i*7919)%ct.Len(), 1)
		}
		b.StopTimer()
		leavesAfterStorm = ct.leafCount()
		b.StartTimer()

		for i := 0; i < postInserts; i++ {
			ct.Insert((i*104729)%ct.Len(), runeText("y"))
		}
		// No trailing b.StopTimer(): on Go 1.24+ StopTimer poisons the loop and
		// the next b.Loop call would fail with "B.Loop called with timer
		// stopped". b.Loop stops the timer itself on the final call.
	}
	b.ReportMetric(float64(leavesAfterStorm), "leaves-after-storm")

	if ct.Len() != buildChars-stormDeletes+postInserts {
		b.Fatalf("final rope length %d, want %d", ct.Len(), buildChars-stormDeletes+postInserts)
	}
}
