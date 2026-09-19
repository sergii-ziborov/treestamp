# Treestamp

[![Go Reference](https://pkg.go.dev/badge/github.com/sergii-ziborov/treestamp.svg)](https://pkg.go.dev/github.com/sergii-ziborov/treestamp)

Native Go library for walking a tree, selecting files, hashing what you
selected, explaining why a path was kept or dropped, and verifying the
next tree. Same scanner from a CLI when you are not writing Go.

| What | Where | Open this |
| --- | --- | --- |
| **Library** | this repository root | [pkg.go.dev/github.com/sergii-ziborov/treestamp](https://pkg.go.dev/github.com/sergii-ziborov/treestamp) |
| **CLI** | [`cmd/treestamp`](cmd/treestamp) | [cmd/treestamp/README.md](cmd/treestamp/README.md) |
| **Driver** | [`cmd/treestamp-driver`](cmd/treestamp-driver) | [cmd/treestamp-driver/README.md](cmd/treestamp-driver/README.md) — fixture protocol only, not the product |

Current tags: library [`v0.1.4`](https://github.com/sergii-ziborov/treestamp/releases/tag/v0.1.4),
CLI [`cmd/treestamp/v0.1.4`](https://github.com/sergii-ziborov/treestamp/releases/tag/cmd/treestamp/v0.1.4).
`go install` uses the CLI module version (`@v0.1.4`), not the Git tag prefix.

## Install

Requires **Go 1.21** or newer. `CGO_ENABLED=0`.

### Library

```text
go get github.com/sergii-ziborov/treestamp@v0.1.4
```

Import `github.com/sergii-ziborov/treestamp`. Docs and examples:
[pkg.go.dev/github.com/sergii-ziborov/treestamp](https://pkg.go.dev/github.com/sergii-ziborov/treestamp).

### CLI

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.4
```

Without Go, download a `treestamp-*` binary from the
[CLI v0.1.4 release](https://github.com/sergii-ziborov/treestamp/releases/tag/cmd/treestamp/v0.1.4)
and check `SHA256SUMS.txt`. That release is the user command `treestamp`.
It is not `treestamp-driver`.

```text
treestamp
treestamp scan --help
```

A bare `treestamp` prints help. It does not hash the current directory.

### Not the CLI

`cmd/treestamp-driver` speaks a JSON fixture protocol for parity tests.
Do not `go install` it. Do not ship it as Treestamp.

## Library example

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
API pick: [docs/choose-an-api.md](docs/choose-an-api.md).

`Scan(ctx, root)` and `ScanPaths(ctx, root)` keep two-argument signatures.
`Options{}` is not `DefaultOptions()`. `Explain` is selection only.
`TreeSnapshot` is an in-memory persistent structure, not a disk index.
`ContentProvider.Open` returns loaded bytes, not an `io.Reader`.

## CLI example

Same scanner as the library. Use it from CI or a shell. Do not use it as
`find`, a watcher, or a hashdeep replacement.

```text
# Pin the CLI package, not the whole checkout
treestamp scan ./cmd/treestamp --ext go --json --output ./baselines/cli.tstamp.json
treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp

# Why generated code is absent from that baseline
treestamp explain testdata/generated/model.go --root .

# Names only; paths does not apply binary or size checks
treestamp paths . --ext go --null | xargs -0 gofmt -l
```

`--output` cannot sit inside the scan root. `verify` re-applies the saved
policy; it does not take the root from the file.

```text
treestamp scan ./examples/docquickstart --ext go
```

![treestamp scan](docs/cli/scan.svg)

More commands: [cmd/treestamp/README.md](cmd/treestamp/README.md).
Guide: [docs/guides/cli.md](docs/guides/cli.md).

## Tasks

| Task | Start here |
| --- | --- |
| Choose an API | [docs/choose-an-api.md](docs/choose-an-api.md) |
| List paths | [docs/recipes/scan-paths.md](docs/recipes/scan-paths.md) |
| Manifest / EachFile / Explain | [docs/index.md](docs/index.md) |
| Scan an `fs.FS` | [docs/choose-an-api.md](docs/choose-an-api.md) |
| Replace godirwalk | [docs/recipes/walk-dirs.md](docs/recipes/walk-dirs.md) |
| Why a file was skipped | [docs/recipes/explain.md](docs/recipes/explain.md) |
| Cache, snapshot, tree2 | [docs/guides/snapshots.md](docs/guides/snapshots.md) |
| Symptom → check | [docs/troubleshooting.md](docs/troubleshooting.md) |
| Walker migration | [MIGRATING.md](MIGRATING.md) |
| CLI install and flags | [cmd/treestamp/README.md](cmd/treestamp/README.md) |

The library is a native Go port of pinned Weavatrix Scan **0.5.2**, plus
Go-side additions (`ScanFS`, a growing-file read budget,
`MultiScanReport.Revision`, `WalkDirs` / `compat/godirwalk`). It is not a
parser, search engine, graph, embedder, secret scanner, MCP server, web
service, or daemon. Search, when ported, stays a consumer of this module.

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
Go **1.26.5** (`go env GOVERSION`), module line **1.21.0**, `CGO_ENABLED=0`,
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
WalkDirs and listing methods vs godirwalk v1.17.0 (19 September 2026)
are [`bench/go-compat/WALKDIRS_G05.md`](bench/go-compat/WALKDIRS_G05.md);
they do not replace the table above.

## Authorship and license

Personal public repository of [Sergii Ziborov](https://github.com/sergii-ziborov).
Module: [`github.com/sergii-ziborov/treestamp`](https://pkg.go.dev/github.com/sergii-ziborov/treestamp).
Not a Weavatrix or EdgeHawk organization repository. Oracle pin:
weavatrix-scan **0.5.2**, commit `29c003a6ad541c9a10faf30505235375fa78b9d8`.

`.treestampignore` is not a default ignore file.
Optional `watch/` uses fsnotify and is never a main-module require.

MIT. Copyright (c) 2026 Sergii Ziborov.

Also: [AGENTS.md](AGENTS.md), [ARCHITECTURE.md](ARCHITECTURE.md),
[CONFORMANCE.md](CONFORMANCE.md), [THREAT_MODEL.md](THREAT_MODEL.md),
[RELEASE.md](RELEASE.md).
