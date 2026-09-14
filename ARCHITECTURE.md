# Architecture

Treestamp is one importable module. Implementation lives under `internal/`.
Users should not need a dozen packages.

```text
treestamp            public API
internal/path        slash-aware prefix helpers and later normalization
internal/platform    file/volume identity, hidden, stdout identity
internal/walk        serial walker and serial builders (P1)
internal/ignore      ignore parser and sources (P2)
internal/selection   typed matchers and named types (P2)
internal/runtime     executors and bounded queues (P4)
internal/content     verified reads and sinks (P5)
internal/manifest    reports, hashes, descriptor, revision (P3)
internal/cache       cache format 2 (P6)
internal/incremental sessions, deltas, watch plans (P6)
```

Optional later modules, not part of the required graph:

- `watch/` — fsnotify adapter
- `bench/` — competitor harnesses

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

## Scan API shape (not implemented)

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
stdout-skip. Parallel, pull, and stateful-batch APIs are later stages.

## Out of scope

Parser, search, clone, graph, embeddings, MCP, secrets, HTTP, and a resident
daemon are consumers. They must not be folded into this module.
