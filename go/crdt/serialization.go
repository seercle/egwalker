package crdt

import (
	"fmt"
)

// ColumnarData represents the opLog in a columnar format for efficient compression.
type ColumnarData[E any, C content[E]] struct {
	Types     []opType
	TypeRuns  []int
	Agents    []int
	AgentRuns []int
	Seqs      []int // Start sequence for each agent run
	Positions []int // Delta-encoded positions
	Content   []C
	Parents   [][]lv
	Frontier  []lv
}

// Marshal converts the opLog into a ColumnarData structure.
func (log *opLog[E, C]) Marshal() *ColumnarData[E, C] {
	if len(log.ops) == 0 {
		return &ColumnarData[E, C]{
			Frontier: log.frontier,
		}
	}

	res := &ColumnarData[E, C]{
		Content:  make([]C, 0, len(log.ops)),
		Parents:  make([][]lv, 0, len(log.ops)),
		Frontier: log.frontier,
	}

	var lastType opType
	var lastAgent int
	var typeRun int
	var agentRun int
	var lastPos int

	for i, o := range log.ops {
		// --- 1. Run-Length Encode Types ---
		if i == 0 {
			lastType = o.opType
			typeRun = 1
		} else if o.opType == lastType {
			typeRun++
		} else {
			res.Types = append(res.Types, lastType)
			res.TypeRuns = append(res.TypeRuns, typeRun)
			lastType = o.opType
			typeRun = 1
		}

		// --- 2. Run-Length Encode Agents & Seqs ---
		if i == 0 {
			lastAgent = o.id.agent
			agentRun = 1
			res.Seqs = append(res.Seqs, o.id.seq)
		} else if o.id.agent == lastAgent {
			agentRun++
		} else {
			res.Agents = append(res.Agents, lastAgent)
			res.AgentRuns = append(res.AgentRuns, agentRun)
			lastAgent = o.id.agent
			agentRun = 1
			res.Seqs = append(res.Seqs, o.id.seq)
		}

		// --- 3. Delta-Encode Positions ---
		// We store the raw first position, then deltas.
		if i == 0 {
			res.Positions = append(res.Positions, o.pos)
		} else {
			res.Positions = append(res.Positions, o.pos-lastPos)
		}
		lastPos = o.pos

		// --- 4. Content & Parents ---
		res.Content = append(res.Content, o.content)
		res.Parents = append(res.Parents, o.parents)
	}

	// Flush last runs
	res.Types = append(res.Types, lastType)
	res.TypeRuns = append(res.TypeRuns, typeRun)
	res.Agents = append(res.Agents, lastAgent)
	res.AgentRuns = append(res.AgentRuns, agentRun)

	return res
}

// Unmarshal rebuilds an opLog from ColumnarData.
func Unmarshal[E any, C content[E]](data *ColumnarData[E, C]) *opLog[E, C] {
	log := newOpLog[E, C]()
	log.frontier = data.Frontier

	if len(data.Types) == 0 {
		return log
	}

	totalOps := 0
	for _, r := range data.TypeRuns {
		totalOps += r
	}

	log.ops = make([]op[E, C], totalOps)

	// --- 1. Expand Types ---
	opIdx := 0
	for i, t := range data.Types {
		run := data.TypeRuns[i]
		for j := range run {
			log.ops[opIdx+j].opType = t
		}
		opIdx += run
	}

	// --- 2. Expand Agents & Seqs ---
	opIdx = 0
	for i, agent := range data.Agents {
		run := data.AgentRuns[i]
		startSeq := data.Seqs[i]
		for j := range run {
			log.ops[opIdx+j].id = id{agent: agent, seq: startSeq + j}
		}
		opIdx += run
	}

	// --- 3. Decode Positions ---
	lastPos := 0
	for i, delta := range data.Positions {
		if i == 0 {
			log.ops[i].pos = delta
			lastPos = delta
		} else {
			log.ops[i].pos = lastPos + delta
			lastPos = log.ops[i].pos
		}
	}

	// --- 4. Content & Parents & Maps ---
	for i := 0; i < totalOps; i++ {
		log.ops[i].content = data.Content[i]
		log.ops[i].parents = data.Parents[i]
		if log.ops[i].opType != opTypeDel {
			log.ops[i].length = log.ops[i].content.Len()
		}

		// Rebuild opLV/totalLV accounting (op i's first LV is the running
		// character count before it).
		log.opLV = append(log.opLV, log.totalLV)
		log.totalLV += lv(log.ops[i].length)

		// Rebuild idToLV mapping
		log.idToLV[log.ops[i].id] = lv(i)

		// Update version map
		agent := log.ops[i].id.agent
		seq := log.ops[i].id.seq
		if currentSeq, ok := log.version[agent]; !ok || seq > currentSeq {
			log.version[agent] = seq
		}
	}

	return log
}

// Stats prints comparison between row-based and columnar representation.
func (log *opLog[E, C]) PrintCompressionStats() {
	fmt.Printf("OpLog Stats:\n")
	fmt.Printf("  Total Operations: %d\n", len(log.ops))

	data := log.Marshal()
	fmt.Printf("Columnar Breakdown:\n")
	fmt.Printf("  Type Column Groups:  %d (RLE)\n", len(data.Types))
	fmt.Printf("  Agent Column Groups: %d (RLE)\n", len(data.Agents))
	fmt.Printf("  Position Deltas:     %d (Delta)\n", len(data.Positions))
	fmt.Printf("  Content Items:       %d\n", len(data.Content))
}
