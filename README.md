# Treestamp

[![Go Reference](https://pkg.go.dev/badge/github.com/sergii-ziborov/treestamp.svg)](https://pkg.go.dev/github.com/sergii-ziborov/treestamp)

Native Go library for walking a tree. You can then select files, hash what
you selected, explain why a path was kept or dropped, and verify the next
tree. Same scanner from a CLI when you are not writing Go.

| What | Where | Open this |
| --- | --- | --- |
| **Library** | this repository root | [pkg.go.dev/github.com/sergii-ziborov/treestamp](https://pkg.go.dev/github.com/sergii-ziborov/treestamp) |
| **CLI** | [`cmd/treestamp`](cmd/treestamp) | [cmd/treestamp/README.md](cmd/treestamp/README.md) |
| **Driver** | [`cmd/treestamp-driver`](cmd/treestamp-driver) | [cmd/treestamp-driver/README.md](cmd/treestamp-driver/README.md) — fixture protocol only, not the product |

Current tags: library [`v0.1.4`](https://github.com/sergii-ziborov/treestamp/releases/tag/v0.1.4),
CLI [`cmd/treestamp/v0.1.4`](https://github.com/sergii-ziborov/treestamp/releases/tag/cmd/treestamp/v0.1.4).
`go install` uses the CLI module version (`@v0.1.4`), not the Git tag prefix.
Those tags stay immutable. This tree is not a retag.

## Install

Requires **Go 1.21** or newer. `CGO_ENABLED=0`.

### Library

```text
go get github.com/sergii-ziborov/treestamp@v0.1.4
```

Import `github.com/sergii-ziborov/treestamp`. Docs and examples:
[pkg.go.dev/github.com/sergii-ziborov/treestamp](https://pkg.go.dev/github.com/sergii-ziborov/treestamp).
The runtime module requires `golang.org/x/sys` only. It does not require
fastwalk, godirwalk, gocodewalker, or fsnotify.

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

## Walk

```go
package main

import (
	"fmt"
	"io/fs"
	"log"

	"github.com/sergii-ziborov/treestamp"
)

func main() {
	err := treestamp.WalkWithConfig(".", treestamp.Config{NumWorkers: 4},
		func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			fmt.Println(path)
			return nil
		})
	if err != nil {
		log.Fatal(err)
	}
}
```

`Walk` is serial. `NumWorkers != 0` is the parallel opt-in. `Config.Sort`
is legacy serial global DFS; `SortMode` is local and may stay parallel.
Saved callback entries keep their names. Switch a fastwalk import to
[`compat/fastwalk`](MIGRATING.md) — worked example:
[`consumer/`](consumer). Runnable iterator: [`examples/walk`](examples/walk).

This is not a claimed speed win over charlievieth/fastwalk v1.0.14.

## Proven receipts

Official B01–B14 first-campaign rows are **MEASURED** on a
1000-file tree, `TREESTAMP_OFFICIAL=1`. Receipt:
[`compat/results/official-benches.json`](compat/results/official-benches.json)
(host Windows/AMD64, `go1.26.5`, 2026-09-17). Policy: [BENCHMARKS.md](BENCHMARKS.md).

```text
set CGO_ENABLED=0
python tools/run_official_benches.py
```

These nanoseconds are host-local. They are not a 10k/100k/1M ranking and not
Rust oracle percentages. The informal table below is a 20 September 2026
remasurement of persistable listing entries on this tree. Official B01–B14
stay the 17 September first campaign.

Pinned sources used with those receipts:

| Pin | Value |
| --- | --- |
| weavatrix-scan | 0.5.2, commit `29c003a6ad541c9a10faf30505235375fa78b9d8` |
| fastwalk (bench only) | v1.0.14 |
| gocodewalker (bench only) | v1.5.1 |
| godirwalk (bench only) | v1.17.0 |
| Informal listing table | 20 September 2026 remasure, [`INFORMAL_RUN.json`](bench/go-compat/INFORMAL_RUN.json) |

Do not follow live `main` of a competitor as the oracle. Do not rewrite
`official-benches.json` or `INFORMAL_RUN.json` without a new host run.

## Informal benches

Windows/amd64, Intel Core Ultra 7 255U, 20 September 2026 remasure.
Go **1.26.5** (`go env GOVERSION`), module line **1.21.0**, `CGO_ENABLED=0`,
`GOTOOLCHAIN=local`. Comparators in `bench/go-compat`:
**fastwalk v1.0.14**, **gocodewalker v1.5.1**, **godirwalk v1.17.0**.
Medians of three runs on a ~400-file temp tree. Not a 15–25% release claim.

| Case | Treestamp | Comparator |
| --- | --- | --- |
| Raw serial walk | 365 µs, 158 KiB, 1248 allocs | fastwalk v1.0.14: 400 µs / 144 KiB; godirwalk v1.17.0: 505 µs / 208 KiB |
| Raw parallel walk | 323 µs, 157 KiB, 1233 allocs | fastwalk v1.0.14: 400 µs / 144 KiB |
| Regex `ScanPaths` | 4.89 ms, 99 KiB, 1338 allocs | gocodewalker v1.5.1: 5.33 ms / 130 KiB |
| Cached `Stat` | 346 µs, 164 KiB, 1252 allocs | fastwalk v1.0.14: 405 µs / 140 KiB; `os.Stat` 23.3 ms |
| `DirScanner` | 1.71 ms, 648 KiB, 8152 allocs | godirwalk v1.17.0: 2.01 ms / 564 KiB / 12008 allocs |
| Scratch `ReadDirents` | 1.88 ms, 736 KiB, 8021 allocs | godirwalk v1.17.0: 2.10 ms / 956 KiB / 12023 allocs |

`WalkFS` is `fs.WalkDir` (same 2678 allocs). Compiled test-binary peak working set
**56.1 MiB**, process CPU **129 s** on `-test.count=1`. Per-op memory is
`B/op`, not that RSS. Serial and parallel listing walks drop about 390
callback allocs versus the previous informal table. Wall times are
host-local and still use more `B/op` than fastwalk. That is not a claimed
speed win.

```text
set CGO_ENABLED=0
set GOTOOLCHAIN=local
python tools/run_informal_benches.py
```

Receipt: [`bench/go-compat/INFORMAL_RUN.json`](bench/go-compat/INFORMAL_RUN.json).
WalkDirs and listing methods vs godirwalk v1.17.0 (same remasure)
are [`bench/go-compat/WALKDIRS_G05.md`](bench/go-compat/WALKDIRS_G05.md);
they do not replace the table above.

## Scan and explain

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
| Walk a tree | [docs/recipes/walk-fastwalk.md](docs/recipes/walk-fastwalk.md) |
| List paths | [docs/recipes/scan-paths.md](docs/recipes/scan-paths.md) |
| Manifest / EachFile / Explain | [docs/index.md](docs/index.md) |
| Scan an `fs.FS` | [docs/choose-an-api.md](docs/choose-an-api.md) |
| Replace godirwalk | [docs/recipes/walk-dirs.md](docs/recipes/walk-dirs.md) |
| Replace fastwalk | [MIGRATING.md](MIGRATING.md) |
| Why a file was skipped | [docs/recipes/explain.md](docs/recipes/explain.md) |
| Cache, snapshot, tree2 | [docs/guides/snapshots.md](docs/guides/snapshots.md) |
| Symptom → check | [docs/troubleshooting.md](docs/troubleshooting.md) |
| Walker migration | [MIGRATING.md](MIGRATING.md) |
| CLI install and flags | [cmd/treestamp/README.md](cmd/treestamp/README.md) |

The library is a native Go port of pinned Weavatrix Scan **0.5.2**, plus
Go-side additions (`ScanFS`, a growing-file read budget,
`MultiScanReport.Revision`, `WalkDirs` / `compat/godirwalk`,
`compat/fastwalk`). It is not a
parser, search engine, graph, embedder, secret scanner, MCP server, web
service, or daemon. Search, when ported, stays a consumer of this module.

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
