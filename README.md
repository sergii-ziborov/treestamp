# Treestamp

Deterministic repository scanning for Go.
Select files, hash what you chose, explain the rule, and verify the next tree.

Native Go library. Pinned Weavatrix Scan **0.5.2** is the port oracle.
T01–T35 are implemented. Official B01–B14 first-campaign rows are
**`MEASURED`** on a 1000-file tree
([receipt](compat/results/official-benches.json)). That is not a 10k/100k/1M
ranking. Informal 16 September Windows listing medians stay below.

```text
go get github.com/sergii-ziborov/treestamp@v0.1.0
```

Library tag: `v0.1.0`. CLI tag: `cmd/treestamp/v0.1.1`.
Go 1.23.0+, `CGO_ENABLED=0`.

```go
ctx := context.Background()
report, err := treestamp.ScanWith(ctx, root, treestamp.WithExtensions("go"))
if err != nil {
    return err
}
fmt.Println(report.Summary())
why, err := treestamp.Explain(root, "generated/model.go")
if err != nil {
    return err
}
fmt.Println(why.Outcome, why.Source, why.Line, why.Pattern)
```

Check `err` before using `report`. A nil error means selected work finished
under the chosen policy. Full program:
[`examples/docquickstart`](examples/docquickstart).

| Task | Start here |
| --- | --- |
| Choose an API | [docs/choose-an-api.md](docs/choose-an-api.md) |
| List paths | [docs/recipes/scan-paths.md](docs/recipes/scan-paths.md) |
| Manifest / EachFile / Explain | [docs/index.md](docs/index.md) |
| Why a file was skipped | [docs/recipes/explain.md](docs/recipes/explain.md) |
| Cache, snapshot, tree2 | [docs/guides/snapshots.md](docs/guides/snapshots.md) |
| Symptom → check | [docs/troubleshooting.md](docs/troubleshooting.md) |
| Walker migration | [MIGRATING.md](MIGRATING.md) |

`Scan(ctx, root)` and `ScanPaths(ctx, root)` keep two-argument signatures.
`Options{}` is not `DefaultOptions()`. `Explain` is selection only.
`TreeSnapshot` is an in-memory persistent structure, not a disk index.
`ContentProvider.Open` returns loaded bytes, not an `io.Reader`.

## Command line

The same scanner, as a nested module. Not a second engine.

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.1
treestamp scan . --ext go --json --output ../baseline.tstamp.json
treestamp explain generated/model.go --root .
treestamp verify ../baseline.tstamp.json --root .
```

![treestamp scan](docs/cli/scan.svg)
![treestamp explain](docs/cli/explain.svg)
![treestamp verify](docs/cli/verify.svg)

CLI README: [cmd/treestamp/README.md](cmd/treestamp/README.md).
Guide: [docs/guides/cli.md](docs/guides/cli.md).

## Official first campaign

1000-file tree, one process, `TREESTAMP_OFFICIAL=1`. Receipt:
[`compat/results/official-benches.json`](compat/results/official-benches.json).
Policy: [BENCHMARKS.md](BENCHMARKS.md). Reproduce:

```text
set CGO_ENABLED=0
python tools/run_official_benches.py
```

These nanoseconds are host-local. Do not publish them as a cross-machine
ranking or as Rust oracle percentages.

## Informal benches (reproducible, not official)

Windows/amd64, Intel Core Ultra 7 255U, 16 September 2026.
Go **1.26.5** (`go env GOVERSION`), module line **1.23.0**, `CGO_ENABLED=0`,
`GOTOOLCHAIN=local`. Comparators pinned in `bench/go-compat`:
**fastwalk v1.0.14**, **gocodewalker v1.5.1**, **godirwalk v1.17.0**.
Medians of three runs. A 17 September refresh ran on a loaded host and is
not used as a claimed ranking.

| Case | Treestamp | Comparator |
| --- | --- | --- |
| Raw serial walk | 375 µs, 123 KiB, 1230 allocs | fastwalk v1.0.14: 520 µs / 150 KiB; godirwalk v1.17.0: 609 µs / 227 KiB |
| Raw parallel walk | 633 µs, 125 KiB, 1242 allocs | fastwalk v1.0.14: 520 µs / 150 KiB |
| Regex `ScanPaths` | 2.36 ms, 102 KiB, 1339 allocs | gocodewalker v1.5.1: 3.01 ms / 134 KiB |
| Cached `Stat` | 197 µs, 126 KiB, 1233 allocs | fastwalk v1.0.14: 344 µs / 146 KiB; `os.Stat` 23.2 ms |
| `DirScanner` | 1.22 ms, 648 KiB, 8152 allocs | godirwalk v1.17.0: 1.46 ms / 564 KiB / 12008 allocs |
| Scratch `ReadDirents` | 1.82 ms, 736 KiB, 8021 allocs | godirwalk v1.17.0: 2.26 ms / 955 KiB / 12023 allocs |

`WalkFS` is `fs.WalkDir` (same allocs). Compiled test-binary peak working set
**54.2 MiB**, process CPU **93.9 s** on `-test.count=1`. Per-op memory is
`B/op`, not that RSS. Parallel raw walk is still slower than fastwalk on this
small tree.

```text
set CGO_ENABLED=0
set GOTOOLCHAIN=local
python tools/run_informal_benches.py
```

Receipt: [`bench/go-compat/INFORMAL_RUN.json`](bench/go-compat/INFORMAL_RUN.json).
Method notes: [`bench/go-compat/BENEFITS.md`](bench/go-compat/BENEFITS.md).

## What this is not

Not a parser, search engine, graph, embedder, secret scanner, MCP server,
web service, or daemon. No CGO, WASM, or Rust in the runtime library.
`.treestampignore` is not a default ignore file.
Optional `watch/` uses fsnotify and is never a main-module require.

## Authorship and license

Personal public repository of [Sergii Ziborov](https://github.com/sergii-ziborov).
Module: `github.com/sergii-ziborov/treestamp`. Not a Weavatrix or EdgeHawk
organization repository. Oracle pin: weavatrix-scan **0.5.2**, commit
`29c003a6ad541c9a10faf30505235375fa78b9d8`.

MIT. Copyright (c) 2026 Sergii Ziborov.

Also: [AGENTS.md](AGENTS.md), [ARCHITECTURE.md](ARCHITECTURE.md),
[CONFORMANCE.md](CONFORMANCE.md), [THREAT_MODEL.md](THREAT_MODEL.md),
[RELEASE.md](RELEASE.md).
