package crdt

import (
	"fmt"
	"sort"
)

// ColumnarData represents the opLog in a columnar format for efficient compression.
type ColumnarData[T any] struct {
	OpLengths []int
	Types     []opType
	TypeRuns  []int
	Agents    []int
	AgentRuns []int
	Seqs      []int // Start sequence for each agent run
	Positions []int // Delta-encoded positions
	Content   []T
	Parents   [][]lv
	Frontier  []lv
}

// Marshal converts the opLog into a ColumnarData structure.
func (log *opLog[T]) Marshal() *ColumnarData[T] {
	if len(log.ops) == 0 {
		return &ColumnarData[T]{
			Frontier: log.frontier,
		}
	}

	res := &ColumnarData[T]{
		OpLengths: make([]int, 0, len(log.ops)),
		Content:   make([]T, 0, len(log.ops)),
		Parents:   make([][]lv, 0, len(log.ops)),
		Frontier:  log.frontier,
	}

	var lastType opType
	var lastAgent int
	var lastSeq int
	var lastLen int
	var typeRun int
	var agentRun int
	var lastPos int

	for i, o := range log.ops {
		res.OpLengths = append(res.OpLengths, o.length)

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
			lastSeq = o.id.seq
			lastLen = o.length
			agentRun = 1
			res.Seqs = append(res.Seqs, o.id.seq)
		} else if o.id.agent == lastAgent && o.id.seq == lastSeq+lastLen {
			agentRun++
			lastSeq = o.id.seq
			lastLen = o.length
		} else {
			res.Agents = append(res.Agents, lastAgent)
			res.AgentRuns = append(res.AgentRuns, agentRun)
			lastAgent = o.id.agent
			lastSeq = o.id.seq
			lastLen = o.length
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
		res.Content = append(res.Content, o.content...)
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
func Unmarshal[T any](data *ColumnarData[T]) *opLog[T] {
	log := newOpLog[T]()
	log.frontier = data.Frontier

	if len(data.Types) == 0 {
		return log
	}

	totalOps := len(data.OpLengths)
	log.ops = make([]op[T], totalOps)
	log.opStartLVs = make([]lv, totalOps)

	// --- 1. Expand Types ---
	opIdx := 0
	for i, t := range data.Types {
		run := data.TypeRuns[i]
		for j := 0; j < run; j++ {
			log.ops[opIdx+j].opType = t
		}
		opIdx += run
	}

	// --- 2. Expand Agents & Seqs ---
	opIdx = 0
	for i, agent := range data.Agents {
		run := data.AgentRuns[i]
		currentSeq := data.Seqs[i]
		for j := 0; j < run; j++ {
			log.ops[opIdx+j].id = id{agent: agent, seq: currentSeq}
			log.ops[opIdx+j].length = data.OpLengths[opIdx+j]
			currentSeq += log.ops[opIdx+j].length
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
	contentIdx := 0
	currentLV := lv(0)
	for i := 0; i < totalOps; i++ {
		if log.ops[i].opType == opTypeIns {
			length := log.ops[i].length
			log.ops[i].content = data.Content[contentIdx : contentIdx+length]
			contentIdx += length
		}
		log.ops[i].parents = data.Parents[i]

		// Rebuild metadata
		log.opStartLVs[i] = currentLV
		currentLV += lv(log.ops[i].length)

		agent := log.ops[i].id.agent
		seq := log.ops[i].id.seq
		
		// Rebuild agentOps correctly (sorted by seq)
		idxs := log.agentOps[agent]
		if len(idxs) == 0 || seq > log.ops[idxs[len(idxs)-1]].id.seq {
			log.agentOps[agent] = append(idxs, i)
		} else {
			insertIdx := sort.Search(len(idxs), func(j int) bool {
				return log.ops[idxs[j]].id.seq >= seq
			})
			log.agentOps[agent] = append(idxs, 0)
			copy(log.agentOps[agent][insertIdx+1:], log.agentOps[agent][insertIdx:])
			log.agentOps[agent][insertIdx] = i
		}
		
		lastSeqOfRun := seq + log.ops[i].length - 1
		if currentSeq, ok := log.version[agent]; !ok || lastSeqOfRun > currentSeq {
			log.version[agent] = lastSeqOfRun
		}
	}

	return log
}

// Stats prints comparison between row-based and columnar representation.
func (log *opLog[T]) PrintCompressionStats() {
	fmt.Printf("OpLog Stats:\n")
	fmt.Printf("  Total Runs: %d\n", len(log.ops))

	data := log.Marshal()
	fmt.Printf("Columnar Breakdown:\n")
	fmt.Printf("  OpLengths Column:    %d\n", len(data.OpLengths))
	fmt.Printf("  Type Column Groups:  %d (RLE)\n", len(data.Types))
	fmt.Printf("  Agent Column Groups: %d (RLE)\n", len(data.Agents))
	fmt.Printf("  Position Deltas:     %d (Delta)\n", len(data.Positions))
	fmt.Printf("  Content Items:       %d\n", len(data.Content))
}
