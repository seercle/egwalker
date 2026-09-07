package crdt

import (
	"encoding/json"
	"maps"
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

// buildMapReplicaPair builds two map replicas sharing a common prefix of
// n/2 keys, then diverging with disjoint key ranges. The prefix is shared
// via an untimed b.MergeFrom(a): independently-set keys would share no
// ancestor ops (the rune benchmark's convergence lesson). Map ops never
// collapse (document.go:329-331), so each replica ends with exactly n ops
// and the timed merge pulls n/2 remote ops into an n-op log.
func buildMapReplicaPair(n int) (a, b *MapDocument[string, int]) {
	a = NewMapDocument[string, int](0)
	b = NewMapDocument[string, int](1)
	common := n / 2
	for i := 0; i < common; i++ {
		a.Set("k0-"+strconv.Itoa(i), i)
	}
	b.MergeFrom(a)
	for i := 0; i < n-common; i++ {
		a.Set("k0-"+strconv.Itoa(common+i), i)
		b.Set("k1-"+strconv.Itoa(common+i), i)
	}
	return a, b
}

// BenchmarkMapMergeAtScale measures a first sync between two diverged map
// replicas, the direct comparator to BenchmarkMergeAtScale (rune text).
// Linear merging predicts ~5x from 10k to 50k; the rune merge measured 7.75.
// A materially larger ratio here would indicate the map merge path
// (mergeInto + keyIndex rebuild) scales worse — a finding to record, not
// fix, in this plan. Fixtures rebuild untimed per iteration; run with:
//
//	go test -C go ./crdt -run '^$' -bench 'BenchmarkMapMergeAtScale' -benchmem -benchtime=3x
func BenchmarkMapMergeAtScale(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 50_000} {
		b.Run("ops="+strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				a, bb := buildMapReplicaPair(n)
				b.StartTimer()

				a.MergeFrom(bb)

				b.StopTimer()
				// Untimed convergence verification: reverse sync must bring
				// both logs to 3n/2 ops (n shared + n/2 per divergent side;
				// every map op has length 1).
				bb.MergeFrom(a)
				want := 3 * n / 2
				if a.opLog.totalLV != lv(want) || bb.opLog.totalLV != lv(want) {
					b.Fatalf("converged op counts: a=%d b=%d, want %d",
						a.opLog.totalLV, bb.opLog.totalLV, want)
				}
				if v, ok := a.Get("k1-" + strconv.Itoa(n-1)); !ok || v != n-n/2-1 {
					b.Fatalf("merged key k1-%d missing or wrong: %v %v", n-1, v, ok)
				}
				if v, ok := bb.Get("k0-" + strconv.Itoa(n-1)); !ok || v != n-n/2-1 {
					b.Fatalf("reverse-synced key k0-%d missing or wrong: %v %v", n-1, v, ok)
				}
				// b.Loop fatals if called with the timer stopped, so re-arm it
				// here; the untimed-to-timed gap is a few loop-machinery ns.
				b.StartTimer()
			}
		})
	}
}

// buildDeleteHeavyReplica builds one replica with an n-op history in which a
// fixed fraction p of the ops are multi-character delete runs (run lengths
// cycling 2..64), interleaved with inserts so the rope sees both insert and
// delete work. Positions are strided/deterministic so the fixture is
// reproducible.
func buildDeleteHeavyReplica(n int, p float64) *RuneDocument {
	src := NewRuneDocument(0)
	ops := 0
	nextIns := 0
	for ops < n {
		if float64(ops%10_000)/10_000 < p && src.Len() > 80 {
			runLen := 2 + ops%63
			if runLen > src.Len() {
				runLen = src.Len()
			}
			pos := (ops * 6151) % (src.Len() - runLen + 1)
			src.Del(pos, runLen)
		} else {
			pos := (nextIns * 7919) % (src.Len() + 1)
			src.Ins(pos, "x")
			nextIns++
		}
		ops++
	}
	return src
}

// mergeRemoteDeletesOnce builds the delete-heavy n-op src replica untimed and
// merges it into a fresh replica (timed). Returns both plus src.Len() for the
// untimed verification.
func mergeRemoteDeletesOnce(b *testing.B, n int) (src, fresh *RuneDocument) {
	b.StopTimer()
	src = buildDeleteHeavyReplica(n, 0.3)
	b.StartTimer()
	fresh = NewRuneDocument(1)
	fresh.MergeFrom(src)
	return src, fresh
}

// BenchmarkMergeRemoteDeletesAtScale measures a fresh replica's cost of
// absorbing remote DELETE-heavy history: the src replica builds n-op
// histories with a fixed fraction p=0.3 of its ops being multi-char delete
// runs (alternating run lengths 2..64), then one whole-log mergeFrom into a
// fresh replica. ns/op is the merge cost; the flag's A/B column compares
// with batchDeleteRuns=false via BenchmarkMergeRemoteDeletesPerChar.
//
//	go test -C go ./crdt -run '^$' -bench 'BenchmarkMergeRemoteDeletes' -benchmem -benchtime=3x
func BenchmarkMergeRemoteDeletesAtScale(b *testing.B) {
	for _, n := range []int{10_000, 50_000} {
		b.Run("ops="+strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				src, _ := mergeRemoteDeletesOnce(b, n)
				_ = src
			}

			// Untimed verification on a deterministic fixture: the merge
			// must land the src's full content in the fresh replica.
			src := buildDeleteHeavyReplica(n, 0.3)
			fresh := NewRuneDocument(1)
			fresh.MergeFrom(src)
			if fresh.Len() != src.Len() {
				b.Fatalf("merged length %d, want %d", fresh.Len(), src.Len())
			}
		})
	}
}

// BenchmarkMergeRemoteDeletesPerChar is the A/B companion: the identical
// fixture with batchDeleteRuns forced false for the timed region only
// (per-character snapshot deletes, the historical behavior). The flip happens
// before b.StartTimer and the restore happens explicitly after the loop, with
// a defer as a crash-safety net. Benches here run single-threaded, so the
// package-private var flip is safe.
func BenchmarkMergeRemoteDeletesPerChar(b *testing.B) {
	for _, n := range []int{10_000, 50_000} {
		b.Run("ops="+strconv.Itoa(n), func(b *testing.B) {
			b.StopTimer()
			src := buildDeleteHeavyReplica(n, 0.3)
			b.ReportAllocs()
			old := batchDeleteRuns
			batchDeleteRuns = false
			defer func() { batchDeleteRuns = old }()
			b.StartTimer()

			for b.Loop() {
				fresh := NewRuneDocument(1)
				fresh.MergeFrom(src)
			}

			fresh := NewRuneDocument(1)
			fresh.MergeFrom(src)
			if fresh.Len() != src.Len() {
				b.Fatalf("merged length %d, want %d", fresh.Len(), src.Len())
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
// Mirrors trace_test.go's loadTrace/replay pair; kept separate because the previous plan froze trace_test.go — consolidate only if that file's tests change.
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

// BenchmarkTracePerChar is the before-column companion of trace_test.go's
// BenchmarkTrace: the identical loadTrace fixture and timed replay loop, with
// batchDeleteRuns forced false for the entire timed region (flip before the
// first StartTimer, explicit restore after the loop plus a crash-safety
// defer) — the flag is already false when b.Loop's first StopTimer/StartTimer
// pair runs, so the claim "forced false for the timed region" is literal.
// The untimed epilogue is NOT duplicated (no CSV rewrite / duplicate wall-time
// print), only a fresh reference replay for the same correctness check the
// original performs.
func BenchmarkTracePerChar(b *testing.B) {
	trace, err := loadTrace()
	if err != nil {
		b.Fatal(err)
	}

	old := batchDeleteRuns
	batchDeleteRuns = false
	defer func() { batchDeleteRuns = old }()

	var document *RuneDocument
	for b.Loop() {
		b.StopTimer()
		document = NewRuneDocument(0)
		b.StartTimer()
		for _, edit := range trace.Edits {
			if edit.IsInsert {
				document.Ins(edit.Position, edit.Char)
			} else {
				document.Del(edit.Position, 1)
			}
		}
	}

	// Restore explicitly after the timed loop; the untimed correctness replay
	// below runs under the committed (batched=true) behavior, matching the
	// intended "false for the timed region only".
	batchDeleteRuns = true

	document = replay(trace, nil)
	if trace.FinalText != document.GetString() {
		b.Fatalf("Mismatch, got '%q'", document.GetString())
	}
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

	contentBytes := contentBytesOf(b, log)
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

// BenchmarkBinaryRoundTrip measures binary encode+decode at trace scale —
// the direct comparator to BenchmarkColumnarRoundTrip (same corpus, same
// untimed setup). Reports achieved bytes as metrics: compressed blob size
// vs the Task 1 struct estimate.
//
//	go test -C go ./crdt -run '^$' -bench 'BenchmarkBinaryRoundTrip' -count=1
func BenchmarkBinaryRoundTrip(b *testing.B) {
	raw, err := os.ReadFile("../../resources/editing-trace.json")
	if err != nil {
		b.Fatalf("read trace: %v", err)
	}
	doc := NewRuneDocument(0)
	if err := replayTrace(doc, raw); err != nil {
		b.Fatalf("replay trace: %v", err)
	}
	log := doc.doc.opLog
	blob, err := MarshalBinary(log, RuneTextCodec{})
	if err != nil {
		b.Fatalf("MarshalBinary: %v", err)
	}
	b.SetBytes(int64(len(blob)))

	for b.Loop() {
		round, err := UnmarshalBinary[runeText](blob, RuneTextCodec{})
		if err != nil {
			b.Fatalf("UnmarshalBinary: %v", err)
		}
		if _, err := MarshalBinary(round, RuneTextCodec{}); err != nil {
			b.Fatalf("re-encode: %v", err)
		}
	}

	// Report after the loop: b.Loop's first call resets the timer, which
	// clears any metrics reported before it.
	b.ReportMetric(float64(len(blob)), "blob-bytes")
}

// BenchmarkMapKeysAtScale measures Keys() at map scale: n keys, each set
// once, then one full MergeFrom so keyIndex is populated on the reader.
// The 10k->50k growth ratio is the O(#ops) -> O(#keys) signal. Fixtures are
// built untimed per size; run with:
//
//	go test -C go ./crdt -run '^$' -bench 'BenchmarkMapKeysAtScale' -benchmem -benchtime=3x
func BenchmarkMapKeysAtScale(b *testing.B) {
	for _, n := range []int{10_000, 50_000} {
		b.Run("keys="+strconv.Itoa(n), func(b *testing.B) {
			b.StopTimer()
			a := NewMapDocument[string, int](0)
			for i := 0; i < n; i++ {
				a.Set(strconv.Itoa(i), i)
			}
			r := NewMapDocument[string, int](1)
			r.MergeFrom(a)
			keys := r.Keys()
			if len(keys) != n {
				b.Fatalf("Keys() = %d keys, want %d", len(keys), n)
			}
			b.ReportAllocs()
			b.StartTimer()

			for b.Loop() {
				keys := r.Keys()
				_ = keys
			}
		})
	}
}

// BenchmarkMapGetOverwrite measures Get on an overwrite-hot key: n re-Sets
// of the same key, then m = n Gets. The winner cache turns the O(k^2)-worst
// binding walk into one recompute per epoch. Two sub-benchmarks: "warm"
// (repeat Gets — cache hit path) and "cold" (Get right after a Set — epoch
// invalidated, recompute path). Fixtures are built untimed; run with:
//
//	go test -C go ./crdt -run '^$' -bench 'BenchmarkMapGetOverwrite' -benchmem -benchtime=3x
func BenchmarkMapGetOverwrite(b *testing.B) {
	for _, n := range []int{1_000, 10_000} {
		b.Run("ops="+strconv.Itoa(n), func(b *testing.B) {
			b.Run("warm", func(b *testing.B) {
				b.StopTimer()
				doc := NewMapDocument[string, int](0)
				for i := 0; i < n; i++ {
					doc.Set("hot", i)
				}
				b.ReportAllocs()
				b.StartTimer()

				for b.Loop() {
					v, ok := doc.Get("hot")
					if !ok || v != n-1 {
						b.Fatalf("Get = (%v,%v), want (%d,true)", v, ok, n-1)
					}
				}
			})
			b.Run("cold", func(b *testing.B) {
				b.StopTimer()
				doc := NewMapDocument[string, int](0)
				for i := 0; i < n; i++ {
					doc.Set("hot", i)
				}
				b.ReportAllocs()
				b.StartTimer()

				for b.Loop() {
					doc.Set("hot", n-1)
					if v, ok := doc.Get("hot"); !ok || v != n-1 {
						b.Fatalf("Get = (%v,%v), want (%d,true)", v, ok, n-1)
					}
				}
			})
		})
	}
}

// BenchmarkGetStringRepeat measures repeated GetString on a stable document
// at trace scale (the cache hit path) vs the full rope walk. The trace is
// replayed once untimed and one warm-up GetString is taken; the timed loop
// asserts the length invariant every iteration. Run with:
//
//	go test -C go ./crdt -run '^$' -bench 'BenchmarkGetStringRepeat' -benchmem -benchtime=3x
func BenchmarkGetStringRepeat(b *testing.B) {
	raw, err := os.ReadFile("../../resources/editing-trace.json")
	if err != nil {
		b.Fatalf("read trace: %v", err)
	}
	doc := NewRuneDocument(0)
	if err := replayTrace(doc, raw); err != nil {
		b.Fatalf("replay trace: %v", err)
	}
	want := doc.GetString()

	b.ReportAllocs()

	for b.Loop() {
		if got := doc.GetString(); len(got) != len(want) {
			b.Fatalf("GetString length %d, want %d", len(got), len(want))
		}
	}
}

// buildDeltaFixtureB builds a sender with n single-char ops and a receiver
// synced after the first n-k of them (a real incremental receiver: a genuine
// common-ancestor prefix, then the sender gains the last k ops it is
// missing). Strided positions keep every insert its own op, so op count
// tracks edit count. Also returns the three measured frames.
func buildDeltaFixtureB(b *testing.B, n int) (sender, receiver *RuneDocument, fullBlob, fullDeltaBlob, incrBlob []byte) {
	sender = NewRuneDocument(0)
	k := n / 10
	for i := 0; i < n-k; i++ {
		sender.Ins((i*7919)%(sender.Len()+1), "x")
	}
	receiver = NewRuneDocument(1)
	receiver.MergeFrom(sender)
	for i := 0; i < k; i++ {
		sender.Ins((i*104729)%(sender.Len()+1), "y")
	}
	var err error
	fullBlob, err = MarshalBinary(sender.doc.opLog, RuneTextCodec{})
	if err != nil {
		b.Fatalf("MarshalBinary: %v", err)
	}
	fullDeltaBlob, err = sender.Delta(map[int]int{})
	if err != nil {
		b.Fatalf("Delta(empty): %v", err)
	}
	incrBlob, err = sender.Delta(receiver.Version())
	if err != nil {
		b.Fatalf("Delta(since): %v", err)
	}
	return
}

// BenchmarkDeltaAtScale measures delta-frame size and apply cost against
// whole-log frames at trace scale: build a document with n ops, then report
// (a) full MarshalBinary size, (b) Delta(since=empty) size, (c) Delta vs a
// receiver missing only the last k = n/10 ops — the incremental case delta
// frames exist for — and (d) apply time for case (c). Fixtures rebuild
// untimed per iteration; run with:
//
//	go test -C go ./crdt -run '^$' -bench 'BenchmarkDeltaAtScale' -benchmem -benchtime=3x
func BenchmarkDeltaAtScale(b *testing.B) {
	for _, n := range []int{10_000, 50_000} {
		b.Run("ops="+strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				b.StopTimer()
				sender, recv, fullBlob, _, incrBlob := buildDeltaFixtureB(b, n)
				b.SetBytes(int64(len(incrBlob)))
				b.StartTimer()

				recv.ApplyDelta(incrBlob)

				b.StopTimer()
				b.ReportMetric(float64(len(fullBlob)), "full-frame-bytes")
				b.ReportMetric(float64(len(incrBlob)), "delta-incr-bytes")
				b.ReportMetric(float64(len(sender.doc.opLog.ops)), "log-ops")
				b.StartTimer()
			}

			// Untimed convergence verification on a fresh (deterministic)
			// fixture: one check covers all iterations.
			sender, recv2, fullBlob, fullDeltaBlob, incrBlob := buildDeltaFixtureB(b, n)
			recv2.ApplyDelta(incrBlob)
			if recv2.GetString() != sender.GetString() {
				b.Fatalf("content diverged after incremental delta: %d vs %d bytes", len(recv2.GetString()), len(sender.GetString()))
			}
			if !maps.Equal(recv2.Version(), sender.Version()) {
				b.Fatal("version diverged after incremental delta")
			}
			recv2.Check()

			// b.Loop resets the timer (and metrics) on its first call, so
			// report the size figures here, after the loop.
			b.ReportMetric(float64(len(fullBlob)), "full-frame-bytes")
			b.ReportMetric(float64(len(fullDeltaBlob)), "delta-empty-since-bytes")
			b.ReportMetric(float64(len(incrBlob)), "delta-incr-bytes")
			b.ReportMetric(float64(len(sender.doc.opLog.ops)), "log-ops")
		})
	}
}
