# Code Documentation — Design Spec

**Date:** 2026-09-07
**Status:** approved in chat (user confirmed the design)

## Goal

Extensive, human-friendly documentation of `go/bxtree`, `go/crdt`, and `go/pheap`: Markdown pages with Mermaid diagrams, worked examples, and live code snippets embedded from source, split per package.

## Audience & scope

- Primary audience: the repo owner (personal deep-dive reference) and AI agents.
- Depth: algorithm walkthroughs + invariants (not exhaustive function-by-function).
- Tone: user-friendly prose; every algorithm gets step-by-step diagrams and a worked numeric example; **every** snippet preceded by one sentence explaining why it matters.
- ~10-11 pages total, English, 2000-4000 lines of doc.

## Page map

| Package | Pages | Content |
|---|---|---|
| pheap | `docs/pheap/01-pairing-heap.md` | layout diagram; insert/meld/pop/decreaseKey walkthroughs; consolidation tournament; amortized analysis; invariants |
| bxtree | `docs/bxtree/01-layout-and-invariants.md`, `02-insert-and-splits.md`, `03-delete-and-rebalance.md`, `04-filtering-sums.md` | node structure + summaries; sentinel invariants; find descent; insert/split steps; delete/rebalance; estimateSize & byte-offset SplitAt; invariants verified by Check/fuzz |
| crdt | `docs/crdt/01-replica-model.md`, `02-op-log-and-seq-index.md`, `03-merge-drive.md`, `04-content-tree.md`, `05-binary-and-compaction.md`, `06-deltas-and-checkout.md` | replica/version-tree model; op log + per-agent seq index; merge driver (mergeFrom → do1Operation → apply: insert & delete paths, split/concurrency checks, delTargets, tryMergeAt); contentTree find/update/descend; columnar binary serialization, Compact; delta frames, checkout replay, local run-granular fast path |
| top | `docs/index.md` | TOC, reading order, how snippets/drift-check work |

Worked examples are synthetic, small (5-15 elements), numeric, and rendered as step tables or Mermaid state diagrams showing the actual data-structure states after each operation.

## Technology

- **Markdown + Mermaid** only. Diagrams committed as Mermaid source (GitHub renders them; no generated artifacts). No KaTeX, no HTML site, no mdBook.
- **Live snippets**: fences carrying an anchor line, e.g.
  ```` ```go include go/crdt/crdt.go L440-L459 ```` — body is the verbatim source slice.
- **Drift check**: `scripts/doc-snippets-check.py` re-extracts each anchored slice and compares against the fence body:
  - exact match → OK; mismatch → FAIL with both versions shown and a `% overlap` hint.
  - exit nonzero on any FAIL.
- **Wired into tests**: `doc_drift_test.go` in `go/` (package `egwalker`, root has no test files today — use `go/doc_drift_test.go`, package main or a `//go:build ignore`-free plain test file) runs the checker so `go test -C go ./...` fails on doc drift. Test must skip gracefully if Python is unavailable (env-independent 	`exec.LookPath("python3")`; skip + note). Script must run fast (<1s).
- **Mermaid lint** (optional, on demand): `scripts/check-mermaid.sh` using `npx @mermaid-js/mermaid-cli` if available; not gated into tests.

## Content style rules

1. Every Mermaid diagram node/step names the exact function it corresponds to (e.g. `split()` step 3 → `bxtree.go`).
2. Every code snippet introduced by one sentence containing `file:line` of the definition.
3. Worked examples: numbered table (Op | State after) and/or Mermaid series of states.
4. Invariants page sections name the enforcing code (`Check()`, fuzz targets) and how to trigger them (`go test -C go ./...`).
5. No copy-pasted walls of code without commentary; snippets ≤ ~40 lines. Non-essential middle lines are skipped by listing MULTIPLE ranges on the anchor line (e.g. `go include go/crdt/crdt.go L440-L444 L448-L459`); the checker compares the fence body against the concatenation, with a single `// …` line inserted between extracted ranges.

## Verification

- `go test -C go ./...` stays green including the new drift test.
- Each doc reviewed (SDD task review): diagrams render (geometrically checkable by reading mermaid source), snippets match source, claims cite file:line.
- Final whole-branch review before finish.

## Out of scope

- Publishing/serving the docs, cross-language rendering, generated HTML.
- Documenting test files, `go/main.go` demo, or per-Package godoc generation.
- Crdt docs do not restructure code; doc-driven refactors (split a function for clarity) are out of scope.
