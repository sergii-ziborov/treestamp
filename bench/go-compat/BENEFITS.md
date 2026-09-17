# Informal competitor benches

These are developer benches in `bench/go-compat`. They are **not** official
B01–B14 rows. Official first-campaign `MEASURED` receipts live in
`compat/results/official-benches.json`.

## Toolchain and comparator versions

Recorded 16 September 2026 on Windows/amd64, Intel Core Ultra 7 255U.
The 17 September R0 refresh did not edit these raw paths; a loaded-host
re-run is not used as a claimed ranking.

| Item | Version |
| --- | --- |
| Go (`go env GOVERSION`) | go1.26.5 |
| Module `go` line | 1.23.0 |
| `CGO_ENABLED` | 0 |
| `GOTOOLCHAIN` | local |
| `GOEXPERIMENT` | empty |
| fastwalk | v1.0.14 |
| gocodewalker | v1.5.1 |
| godirwalk | v1.17.0 |
| `golang.org/x/sys` | v0.34.0 |
| Python runner | 3.14.7 |

Time and `B/op` below are medians of three `go test -bench -benchmem -count=3`
runs after the listing fixes (unsorted `ReadDir`, lazy `Info`, `WalkFS` =
`fs.WalkDir`). Process RSS/CPU come from a compiled test binary
(`go test -c`) with `-test.count=1` of the full suite: peak working set
**54.2 MiB**, process CPU **93.9 s**. Sampling `go test` itself undercounts
the child binary.

Reproduce:

```text
set CGO_ENABLED=0
set GOTOOLCHAIN=local
set GOEXPERIMENT=
python tools/run_informal_benches.py
```

Receipt: `INFORMAL_RUN.json` (`modules` and `host.go` record the pin).
Policy: [BENCHMARKS.md](../../BENCHMARKS.md).

The corpus is a small temp tree (about 400 wide files for raw walk, 80
groups for regex selection, 4000 names for `DirScanner`). This is not a
10k/100k/1M campaign.

## Method-matched timings

| Case | Treestamp | Competitor | Note |
| --- | --- | --- | --- |
| Raw walk, serial callback | 375 µs, 123 KiB, 1230 allocs | fastwalk v1.0.14: 520 µs, 150 KiB, 1652 allocs; godirwalk v1.17.0: 609 µs, 227 KiB, 2031 allocs | Unsorted `File.ReadDir`; `Info` only if the callback asks |
| Raw walk, parallel callback | 633 µs, 125 KiB, 1242 allocs | fastwalk v1.0.14: 520 µs, 150 KiB, 1652 allocs | Still more wall time on this small tree |
| Regex filename selection | 2.36 ms, 102 KiB, 1339 allocs | gocodewalker v1.5.1: 3.01 ms, 134 KiB, 1667 allocs | Files-first listing |
| Cached regular `Stat` | 197 µs, 126 KiB, 1233 allocs | fastwalk v1.0.14: 344 µs, 146 KiB, 1641 allocs; `os.Stat`: 23.2 ms, 451 KiB, 2842 allocs | Callback cache, not a full scan |
| Lazy `DirScanner` | 1.22 ms, 648 KiB, 8152 allocs | godirwalk v1.17.0 `Scanner`: 1.46 ms, 564 KiB, 12008 allocs | Wide directory; Treestamp does not `Stat` each name |
| Scratch `ReadDirents` | 1.82 ms, 736 KiB, 8021 allocs | godirwalk v1.17.0 `ReadDirents`: 2.26 ms, 955 KiB, 12023 allocs | No `os.ReadDir` sort |
| `WalkFS` vs `fs.WalkDir` | same allocs as `WalkDir` (2678) | std `WalkDir` | `WalkFS` calls `fs.WalkDir`; `fstest.MapFS` only |

`Scan` vs `ScanWith` on a one-file temp tree: 301 µs / 80 KiB / 155 allocs
versus 424 µs / 82 KiB / 159 allocs. Facade overhead is a few allocations,
not a second walk.

## What changed versus the previous gap report

- `ReadDirentsScratch` on Windows/macOS now uses `os.Open` + `ReadDir(-1)`.
  `os.ReadDir` sorts names; godirwalk does not. That sort was the wall-time
  loss (2.29 ms vs 1.84 ms) despite fewer allocations.
- Callback walk no longer copies every listing into a second `Record` slice
  and no longer calls `Info` unless the consumer does. Cached `Stat` dropped
  from 174 KiB to 126 KiB `B/op` and is now faster than fastwalk on this
  machine.
- `WalkFS` is `fs.WalkDir`. A second implementation cannot beat the
  standard walk on `MapFS` without changing the contract.

Remaining on this run: parallel raw walk is still slower than fastwalk on
the ~400-file tree (633 µs vs 520 µs). Official 10k/100k/1M sizes stay
`NOT_RUN`. Linux/macOS informal medians are not in this receipt.

## Contract benefits these benches do not measure

Treestamp still provides surfaces the walkers do not: normalized relatives,
SHA-256, descriptor v2, typed skips, cache v2, watch-plan apply, path
confinement, and same-pass visit manifests. Those stay capability rows, not
speed rows.
