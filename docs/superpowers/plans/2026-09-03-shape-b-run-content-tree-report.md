# Shape B Report — Run-Based Content Tree (Rope) + Element-Free Engine

Date: 2026-09-03. Branch: `feat/v2`.

Spec: `docs/superpowers/specs/2026-09-03-shape-b-run-content-tree-design.md`
Plan: `docs/superpowers/plans/2026-09-03-shape-b-run-content-tree.md`

## Summary

Shape B makes the CRDT engine generic over the *content run type only* and
replaces the per-character visible-content snapshot with a run-based content
tree ("rope"), so a contiguous run insert costs ~1 snapshot leaf and the visible
document costs O(#runs) not O(#chars). One plan-spec deviation was required —
the delete-run snapshot behavior — because under merge wind-back a delete run's
characters are not contiguous at a snapshot coordinate; the shipped behavior
restores exact Shape A semantics. Additional performance caveats follow in the
same section. The public API
(`Ins`/`Del`/`Len`/`GetString`/`GetItems`/`MergeFrom`/`Check`/`Reset`, map
`Set`/`Get`/`Keys`, and `Mergeable`/`MergeFromAny`) is unchanged.

## Commits

- `2109730` — `refactor: content-generic engine with run-based content tree (Shape B)` (Task 1: the whole flip — content contract + rope + engine erasure + family split + tests, as one green milestone).
- `327f450` — `test: Shape B leaf-count regressions + trace measurement` (Task 2).
- This report (Task 3) commits only the report file.

Task implementation reports:
- `.superpowers/sdd/2026-09-03-shape-b-run-content-tree/task-1-report.md`
- `.superpowers/sdd/2026-09-03-shape-b-run-content-tree/task-2-report.md`
- `.superpowers/sdd/2026-09-03-shape-b-run-content-tree/task-3-report.md`

## What changed (spec + plan)

The design's two goals were implemented together (one atomic compile-breaking
milestone, exactly as the plan flags):

1. **Self-referential content contract** (`go/crdt/content.go`): `content[C]`
   with `Len`/`SplitAt`/`Concat`/`Collapsible`, plus concrete `runeText`
   (string-backed, rune-indexed), `itemRun[E]` (slice-backed, capped `SplitAt`
   halves so a later `Concat`/`append` cannot alias a sibling leaf), and
   `mapRun[K,V]`. Element access is not part of the interface; it lives only on
   the concrete types / family code.
2. **Run-based content tree** (`go/crdt/content_tree.go`): `contentTree[C]`
   over `bxtree.BxTree[C,int]`.
3. **Engine erasure**: `E` removed from every machinery type and function
   (`op[C]`, `opLog[C]`, `branch[C]`, `ColumnarData[C]`, all walker/merge/split
   code).
4. **Family split** (`go/crdt/document.go`): content-generic `doc[C]` core plus
   `RuneDocument` (now parameterless), `ArrayDocument[T]` (Mergeable fan-out +
   recursive merge in family code), and `MapDocument[K,V]` (kept, type-param
   update only). `Document[E,C]`/`NewDocument` deleted; constructors unchanged.

Out-of-scope items were untouched: `bxtree`, the serialization wire format
(still one entry per run op), the `MapDocument` snapshot (none), the parked
`runIdxForSeq` perf item, and `go/main.go`.

## The rope algorithm

- **Leaf = one content run.** A `runeText`/`itemRun`/`mapRun` value is an
  immutable leaf; `bxtree.Size()` still counts *leaves*, so all positional work
  goes through a plain **character-count summary** (`FromItem = c.Len()`, `Add`/
  `Sub` componentwise) exactly like the item tree does.
- **`locateChar(pos)`** resolves a character position to `(item index, leaf,
  in-leaf offset)` via `bxtree.FindPath` on the accumulated summary — the same
  pattern as the item tree's `findByCurrentPos`. All character positions are in
  `[0, Len]`.
- **`ropeLeafCap = 256`** bounds folded-leaf size. Folding two adjacent leaves
  is `Concat`, costing O(result) ≤ O(ropeLeafCap), so append-heavy workloads
  amortize to O(n) total and end at ~n/ropeLeafCap leaves.
- **`Insert(pos, run)`**: empty-run no-op. End-of-tree and leaf-boundary
  positions fold into the adjacent leaf when it fits `ropeLeafCap`
  (`replaceLeaf(last, tail.Concat(run))`); a position interior to a leaf splits
  that leaf at the offset via `SplitAt` and inserts the run as its own leaf
  (folding into the left half when it fits). `replaceLeaf` deletes the leaf and
  re-inserts non-empty parts, so the tree never stores an empty leaf.
- **`Delete(pos, length)`**: verifies range; whole-document delete is one
  `DeleteRange(0, Size)`. Otherwise it **isolates the range to whole leaves**
  and does one interior range delete: a single-leaf deletion splits the leaf
  into `[before][after]`; a multi-leaf deletion first splits the left
  overlapping leaf so the range starts on a boundary (dropping nothing but
  keeping the out-of-range prefix), **re-runs `locateChar` on the right
  boundary** (indices shift after the left split), splits the right leaf to keep
  only its out-of-range suffix, then `DeleteRange`s the interior leaves.
- `Len()` reads the root summary (0 for an empty/nil tree);
  `ForEachContent` visits leaves in document order.

## The erasure sweep

- `content[C]` interface (Len/SplitAt/Concat/Collapsible) — element-free by
  construction.
- `op[E,C]→op[C]`, `opLog[E,C]→opLog[C]`, `branch[T]→branch[C]` with
  `snapshot *contentTree[C]`, `ColumnarData[E,C]→ColumnarData[C]` — every
  `[E any, C content[E]]` function became `[C content[C]]` across types.go,
  op_log.go, crdt.go, serialization.go.
- `mergeable(o)` deleted; the `pushLocalOp` collapse rule reads
  `o.content.Collapsible()`. `splitRunOp`/`pushRemoteOpLV` use `content.SplitAt`.
  `localInsert` deleted (its Mergeable fan-out moved to the ArrayDocument
  family); `localDelete[C]` stays.
- Snapshot behavior edits: `integrate` inserts whole-run content
  (`snapshot.Insert(endPos, o.content)`); `checkout` returns a
  `*contentTree[C]`; `newBranch` builds `newContentTree[C]()`; `doc[C]` core
  owns `syncRun` (snapshot insert + frontier copy) and `InsRun`.
- Families: `RuneDocument{doc *doc[runeText]}` (no element param anywhere),
  `ArrayDocument[T]{doc *doc[itemRun[T]]}` with `mergeRecursive` in family
  code, `MapDocument[K,V]` updated only for the generic-param change with
  concrete-type element reads (`[]MapOp[K,V](o.content)`). `MergeFromAny`
  kept for all three families.
- Tests were mechanically re-typed (`opLog[runeText]`, `op[runeText]`,
  `Unmarshal[runeText]`, `RuneDocument{doc: …}` clone literals, etc.); the
  full suite is the oracle. `go/crdt/fuzz_test.go`, `trace_test.go`, and
  `go/main.go` were unchanged.

## Delete-run snapshot behavior (deviation from the plan sketch) + caveats

1. **Delete-run snapshot deviation (important).** The plan/spec sketch deleted
   the rope once per remote delete run
   (`snapshot.Delete(o.pos, o.length)`). Implementation proved this wrong under
   merge wind-back: `retreat` moves the staged item tree while the branch
   snapshot is untouched, so `o.pos` is not a snapshot coordinate; and a
   concurrently-merged run may already be partially deleted (the one-shot call
   over-deleted and/or panicked — caught immediately by the `FuzzArrayDocument`
   seed corpus and `TestRunOpsTwoSidedMerge`). The shipped behavior restores
   exact Shape A semantics: `deleteOne` returns the live (not-deleted) position
   of the removed character (or −1 when the target was already deleted, in which
   case the snapshot is untouched) and `apply` does per-character
   `snapshot.Delete(endPos, 1)` under the `snapshot != nil` guard. Consequence:
   remote delete runs touch the rope per character on merge/replay paths; the
   run-granular win is preserved on the local `doc.Del` path
   (`snapshot.Delete(pos, delLen)` once) and for all inserts. Batching remote
   delete runs into fewer rope calls is possible follow-up but needs a
   correctness argument first.
2. **Rope fragmentation under scattered deletes.** `contentTree.Delete` splits
   leaves but never re-coalesces adjacent halves, so merge-heavy per-character
   delete workloads can fragment leaves toward single characters (unlike
   `Insert`, which folds at `ropeLeafCap`). Correctness is unaffected — only
   leaf count / flatten cost.
3. **O(leaf) interior cost.** A single over-cap leaf (one huge paste) costs
   O(leaf length) per interior `Delete`/`Insert` and per `runeText`
   `SplitAt`/`Len` (byte-backed full `[]rune` conversion). `ropeLeafCap=256`
   bounds this for typed text; a single large paste is one leaf of its full
   length by design.
4. **Vacuous compaction target / future work.** Gated
   `TestTargetCriticalVersionCompaction` passes vacuously — the same-agent
   history collapses to a single run op, so the retention set already equals
   the op count and no truncation logic exists. Binary/varint columnar
   compression remains unimplemented (`TestTargetBinaryCompression` FAILs by
   design). See gated results below.

## Verification results

**Full suite green at every task boundary.** `go test -C go ./... -count=1`:
bxtree, crdt, pheap all ok. Correctness anchors all pass (re-run for this
report): `TestVeryDeepHistory`, `TestRuneDocument_Merge`, `TestRuneDocument_Basic`,
`TestArrayDocument_Basic`, `TestItemMerging*`, the recursive-merge tests,
`TestSerialization_RoundTripCheckout`, `TestTrace`, and every `doc.Check()`
call site.

**New rope tests (Task 1, TDD — RED before `content_tree.go` existed):**
`TestRopeMatchesNaive`, `TestRopeArrayMatchesNaive` (rune/[]int naive models,
including multibyte `"a€z"` interiors), `TestRopeLeafCountSmallAfterAppends`
(5000 appends → ≤100 leaves), `TestRuneTextSplitAt`.

**New regressions (Task 2):** `TestShapeB5000CharInsertOneSnapshotLeaf`
(exactly 1 leaf for a 5000-char run insert), `TestShapeBAppendTypingKeepsLeavesBounded`
(≤100 leaves after 5000 single-char appends, content equality + `Check()`),
`TestShapeBInteriorDeleteSplitsOneLeaf` (lenient by design: content + `Check()`
only — an interior delete of one leaf in a 2-leaf tree leaves 3 leaves here, so
the strong `leafCount()==2` form does not hold; see the in-test comment and the
Task 2 report).

**Fuzz (Task 3, this report):**
- `go test -C go ./crdt -fuzz=FuzzMergeConvergence -fuzztime=60s` → **PASS**,
  clean (no failures, no crash corpus entries written to `testdata/fuzz/`).
  90/90 baseline coverage; 47 new "interesting" inputs (137 total incl. the
  baseline) discovered over the fuzz window, held in the Go build cache —
  nothing committed to the repo. (Throughput stalled at ~76k executions for
  most of the window as workers ground on long inputs; still clean.)
- Other targets' seed corpora
  (`go test -C go ./crdt -run 'Fuzz' -count=1`, covering `FuzzDocumentOps`,
  `FuzzMergeConvergence`, `FuzzMapDocument`, `FuzzArrayDocument`) → **PASS**;
  no new crash entries after Task 1. No new seeds were added (none warranted).

**Gated `-tags optimization_targets`:**
- `TestTargetCriticalVersionCompaction` → PASS (vacuous — see caveat 4).
- `TestTargetBinaryCompression` → FAIL (expected by design — binary codec not
  implemented; raw columnar ~800 KB vs the ~10-byte varint+compressed target).

**Trace (Task 2):** `TestTrace` applies the 259,778-edit real-world trace
(`resources/editing-trace.json`) in **2726 ms** with **23.30 MB** final memory.
Timing varies run to run (an incidental re-run during this task's fuzz
startup reported ~1640 ms with the same ~23.3 MB final memory). **No
pre-Shape-B baseline exists in the repo** (searched commit messages/bodies and
`CONTEXT.md`; neither records earlier trace numbers), so no before/after
memory comparison is possible — the numbers are recorded as-is.

**Tooling:** `go vet -C go ./...` clean. `gofmt -l go/crdt` flags only the
pre-existing trailing-whitespace complaint on `go/crdt/crdt.go` (untouched, as
instructed).

## Out-of-scope confirmation

`bxtree` unmodified; serialization wire format unchanged (`Content []C`); the
parked `runIdxForSeq` item untouched; `go/main.go` untouched and compiling.
The plan's later Task 4 (`crdtSummary` two-field struct) is independent of this
work.

## Addendum (post-report follow-up): cached rope leaf counts

The original report recorded `TestTrace` at 2113-2726 ms — a ~2.4x regression
vs the pre-Shape-B ~920 ms. CPU profiling attributed ~46% of the time to
`runtime.countrunes`: `runeText.Len()` (`len([]rune(t))`) rescanned the whole
byte string, and the rope's summarizer (`FromItem`) called it for every item
`bxtree.FindPath` walks and on every node refit, so each positional operation
paid O(runs-before-target x run length) in rescanning — the old per-character
snapshot never summarized anything.

Fix (commit `962d43a`): rope leaves are now `ropeLeaf[C]{c C; n int}` — the
content value plus a cached character count maintained arithmetically at
split/concat time (SplitAt splits after the k-th character, so half lengths
are known without rescanning); only the incoming run's `Len()` at Insert time
ever scans. Summaries and fold checks are O(1) field reads.

Results (same machine, i5-8350U): `BenchmarkTrace` (added to
`trace_test.go`; JSON decode excluded from timing) **1414 ms → 352 ms/op**
(~4x); `TestTrace` **~2.0-2.2 s → ~410 ms** — now faster than the pre-Shape-B
baseline (~920 ms including decode). Final memory unchanged (~23.3 MB);
allocation count unchanged (the hot path was scanning, not allocating).
Remaining trace-test profile: `FindPath` ~24%, one-time JSON decode ~40%.
The per-char delete-run and fragmentation caveats above are unchanged.
`TestTrace` was restored alongside the new `BenchmarkTrace` (commit
`b4456d8`) so the trace stays in the default suite.

Follow-ups (post-addendum): `locateChar` renamed to `locate` (it resolves a
content position in content units — chars for text, elements for arrays — not
only characters), and `Delete` reuses the left lookup when `length == 1`
instead of running an identical second `locate` (the trace's 77k single-char
deletes each lose one of their two `FindPath` descents, ~23% of all rope
lookups). Profile after the cached-count fix: `FindPath` ~33%, rune string
conversions ~25%, GC scan ~9%; the remaining per-edit cost is the
summary-descent itself — the rope's intrinsic price for run-granular
positional indexing, which the per-character snapshot never paid because leaf
index == character position there.
