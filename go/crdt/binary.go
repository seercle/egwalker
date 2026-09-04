package crdt

import (
	"encoding/binary"
	"fmt"
	"math"
)

// binaryMagic frames every encoded opLog: magic + version, then columns.
const binaryMagic = "EGW1"

// binaryVersion is the frame version written by MarshalBinary; decode rejects
// any other value.
const binaryVersion = 1

// opTypeCode maps an opType to its one-byte wire code (Types column body).
func opTypeCode(t opType) (byte, bool) {
	switch t {
	case opTypeIns:
		return 0, true
	case opTypeDel:
		return 1, true
	}
	return 0, false
}

// opTypeFromCode maps a wire code back to its opType.
func opTypeFromCode(b byte) (opType, bool) {
	switch b {
	case 0:
		return opTypeIns, true
	case 1:
		return opTypeDel, true
	}
	return "", false
}

// MarshalBinary encodes the opLog's columnar form into a byte frame: a magic
// and version header followed by ten length-prefixed column bodies. Integers
// use varints (zigzag where values may be negative); content values go
// through the codec and travel as opaque length-prefixed blobs. The frame
// knows nothing about content semantics.
func MarshalBinary[C content[C]](log *opLog[C], codec ContentCodec[C]) ([]byte, error) {
	data := log.Marshal()
	frame := make([]byte, 0, 256)
	frame = append(frame, binaryMagic...)
	frame = binary.AppendUvarint(frame, binaryVersion)

	var body []byte

	// Column 1: Types — one byte per type run.
	body = binary.AppendUvarint(body[:0], uint64(len(data.Types)))
	for _, t := range data.Types {
		code, ok := opTypeCode(t)
		if !ok {
			return nil, fmt.Errorf("binary: unknown op type %q", string(t))
		}
		body = append(body, code)
	}
	frame = appendBinaryColumn(frame, body)

	// Column 2: TypeRuns.
	body = binary.AppendUvarint(body[:0], uint64(len(data.TypeRuns)))
	for _, v := range data.TypeRuns {
		body = binary.AppendUvarint(body, uint64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 3: Agents (may be negative).
	body = binary.AppendUvarint(body[:0], uint64(len(data.Agents)))
	for _, v := range data.Agents {
		body = binary.AppendVarint(body, int64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 4: AgentRuns.
	body = binary.AppendUvarint(body[:0], uint64(len(data.AgentRuns)))
	for _, v := range data.AgentRuns {
		body = binary.AppendUvarint(body, uint64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 5: Seqs (may be negative).
	body = binary.AppendUvarint(body[:0], uint64(len(data.Seqs)))
	for _, v := range data.Seqs {
		body = binary.AppendVarint(body, int64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 6: Positions (deltas may be negative).
	body = binary.AppendUvarint(body[:0], uint64(len(data.Positions)))
	for _, v := range data.Positions {
		body = binary.AppendVarint(body, int64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 7: Lengths.
	body = binary.AppendUvarint(body[:0], uint64(len(data.Lengths)))
	for _, v := range data.Lengths {
		body = binary.AppendUvarint(body, uint64(v))
	}
	frame = appendBinaryColumn(frame, body)

	// Column 8: Content — one opaque codec blob per op.
	body = binary.AppendUvarint(body[:0], uint64(len(data.Content)))
	for _, c := range data.Content {
		blob, err := codec.Encode(c)
		if err != nil {
			return nil, fmt.Errorf("binary: encode content: %w", err)
		}
		body = binary.AppendUvarint(body, uint64(len(blob)))
		body = append(body, blob...)
	}
	frame = appendBinaryColumn(frame, body)

	// Column 9: Parents — one count per op, then all lvs.
	body = binary.AppendUvarint(body[:0], uint64(len(data.Parents)))
	for _, ps := range data.Parents {
		body = binary.AppendUvarint(body, uint64(len(ps)))
	}
	for _, ps := range data.Parents {
		for _, p := range ps {
			body = binary.AppendUvarint(body, uint64(p))
		}
	}
	frame = appendBinaryColumn(frame, body)

	// Column 10: Frontier.
	body = binary.AppendUvarint(body[:0], uint64(len(data.Frontier)))
	for _, v := range data.Frontier {
		body = binary.AppendUvarint(body, uint64(v))
	}
	frame = appendBinaryColumn(frame, body)

	return frame, nil
}

// appendBinaryColumn appends one column to the frame: uvarint bodyLen || body.
func appendBinaryColumn(frame, body []byte) []byte {
	frame = binary.AppendUvarint(frame, uint64(len(body)))
	return append(frame, body...)
}

// binaryReader walks a frame or a column body with bounds-checked reads.
type binaryReader struct {
	buf []byte
	off int
}

func (r *binaryReader) uvarint(what string) (uint64, error) {
	v, n := binary.Uvarint(r.buf[r.off:])
	if n == 0 {
		return 0, fmt.Errorf("binary: truncated varint in %s", what)
	}
	if n < 0 {
		return 0, fmt.Errorf("binary: varint overflow in %s", what)
	}
	r.off += n
	return v, nil
}

func (r *binaryReader) varint(what string) (int64, error) {
	v, n := binary.Varint(r.buf[r.off:])
	if n == 0 {
		return 0, fmt.Errorf("binary: truncated varint in %s", what)
	}
	if n < 0 {
		return 0, fmt.Errorf("binary: varint overflow in %s", what)
	}
	r.off += n
	return v, nil
}

// column reads one length-prefixed column body: uvarint bodyLen || body.
func (r *binaryReader) column(name string) ([]byte, error) {
	n, err := r.uvarint(name + " bodyLen")
	if err != nil {
		return nil, err
	}
	if n > uint64(len(r.buf)-r.off) {
		return nil, fmt.Errorf("binary: %s column truncated: bodyLen %d exceeds %d remaining bytes", name, n, len(r.buf)-r.off)
	}
	body := r.buf[r.off : r.off+int(n)]
	r.off += int(n)
	return body, nil
}

// count reads a column's entry count and checks that n entries (at least one
// byte each) fit in the body that remains.
func (r *binaryReader) count(name string) (uint64, error) {
	n, err := r.uvarint(name + " count")
	if err != nil {
		return 0, err
	}
	if n > uint64(len(r.buf)-r.off) {
		return 0, fmt.Errorf("binary: %s count %d exceeds %d remaining body bytes", name, n, len(r.buf)-r.off)
	}
	return n, nil
}

// exact checks that a column body was consumed to its end.
func (r *binaryReader) exact(name string) error {
	if r.off != len(r.buf) {
		return fmt.Errorf("binary: %s column has %d trailing bytes", name, len(r.buf)-r.off)
	}
	return nil
}

// uvarintToInt converts an unsigned varint to int, rejecting overflow.
func uvarintToInt(v uint64, what string) (int, error) {
	if v > math.MaxInt {
		return 0, fmt.Errorf("binary: %s overflows int: %d", what, v)
	}
	return int(v), nil
}

// varintToInt converts a signed varint to int, rejecting overflow.
func varintToInt(v int64, what string) (int, error) {
	if v < math.MinInt || v > math.MaxInt {
		return 0, fmt.Errorf("binary: %s overflows int: %d", what, v)
	}
	return int(v), nil
}

// readRunLens decodes n uvarint run lengths. The running sum is capped at
// maxOps: a valid frame's op count is bounded by its own size, since every
// op occupies at least one byte in each per-op column.
func readRunLens(br *binaryReader, n uint64, maxOps uint64, name string) ([]int, uint64, error) {
	runs := make([]int, n)
	sum := uint64(0)
	for i := range runs {
		v, err := br.uvarint(name)
		if err != nil {
			return nil, 0, err
		}
		run, err := uvarintToInt(v, name)
		if err != nil {
			return nil, 0, err
		}
		if uint64(run) > maxOps-sum {
			return nil, 0, fmt.Errorf("binary: %s op count %d exceeds frame bound %d", name, sum+uint64(run), maxOps)
		}
		sum += uint64(run)
		runs[i] = run
	}
	return runs, sum, nil
}

// UnmarshalBinary decodes a frame produced by MarshalBinary back into an
// opLog: parse magic and version, then each length-prefixed column with
// bounds-checked reads, validating the count invariants the columnar form
// relies on, and rebuild the log through Unmarshal.
func UnmarshalBinary[C content[C]](data []byte, codec ContentCodec[C]) (*opLog[C], error) {
	if len(data) < len(binaryMagic) || string(data[:len(binaryMagic)]) != binaryMagic {
		return nil, fmt.Errorf("binary: bad magic")
	}
	r := &binaryReader{buf: data, off: len(binaryMagic)}

	version, err := r.uvarint("version")
	if err != nil {
		return nil, err
	}
	if version != binaryVersion {
		return nil, fmt.Errorf("binary: unsupported version %d", version)
	}

	// Op counts can never exceed the frame's own byte count: every op costs
	// at least one byte in each per-op column.
	maxOps := uint64(len(data))

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
		code := br.buf[br.off]
		ot, ok := opTypeFromCode(code)
		if !ok {
			return nil, fmt.Errorf("binary: unknown op type code %d", code)
		}
		br.off++
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

	// Column 5: Seqs (one per Agents entry).
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

	// Column 6: Positions (one delta per op).
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
	positions := make([]int, n)
	for i := range positions {
		v, err := br.varint("Positions")
		if err != nil {
			return nil, err
		}
		if positions[i], err = varintToInt(v, "Positions"); err != nil {
			return nil, err
		}
	}
	if err := br.exact("Positions"); err != nil {
		return nil, err
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

	// Column 9: Parents — one count per op, then all lvs.
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
	lvBudget := uint64(len(br.buf) - br.off)
	for i := range counts {
		v, err := br.uvarint("Parents")
		if err != nil {
			return nil, err
		}
		cnt, err := uvarintToInt(v, "Parents")
		if err != nil {
			return nil, err
		}
		// Each lv costs at least one byte, so the counts cannot claim more
		// lvs than the body has bytes left.
		if uint64(cnt) > lvBudget {
			return nil, fmt.Errorf("binary: Parents lv count %d exceeds %d remaining body bytes", cnt, lvBudget)
		}
		lvBudget -= uint64(cnt)
		counts[i] = cnt
	}
	parents := make([][]lv, n)
	for i, cnt := range counts {
		// Zero-count ops get an empty non-nil slice: pushLocalOp and the
		// merge path always give parentless ops a non-nil parents slice,
		// and the round-trip must preserve that for DeepEqual comparisons.
		ps := make([]lv, cnt)
		for j := range ps {
			v, err := br.uvarint("Parents")
			if err != nil {
				return nil, err
			}
			p, err := uvarintToInt(v, "Parents")
			if err != nil {
				return nil, err
			}
			ps[j] = lv(p)
		}
		parents[i] = ps
	}
	if err := br.exact("Parents"); err != nil {
		return nil, err
	}

	// Column 10: Frontier.
	body, err = r.column("Frontier")
	if err != nil {
		return nil, err
	}
	br = &binaryReader{buf: body}
	n, err = br.count("Frontier")
	if err != nil {
		return nil, err
	}
	frontier := make([]lv, n)
	for i := range frontier {
		v, err := br.uvarint("Frontier")
		if err != nil {
			return nil, err
		}
		f, err := uvarintToInt(v, "Frontier")
		if err != nil {
			return nil, err
		}
		frontier[i] = lv(f)
	}
	if err := br.exact("Frontier"); err != nil {
		return nil, err
	}

	if r.off != len(r.buf) {
		return nil, fmt.Errorf("binary: %d trailing bytes after Frontier column", len(r.buf)-r.off)
	}

	return Unmarshal[C](&ColumnarData[C]{
		Types:     types,
		TypeRuns:  typeRuns,
		Agents:    agents,
		AgentRuns: agentRuns,
		Seqs:      seqs,
		Positions: positions,
		Lengths:   lengths,
		Content:   content,
		Parents:   parents,
		Frontier:  frontier,
	}), nil
}
