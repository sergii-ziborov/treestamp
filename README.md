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

Current tags: library [`v0.1.5`](https://github.com/sergii-ziborov/treestamp/releases/tag/v0.1.5),
CLI [`cmd/treestamp/v0.1.5`](https://github.com/sergii-ziborov/treestamp/releases/tag/cmd/treestamp/v0.1.5).
`go install` uses the CLI module version (`@v0.1.5`), not the Git tag prefix.
Those tags stay immutable. `v0.1.4` is not retagged.

## Install

Requires **Go 1.21** or newer. `CGO_ENABLED=0`.

### Library

```text
go get github.com/sergii-ziborov/treestamp@v0.1.5
```

Import `github.com/sergii-ziborov/treestamp`. Docs and examples:
[pkg.go.dev/github.com/sergii-ziborov/treestamp](https://pkg.go.dev/github.com/sergii-ziborov/treestamp).
The runtime module requires `golang.org/x/sys` only. It does not require
fastwalk, godirwalk, gocodewalker, or fsnotify.

### CLI

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.5
```

Without Go, download a `treestamp-*` binary from the
[CLI v0.1.5 release](https://github.com/sergii-ziborov/treestamp/releases/tag/cmd/treestamp/v0.1.5)
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

## New in this tree

Capability rows, not a ranking over fastwalk v1.0.14.

- `Walk` / `WalkDirs` keep persistable listing entries. WalkDirs holds one
  `[]Entry`; a saved callback keeps its name.
- Parallel unsorted walk streams those entries and uses listing `FileInfo`.
  A Linux lazy dent is not `Stat`ed for that cache.
- Workers start only when more than one directory remains. An explicit
  `NumWorkers` is not clipped to the native default cap of 8.
- `ReadDirnames` calls `Readdirnames(-1)` (no `ReadDir` plus a name copy).
- `SortMode` includes `SortDirsFirst` (directory, regular file, other).
  Serial `Walk` honors `SortMode`.
- Ordered pull reserves a ready slot before listing, then adds bytes.
  `ParallelWalkIter.Err` is the constructor error from
  `IntoIterOrderedBounded`.
- `Options.IgnoreRules` and `FileWalker.CustomIgnorePatterns` ignore; they
  do not invert. `OverrideRules` is the include list. Empty `IgnoreRules`
  keep the oracle descriptor v2 hash.
- `compat/fastwalk` keeps a relative root relative. `FollowOutside` is
  always on so `ErrTraverseLink` can leave the root; `Follow` still
  decides auto-follow of every directory symlink.

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
remasurement after persistable WalkDirs, `Readdirnames`, listing `FileInfo`,
ordered ready reserve, and compat `FollowOutside`. Official B01–B14 stay
the 17 September first campaign.

Pinned sources used with those receipts:

| Pin | Value |
| --- | --- |
| weavatrix-scan | 0.5.2, commit `29c003a6ad541c9a10faf30505235375fa78b9d8` |
| fastwalk (bench only) | v1.0.14 |
| gocodewalker (bench only) | v1.5.1 |
| godirwalk (bench only) | v1.17.0 |
| Informal listing table | 20 September 2026 remasure (0.1.5 tree), [`INFORMAL_RUN.json`](bench/go-compat/INFORMAL_RUN.json) |

Do not follow live `main` of a competitor as the oracle. Do not rewrite
`official-benches.json` or `INFORMAL_RUN.json` without a new host run.

## Informal benches

Windows/amd64, Intel Core Ultra 7 255U, 20 September 2026 remasure.
Go **1.26.5** (`go env GOVERSION`), module line **1.21.0**, `CGO_ENABLED=0`,
`GOTOOLCHAIN=local`. Comparators in `bench/go-compat`:
**fastwalk v1.0.14**, **gocodewalker v1.5.1**, **godirwalk v1.17.0**.
Medians of three runs on a ~400-file temp tree. Not a 15–25% release claim.
Same-day earlier tables on this host swung hundreds of microseconds.

| Case | Treestamp | Comparator |
| --- | --- | --- |
| Raw serial walk | 571 µs, 158 KiB, 1248 allocs | fastwalk v1.0.14: 327 µs / 143 KiB; godirwalk v1.17.0: 489 µs / 208 KiB |
| Raw parallel walk | 395 µs, 153 KiB, 1637 allocs | fastwalk v1.0.14: 327 µs / 143 KiB |
| Regex `ScanPaths` | 5.60 ms, 99 KiB, 1338 allocs | gocodewalker v1.5.1: 8.18 ms / 134 KiB |
| Cached `Stat` | 424 µs, 164 KiB, 1252 allocs | fastwalk v1.0.14: 409 µs / 139 KiB; `os.Stat` 24.5 ms |
| `DirScanner` | 2.01 ms, 647 KiB, 8152 allocs | godirwalk v1.17.0: 1.86 ms / 564 KiB / 12008 allocs |
| Scratch `ReadDirents` | 2.95 ms, 736 KiB, 8021 allocs | godirwalk v1.17.0: 3.80 ms / 956 KiB / 12023 allocs |
| `ReadDirnames` | 1.93 ms, 298 KiB, 4020 allocs | godirwalk v1.17.0: 1.81 ms / 801 KiB / 8022 allocs |

`WalkFS` is `fs.WalkDir` (same 2678 allocs). Compiled test-binary peak working set
**55.0 MiB**, process CPU **145 s** on `-test.count=1`. Per-op memory is
`B/op`, not that RSS. `ReadDirnames` drops about 4000 allocs versus
godirwalk on this host because it no longer copies `ReadDir` entries.
Raw walk still uses more `B/op` than fastwalk, and this remasure’s wall
times are not a claimed speed win.

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
`compat/fastwalk`, `IgnoreRules`). It is not a
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
