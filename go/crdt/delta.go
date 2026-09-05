package crdt

import (
	"encoding/binary"
	"fmt"
)

// Delta frames are the incremental-sync counterpart of full binary frames:
// magic "EGD1" || uvarint deltaVersion || column × N, zstd-wrapped with the
// same 64 MiB decode cap. Columns reuse the full-frame encodings (mirroring
// binary.go's MarshalBinary encoder EXACTLY — see the column-by-column
// comments there) except Parents, which carries (agent, endSeq) references
// instead of sender-local lvs — a delta's parents are exactly the ops the
// receiver already holds, so lv references would be unresolvable. There is no
// Frontier column: the receiver never takes the sender's frontier.
// SenderVersion is informational (debugging, future ack/watermark wrappers);
// it is NEVER folded into the receiver's version, which would claim ops the
// receiver does not hold.
const (
	deltaMagic   = "EGD1"
	deltaVersion = 1
)

// deltaOp is one wire record of a delta frame. op.parents stays zero on the
// wire record; parents holds (agent, endSeq) causal references, resolvable
// against whatever op boundaries the receiver holds. coverage is set only on
// anchor records.
type deltaOp[C content[C]] struct {
	op       op[C]         // parents field unused on the wire (zero)
	parents  []id          // (agent, end seq) causal references
	coverage remoteVersion // set only on anchor records
}

// deltaFrame is a decoded delta frame: a causally-ordered slice of wire
// records plus the sender's version vector (informational) and, when the
// sender's log is compacted but no anchor record is present, the log-level
// anchor coverage table the applier must fold in (nil means absent).
type deltaFrame[C content[C]] struct {
	ops            []deltaOp[C]
	senderVersion  remoteVersion
	anchorCoverage remoteVersion
}

// MarshalDelta encodes every op the receiver lacks: ops whose seq range
// extends past since[agent], plus the anchor iff not redundant vs since.
// Log order is topological (parents always precede children), so the kept
// slice is a causally-valid delta.
func MarshalDelta[C content[C]](log *opLog[C], codec ContentCodec[C], since remoteVersion) ([]byte, error) {
	kept := make([]op[C], 0, len(log.ops))
	for i := range log.ops {
		o := &log.ops[i]
		if o.id.agent == anchorAgent {
			// Include the anchor iff some covered seq is missing at since.
			for agent, seq := range o.coverage {
				if since[agent] < seq {
					kept = append(kept, *o)
					break
				}
			}
			continue
		}
		if last, ok := since[o.id.agent]; ok && last >= o.id.seq+o.length-1 {
			continue
		}
		kept = append(kept, *o)
	}

	// The per-op column pass mirrors MarshalBinary's encoder over `kept`
	// (binary.go:109-221): same column ORDER, same integer encodings, same
	// opTypeCode use. This duplication is deliberate and bounded: it keeps
	// the proven v2 full-frame path untouched; the round-trip tests and
	// FuzzDeltaFrame are the drift guards.

	// Types/TypeRuns and Agents/AgentRuns/Seqs via the same RLE pass
	// opLog.Marshal performs (serialization.go:26).
	var types []opType
	var typeRuns []int
	var agents []int
	var agentRuns []int
	var seqs []int
	var positions []int
	var lengths = make([]int, 0, len(kept))
	var parentsIDs = make([][]id, 0, len(kept))
	var contents []C

	var lastType opType
	var lastAgent int
	var typeRun int
	var agentRun int
	lastPos := 0

	for i := range kept {
		o := &kept[i]
		// --- 1. Run-Length Encode Types ---
		if i == 0 {
			lastType = o.opType
			typeRun = 1
		} else if o.opType == lastType {
			typeRun++
		} else {
			types = append(types, lastType)
			typeRuns = append(typeRuns, typeRun)
			lastType = o.opType
			typeRun = 1
		}

		// --- 2. Run-Length Encode Agents & Seqs ---
		if i == 0 {
			lastAgent = o.id.agent
			agentRun = 1
			seqs = append(seqs, o.id.seq)
		} else if o.id.agent == lastAgent {
			agentRun++
		} else {
			agents = append(agents, lastAgent)
			agentRuns = append(agentRuns, agentRun)
			lastAgent = o.id.agent
			agentRun = 1
			seqs = append(seqs, o.id.seq)
		}

		// --- 3. Delta-Encode Positions (vs the previous kept op) ---
		if i == 0 {
			positions = append(positions, o.pos)
		} else {
			positions = append(positions, o.pos-lastPos)
		}
		lastPos = o.pos

		// --- 4. Length, Content & remapped Parents ---
		lengths = append(lengths, o.length)
		parentsIDs = append(parentsIDs, nil)
		if len(o.parents) > 0 {
			ps := make([]id, len(o.parents))
			for j, p := range o.parents {
				// Parents on the wire are (agent, endSeq) character
				// references, not sender-local lvs.
				pa := log.opAt(p)
				ps[j] = id{agent: pa.id.agent, seq: log.seqAt(p)}
			}
			parentsIDs[i] = ps
		}
		contents = append(contents, o.content)
	}
	if len(kept) > 0 {
		types = append(types, lastType)
		typeRuns = append(typeRuns, typeRun)
		agents = append(agents, lastAgent)
		agentRuns = append(agentRuns, agentRun)
	}

	frame := make([]byte, 0, 256)
	frame = append(frame, deltaMagic...)
	frame = binary.AppendUvarint(frame, deltaVersion)

	var body []byte

	// Column 1: Types — one byte per type run.
	body = binary.AppendUvarint(body[:0], uint64(len(types)))
	for _, t := range types {
		code, ok := opTypeCode(t)
		if !ok {
			return nil, fmt.Errorf("binary: unknown op type %q", string(t))
		}
		body = append(body, code)
	}
	frame = appendBinaryColumn(frame, body)

	// Column 2: TypeRuns.
	body = binary.AppendUvarint(body[:0], uint64(len(typeRuns)))
	for _, v := range typeRuns {
		body = binary.AppendUvarint(body, uint64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 3: Agents (may be negative: anchors).
	body = binary.AppendUvarint(body[:0], uint64(len(agents)))
	for _, v := range agents {
		body = binary.AppendVarint(body, int64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 4: AgentRuns.
	body = binary.AppendUvarint(body[:0], uint64(len(agentRuns)))
	for _, v := range agentRuns {
		body = binary.AppendUvarint(body, uint64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 5: Seqs (one start seq per Agents entry; may be negative).
	body = binary.AppendUvarint(body[:0], uint64(len(seqs)))
	for _, v := range seqs {
		body = binary.AppendVarint(body, int64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 6: Positions (deltas may be negative).
	body = binary.AppendUvarint(body[:0], uint64(len(positions)))
	for _, v := range positions {
		body = binary.AppendVarint(body, int64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 7: Lengths.
	body = binary.AppendUvarint(body[:0], uint64(len(lengths)))
	for _, v := range lengths {
		body = binary.AppendUvarint(body, uint64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 8: Content — one opaque codec blob per op.
	body = binary.AppendUvarint(body[:0], uint64(len(contents)))
	for _, c := range contents {
		blob, err := codec.Encode(c)
		if err != nil {
			return nil, fmt.Errorf("binary: encode content: %w", err)
		}
		body = binary.AppendUvarint(body, uint64(len(blob)))
		body = append(body, blob...)
	}
	frame = appendBinaryColumn(frame, body)

	// Column 9: Parents — one count per op, then all (agent, endSeq) pairs.
	// The agent deltas use the same zigzag-delta-from-previous-agent
	// convention encodeCoverageTable uses (first delta from 0).
	body = binary.AppendUvarint(body[:0], uint64(len(parentsIDs)))
	for _, ps := range parentsIDs {
		body = binary.AppendUvarint(body, uint64(len(ps)))
	}
	prevAgent := 0
	for _, ps := range parentsIDs {
		for _, p := range ps {
			body = binary.AppendVarint(body, int64(p.agent-prevAgent))
			prevAgent = p.agent
			body = binary.AppendUvarint(body, uint64(p.seq))
		}
	}
	frame = appendBinaryColumn(frame, body)

	// Column 10: Coverage — v2's row scheme (binary.go:201-217). When the
	// sender log is compacted and no anchor record is kept (a fully-held
	// anchor), the table rides in an extra row; the kept-anchor shape gives
	// rowCount == op count and row 0 is the anchor's per-op row.
	body = body[:0]
	if log.isCompacted() {
		anchorless := len(kept) == 0 || kept[0].id.agent != anchorAgent
		rows := len(kept)
		if anchorless {
			rows++ // no anchor record to carry the table: it rides alone
		}
		body = binary.AppendUvarint(body, uint64(rows))
		if len(kept) > 0 && kept[0].id.agent == anchorAgent {
			// Kept-anchor shape: row 0 is the anchor record's per-op row and
			// carries its table; every later row is an empty per-op row.
			body = appendCoverageRow(body, encodeCoverageTable(nil, kept[0].coverage))
			for range rows - 1 {
				body = binary.AppendUvarint(body, 0)
			}
		} else {
			// Anchorless shape: empty per-op rows, then the extra row that
			// carries the log-level table.
			for range len(kept) {
				body = binary.AppendUvarint(body, 0)
			}
			body = appendCoverageRow(body, encodeCoverageTable(nil, log.anchorCoverage))
		}
	}
	frame = appendBinaryColumn(frame, body)

	// Column 11: SenderVersion — one coverage table, always present.
	body = body[:0]
	body = encodeCoverageTable(body, log.version)
	frame = appendBinaryColumn(frame, body)

	return binaryZstdEncoder.EncodeAll(frame, nil), nil
}

// UnmarshalDelta decodes a delta frame produced by MarshalDelta back into a
// deltaFrame. It mirrors UnmarshalBinary's structure (binary.go:420): zstd
// Decomall, magic and version check, a maxOps bound of the frame's own byte
// length, then per column with binaryReader + count/exact discipline. All
// syntactic violations return an error with a `binary: ` prefix; semantic
// violations (invalid topology) are the applier's business, not the
// decoder's.
func UnmarshalDelta[C content[C]](data []byte, codec ContentCodec[C]) (*deltaFrame[C], error) {
	frame, err := binaryZstdDecoder.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("binary: decompress: %w", err)
	}
	if len(frame) < len(deltaMagic) || string(frame[:len(deltaMagic)]) != deltaMagic {
		return nil, fmt.Errorf("binary: bad magic")
	}
	r := &binaryReader{buf: frame, off: len(deltaMagic)}

	version, err := r.uvarint("version")
	if err != nil {
		return nil, err
	}
	if version != deltaVersion {
		return nil, fmt.Errorf("binary: unsupported version %d", version)
	}

	// Op counts can never exceed the frame's own byte count: every op costs
	// at least one byte in each per-op column.
	maxOps := uint64(len(frame))

	// Column 1: Types.
	body, err := r.column("Types")
	if err != nil {
		return nil, err
	}
	br := &binaryReader{buf: body}
	n, err := br.count("Types")
	if err != nil {
		return nil, err
	}
	types := make([]opType, n)
	for i := range types {
		code, err := br.byte("Types")
		if err != nil {
			return nil, err
		}
		ot, ok := opTypeFromCode(code)
		if !ok {
			return nil, fmt.Errorf("binary: unknown op type code %d", code)
		}
		types[i] = ot
	}
	if err := br.exact("Types"); err != nil {
		return nil, err
	}

	// Column 2: TypeRuns (one run length per Types entry).
	body, err = r.column("TypeRuns")
	if err != nil {
		return nil, err
	}
	br = &binaryReader{buf: body}
	n, err = br.count("TypeRuns")
	if err != nil {
		return nil, err
	}
	if n != uint64(len(types)) {
		return nil, fmt.Errorf("binary: TypeRuns count %d != Types count %d", n, len(types))
	}
	typeRuns, totalOps, err := readRunLens(br, n, maxOps, "TypeRuns")
	if err != nil {
		return nil, err
	}
	if err := br.exact("TypeRuns"); err != nil {
		return nil, err
	}

	// Column 3: Agents.
	body, err = r.column("Agents")
	if err != nil {
		return nil, err
	}
	br = &binaryReader{buf: body}
	n, err = br.count("Agents")
	if err != nil {
		return nil, err
	}
	agents := make([]int, n)
	for i := range agents {
		v, err := br.varint("Agents")
		if err != nil {
			return nil, err
		}
		if agents[i], err = varintToInt(v, "Agents"); err != nil {
			return nil, err
		}
	}
	if err := br.exact("Agents"); err != nil {
		return nil, err
	}

	// Column 4: AgentRuns (one run length per Agents entry).
	body, err = r.column("AgentRuns")
	if err != nil {
		return nil, err
	}
	br = &binaryReader{buf: body}
	n, err = br.count("AgentRuns")
	if err != nil {
		return nil, err
	}
	if n != uint64(len(agents)) {
		return nil, fmt.Errorf("binary: AgentRuns count %d != Agents count %d", n, len(agents))
	}
	agentRuns, agentOps, err := readRunLens(br, n, maxOps, "AgentRuns")
	if err != nil {
		return nil, err
	}
	if agentOps != totalOps {
		return nil, fmt.Errorf("binary: AgentRuns sum %d != op count %d", agentOps, totalOps)
	}
	if err := br.exact("AgentRuns"); err != nil {
		return nil, err
	}

	// Column 5: Seqs (one start seq per Agents entry).
	body, err = r.column("Seqs")
	if err != nil {
		return nil, err
	}
	br = &binaryReader{buf: body}
	n, err = br.count("Seqs")
	if err != nil {
		return nil, err
	}
	if n != uint64(len(agents)) {
		return nil, fmt.Errorf("binary: Seqs count %d != Agents count %d", n, len(agents))
	}
	seqs := make([]int, n)
	for i := range seqs {
		v, err := br.varint("Seqs")
		if err != nil {
			return nil, err
		}
		if seqs[i], err = varintToInt(v, "Seqs"); err != nil {
			return nil, err
		}
	}
	if err := br.exact("Seqs"); err != nil {
		return nil, err
	}

	// Column 6: Positions (one delta per op; first entry absolute).
	body, err = r.column("Positions")
	if err != nil {
		return nil, err
	}
	br = &binaryReader{buf: body}
	n, err = br.count("Positions")
	if err != nil {
		return nil, err
	}
	if n != totalOps {
		return nil, fmt.Errorf("binary: Positions count %d != op count %d", n, totalOps)
	}
	posDeltas := make([]int, n)
	for i := range posDeltas {
		v, err := br.varint("Positions")
		if err != nil {
			return nil, err
		}
		if posDeltas[i], err = varintToInt(v, "Positions"); err != nil {
			return nil, err
		}
	}
	if err := br.exact("Positions"); err != nil {
		return nil, err
	}
	positions := make([]int, len(posDeltas))
	for i, delta := range posDeltas {
		if i == 0 {
			positions[i] = delta
		} else {
			positions[i] = positions[i-1] + delta
		}
	}

	// Column 7: Lengths (one per op).
	body, err = r.column("Lengths")
	if err != nil {
		return nil, err
	}
	br = &binaryReader{buf: body}
	n, err = br.count("Lengths")
	if err != nil {
		return nil, err
	}
	if n != totalOps {
		return nil, fmt.Errorf("binary: Lengths count %d != op count %d", n, totalOps)
	}
	lengths := make([]int, n)
	for i := range lengths {
		v, err := br.uvarint("Lengths")
		if err != nil {
			return nil, err
		}
		if lengths[i], err = uvarintToInt(v, "Lengths"); err != nil {
			return nil, err
		}
	}
	if err := br.exact("Lengths"); err != nil {
		return nil, err
	}

	// Column 8: Content — one codec blob per op.
	body, err = r.column("Content")
	if err != nil {
		return nil, err
	}
	br = &binaryReader{buf: body}
	n, err = br.count("Content")
	if err != nil {
		return nil, err
	}
	if n != totalOps {
		return nil, fmt.Errorf("binary: Content count %d != op count %d", n, totalOps)
	}
	content := make([]C, n)
	for i := range content {
		blobLen, err := br.uvarint("Content")
		if err != nil {
			return nil, err
		}
		if blobLen > uint64(len(br.buf)-br.off) {
			return nil, fmt.Errorf("binary: content blob %d truncated: %d bytes claimed, %d remain", i, blobLen, len(br.buf)-br.off)
		}
		blob := br.buf[br.off : br.off+int(blobLen)]
		br.off += int(blobLen)
		c, err := codec.Decode(blob)
		if err != nil {
			return nil, fmt.Errorf("binary: decode content op %d: %w", i, err)
		}
		content[i] = c
	}
	if err := br.exact("Content"); err != nil {
		return nil, err
	}

	// Column 9: Parents — one count per op, then all (agent, endSeq) pairs.
	// Agent deltas are zigzag-delta from the previous parent's agent (first
	// from 0); seqs are uvarints.
	body, err = r.column("Parents")
	if err != nil {
		return nil, err
	}
	br = &binaryReader{buf: body}
	n, err = br.count("Parents")
	if err != nil {
		return nil, err
	}
	if n != totalOps {
		return nil, fmt.Errorf("binary: Parents count %d != op count %d", n, totalOps)
	}
	counts := make([]int, n)
	pairBudget := uint64(len(br.buf) - br.off)
	for i := range counts {
		v, err := br.uvarint("Parents")
		if err != nil {
			return nil, err
		}
		cnt, err := uvarintToInt(v, "Parents")
		if err != nil {
			return nil, err
		}
		// Each pair costs at least two bytes (agent delta + seq), so the
		// counts cannot claim more pairs than half the body's bytes left.
		if uint64(cnt)*2 > pairBudget {
			return nil, fmt.Errorf("binary: Parents pair count %d exceeds %d remaining body bytes", cnt, pairBudget)
		}
		pairBudget -= uint64(cnt) * 2
		counts[i] = cnt
	}
	parents := make([][]id, n)
	prevAgent := 0
	for i, cnt := range counts {
		ps := make([]id, cnt)
		for j := range ps {
			v, err := br.varint("Parents")
			if err != nil {
				return nil, err
			}
			d, err := varintToInt(v, "Parents agent delta")
			if err != nil {
				return nil, err
			}
			agent := prevAgent + d
			seqV, err := br.uvarint("Parents seq")
			if err != nil {
				return nil, err
			}
			seq, err := uvarintToInt(seqV, "Parents seq")
			if err != nil {
				return nil, err
			}
			ps[j] = id{agent: agent, seq: seq}
			prevAgent = agent
		}
		parents[i] = ps
	}
	if err := br.exact("Parents"); err != nil {
		return nil, err
	}

	// There is no Frontier column: the receiver never takes the sender's
	// frontier.

	// Expand run-level Types (one code per run) into per-op types, and reuse
	// the same expansion Unmarshal performs for agent runs: within an agent
	// run each op advances the seq by its length.
	opTypes := make([]opType, totalOps)
	opIdx := 0
	for i, t := range types {
		for range typeRuns[i] {
			opTypes[opIdx] = t
			opIdx++
		}
	}
	dops := make([]deltaOp[C], totalOps)
	opIdx = 0
	for i, agent := range agents {
		run := agentRuns[i]
		seq := seqs[i]
		for j := range run {
			dops[opIdx+j].op.opType = opTypes[opIdx+j]
			dops[opIdx+j].op.id = id{agent: agent, seq: seq}
			dops[opIdx+j].op.pos = positions[opIdx+j]
			dops[opIdx+j].op.length = lengths[opIdx+j]
			dops[opIdx+j].op.content = content[opIdx+j]
			dops[opIdx+j].parents = parents[opIdx+j]
			seq += lengths[opIdx+j]
		}
		opIdx += run
	}

	// Column 10: Coverage — v2's row scheme. A zero-length body means the
	// sender log is uncompacted. rowCount ∈ {opCount, opCount+1}: the
	// kept-anchor shape (row 0 is the anchor record's table) or the
	// anchorless shape (an extra last row carries the log-level table).
	var frameAnchorCoverage remoteVersion
	body, err = r.column("Coverage")
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		cbr := &binaryReader{buf: body}
		rowCount, err := cbr.uvarint("Coverage row count")
		if err != nil {
			return nil, err
		}
		if rowCount != totalOps && rowCount != totalOps+1 {
			return nil, fmt.Errorf("binary: coverage rowCount %d != %d or %d", rowCount, totalOps, totalOps+1)
		}
		rows := make([][]byte, rowCount)
		for i := range rows {
			rowLen, err := cbr.uvarint("Coverage row length")
			if err != nil {
				return nil, err
			}
			if rowLen > uint64(len(cbr.buf)-cbr.off) {
				return nil, fmt.Errorf("binary: Coverage row %d truncated: %d bytes claimed, %d remain", i, rowLen, len(cbr.buf)-cbr.off)
			}
			rows[i] = cbr.buf[cbr.off : cbr.off+int(rowLen)]
			cbr.off += int(rowLen)
		}
		if err := cbr.exact("Coverage"); err != nil {
			return nil, err
		}
		if rowCount == 0 {
			return nil, fmt.Errorf("binary: coverage rowCount %d != %d or %d", rowCount, totalOps, totalOps+1)
		}
		for i := 0; i < int(rowCount) && (rowCount == totalOps || i < int(totalOps)); i++ {
			// Per-op row: non-empty only on an anchor record, empty never on
			// an anchor record.
			if len(rows[i]) == 0 {
				if dops[i].op.id.agent == anchorAgent {
					return nil, fmt.Errorf("binary: anchor record without coverage")
				}
				continue
			}
			if dops[i].op.id.agent != anchorAgent {
				return nil, fmt.Errorf("binary: coverage on non-anchor op")
			}
			tbl, err := parseCoverageTable(rows[i])
			if err != nil {
				return nil, err
			}
			dops[i].coverage = tbl
		}
		if rowCount == totalOps+1 {
			// Anchorless layout: the extra row (index opCount, not an op)
			// carries the log-level table. It is invalid for a kept anchor
			// record to coexist with the extra row (the encoder emits exactly
			// one of the two shapes).
			if len(rows[int(totalOps)]) == 0 {
				return nil, fmt.Errorf("binary: coverage column without anchor coverage")
			}
			tbl, err := parseCoverageTable(rows[int(totalOps)])
			if err != nil {
				return nil, err
			}
			frameAnchorCoverage = tbl
		}
	}

	// Column 11: SenderVersion — one coverage table, always present.
	body, err = r.column("SenderVersion")
	if err != nil {
		return nil, err
	}
	senderVersion, err := parseCoverageTable(body)
	if err != nil {
		return nil, fmt.Errorf("binary: SenderVersion column truncated: %w", err)
	}

	if r.off != len(r.buf) {
		return nil, fmt.Errorf("binary: %d trailing bytes after SenderVersion column", len(r.buf)-r.off)
	}

	return &deltaFrame[C]{ops: dops, senderVersion: senderVersion, anchorCoverage: frameAnchorCoverage}, nil
}
