# TODO

## Perf

- [ ] **Rope coalescing trade-off — decide keep / refine / revert** — seam
  coalescing (this branch) bounds leaf fragmentation (storm benchmark
  leaves-after-storm 6655→117) but costs wall time (TestTrace replay
  ~410ms→~700ms, ~1.7×; storm ns/op ~3×). Refine option: merge only
  empty-side or small pairs. See Addendum 2 of the Shape B report.
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

- [ ] **Rope test: delete across a multibyte rune leaf boundary** (multibyte
  insert is covered; delete is not — risk low, rune-accurate SplitAt makes a
  straddling rune impossible, but the case deserves a test).
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
