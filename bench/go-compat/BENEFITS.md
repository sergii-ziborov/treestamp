# Informal competitor benches

These are developer benches in `bench/go-compat`. They are **not** official
B01–B14 rows. Official first-campaign `MEASURED` receipts live in
`compat/results/official-benches.json`.

## Toolchain and comparator versions

Recorded 20 September 2026 on Windows/amd64, Intel Core Ultra 7 255U.
This remasures persistable listing entries (no per-callback `Clone`),
lazy parallel workers, and listing-ready `FileInfo`. It is not a 15–25%
release claim.

| Item | Version |
| --- | --- |
| Go (`go env GOVERSION`) | go1.26.5 |
| Module `go` line | 1.23.0 (bench module; library is 1.21.0) |
| `CGO_ENABLED` | 0 |
| `GOTOOLCHAIN` | local |
| `GOEXPERIMENT` | empty |
| fastwalk | v1.0.14 |
| gocodewalker | v1.5.1 |
| godirwalk | v1.17.0 |
| `golang.org/x/sys` | v0.30.0 |
| Python runner | 3.14.7 |

Time and `B/op` below are medians of three `go test -bench -benchmem -count=3`
runs after persistable listing entries. Process RSS/CPU come from a compiled
test binary (`go test -c`) with `-test.count=1` of the full suite: peak
working set **56.1 MiB**, process CPU **129 s**. Sampling `go test` itself
undercounts the child binary. The RSS sample now includes the bushy
parallel-walk benches.

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
| Raw walk, serial callback | 365 µs, 158 KiB, 1248 allocs | fastwalk v1.0.14: 400 µs, 144 KiB, 1652 allocs; godirwalk v1.17.0: 505 µs, 208 KiB, 2031 allocs | Persistable listing entry, no per-callback `Clone` |
| Raw walk, parallel callback | 323 µs, 157 KiB, 1233 allocs | fastwalk v1.0.14: 400 µs, 144 KiB, 1652 allocs | Workers start only when more than one directory remains |
| Regex filename selection | 4.89 ms, 99 KiB, 1338 allocs | gocodewalker v1.5.1: 5.33 ms, 130 KiB, 1667 allocs | Files-first listing |
| Cached regular `Stat` | 346 µs, 164 KiB, 1252 allocs | fastwalk v1.0.14: 405 µs, 140 KiB, 1641 allocs; `os.Stat`: 23.3 ms, 445 KiB, 2860 allocs | Listing `FileInfo` on the owned entry |
| Lazy `DirScanner` | 1.71 ms, 648 KiB, 8152 allocs | godirwalk v1.17.0 `Scanner`: 2.01 ms, 564 KiB, 12008 allocs | Wide directory; Treestamp does not `Stat` each name |
| Scratch `ReadDirents` | 1.88 ms, 736 KiB, 8021 allocs | godirwalk v1.17.0 `ReadDirents`: 2.10 ms, 956 KiB, 12023 allocs | No `os.ReadDir` sort |
| `WalkFS` vs `fs.WalkDir` | same allocs as `WalkDir` (2678) | std `WalkDir` | `WalkFS` calls `fs.WalkDir`; `fstest.MapFS` only |

## What changed versus the previous gap report

- Serial Walk no longer `Clone`s every callback. Persistable entries live
  in frame chunks (or one `[]Entry` on the parallel path). Allocs dropped
  from 1638 to 1248 on serial raw walk.
- Parallel unsorted walk attaches listing `FileInfo` and starts workers
  only when more than one directory remains.
- `ReadDirentsScratch` on Windows/macOS still uses `os.Open` + `ReadDir(-1)`.
- `WalkFS` is `fs.WalkDir`. A second implementation cannot beat the
  standard walk on `MapFS` without changing the contract.

Remaining on this run: Treestamp still uses more `B/op` than fastwalk on
raw walk (158 KiB vs 144 KiB serial). Wall times are host-local (365 µs
vs 400 µs serial). Official 10k/100k/1M sizes stay `NOT_RUN`.
Linux/macOS informal medians are not in this receipt. Bushy 32×16
parallel walk is in `INFORMAL_RUN.json` only (968 µs / 1997 allocs vs
fastwalk 1.06 ms / 2513 allocs) and is not a README ranking row.

## WalkDirs G05 addendum (20 September 2026)

Matched `WalkDirs` rows live in `WALKDIRS_G05.md`. They do **not**
replace the table above. Sorted WalkDirs and sorted godirwalk v1.17.0
were the same order of magnitude on this host; owned entries used more
`B/op`. Unsorted godirwalk is not compared on Windows (`EOF`). Linux and
macOS logs are CI artifacts, not a rewritten official campaign.

## Contract benefits these benches do not measure

Treestamp still provides surfaces the walkers do not: normalized relatives,
SHA-256, descriptor v2, typed skips, cache v2, watch-plan apply, path
confinement, and same-pass visit manifests. Those stay capability rows, not
speed rows.
