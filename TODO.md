# TODO

## Perf

- [x] **Rope coalescing trade-off — resolved: kept + refined** — leaves stay
  bounded (storm leaves-after-storm 6655→117/118) at allocs/op 78091→13119
  (−83%); plugged-in re-measurement (see Addendum 3's power-state correction)
  shows replay 338–381 ms vs 473–475 ms for the coalescing-only state on the
  same machine — ~33% faster, no regression. The earlier "wall time remains
  above ~410 ms" reading was a battery-throttling artifact and is retracted.
- [ ] **bxtree: closure-free summary descent** — `FindPath` is ~33% of the
  trace profile; the per-item predicate closure (up to 128 calls/leaf) and the
  generic indirection could be replaced by a summary-targeted descent with
  plain integer compares. Requires touching bxtree (currently off-limits by
  plan convention) — needs explicit go-ahead.
- [x] **runeText SplitAt/Concat rune round-trips — resolved: `c2b60da`** —
  allocation-free `runeText` (`Len` via `utf8.RuneCountInString`, zero-copy
  byte-offset `SplitAt`) removed the `[]rune` conversions; storm allocs/op
  78081→13119 (−83%). See Addendum 3 of the Shape B report.
- [ ] **Batched remote merge-path deletes** — `apply`'s per-character
  `snapshot.Delete(endPos, 1)` loop could collapse into range deletes by
  recording each char's endPos and reconstructing contiguous ranges (equal
  consecutive endPos = statically adjacent chars; backward jumps = gaps).
  Needs the correctness argument the Shape B report deferred; zero effect on
  the single-replica trace (its deletes take the local run-granular path).
- [ ] **(parked, RLE era) runIdxForSeq backward scan** — O(#ops) per lookup;
  measured 2026-09-05: t(50k)/t(10k) ≈ 7.75 (127.4 ms → 987.6 ms for 5× op
  growth) — below the ≥10 quadratic trigger. Common-case lookups eliminated
  2026-09-05 by the `mergeInto` skip (see map merge scaling item);
  cold-path lookups (ops the destination does not fully hold) remain
  O(#ops) — still parked.
- [x] **map merge scaling — resolved 2026-09-05 by the `mergeInto` skip** —
  BenchmarkMapMergeAtScale measured t(50k)/t(10k) = 17.8× (rune: 7.75,
  linear ~5); allocs linear, time superlinear (mergeInto + keyIndex
  rebuild). Post-fix same-session A/B: 26.36× → 4.33×, ~linear again
  (rune: 4.41× → 4.72×); map 50k 57.1 ms (was 3334.0 ms — 58.4× faster),
  10k 13.2 ms (was 126.5 ms — 9.6× faster).
- [x] **Critical-version compaction (Section 3.5) — resolved 2026-09-05** —
  snapshot-anchor `Compact()` API on all three document types (explicit
  call, single-tip-frontier precondition; the automatic watermark/ack
  wrapper is deliberately deferred), anchor op + per-agent coverage table
  (`version` never rewritten), merge rules for compacted peers, and the
  binary frame v2 coverage column. Trace scale (`TestCompactTraceScale`,
  83,751-op editing trace): `before: ops=83751 heap=14MB (14928544
  bytes)` → `after: ops=1 heap=0MB (543336 bytes)`, content
  byte-identical, `Check()` green. Parked extensions: automatic
  watermark/ack-tracking wrapper; DAG-skeleton anchor variant (one anchor
  per converged subtree instead of one global snapshot); merging from
  compacted into partially-converged state (panics today — unsupported in
  v1); late-compaction divergence is silently absorbed (documented v1
  boundary); a compacted src WITH post-compaction edits merging into a
  full-history dest also panics (the F1 boundary: the incoming anchor is
  covered-skipped at op_log.go:389-399, then the new op's `(-1, seq)`
  parent edge has no agent-`-1` op to resolve against and panics in
  `runIdxForSeq`, op_log.go:202). Supported merge directions today: both
  sides compacted (shared or aligned compaction points, modulo the silent
  absorption above); compacted dest ← full src; fresh (empty) dest
  adoption; compacted src with NO post-compaction edits ← full dest.
  Follow-up: `runIdxForSeq`'s "no op from agent -1 covers seq N" message
  is unclear for this topology — a future hardening can convert it to the
  documented-topology message.

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
