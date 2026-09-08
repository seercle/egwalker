# AGENTS.md

Research codebase: Go CRDT framework + positional B+Tree (`bxtree`) and pairing heap (`pheap`). Module root is `go/`, so every `go` command needs `-C go` (or run inside `go/`).

## Commands (from repo root)

- Test everything: `go test -C go ./...` (~1.5s; fuzz targets run their seed corpora here — deep search needs `-fuzz`)
- Fuzz a target: `go test -C go ./crdt -fuzz=FuzzMergeConvergence -fuzztime=30s` (also `FuzzDocumentOps`, `FuzzMapDocument`, `FuzzArrayDocument`; `./pheap -fuzz=FuzzHeap`; `./bxtree -fuzz=FuzzBxTree`)
- Focused test: `go test -C go ./crdt -run TestSerialization -count=1`
- Benchmarks (bxtree, pheap): `go test -C go ./bxtree -bench=. -run=^$`
- Build / run demo: `go build -C go ./...`, `go run -C go main.go`
- Plot benchmark CSV (see trace pipeline below): `python scripts/plot-trace.py <csv>`

## Code documentation

- Pages: `docs/index.md` (TOC + reading order) over `docs/pheap/` (1 page), `docs/bxtree/` (`01-bxtree-structure.md`, `02-insert-delete.md`, `03-filtered-summaries.md`), and `docs/crdt/` (`01-replica-model.md` … `06-deltas-and-checkout.md`).
- Code fences in those pages are **live snippets**: info line ` ```go include <repo-relative-src> L<a>-L<b>[ L<c>-L<d>…] ``` ` anchors verbatim source ranges (1-based inclusive; a single `// …` line only *between* two ranges).
- Drift check: `python3 scripts/doc-snippets-check.py` re-extracts every anchored fence and fails on any mismatch; `go/doc_drift_test.go` wraps it, so `go test -C go ./...` catches doc drift (skips if `python3` is missing).
- Re-anchoring after source changes: run the checker, then for each flagged fence fix the `L`-ranges to bracket the moved code again (or re-copy the body if the content itself changed); rerun until it reports snippets in sync.

## Layout

- `go/bxtree` — positional B+Tree with per-node summaries; self-contained.
- `go/pheap` — pairing heap; self-contained.
- `go/crdt` — documents (`RuneDocument`, generic `ArrayDocument`, `MapDocument`), op log, columnar serialization, and the merge/walker logic. `MapDocument` values that implement `Mergeable` recurse.
- `go/main.go` — demo/example program, not a library entrypoint.
- `resources/editing-trace.json` — real-world trace consumed by crdt's `TestTrace`/`BenchmarkTrace`.

## Gotchas

- All three packages expose **native Go fuzz targets** (`FuzzBxTree`, `FuzzHeap`, `FuzzDocumentOps`, `FuzzMergeConvergence`, `FuzzMapDocument`, `FuzzArrayDocument`). Under plain `go test` only their seed corpora run; use `-fuzz` for deep search. Seed corpus entries are added in-code (`f.Add`); check `testdata/fuzz/` for crash regressions. Test files are named `<what it tests>_test.go` (`fuzz_test.go`, `bxtree_test.go`, `crdt_test.go`, …).
- `go.mod` and the pinned toolchain are both Go 1.26.6; if you bump the nixpkgs pin, update `go.mod` (and the CONTEXT version) to match the new default `go`.
- `doc.Check()` is the invariant checker — call it after edits/merges when adding tests.
- Environment is a **pinned Nix flake** (`github:NixOS/nixpkgs?rev=a3116115…` = nixos-26.05). `.envrc` is gitignored, so in a fresh checkout run `nix develop` (direnv won't exist). If you bump the pin, sync the Go/Python versions listed in CONTEXT to what the new nixpkgs resolves.
- Nix flakes only evaluate **git-tracked** files: after creating/editing `flake.nix`/`flake.lock`, `git add` them before `nix develop`/`nix flake lock`, or evaluation fails with "Path 'flake.nix' … not tracked by Git".
- `go test` runs each package with cwd = package dir. The crdt trace tests open `../../resources/editing-trace.json` (repo-relative). `TestTrace` only checks replay correctness; `BenchmarkTrace` writes `go/crdt/trace-data.csv` (untimed epilogue) and prints wall time + final memory. That CSV (and `*.prof`, `*.test`) is gitignored — regenerate it with `go test -C go ./crdt -run '^$' -bench BenchmarkTrace` before plotting. A `TestMain` preloads the trace when profiling so the JSON decode stays out of `-cpuprofile` output.

## Workflow notes

- Verify with `go test -C go ./...` before claiming changes pass; it is the project's whole test suite.
- Commit style is short conventional subjects (e.g. `feat: nix flake`).
