# Code Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Write the three-package algorithm documentation (`docs/pheap`, `docs/bxtree`, `docs/crdt`, + index) with Mermaid diagrams, worked examples, and source-anchored live snippets, verified by an automated drift check wired into `go test`.

**Architecture:** Markdown pages under `docs/<package>/` carry Mermaid diagram source and Go snippets embedded with an anchor line (` ```go include <path> L<int>-L<int>[ L<int>-L<int>…] `). A Python checker (`scripts/doc-snippets-check.py`) re-extracts every anchored slice from current source and fails on any mismatch; `go/doc_drift_test.go` runs the checker so `go test -C go ./...` catches doc drift. Documentation is authored one page-set per task after the checker exists, since every task's pages must pass it.

**Tech Stack:** Python 3 (checker; stdlib only), Mermaid rendered by GitHub, Go test harness.

**Spec:** `docs/superpowers/specs/2026-09-07-code-documentation-design.md` (read it before working — it binds style, page map, out-of-scope).

## Global Constraints

- Module root `go/`: every go command needs `-C go`.
- gofmt clean everywhere except `go/crdt/crdt.go` (standing exception).
- Doc trees created: `docs/pheap/`, `docs/bxtree/`, `docs/crdt/`, plus `docs/index.md` (links, not prose).
- Every snippet: ` ```go include <repo-relative-src-path> L<a>-L<b>[ L<a>-L<b>…] ``` ` where ranges are 1-based inclusive; checker compares against the fence body verbatim, with `// …` inserted only between the ranges of one fence (never inside one range).
- Every diagram node/step names the function it corresponds to; every snippet introduced by one sentence citing `file:line`; every algorithm page ends in an invariants section naming the enforcing code and how to run it.
- Mermaid diagrams: flowchart/graph `TD` for control flow, sequence diagrams for cross-component calls, longest possible ≤ ~25 nodes per diagram; state series for worked examples.
- Docs prose in English; address the reader directly; sections end with pointers to who verifies the claims.
- Worked examples synthetic and small (5-15 elements), showing actual state after each step, never invented semantics: every claimed behavior must be reproducible by running the code the doc points at.
- Per-task verification gate (every task): `python3 scripts/doc-snippets-check.py` exit 0; `go test -C go ./... -count=1` green; `gofmt -l go/crdt` lists only crdt.go when go files changed.
- Never push; conventional short subjects; per-task exact file lists.

---

### Task 1: Snippet checker + drift test wiring

**Files:**
- Create: `scripts/doc-snippets-check.py`
- Create: `go/doc_drift_test.go`

**Interfaces:**
- Produces: `python3 scripts/doc-snippets-check.py [paths…]`, run from repo root; scans all `*.md` under `docs/` (excluding `docs/superpowers/`), finds fences whose info line matches `^go include (<path>)( L\d+-L\d+)+$`, and compares each fence body against the file's listed ranges concatenated. `doc_drift_test.go` calls it via `exec.Command("python3", "../scripts/doc-snippets-check.py", ...)` with cwd the repo root's parent go dir; must `t.Skipf` if `exec.LookPath("python3")` fails.

- [ ] **Step 1: Write the checker first, then a one-fixture doc to test it against (checker-first, TDD)**

`scripts/doc-snippets-check.py` — complete contents to write:

```python
#!/usr/bin/env python3
"""Verify embedded code snippets in docs match the source files they cite.

Fence format:
    ```go include go/crdt/crdt.go L440-L444 L448-L459
    <verbatim concatenation of the 1-based inclusive ranges,
     with a single `// …` line only BETWEEN ranges>
    ```
Path on the anchor line is repo-root-relative. Multiple ranges allowed;
the checker inserts no content for gap handling beyond the `// …` rule.
Collect all drifts, report them, exit 1 if any.
"""
import re
import sys
from pathlib import Path

def main() -> int:
    root = Path(__file__).resolve().parent.parent
    md_dirs = [root / "docs"]
    failures = []
    checked = 0
    for md in sorted(md_dirs[0].rglob("*.md")):
        if ".superpowers" in md.parts or "superpowers" in md.parts:
            continue
        text = md.read_text(encoding="utf-8")
        i = 0
        lines = text.splitlines()
        while i < len(lines):
            info = lines[i].strip()
            rest = info[3:].strip() if info.startswith("```go ") else None
            if not rest or not rest.startswith("include "):
                i += 1
                continue
            m = re.match(r"include (\S+)((?:\s+L\d+-L\d+)+)$", rest)
            if not m:
                i += 1
                continue
            path = m.group(1)
            ranges = [(int(a), int(b)) for a, b in
                      re.findall(r"L(\d+)-L(\d+)", m.group(2))]
            src = root / path
            if not src.exists():
                failures.append(f"{md}: source {path} does not exist")
                while i < len(lines) and not lines[i].strip().startswith("```"):
                    i += 1
                i += 1
                continue
            src_lines = src.read_text(encoding="utf-8").splitlines()
            expect = []
            for r_idx, (a, b) in enumerate(ranges):
                if r_idx:
                    expect.append("// …")
                if a < 1 or b > len(src_lines) or a > b:
                    failures.append(
                        f"{md}: fence at line {i+1} range L{a}-L{b} out of "
                        f"bounds for {path} ({len(src_lines)} lines)")
                    continue
                expect.extend(src_lines[a - 1 : b])
            body = []
            j = i + 1
            while j < len(lines) and not lines[j].strip() == "```":
                body.append(lines[j])
                j += 1
            got = "\n".join(expect)
            want = "\n".join(body)
            checked += 1
            if got != want:
                failures.append(
                    f"{md}: fence `` `go include {path}` `` drifts from source "
                    f"(expected {len(expect)} lines, found {len(body)}).\n"
                    f"--- expected ---\n{got}\n--- actual ---\n{want}")
            i = j + 1
    if failures:
        for f in failures:
            print("DRIFT:", f)
        print(f"{checked} snippets checked, {len(failures)} drifting")
        return 1
    print(f"{checked} snippets verified, all in sync")
    return 0

if __name__ == "__main__":
    sys.exit(main())
```

NOTE to implementer: the script above is final as written (the docstring example uses the same grammar the regex parses; `// …` join between gap-separated ranges is built into the expected-body construction).

- [ ] Step 2: Create `docs/pheap/01-pairing-heap.md` as a THROWAWAY self-check fixture with one real snippet: an anchor to `go/pheap/pheap.go`'s `insert` method (use the actual current line range after reading the file), body copy-pasted from the file. Run the checker — expect exit 0 (fixture created only to prove end-to-end extraction works; the real page is authored by Task 2 over the same path).

- [ ] Step 3: Wire the test `go/doc_drift_test.go`:

```go
package main

import (
	"os/exec"
	"os"
	"path/filepath"
	"testing"
)

func TestDocSnippetsInSync(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; doc drift not checked")
	}
	root, err := filepath.Abs("..") // go/ -> repo root; go test cwd = package dir
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "scripts/doc-snippets-check.py")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doc snippets out of sync:\n%s", out)
	}
}
```

Verify wiring: deliberately break the fixture snippet (add a whitespace line), `go test -C go ./doc_drift_test.go`? No — plain `go test -C go ./ -count=1 -run TestDocSnippetsInSync -v` should FAIL; restore fixture; PASS. (Creating a test at `go/` package main — the module `egwalker` root package; check `head -3 go/main.go` for package name and reuse it.)

- [ ] Step 4: Commit exactly `scripts/doc-snippets-check.py go/doc_drift_test.go docs/pheap/01-pairing-heap.md` — subject `feat: doc snippet drift check`.

---

### Task 2: pheap documentation page

**Files:**
- Create (overwrite fixture): `docs/pheap/01-pairing-heap.md`

**Interfaces:**
- Consumes: Task 1 checker; anchored snippets must use ranges verified against `go/pheap/pheap.go` / `go/pheap/types.go` as they exist on the current HEAD.

REQUIRED page structure (in order; work the real function names from `go/pheap/pheap.go:1-158`, renaming these bullets to the actual exported names — e.g. `Push`, `Meld`, `Pop`, `Fix`, `VPointer` as they appear):

1. Title + 1-paragraph what/why (ordering service over a forest of heap-ordered trees, pairing = two-pass consolidation).
2. Layout diagram: Mermaid `graph TD` of a 3-4 node heap with pointers child/sibling/prev+rank bits, nodes labeled with the Go struct field names (`head`, `child`, `sibling`, `prev`, payload kinds from `types.go`).
3. `Push` walkthrough: function name → what it does in one line → snippet → 3-step mermaid series with a worked example (`push 5, push 3` states).
4. `Meld` + `Pop` (delete-min): the two-pass pass-1 (left-to-right pairwise meld) / pass-2 (right-to-left consolidation) tournament as one labeled mermaid diagram, plus a worked example table over `push 5,3,8,1` then one pop showing the exact tree states.
5. `decreaseKey` walkthrough and its restack trick.
6. Amortized O(1)/O(log n) analysis in prose + a mermaid diagram of cost over operations.
7. Invariants section: each invariant (heap order, size/prev consistency, no orphan trees) named, one sentence, pointing at the enforcing code (tests/fuzzer in `pheap_test.go`, `fuzz_test.go`) and `go test -C go ./pheap`.

Steps: write page with anchors cut from source (read source first; re-verify line numbers), run checker (fix any mismatch), run full suite, commit exactly `docs/pheap/01-pairing-heap.md` — subject `docs: pheap pairing-heap walkthrough`.

---

### Task 3: bxtree layout & invariants page

**Files:**
- Create: `docs/bxtree/01-bxtree-structure.md`

Structure (actual struct/field names MUST be transcribed from `go/bxtree/bxtree.go` / `types.go`; if a listed section doesn't apply to the code, replace with the real mechanism instead of inventing):

1. What it is: positional B+tree over a generic payload with per-node summary math (orders: leaf/branch constants from `types.go` quoted).
2. Node layout diagram: a tree of 5-7 keys across 3 leaves showing `keys/children/counts/summary` fields, total = sum(children.counts) invariants highlighted.
3. Structural invariants (sentinel? occupancy bounds − from `Check()` in the package, whatever it exists as) — list each with the enforcing line cited.
5. `find`/descent walkthrough diagram (binary vs linear scan within node, per code).
6. range aggregation & summaries (how counts/other sums flow up).
7. Invariants: enumerate `Check()`'s assertions with file:line ties; who verifies: `bxtree_test.go` + `FuzzBxTree`.
(insert/split and delete/rebalance step series live in the next two pages — reference them, do not duplicate.)

Steps: write, checker-clean (fix anchor drift), suite green, commit exactly `docs/bxtree/01-bxtree-structure.md` (path may match the table name from this plan header exactly; if the authoring deviates in split, adjust heading content, not page count) — subject `docs: bxtree layout and invariants`.

---

### Task 4: bxtree delete & rebalance page; Task 5: filtering/summaries page — SINGLE DISPATCH (same package, one author keeps node-state diagrams consistent)

Batch as ONE dispatch to one implementer (batched small same-shape work):

**Files:**
- Create: `docs/bxtree/02-insert-delete.md`
- Create: `docs/bxtree/03-filtered-summaries.md`

`02-delete-and-rebalance.md` required sections:
`02-insert-delete.md` required sections (the mutation pair lives together here):
1. `insert`/`split` step-by-step state series (5+ steps, worked example).
2. `delete`/underflow decision diagram (borrow-left/borrow-right/merge, exact thresholds from code) + worked example: 4-level tree, deleting 3 keys producing one borrow and one merge, state diagrams per step.
3. Summary maintenance: re-accumulation of node summaries after every mutation (cite the code lines).
4. Invariants recap + where verified (tests/fuzz as in Task 3).

`03-filtered-summaries.md` (if the package supports predicate-based filtered traversal — read `bxtree.go` for actual API; skip part if not present and note "not supported"):
1. The filtering predicate flow diagram.
2. Worked example (10-item tree, odd-only filter, each node annotated with kept/dropped counts).

commit exactly the two new files with subject `docs: bxtree insertion, deletion, filtered summaries`.

---

### Tasks 5a-5f: crdt documentation — SIX per-page dispatches, each its own reviewer gate

(No batching — crdt pages are the hard core; each page = one task row numbered 5a…5f for the ledger.)

**Pages and their mandated section lists** (implementers read actual code; names below ground them in the right neighborhoods):

- `docs/crdt/01-replica-model.md`: exported family (`RuneDocument`, `ArrayDocument` via generics, `MapDocument`), what "replica" means here, version tree (ops never deleted until compact), local vs remote-shape paths diagram, `doc.Check()` as the whole-invariant tooling page-anchor.
- `docs/crdt/02-op-log.md`: op/op structs + `lv` numbering scheme (that "LV" is a log-version identifier, not a wall-clock), parent links, `first`, per-agent seq index (what it replaced, the shape it has now), `runIdxForSeq` lookup diagram.
- `docs/crdt/03-merge-drive.md`: `mergeFrom` → `do1Operation` → `apply` call graph, snapshot/checkout interplay, insert-path walkthrough (item atomization, position resolve) AND delete-path walkthrough (staged run + per-char `deleteOne` + `-1` skips) — cite crdt.go lines for both; `delTargets`/`tryMergeAt` recurrence diagram.
- `docs/crdt/04-content-tree.md`: `contentTree` find/insert/delete (incl. multi-leaf `Delete(pos,length)`) with diagram series; slot atomization; the local run-granular fast path vs per-char path trade.
- `docs/crdt/05-binary-and-compaction.md`: columnar serialization layout, Go-blob format, `Compact`, what strips (delTargets, caches, seq index) vs what persists.
- `docs/crdt/06-deltas-and-checkout.md`: delta frames, checkout replay scheme, shared-ops fast path, trace replay (point to `BenchmarkTrace` numbers recorded in TODO/CONTEXT).

Page-by-page dispatch as six separate tasks (each page its own reviewer gate; no batching — crdt pages are the product's hard core). Each executes: read target files (file list given per discipline by the task brief you generate with scripts/task-brief), author page, checker green, suite green, commit exactly that page — subjects in the pattern `docs: <page-topic>`.

---

### Final Task: index & wiring

**Files:**
- Create: `docs/index.md`
- Modify: `AGENTS.md` (add a "Code documentation" bullet: page map + how the drift check works + `python3 scripts/doc-snippets-check.py` command line)

Steps: TOC listing all pages with one-line summaries and reading order (pheap → bxtree → crdt); cross-links from each page's "related pages" (one line each, links relative); AGENTS.md section; full verification: `go test -C go ./... -count=1`, `python3 scripts/doc-snippets-check.py`, `gofmt -l go/crdt` (only crdt.go), and every Mermaid block syntactically checked by the optional `scripts/check-mermaid.sh` IF `npx` exists in the devshell (skip with a printed note otherwise); commit exactly `docs/index.md AGENTS.md` — subject `docs: index and agent notes for code documentation`.

---

## Self-Review

**Spec coverage** — checker (Task 1 ✓), drift wired into go test (Task 1 ✓), per-package pages pheap 1 / bxtree ~3 / crdt 6 (Tasks 2-7 ✓ — spec table listed 4 bxtree pages; the plan consolidates "layout & invariants" as one and drop the standalone sentinel page, matching the spec's "~10-11 pages total" rather than its 4-row bxtree sketch; recorded as a ruling here — page count 10 + index is within tolerance), index + AGENTS.md (final task ✓), review loops (SDD task reviews), out-of-scope list (whole-file tiers page task list only; no code changes shipped except the drift test file).

**Placeholder scan** — doc page tasks specify required section lists and the content rule set; the prose/diagrams themselves cannot be premade (that is the deliverable, like test assertions produced from observations in Task 1 of the previously-written batched-deletes plan — one precedent). The checker script and wiring test are fully code-specified. No TBDs remain.

**Type consistency** — checker CLI flags/behavior (paths optional→repo scan), fence grammar, and test file names are used identically in Tasks 1, 2, 4-7, and the final task; the `// …` between-ranges marker matches the spec's notation.
