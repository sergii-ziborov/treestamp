# Informal competitor benches

These are developer benches in `bench/go-compat`. They are **not** official
B01–B14 rows. Official first-campaign `MEASURED` receipts live in
`compat/results/official-benches.json`.

## Toolchain and comparator versions

Recorded 20 September 2026 on Windows/amd64, Intel Core Ultra 7 255U.
This remasures the F01–F06 walk path. It is not a 15–25% release claim.

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
runs after the listing fixes (unsorted `ReadDir`, lazy `Info`, `WalkFS` =
`fs.WalkDir`). Process RSS/CPU come from a compiled test binary
(`go test -c`) with `-test.count=1` of the full suite: peak working set
**20.2 MiB**, process CPU **89.8 s**. Sampling `go test` itself undercounts
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
| Raw walk, serial callback | 212 µs, 153 KiB, 1638 allocs | fastwalk v1.0.14: 240 µs, 142 KiB, 1652 allocs; godirwalk v1.17.0: 382 µs, 208 KiB, 2031 allocs | Windows listings keep FindFirstFile `FileInfo` |
| Raw walk, parallel callback | 243 µs, 161 KiB, 1651 allocs | fastwalk v1.0.14: 240 µs, 142 KiB, 1652 allocs | Essentially tied wall time, more `B/op` |
| Regex filename selection | 3.25 ms, 98 KiB, 1338 allocs | gocodewalker v1.5.1: 3.46 ms, 129 KiB, 1667 allocs | Files-first listing |
| Cached regular `Stat` | 231 µs, 156 KiB, 1641 allocs | fastwalk v1.0.14: 221 µs, 139 KiB, 1641 allocs; `os.Stat`: 11.4 ms, 436 KiB, 3250 allocs | Callback cache, not a full scan |
| Lazy `DirScanner` | 1.30 ms, 648 KiB, 8152 allocs | godirwalk v1.17.0 `Scanner`: 1.45 ms, 564 KiB, 12008 allocs | Wide directory; Treestamp does not `Stat` each name |
| Scratch `ReadDirents` | 1.30 ms, 736 KiB, 8021 allocs | godirwalk v1.17.0 `ReadDirents`: 1.61 ms, 955 KiB, 12023 allocs | No `os.ReadDir` sort |
| `WalkFS` vs `fs.WalkDir` | same allocs as `WalkDir` (2678) | std `WalkDir` | `WalkFS` calls `fs.WalkDir`; `fstest.MapFS` only |

## What changed versus the previous gap report

- `ReadDirentsScratch` on Windows/macOS now uses `os.Open` + `ReadDir(-1)`.
  `os.ReadDir` sorts names; godirwalk does not.
- Serial Walk/Stat keeps FindFirstFile `FileInfo` on each listing. That
  costs `B/op` versus a lazy-`Info` path. Cached `Stat` is 231 µs / 156 KiB
  versus fastwalk 221 µs / 139 KiB on this host — not a Stat win.
- `WalkFS` is `fs.WalkDir`. A second implementation cannot beat the
  standard walk on `MapFS` without changing the contract.

Remaining on this run: Treestamp still uses more `B/op` than fastwalk on
raw walk (153 KiB vs 142 KiB serial). Parallel raw walk is essentially
tied (243 µs vs 240 µs). Official 10k/100k/1M sizes stay `NOT_RUN`.
Linux/macOS informal medians are not in this receipt.

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
