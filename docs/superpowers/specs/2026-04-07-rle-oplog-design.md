# Design: Run-Length Encoded (RLE) Operations in OpLog

## Goal
Implement Run-Length Encoding (RLE) for operations in the `opLog` to reduce memory overhead and improve performance for large documents. This moves the implementation from $O(N)$ (characters) toward $O(S)$ (segments), as described in the "Eg-walker" research paper.

## Core Data Structure Changes

### 1. Virtual Logical Version (LV)
`lv` will now represent a **global character/operation index** in the document history.
If `ops[0]` has `length: 5`, it covers LVs `0` to `4`. `ops[1]` then starts at LV `5`.

### 2. Updated Types (`go/crdt/types.go`)

```go
type lv int

type id struct {
    agent int
    seq   int // Starting sequence number for this run
}

type op[T any] struct {
    opType  opType
    content []T   // Slice of items for insertions (empty for deletions)
    length  int   // Total number of items in this run
    pos     int   // Starting document position for local ops
    id      id    // ID of the FIRST item in the run
    parents []lv  // LVs that the START of this run directly depends on
}

type opLog[T any] struct {
    ops        []op[T]
    opStartLVs []lv            // Starting LV for each op in 'ops' (sorted)
    agentOps   map[int][]int   // Maps agent ID -> list of indexes into 'ops', sorted by seq
    frontier   []lv            // Current maxima (ends of runs)
    version    remoteVersion   // Max seq seen per agent
}
```

## Key Algorithms

### 1. LV Resolution (`getOpByLV`)
To find which `op` contains a given `lv`:
- Use `sort.Search` on `opStartLVs` to find the largest index `i` such that `opStartLVs[i] <= v`.
- Offset within the `op` is `v - opStartLVs[i]`.

### 2. ID Resolution (`resolveID`)
To find the `lv` for any `(agent, seq)`:
1. Look up the `op` indexes for the agent in `agentOps`.
2. Binary search for the `op` index where `ops[opIdx].id.seq <= target.seq`.
3. Calculate the LV: `opStartLVs[opIdx] + lv(target.seq - ops[opIdx].id.seq)`.

### 3. Local Operations
- `localInsert` will push a single `op` with `length = len(content)` and `content = content`.
- `localDelete` will push a single `op` with `length = delLen` and `content = nil`.
- This reduces the number of operations in the log by orders of magnitude for typical editing.

### 4. BxTree Integration
- `apply` will create a `crdtItem` with the full `length` of the RLE `op`.
- `integrate` will handle these multi-length items.
- `ensureAtomized(targetLV)` and `split(idx, offset)` will handle breaking `crdtItem` segments when concurrent operations or deletions target the middle of a run.

## Serialization
Update `go/crdt/serialization.go` to handle the new `op` structure. The columnar format will naturally become even more efficient as it already implements its own RLE logic, which will now have fewer "rows" to process.

## Testing & Verification
- **Unit Tests**: Update existing tests in `crdt_unit_test.go`.
- **Fuzzer Tests**: `TestFuzzerMerge` and `TestFuzzerSlice` are the primary verification tools. They must pass with high iteration counts (10,000+).
- **Performance**: Benchmark `opLog` memory usage and merge speed on the `editing-trace.json` dataset.
