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
- [x] **(parked, RLE era) runIdxForSeq backward scan — resolved 2026-09-06
  by the per-agent seq index** — the backward O(#ops) scan in
  `runIdxForSeq` / `resolveParentLV` lookups was replaced by a per-agent
  index (agent → ascending `{startSeq, headLV}`; headLV stable across run
  splits, resolution headLV → `opIdxAt`). Measured (same-session):
  `MergeAtScale` 1k 24.4 ms → 4.7 ms (5.2×), 10k 296.5 ms → 65.0 ms
  (4.6×), 50k 1157.7 ms → 565.3 ms (2.05×); growth 3.9× → 8.7× ns
  (10k → 50k), byte growth steady at 4.8× B (index ~5 B/replica-op at
  trace scale, +1.34 MB for the 83,751-op trace; replay wall 602 ms →
  250 ms, 2.4×). Fuzzed ~3.0M total execs, 0 crashers
  (`FuzzMergeConvergence` 145,952 @ 120 s, `FuzzDeltaConvergence`
  66,288 @ 120 s, `FuzzBinaryFrame` 2,800,492 @ 60 s). Parked: delta
  emission could use per-agent suffix cursors for O(kept) frames —
  parked, not measured as a hotspot.
- [x] **Incremental accessor views — resolved 2026-09-06** — `MapDocument.Keys()`
  iterates `keyIndex` instead of scanning the log (O(#ops) → O(#keys)):
  10k 1.37 ms → 0.23 ms (5.9×, 80 → 1 allocs), 50k 6.91 ms → 0.65 ms (10.6×,
  274 → 1 allocs); per-key LWW winner cache on `Get` with epoch invalidation
  (`epoch = len(keyIndex[key])`; `Compact()` strips the cache): warm Get 1k
  144 µs → 33 µs (4.3×), 10k 1.15 ms → 0.40 ms (2.8×). Parked note: cold Get
  at 10k is +33% (the miss pays a one-time cache fill on top of the walk) —
  inherent to caching, fine at this scale. Lazy render caches on
  RuneDocument/ArrayDocument (one dirty-flag invalidation at the
  `Ins`/`Del`/`MergeFrom`/`ApplyDelta`/`Compact`/`Reset` tails; `GetItems`
  returns a defensive copy preserving the fresh-slice contract): repeated
  `GetString` on untouched text 239 µs → 143 ns (~1670×), warm path only —
  the per-edit render still pays the full rope walk cold, so render caches
  profit read-after-read workloads, not one-render-per-edit editors.
  Oracles: `FuzzMapDocument` 60 s → 197,963 execs, 0 crashers; trace heap
  +~0.01 MB (24.23 → 24.24 MB).
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
  covered-skipped at op_log.go:383-421, then the new op's `(-1, seq)`
  parent edge has no agent-`-1` op to resolve against and panics in
  `runIdxForSeq`, op_log.go:206); sentinel-agent documents
  (`NewRuneDocument(-1)` & co.) are constructible and panic confusingly in
  `mergeInto`'s anchor rules — a constructor rejection is parked (behavior
  change out of scope). Supported merge directions today: both sides
  compacted (shared or aligned compaction points — a non-aligned anchor
  reference now panics loudly at the causing merge with the
  documented-topology message instead of silently splitting dest's anchor,
  pinned by `TestNonAlignedCompactionPointsPanic`; modulo the silent
  absorption above); compacted dest ← full src for ANCHORED dests — a
  zero-op / edited-empty-anchor dest holds no anchor lv and accepts only
  root or post-critical parentage; a pre-critical parent query on it
  panics (pinned by `TestEmptyAnchorCoveredParentPanics`); fresh (empty)
  dest adoption; compacted src with NO post-compaction edits ← full dest.
  Resolved 2026-09-05: the "unclear `runIdxForSeq` message" follow-up —
  the case where dest's own `-1` op matches the query (non-aligned
  compaction points) converts to the documented-topology message
  (`nonAlignedAnchorMsg`) instead of splitting the anchor; the F1 case
  above keeps `runIdxForSeq`'s message, which is literally accurate there
  (dest holds no `-1` op at all).

## Tests / hygiene

- [x] **Delta frames — resolved 2026-09-05** — `EGD1` delta frame format
  (eleven columnar sections: run-encoded ops with `(agent, endSeq)` parents,
  no Frontier, optional Coverage, informational-never-folded SenderVersion),
  shared `ingestOp` ingestion, `Version()/Delta/ApplyDelta` on all three
  document types, and the binding equivalence oracle `FuzzDeltaConvergence`
  (delta/merge interleaving + all-or-nothing compaction checkpoints;
  120 s → 42,006 execs, 0 crashers; re-runs clean: `FuzzMergeConvergence`
  60 s → 66,947 execs, `FuzzBinaryFrame` 60 s → 1,863,656 execs after the
  `ingestOp` refactor). Measured (`BenchmarkDeltaAtScale`, -benchtime=3x):
  n=10,000: full binary frame 20,277 B / empty-since delta 22,963 B /
  incremental delta 4,336 B, applied in 82.70 ms (82,697,750 ns/op);
  n=50,000: 40,851 / 50,590 / 25,574 B, applied in 138.23 ms
  (138,231,615 ns/op) — incremental apply is superlinear in log size
  (parent resolution's backward scan; see the parked `runIdxForSeq` item).
  Trace scale (`TestDeltaTraceScale`, 83,751 log ops): empty-since delta
  397,126 B vs full binary frame 327,592 B, content byte-identical,
  `Check()` green. Parked notes: `SenderVersion` is unused by the applier
  (a future ack/watermark wrapper would be its first consumer); map/array
  delta reconciliation inherits `mergeRecursive`'s shape (anchor-by-key for
  map values, element-0-only recursive reconciliation for array elements —
  the F4 carried carve-out); delta emission is an O(#ops) scan per sync (a
  per-agent seq index would make it O(missing), tying into the parked
  `runIdxForSeq` item above); document-typed `Mergeable` values (e.g.
  `ArrayDocument[*MapDocument]`) stay merge-only — deltas for those shapes
  are out of scope, so the fuzz oracle exercises rune, plain map, and
  non-document Mergeable value shapes; `UnmarshalDelta` accepts the
  reduplicated dual-coverage shape (anchor record with coverage AND riding
  log-level table where the wire format only requires one) — decoding is
  permissive where the encoder never emits it, parked as a tightening
  candidate (the hostile suite's `rowCount` cases pin the rejection it does
  enforce); the parent `(agent, endSeq)` encoding has a round-trip drift
  guard only via `TestDeltaFrameLevelRoundTrip` (byte-level re-encode
  identity of Parents is not pinned); the Coverage column truncation case
  has no dedicated hostile case (an empty/1-row body truncation overlaps
  the generic column-truncation test's `uncompacted Coverage body` skip).

- [x] **Extend FuzzDocumentOps's textChar alphabet — resolved 2026-09-06,
  multibyte-but-valid** — the font now includes valid 2/3/4-byte runes ("é",
  "你", emoji) flowing through insert/delete/merge; raw invalid-UTF-8 bytes
  were dropped after fuzzing surfaced that run fusion re-decodes adjacent
  invalid bytes as one valid rune ("\xc3"+"\x80" → "À", 1+1 runes → 1),
  breaking per-token rune invariants — runeText keeps invalid bytes intact
  within a single run but per-rune invariants only hold for valid UTF-8
  (pinned by the in-code comment). 60 s fuzz: 180,814 execs, 0 crashers.
- [x] **Pin an upper leaf bound in TestShapeBInteriorDeleteSplitsOneLeaf —
  resolved 2026-09-06 at exactly 2** — mechanism-derived, not lucky-run: the
  single-leaf interior delete directly rebuilds one 999-char survivor leaf
  (no split; the prior "3 leaves" note predates the direct-build delete) and
  both survivors (999/1000 chars) exceed ropeLeafCap so no coalescing can
  join them; asserted == 2 with message, stable at -count=5.
- [x] **estimateSize hardcodes rune byte width — resolved 2026-09-06
  (test-only)** — width now derived from the instantiated content type
  (`contentByteWidth[C]`: runeText → sizeof(rune); other instantiations
  panic with the type name instead of silently misestimating).
- [x] **Report cosmetics — resolved 2026-09-06** — Shape B report caveats 2-4
  moved under an "Additional caveats" subhead; caveat 1 stays with the
  delete-run heading.

## Housekeeping

- [ ] **Decide whether to commit `.gitignore` (local uncommitted edit) and
  the `docs/` tree** (plans/specs/reports; `docs/superpowers/plans/` rule is
  commented out in .gitignore but the tree is untracked — currently
  half-tracked: the Shape B report file is committed).
