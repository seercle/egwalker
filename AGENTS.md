# AGENTS.md

Research codebase: Go CRDT framework + positional B+Tree (`bxtree`) and pairing heap (`pheap`). Module root is `go/`, so every `go` command needs `-C go` (or run inside `go/`).

## Commands (from repo root)

- Test everything: `go test -C go ./...` (~1.5s; fuzz targets run their seed corpora here — deep search needs `-fuzz`)
- Fuzz a target: `go test -C go ./crdt -fuzz=FuzzMergeConvergence -fuzztime=30s` (also `FuzzDocumentOps`, `FuzzMapDocument`, `FuzzArrayDocument`; `./pheap -fuzz=FuzzHeap`; `./bxtree -fuzz=FuzzBxTree`)
- Focused test: `go test -C go ./crdt -run TestSerialization -count=1`
- Benchmarks (bxtree, pheap): `go test -C go ./bxtree -bench=. -run=^$`
- Build / run demo: `go build -C go ./...`, `go run -C go main.go`
- Plot benchmark CSV (see trace pipeline below): `python scripts/plot-trace.py <csv>`

## Layout

- `go/bxtree` — positional B+Tree with per-node summaries; self-contained.
- `go/pheap` — pairing heap; self-contained.
- `go/crdt` — documents (`RuneDocument`, generic `ArrayDocument`, `MapDocument`), op log, columnar serialization, and the merge/walker logic. `MapDocument` values that implement `Mergeable` recurse.
- `go/main.go` — demo/example program, not a library entrypoint.
- `resources/editing-trace.json` — real-world trace consumed by crdt's `TestTrace`.

## Gotchas

- All three packages expose **native Go fuzz targets** (`FuzzBxTree`, `FuzzHeap`, `FuzzDocumentOps`, `FuzzMergeConvergence`, `FuzzMapDocument`, `FuzzArrayDocument`). Under plain `go test` only their seed corpora run; use `-fuzz` for deep search. Seed corpus entries are added in-code (`f.Add`); check `testdata/fuzz/` for crash regressions. Test files are named `<what it tests>_test.go` (`fuzz_test.go`, `bxtree_test.go`, `crdt_test.go`, …).
- `go.mod` and the pinned toolchain are both Go 1.26.6; if you bump the nixpkgs pin, update `go.mod` (and the CONTEXT version) to match the new default `go`.
- `doc.Check()` is the invariant checker — call it after edits/merges when adding tests.
- Environment is a **pinned Nix flake** (`github:NixOS/nixpkgs?rev=a3116115…` = nixos-26.05). `.envrc` is gitignored, so in a fresh checkout run `nix develop` (direnv won't exist). If you bump the pin, sync the Go/Python versions listed in CONTEXT to what the new nixpkgs resolves.
- Nix flakes only evaluate **git-tracked** files: after creating/editing `flake.nix`/`flake.lock`, `git add` them before `nix develop`/`nix flake lock`, or evaluation fails with "Path 'flake.nix' … not tracked by Git".
- `go test` runs each package with cwd = package dir. The crdt trace test opens `../../resources/editing-trace.json` (repo-relative) and writes `go/crdt/trace-data.csv`. That CSV (and `*.prof`, `*.test`) is gitignored — regenerate it with `go test -C go ./crdt -run TestTrace` before plotting.

## Workflow notes

- Verify with `go test -C go ./...` before claiming changes pass; it is the project's whole test suite.
- Commit style is short conventional subjects (e.g. `feat: nix flake`).
