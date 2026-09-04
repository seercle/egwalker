package crdt

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

// buildReplicaPair builds two replicas sharing a common prefix of ops (a real
// common ancestor: the prefix is built once on a and merged into b, so both
// hold the same op ids), then diverging with each replica's own strided
// single-char inserts. Positions are strided so consecutive inserts do not
// fold into run ops: op count tracks edit count, which is what the merge
// path's cost depends on.
func buildReplicaPair(n int) (a, b *RuneDocument) {
	a, b = NewRuneDocument(0), NewRuneDocument(1)
	common := n / 2
	for i := 0; i < common; i++ {
		pos := (i * 7919) % (a.Len() + 1)
		a.Ins(pos, "x")
	}
	b.MergeFrom(a)
	for i := 0; i < n-common; i++ {
		a.Ins((i*104729)%(a.Len()+1), "a")
		b.Ins((i*15485863)%(b.Len()+1), "b")
	}
	return a, b
}

// BenchmarkMergeAtScale measures a first sync between two diverged replicas:
// a.MergeFrom(b) pulls k = n/2 remote ops through pushRemoteOp, each paying
// resolveParentLV -> runIdxForSeq (a backward O(#ops) scan). Time growth from
// 10k -> 50k is the O(N*k) signal this benchmark exists to expose.
//
// Untimed per-iteration setup rebuilds both replicas; the timed region is the
// merge only. The 1s default benchtime would loop for minutes on large sizes
// — run with explicit counts, e.g.:
//
//	go test -C go ./crdt -run '^$' -bench 'BenchmarkMergeAtScale' -benchtime=3x
func BenchmarkMergeAtScale(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 50_000} {
		b.Run("ops="+strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				a, bb := buildReplicaPair(n)
				b.StartTimer()

				a.MergeFrom(bb)
			}

			// Untimed convergence verification on a fresh pair: a.MergeFrom
			// is one-directional, so a full sync (reverse merge, untimed) must
			// leave both replicas with the same length. Run once after the
			// loop; the pair is deterministic, so one check covers all
			// iterations without slowing every one of them down.
			a, bb := buildReplicaPair(n)
			a.MergeFrom(bb)
			bb.MergeFrom(a)
			if a.Len() != bb.Len() {
				b.Fatalf("converged lengths differ: %d vs %d", a.Len(), bb.Len())
			}
		})
	}
}

// BenchmarkCheckoutScale measures full-history replay cost via checkout(log)
// (crdt.go:598) at three log sizes. The log is built once per size (untimed);
// checkout is pure and re-runnable.
func BenchmarkCheckoutScale(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 50_000} {
		b.Run("ops="+strconv.Itoa(n), func(b *testing.B) {
			b.StopTimer()
			a, bb := buildReplicaPair(n)
			a.MergeFrom(bb) // merged log exercises branchy history
			log := a.doc.opLog
			ops := len(log.ops)
			b.StartTimer()
			b.ReportAllocs()

			for b.Loop() {
				ct := checkout(log)
				if ct.Len() != a.Len() {
					b.Fatalf("checkout len %d, want %d", ct.Len(), a.Len())
				}
			}
			b.ReportMetric(float64(ops), "log-ops")
		})
	}
}

// replayTrace decodes raw trace JSON and applies every edit to doc, mirroring
// the load-and-replay loop of TestTrace in trace_test.go (which must not be
// modified). It errors on malformed input and leaves doc with the full trace
// applied.
func replayTrace(doc *RuneDocument, raw []byte) error {
	var trace Trace
	if err := json.Unmarshal(raw, &trace); err != nil {
		return err
	}
	for _, edit := range trace.Edits {
		if edit.IsInsert {
			doc.Ins(edit.Position, edit.Char)
		} else {
			doc.Del(edit.Position, 1)
		}
	}
	return nil
}

// BenchmarkColumnarRoundTrip measures struct-level Marshal+Unmarshal at
// trace scale and reports the size metrics the binary format (Task 6) must
// beat: content bytes and a naive struct-size estimate (content bytes plus
// 8 bytes per integer entry). The trace is loaded and replayed once, untimed.
//
//	go test -C go ./crdt -run '^$' -bench 'BenchmarkColumnarRoundTrip' -count=1
func BenchmarkColumnarRoundTrip(b *testing.B) {
	raw, err := os.ReadFile("../../resources/editing-trace.json")
	if err != nil {
		b.Fatalf("read trace: %v", err)
	}
	doc := NewRuneDocument(0)
	if err := replayTrace(doc, raw); err != nil {
		b.Fatalf("replay trace: %v", err)
	}
	log := doc.doc.opLog

	contentBytes := 0
	for _, o := range log.ops {
		contentBytes += len([]byte(o.content))
	}
	data := log.Marshal()
	structBytes := contentBytes + 8*(len(data.Types)+len(data.TypeRuns)+len(data.Agents)+
		len(data.AgentRuns)+len(data.Seqs)+len(data.Positions)+len(data.Lengths)+
		len(data.Parents)+len(data.Frontier))

	b.ReportAllocs()

	for b.Loop() {
		d := log.Marshal()
		round := Unmarshal[runeText](d)
		if len(round.ops) != len(log.ops) {
			b.Fatalf("round-trip op count %d, want %d", len(round.ops), len(log.ops))
		}
	}

	// Report after the loop: b.Loop's first call resets the timer, which
	// clears any metrics reported before it.
	b.ReportMetric(float64(contentBytes), "content-bytes")
	b.ReportMetric(float64(structBytes), "struct-estimate-bytes")
	b.ReportMetric(float64(len(log.ops)), "log-ops")
}
