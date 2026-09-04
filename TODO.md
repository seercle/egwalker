# TODO

## Perf

- [x] **Rope coalescing trade-off — resolved: kept + refined** — leaves stay
  bounded (storm leaves-after-storm 6655→117/118) at allocs/op 78091→13119
  (−83%) and same-session trace replay 801→569ms (−29%); absolute wall time
  remains above the ~410ms pre-coalescing figure. See Addendum 3 of the Shape
  B report.
- [ ] **bxtree: closure-free summary descent** — `FindPath` is ~33% of the
  trace profile; the per-item predicate closure (up to 128 calls/leaf) and the
  generic indirection could be replaced by a summary-targeted descent with
  plain integer compares. Requires touching bxtree (currently off-limits by
  plan convention) — needs explicit go-ahead.
- [ ] **runeText SplitAt/Concat rune round-trips** — ~25% of the trace
  profile; every interior rope edit converts the leaf to []rune and back.
  Options: byte-offset fast path for ASCII boundaries, or a counted-string
  run type (carries the rune count alongside the bytes).
- [ ] **Batched remote merge-path deletes** — `apply`'s per-character
  `snapshot.Delete(endPos, 1)` loop could collapse into range deletes by
  recording each char's endPos and reconstructing contiguous ranges (equal
  consecutive endPos = statically adjacent chars; backward jumps = gaps).
  Needs the correctness argument the Shape B report deferred; zero effect on
  the single-replica trace (its deletes take the local run-granular path).
- [ ] **(parked, RLE era) runIdxForSeq backward scan** — O(#ops) per lookup;
  benchmark before optimizing.

## Tests / hygiene

- [ ] **Extend FuzzDocumentOps's textChar alphabet** so fuzz inputs can
  become multibyte / invalid-UTF-8 document content — op-stream bytes
  currently map to ASCII only, so the raw-byte path is pinned by unit tests
  alone (parked review finding).
- [ ] **Pin an upper leaf bound in TestShapeBInteriorDeleteSplitsOneLeaf** —
  still lenient; its 1000-char leaves legitimately stay split (> cap after
  coalescing), so a bound must account for that (e.g. <= 4).
- [ ] **estimateSize hardcodes rune byte width** (serialization_test.go,
  test-only) — silently wrong for future non-runeText instantiations; take
  the width from the instantiated content type or document harder.
- [ ] **Report cosmetics** — Shape B report caveats 2-4 sit under the
  delete-run heading; split into an "Additional caveats" subhead.

## Housekeeping

- [ ] **Decide whether to commit `.gitignore` (local uncommitted edit) and
  the `docs/` tree** (plans/specs/reports; `docs/superpowers/plans/` rule is
  commented out in .gitignore but the tree is untracked — currently
  half-tracked: the Shape B report file is committed).
