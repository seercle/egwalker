# Project Overview

`egwalker` is a Go-based project providing high-performance data structures and a generic CRDT (Conflict-free Replicated Data Type) framework. It includes implementations for a positional B+Tree (`bxtree`) and a pairing heap (`pheap`), which are used to build efficient, syncable document types.

## Main Technologies

- **Go (1.26.6)**: Core implementation language, utilizing generics for flexibility.
- **Nix**: Development environment management via a pinned `flake.nix` (nixpkgs `nixos-26.05`, locked in `flake.lock`).
- **Python (3.13.15)**: Used for visualization and trace analysis (`scripts/plot-trace.py`).
- **Data Structures**:
    - **`bxtree`**: A high-performance, positional B+Tree with customizable summaries, used for O(log N) indexed operations and metadata tracking.
    - **`pheap`**: A fast, generic pairing heap implementation.
- **CRDT Framework**: Supports `RuneDocument` (text), `ArrayDocument`, and `MapDocument` with recursive merging capabilities for nested structures.

## Project Structure

```text
/
├── go/                     # Go project root (module 'egwalker')
│   ├── bxtree/             # Positional B+Tree implementation
│   │   ├── bxtree.go       # Core B+Tree logic (Insert, Delete, Search)
│   │   ├── types.go        # B+Tree node, summary, and summarizer types
│   │   ├── bxtree_test.go  # B+Tree unit tests
│   │   ├── fuzz_test.go    # Native fuzz target (FuzzBxTree)
│   │   └── bench_test.go   # B+Tree benchmarks
│   ├── pheap/              # Pairing heap implementation
│   │   ├── pheap.go        # Pairing heap operations (Push, Pop, Peek)
│   │   └── types.go        # Pairing heap node and structure types
│   ├── crdt/               # CRDT framework and document types
│   │   ├── crdt.go         # Core CRDT algorithms (integrate, checkout, merge)
│   │   ├── document.go     # High-level Document, RuneDocument, MapDocument APIs
│   │   ├── op_log.go       # Causal operation log and frontier management
│   │   ├── types.go        # CRDT internal types (item, op, lv)
│   │   └── fuzz_test.go    # Native fuzz targets (FuzzDocumentOps, FuzzMergeConvergence, FuzzMapDocument, FuzzArrayDocument)
│   ├── main.go             # Usage examples and interactive demo
│   └── go.mod              # Go module definition
├── resources/              # Research papers and trace data
│   ├── 2409.14252v1.pdf    # "Eg-walker" research paper
│   └── editing-trace.json  # Real-world editing trace for benchmarking
├── scripts/                # Support scripts
│   └── plot-trace.py       # Performance visualization script
├── flake.nix               # Nix development environment (pinned nixpkgs)
└── flake.lock              # Locks flake inputs to exact revisions
```

## Building and Running

Since the Go module root is within the `go/` directory, all `go` commands should be executed from there or using the `-C go` flag.

### Core Commands

- **Build**: `go build -C go ./...`
- **Run Example**: `go run -C go main.go`
- **Test All**: `go test -C go ./...`
- **Run Fuzzers**: native Go fuzz targets run their seed corpora under `go test`; deep search uses `-fuzz`, e.g. `go test -C go ./crdt -fuzz=FuzzMergeConvergence -fuzztime=30s` (also `FuzzDocumentOps`, `FuzzMapDocument`, `FuzzArrayDocument`, `go test -C go ./pheap -fuzz=FuzzHeap`, `go test -C go ./bxtree -fuzz=FuzzBxTree`)
- **Visualize Traces**: `python scripts/plot-trace.py` (requires dependencies from `flake.nix`)

## Implementation Status & Comparison with Research Paper

This implementation is based on the research paper **"Collaborative Text Editing with Eg-walker: Better, Faster, Smaller"** (2409.14252v1.pdf).

### Implemented Optimizations

- **Positional B+Tree (Section 3.4):** The core of the document state is a B+Tree that uses custom **Summaries** to track character counts across the tree. This allows for $O(\log N)$ translation between document indexes and internal records.
- **Secondary Range-based Index (Section 3.4):** Replaced the standard $O(N)$ hash map with a sorted index (`sortedItems`) that maps Logical Versions to `crdtItem` segments in $O(\log S)$ time and $O(S)$ space.
- **Columnar Oplog Serialization (Section 3.8):** The `opLog` is marshaled using a columnar format that applies **Run-Length Encoding (RLE)** to agent IDs and operation types, and **Delta-encoding** to document positions.
- **Run-Length-Encoded Ops:** consecutive same-agent edits collapse into single run ops in the `opLog` (insert content stored as a run; delete runs carry a length), reducing log size and merge work.
- **Advanced "Fancy" Checkout (Section 3.2):** The `checkoutFancy` function implements the core walker logic. It identifies the common ancestor between the current state and the target version, avoiding a full replay of the history.
- **Placeholders (Section 3.6):** History prior to the common ancestor is represented as a single `crdtItem` with a `length` property, rather than individual nodes, drastically reducing merge overhead for large histories.
- **Merged Items (Section 3.3):** Consecutive operations from the same agent are automatically merged into a single `crdtItem` in the `BxTree`, significantly reducing the number of nodes for typical editing patterns.
- **Recursive Merging:** An extension beyond the paper, `MapDocument` supports recursive merging for values that implement the `Mergeable` interface, enabling nested CRDT structures.
- **Topological Sort Heuristic (Section 3.2):** The `findOpsToVisit` function uses a DFS traversal that prioritizes branches with **fewer events** to minimize the number of `retreat` and `advance` calls during a merge.
- **Batch Merging:** `MergeFrom` performs a batch merge into the `opLog` before executing a single `checkoutFancy`, significantly reducing state switching distance.
- **Run-Based Content Tree (Shape B):** The visible-content snapshot is a tree of content runs (a rope) indexed by character count, instead of one node per character. Contiguous inserts cost ~1 leaf and append-heavy editing stays bounded by the leaf-folding cap, and delete seams are re-coalesced so scattered edits do not fragment leaves, so memory and per-edit cost scale with edit runs rather than characters; allocation-free `runeText` splits (zero-copy byte-offset `SplitAt`) and a direct-build single-leaf delete keep per-edit cost near pre-rope levels while leaves stay bounded.

### Missing Optimizations (Future Work)

- **Critical Version Detection & Truncation (Section 3.5):** The paper proposes identifying "critical versions" to discard old history. Currently, the `opLog` grows indefinitely. Implementing this would allow the system to truncate the log and significantly reduce memory usage for long-running sessions.
- **Binary Compression (Section 3.8):** The columnar format is currently raw Go types. Adding **Varint encoding** for integers and **LZ4/Zstd compression** for content would bring the storage size down to the "orders of magnitude smaller" claims of the paper.

## Development Conventions

- **Generics**: All data structures and document types are generic where possible.
- **Testing**: The project emphasizes robustness through extensive unit tests and native Go fuzz targets (`fuzz_test.go` files: `FuzzBxTree`, `FuzzHeap`, `FuzzDocumentOps`, `FuzzMergeConvergence`, `FuzzMapDocument`, `FuzzArrayDocument`).
- **CRDT Logic**: The `opLog` maintains causality and history, while `bxtree` provides efficient snapshots of the document state.
- **Nested Documents**: `MapDocument` supports recursive merging if its values implement the `Mergeable` interface.
- **Environment**: Use `nix develop` or `direnv` to ensure all dependencies (Go, Python packages, Graphviz) are available.
