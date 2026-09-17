# Treestamp

Native Go library for deterministic repository scanning.

It selects files under ignore and filter rules, hashes what it selected,
explains why a path was kept or dropped, and verifies the next tree.

Docs: [pkg.go.dev/github.com/sergii-ziborov/treestamp](https://pkg.go.dev/github.com/sergii-ziborov/treestamp)

```text
go get github.com/sergii-ziborov/treestamp@v0.1.2
```

Requires **Go 1.23.2** or newer. `CGO_ENABLED=0`. The CLI is a nested module:

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.2
```

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/sergii-ziborov/treestamp"
)

func main() {
	ctx := context.Background()
	report, err := treestamp.ScanWith(ctx, ".", treestamp.WithExtensions("go"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(report.Summary())
	why, err := treestamp.Explain(".", "generated/model.go")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(why.Outcome, why.Source, why.Line, why.Pattern)
}
```

A nil error means selected work finished under the chosen policy.
Runnable copy: [`examples/docquickstart`](examples/docquickstart).

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

The library is a native Go port of pinned Weavatrix Scan **0.5.2**. It is not
a parser, search engine, graph, embedder, secret scanner, MCP server, web
service, or daemon.

## Command line

The same scanner. Not a second engine.

```text
treestamp scan . --ext go --json --output ../baseline.tstamp.json
treestamp explain generated/model.go --root .
treestamp verify ../baseline.tstamp.json --root .
```

![treestamp scan](docs/cli/scan.svg)
![treestamp explain](docs/cli/explain.svg)
![treestamp verify](docs/cli/verify.svg)

CLI README: [cmd/treestamp/README.md](cmd/treestamp/README.md).
Guide: [docs/guides/cli.md](docs/guides/cli.md).
CLI docs: [pkg.go.dev/github.com/sergii-ziborov/treestamp/cmd/treestamp](https://pkg.go.dev/github.com/sergii-ziborov/treestamp/cmd/treestamp).

## Official first campaign

Official B01–B14 first-campaign rows are **MEASURED** on a
1000-file tree, `TREESTAMP_OFFICIAL=1`. Receipt:
[`compat/results/official-benches.json`](compat/results/official-benches.json).
Policy: [BENCHMARKS.md](BENCHMARKS.md).

```text
set CGO_ENABLED=0
python tools/run_official_benches.py
```

These nanoseconds are host-local. They are not a 10k/100k/1M ranking and not
Rust oracle percentages.

## Informal benches

Windows/amd64, Intel Core Ultra 7 255U, 16 September 2026.
Go **1.26.5** (`go env GOVERSION`), module line **1.23.2**, `CGO_ENABLED=0`,
`GOTOOLCHAIN=local`. Comparators in `bench/go-compat`:
**fastwalk v1.0.14**, **gocodewalker v1.5.1**, **godirwalk v1.17.0**.
Medians of three runs.

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

## Authorship and license

Personal public repository of [Sergii Ziborov](https://github.com/sergii-ziborov).
Module: `github.com/sergii-ziborov/treestamp`. Not a Weavatrix or EdgeHawk
organization repository. Oracle pin: weavatrix-scan **0.5.2**, commit
`29c003a6ad541c9a10faf30505235375fa78b9d8`.

`.treestampignore` is not a default ignore file.
Optional `watch/` uses fsnotify and is never a main-module require.

MIT. Copyright (c) 2026 Sergii Ziborov.

Also: [AGENTS.md](AGENTS.md), [ARCHITECTURE.md](ARCHITECTURE.md),
[CONFORMANCE.md](CONFORMANCE.md), [THREAT_MODEL.md](THREAT_MODEL.md),
[RELEASE.md](RELEASE.md).
