# Treestamp

Public personal repository of [Sergii Ziborov](https://github.com/sergii-ziborov).
Module path: [`github.com/sergii-ziborov/treestamp`](https://github.com/sergii-ziborov/treestamp).

This is **not** a Weavatrix organization repository and **not** an EdgeHawk
repository. Weavatrix Scan is the pinned Rust oracle being ported.

Treestamp is a native Go library for verifiable repository scanning: walk,
select, read, and produce a deterministic manifest. It is not a parser, search
engine, graph, embedder, secret scanner, MCP server, web service, or daemon.

**Status on 16 September 2026:** native walk, ignore selection, Scan family,
content visit, cache v2, incremental watch apply, Go-market walk APIs,
query-scope pruning, `Explain`, identity-keyed content reuse, a shared
admission budget, ordered-parallel directory pull, and a persistent Merkle
snapshot (`tree2:`) that is not the legacy flat revision. A Windows/NTFS
fixture and a symlink-capable Linux/overlayfs Docker fixture pass the pinned
Rust driver and equivalent Go-competitor checks, with documented platform
differences. Informal go-compat medians live in
[`bench/go-compat/BENEFITS.md`](bench/go-compat/BENEFITS.md); they are not
official B01–B14 rows. Official benches stay `NOT_RUN`.
This is not a full port.

## What exists now

| Surface | Status |
| --- | --- |
| Iterative serial `Walker`, `WalkBuilder` | Implemented |
| `WalkParallel`, `ParallelWalker` visit/collect/pull | Implemented; ordered pull lists directories in parallel and emits DFS order |
| `Walk` / `WalkDirs` / `WalkUnsorted` / `ReadDirents` / `DirScanner` / `FileWalker` | Implemented; `FileWalker` streams paths during discovery |
| Nested `.gitignore` / `.ignore` / `.weavatrixignore`, overrides | Implemented; override includes compile to `MayContainMatch` prefixes |
| `Scanner.Explain` (winning source, pattern, line) | Implemented |
| Optional `.gitmodules` path skip (`WithGitModules`, off by default) | Implemented |
| `FindRepositoryRoot`, `Filters`, FileWalker terminate/error helpers | Implemented |
| Standard skips, hidden policy, 265 named types, `WithGlobs` | Implemented |
| `Scan`, `ScanCompact`, `ScanPaths` (no hash / no `max_file_bytes`) | Implemented |
| SHA-256 (`sha256:`), legacy revision, descriptor v2 byte feed | Implemented |
| `TreeSnapshot` / `TreeRevision` (`tree2:`), not a substitute for legacy revision | Implemented |
| Identity-keyed hash reuse; shared roots/dir/metadata/content budget | Implemented |
| Cache v2, `ScanCached`, `ScanIncremental`, sessions | Implemented; session still walks retained records |
| Watch plans, typed rescan reasons, portable report, delta | Implemented; `..` confined, prefix skip/warning replace |
| `VisitChangedContent` | Implemented; visits only `WatchPlan.Changed` |
| `VisitContent` / `ScanInto` (no retained manifest) / snapshot verify | Implemented |
| Optional fsnotify module | Not implemented |
| Rust/Go-competitor functional differential | Windows/Linux/macOS CI plus local NTFS/overlayfs records; cache/watch sequences included |
| Official B01–B14 campaign | `NOT_RUN` |

Do not treat `filepath.WalkDir` usage elsewhere as this library. The serial
walker is iterative, bounds open directory handles, and is not a WalkDir
wrapper renamed as a port.

## Requirements

- Go 1.23.0 or newer
- `CGO_ENABLED=0` for the intended runtime
- Rust is optional and only for developers who run the pinned oracle and
  differential drivers

```text
go get github.com/sergii-ziborov/treestamp@latest
```

```go
package main

import (
    "fmt"
    "io"
    "log"

    "github.com/sergii-ziborov/treestamp"
)

func main() {
    walker, err := treestamp.NewWalker(".")
    if err != nil {
        log.Fatal(err)
    }
    defer walker.Close()
    for {
        entry, err := walker.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            log.Println(err)
            continue
        }
        fmt.Println(entry.Path())
    }
}
```

Runnable samples: [`examples/walk`](examples/walk) and
[`examples/scan`](examples/scan).

```text
go run ./examples/walk [root]
go run ./examples/scan [root]
```

Scan, path-only listing, and compact reports are live. `ScanPaths` does not
hash and does not apply `max_file_bytes`. Positive override includes prune
unrelated subtrees when skip evidence is not required.

```go
ctx := context.Background()
paths, err := treestamp.ScanPaths(ctx, root)
report, err := treestamp.Scan(ctx, root)
compact, err := treestamp.ScanCompact(ctx, root)

opts := treestamp.DefaultOptions()
opts.OverrideRules = []string{"services/payments/**", "libs/contracts/**"}
scanner, err := treestamp.NewScanner(root, treestamp.WithOptions(opts))
narrow, err := scanner.ScanPaths(ctx)
why, err := scanner.Explain("src/generated/model.go")
fmt.Println(why.Outcome, why.Source, why.Line, why.Pattern)

snap := treestamp.SnapshotFromFiles(report.Files)
fmt.Println(report.Revision)      // legacy sha256: of the flat manifest
fmt.Println(snap.TreeRevision())  // tree2: Merkle root; different format
_ = paths
_ = compact
_ = narrow
```

`FileWalker` sends paths on the queue while discovery still runs. Close the
queue by calling `Start`; `Terminate` cancels the walk.

```go
files := make(chan *treestamp.File, 8)
walker := treestamp.NewFileWalker(root, files)
go func() {
    if err := walker.Start(); err != nil {
        log.Println(err)
    }
}()
for file := range files {
    fmt.Println(file.Path())
}
```

`Walk` / `WalkUnsorted` / `WalkParallel` are the fastwalk/godirwalk-class
walks. Return `ErrTraverseLink`, `WalkTraverseLink` from a parallel visitor,
or call `Walker.TraverseCurrentSymlink` to select one directory symlink;
path-escape, loop, depth, and filesystem guards remain active. `ScanPaths` is
the gocodewalker-class ignore-aware listing. Cache, incremental watch apply,
and verified streaming are live.

Informal developer benches (small Windows temp trees, 16 September 2026)
are in [`bench/go-compat/BENEFITS.md`](bench/go-compat/BENEFITS.md). They
compare method-matched listing against fastwalk v1.0.14, gocodewalker v1.5.1,
and godirwalk v1.17.0. Do not treat those medians as official Treestamp
results or as a 10k/100k/1M ranking. Official B01–B14 stay `NOT_RUN`.

Every OS-walk callback entry implements `treestamp.DirEntry`: `Stat()` returns
cached target metadata and `Depth()` reports walk depth. `StatDirEntry` and
`DirEntryDepth` provide fallback helpers matching fastwalk's call pattern.

`WalkFS` and `NewFSWalker` traverse any `fs.FS` with slash paths,
deterministic lexical DFS, and standard `SkipDir` / `SkipAll` control.
Child symlinks are not followed, matching `fs.WalkDir`; this is a walk API,
not a claim that repository scanning works over virtual filesystems.

`DirScanner` yields one child at a time from a single directory. Loop
`ReadDirentsScratch` / `ReadDirnames` with `NewScratchBuffer` to reuse the
getdents scratch on Linux. This is not the repository `Scanner`.

## What “full” means later

A full port repeats the Weavatrix Scan contract in Go: serial and parallel
walkers, ignore selection, full and compact reports, path-only scan without
full-scan I/O, verified content, cache v2, incremental rescan reasons, and
honest benches. It does not wrap the Rust crate, Node, a Git executable, or
another process at runtime.

Default scan options in the oracle are not “every resource limited”:
`max_file_bytes=1_500_000`, hashing and binary detection on, complete
evidence, standard skips on, hidden skipping off, repository-local ignores
`.gitignore` / `.ignore` / `.weavatrixignore`, cache Fast, content Strict,
streaming discovery. Whole-scan limits are off. The first compatible ignore
preset keeps those three file names. `.treestampignore` is added only by an
explicit or versioned preset.

Descriptor v2 is a canonical byte feed, not a JSON hash. Cache format is 2.

Pinned oracle: weavatrix-scan **0.5.2**, commit
`29c003a6ad541c9a10faf30505235375fa78b9d8`, tree
`108c59e66c90c3b649b3ff867360a6c0e9ff7f6e`.

## Documents

- [AGENTS.md](AGENTS.md) — how to continue the port
- [ARCHITECTURE.md](ARCHITECTURE.md)
- [CONFORMANCE.md](CONFORMANCE.md)
- [COMPETITORS.md](COMPETITORS.md)
- [BENCHMARKS.md](BENCHMARKS.md)
- [THREAT_MODEL.md](THREAT_MODEL.md)
- [RELEASE.md](RELEASE.md)
- Machine-readable plan: [compat/](compat/) and [bench/](bench/)

## Check the bootstrap

```text
python3 tools/audit.py
python3 -m unittest discover -s tools -p "test_*.py"
python3 tools/run_functional_parity.py
python3 tools/audit.py --require-full
```

`--require-full` must fail until every T01–T35 contract is closed with
evidence. That failure is correct.

```text
go test ./...
cd bench/go-compat && go test ./...
```

## License

MIT. Copyright (c) 2026 Sergii Ziborov. The upstream weavatrix-scan notice of
Sergii Ziborov is preserved.
