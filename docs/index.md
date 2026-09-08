# Code Documentation

Algorithm documentation for the three packages in `go/`: the pairing heap
(`pheap`), the positional B+tree (`bxtree`), and the CRDT framework
(`crdt`). Every page uses Mermaid diagrams for control flow and worked
examples, and embeds live snippets pulled verbatim from the source files they
describe (see [How this documentation is kept correct](#how-this-documentation-is-kept-correct)).

All claims name the code that backs them (`file:line`), and each page ends
with an invariants section pointing at the test or fuzz target that verifies
it.

## Reading order

Read `pheap` → `bxtree` → `crdt`. The heap is the smallest self-contained
structure and introduces the doc conventions; the B+tree builds on the same
"per-node summary" vocabulary; the CRDT pages assume both as dependencies
and are the largest package.

## Table of contents

### `docs/pheap/`

1. [Pairing Heap](pheap/01-pairing-heap.md) — one heap-ordered tree, three
   pointers; `Push`/`Meld`/`Pop`/decrease-key walkthroughs, the two-pass
   consolidation tournament, amortized O(1)/O(log n) analysis, and invariants
   enforced by the package tests and `FuzzHeap`.

### `docs/bxtree/`

2. [BxTree: Layout and Invariants](bxtree/01-bxtree-structure.md) — the
   positional B+tree idea (indexes, not keys), the single node struct's
   leaf/branch shapes, `size`/`summary` plumbing, the occupancy envelope,
   and the `find` descent.
3. [Insert, Split, Delete, and Rebalance](bxtree/02-insert-delete.md) — the
   mutation pair step by step: insert path and splits, delete path with the
   borrow/merge decision tree, and the summary-maintenance table every
   mutation site obeys.
4. [Filtered Traversal and Summaries](bxtree/03-filtered-summaries.md) — why
   there is no per-item filtered walk API and what the package offers
   instead: summaries as precomputed aggregates and `FindPath` as the
   generalized descent.

### `docs/crdt/`

5. [The Replica Model](crdt/01-replica-model.md) — what a replica is, the
   append-only log contract, the three exported document types
   (`RuneDocument`, `ArrayDocument`, `MapDocument`), local vs remote paths,
   and `doc.Check()` as the whole-invariant tool.
6. [The Op Log](crdt/02-op-log.md) — ops and `lv` (log-version) addressing,
   parent links and causal ordering, and the per-agent sequence index with
   its `runIdxForSeq` lookup.
7. [The Merge Drive](crdt/03-merge-drive.md) — `mergeFrom` →
   `do1Operation` → `apply`: the insert and delete paths, split/concurrency
   checks, `delTargets`, and the `tryMergeAt` recurrence.
8. [The Content Tree](crdt/04-content-tree.md) — the visible document:
   `contentTree` find/insert/delete, run atomization into slots, and the
   local run-granular fast path vs per-char editing.
9. [The Binary Frame and Compaction](crdt/05-binary-and-compaction.md) — the
   columnar wire format, what `Compact` strips vs what persists, and
   round-tripping a whole replica through one blob.
10. [Delta Frames and Checkout](crdt/06-deltas-and-checkout.md) —
    incremental delta frames, branch re-checkout against a changed log, the
    shared-ops fast path, and real-trace replay (`BenchmarkTrace`).

## How this documentation is kept correct

Snippets are **live, not copies**. Each code fence carries an anchor info
line of the form `` ```go include go/crdt/crdt.go L440-L444 L448-L459 ``` ``
before the verbatim source lines and the closing fence.
The anchor names the repo-relative source file and one or more 1-based
inclusive line ranges; the fence body must be exactly those lines
concatenated, with a single `// …` line only *between* two ranges (used to
skip uninteresting middle lines). If you edit source and a doc snippet
drifts:

1. Run `python3 scripts/doc-snippets-check.py` — it fails on every fence
   whose anchor no longer reproduces its body, showing both versions.
2. Fix the fence: either correct the `L<a>-L<b>` ranges to bracket the moved
   code again (re-anchoring around it), or update the doc prose if the
   behavior changed.

The checker is wired into the test suite via `go/doc_drift_test.go`, so
`go test -C go ./...` fails on any drift (it skips gracefully when `python3`
is unavailable). Run it the same way at any time from the repo root:

    python3 scripts/doc-snippets-check.py

The check covers every `*.md` under `docs/` except `docs/superpowers/`.
