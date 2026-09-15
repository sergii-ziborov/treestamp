# Architecture

Treestamp is one importable module. Implementation lives under `internal/`.
Users should not need a dozen packages.

Public import: `treestamp`. The facade converts oracle-shaped options and
reports. Work runs as a layered pipeline:

```text
walk → selection → inspect/stream → report
```

```text
treestamp            public facade (types stay in this package)
internal/path        slash-aware prefix helpers and root containment
internal/platform    file/volume identity, hidden, stdout identity
internal/walk        serial, parallel, pull, and stateful walkers
internal/walkfs      deterministic traversal over arbitrary io/fs filesystems
internal/ignore      ignore parser and sources
internal/selection   typed matchers and named types
internal/runtime     executors and bounded queues
internal/scan        discover, inspect, content visit, cache v2, watch apply
internal/report      snapshot reads, deltas, portable hashing
internal/hashx       SHA-256 prefix and content fingerprint
internal/filetypes   named type catalog
```

Optional later modules, not part of the required graph:

- `watch/` — fsnotify adapter
- `bench/` — competitor harnesses

Unused alias packages (`internal/cache`, `internal/content`,
`internal/incremental`, `internal/manifest`) are not part of the tree.

## Runtime constraints

- Minimum Go: 1.25.0
- Intended build: `CGO_ENABLED=0`
- `golang.org/x/sys` is allowed where the standard library cannot express
  volume/file identity
- SHA-256 comes from `crypto/sha256`
- No goroutine-per-file
- Do not change the host `GOMAXPROCS`
- Host executor accepts a job once or rejects it; it does not store the job
- One worker budget across several roots
- Callback-scoped `[]byte`; owned reports
- `context.Context` on scan APIs
- Pull APIs have explicit `Close`

## Scan API shape

```go
report, err := treestamp.Scan(ctx, root)
compact, err := treestamp.ScanCompact(ctx, root)
paths, err := treestamp.ScanPaths(ctx, root)

scanner, err := treestamp.NewScanner(root,
    treestamp.WithOptions(treestamp.DefaultOptions()),
    treestamp.WithTraversalWorkers(4),
    treestamp.WithContentWorkers(4),
)
```

Zero option values have documented meaning. `DefaultOptions` matches the
oracle defaults and does not invent extra limits.

## Walker (P1)

`Walker` is iterative depth-first traversal. Directory handles are bounded by
`max_open`. When the bound is reached, the oldest open directory is drained
into memory and closed. `File.ReadDir` is batched; the walker does not call
`filepath.WalkDir`.

A file root is a single yielded file, matching the Rust walker.

`WalkBuilder` adds serial multi-root, name sort, filters, contents-first, and
stdout-skip. `ParallelWalker` adds unordered visit, collect, and a bounded
pull iterator. `Walk` / `WalkUnsorted` / `ReadDirents` are the Go-market
callback surfaces. Their callback entries cache target `Stat` results and
depth. `WalkFS` and `NewFSWalker` provide lexical, no-follow traversal for
arbitrary `fs.FS` implementations without OS identity claims.

## Out of scope

Parser, search, clone, graph, embeddings, MCP, secrets, HTTP, and a resident
daemon are consumers. They must not be folded into this module.
