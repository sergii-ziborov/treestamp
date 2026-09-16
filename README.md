# Treestamp

Deterministic repository scanning for Go.
Select files, verify content, and produce manifests with explainable decisions.

Alpha native port of pinned Weavatrix Scan 0.5.2. This is **not a full port**.
Official B01–B14 benches stay **`NOT_RUN`**. Informal listing medians below
are a small Windows temp-tree campaign, not a 10k/100k/1M ranking.

```text
git clone https://github.com/sergii-ziborov/treestamp.git
cd treestamp
go test .
```

There is no certified published tag yet. Pin the commit you clone, not an
invented latest-stable version. Go 1.23.0+, `CGO_ENABLED=0`.

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

## Informal benches (reproducible, not official)

Windows/amd64, Intel Core Ultra 7 255U, 16 September 2026, Go 1.26.5,
`CGO_ENABLED=0`. Medians of three runs. Official B01–B14 stay **`NOT_RUN`**.

| Case | Treestamp | Comparator |
| --- | --- | --- |
| Raw serial walk | 386 µs, 172 KiB, 1241 allocs | fastwalk 502 µs / 150 KiB; godirwalk 365 µs / 227 KiB |
| Raw parallel walk | 288 µs, 174 KiB, 1252 allocs | fastwalk 502 µs / 150 KiB |
| Regex `ScanPaths` | 2.63 ms, 101 KiB, 1339 allocs | gocodewalker 3.24 ms / 137 KiB |
| Cached `Stat` | 266 µs, 174 KiB, 1245 allocs | fastwalk 265 µs / 146 KiB; `os.Stat` 20.3 ms / 496 KiB |
| `DirScanner` | 1.55 ms, 648 KiB, 8152 allocs | godirwalk 2.06 ms / 564 KiB / 12008 allocs |

Compiled test-binary peak working set **54.7 MiB**, process CPU **80.3 s**
on a one-pass `-test.count=1` of the same suite. Per-op memory is `B/op`,
not that RSS. `ScanWith` added about 4 allocs / 1.5 KiB versus `Scan` on a
one-file tree.

```text
set CGO_ENABLED=0
set GOTOOLCHAIN=local
python tools/run_informal_benches.py
cd bench/go-compat
go test -c -o compat.test.exe .
.\compat.test.exe -test.bench=. -test.benchmem -test.count=1
```

Receipt: [`bench/go-compat/INFORMAL_RUN.json`](bench/go-compat/INFORMAL_RUN.json).
Method notes: [`bench/go-compat/BENEFITS.md`](bench/go-compat/BENEFITS.md).
Policy: [BENCHMARKS.md](BENCHMARKS.md).

## What this is not

Not a parser, search engine, graph, embedder, secret scanner, MCP server,
web service, or daemon. No CGO, WASM, or Rust in the runtime library.
`.treestampignore` is not a default ignore file.

## Authorship and license

Personal public repository of [Sergii Ziborov](https://github.com/sergii-ziborov).
Module: `github.com/sergii-ziborov/treestamp`. Not a Weavatrix or EdgeHawk
organization repository. Oracle pin: weavatrix-scan **0.5.2**, commit
`29c003a6ad541c9a10faf30505235375fa78b9d8`.

MIT. Copyright (c) 2026 Sergii Ziborov.

Also: [AGENTS.md](AGENTS.md), [ARCHITECTURE.md](ARCHITECTURE.md),
[CONFORMANCE.md](CONFORMANCE.md), [THREAT_MODEL.md](THREAT_MODEL.md),
[RELEASE.md](RELEASE.md).
