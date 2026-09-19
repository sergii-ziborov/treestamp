# Architecture

Treestamp is one Git repository and two Go modules. Implementation of the
scanner lives under `internal/`. Users should not need a dozen packages.

```text
github.com/sergii-ziborov/treestamp              library
github.com/sergii-ziborov/treestamp/cmd/treestamp  CLI
```

The library does not import the CLI. The CLI imports only the public facade.
`cmd/treestamp-driver` stays a fixture protocol driver.

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
internal/runtime     executors, admission, and a shared resource budget
internal/selection   typed matchers, named types, and compiled query scope
internal/scan        discover, inspect, content visit, cache v2, watch apply
internal/merkle      persistent keyed TreeRevision (not legacy flat digest)
internal/report      snapshot reads, deltas, portable hashing
internal/hashx       SHA-256 prefix and content fingerprint
internal/filetypes   named type catalog
```

CLI process, not part of the library graph:

```text
cmd/treestamp
  arguments / config / signals
  render (human, JSON, NUL)
  store (manifest v1, atomic --output)
        ↓ public API
  ScanWith / ScanPathsWith / Explain / DeltaBetween
        ↓
walk → selection → inspect/stream → report
```

Optional later modules, not part of the required graph:

- `watch/` — fsnotify adapter
- `bench/` — competitor harnesses

Unused alias packages (`internal/cache`, `internal/content`,
`internal/incremental`, `internal/manifest`) are not part of the tree.

## Runtime constraints

- Minimum Go: 1.21.0
- Intended build: `CGO_ENABLED=0`
- `golang.org/x/sys` is allowed where the standard library cannot express
  volume/file identity
- SHA-256 comes from `crypto/sha256`
- No goroutine-per-file
- Do not change the host `GOMAXPROCS`
- Host executor accepts a job once or rejects it; it does not store the job
- `Runtime.Run` and `Group.Go` admit and wait; they do not silently caller-run
  unless `OverflowCallerRuns` is set
- `WithAdmitTimeout` bounds how long a job waits for a worker slot
- One worker budget across several roots
- `Budget` also bounds ready results by count and bytes
- `TreeRevision` (`tree2:`) is not interchangeable with legacy `sha256:` revision
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
pull iterator. `Walk` / `WalkUnsorted` / `WalkDirs` / `ReadDirents` / `DirScanner` are the
Go-market callback surfaces. `WalkDirs` is the godirwalk-shaped walk:
lexical DFS unless `Unsorted`, owned persistable entries, `SkipThis`.
`compat/godirwalk` is an import alias on that engine. Their callback
entries cache target `Stat` results and depth. `WalkFS` and `NewFSWalker`
provide lexical, no-follow traversal for arbitrary `fs.FS` implementations
without OS identity claims.
`ScanFS` / `EachFileFS` / `ScanPathsFS` apply the same ignore and hash
engine on that walk. Content reads stop at the discovered size.
`DirScanner` and `ReadDirentsScratch` reuse an optional getdents buffer.

## Out of scope

Parser, search, clone, graph, embeddings, MCP, secrets, HTTP, and a resident
daemon are consumers. They must not be folded into this module.
