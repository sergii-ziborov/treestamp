# Informal competitor benches

These are developer benches in `bench/go-compat`. They are **not** official
B01–B14 rows. Official B01–B14 stay `NOT_RUN`.

Pinned comparators: fastwalk v1.0.14, gocodewalker v1.5.1, godirwalk v1.17.0.

Recorded 16 September 2026 on Windows/amd64, Intel Core Ultra 7 255U,
Go 1.26.5, `CGO_ENABLED=0`, `GOTOOLCHAIN=local`.

Time and `B/op` are medians of three `go test -bench . -benchmem -count=3`
runs (251.8 s wall). Process RSS/CPU come from a compiled test binary
(`go test -c`) with `-test.count=1`: peak working set **54.7 MiB**,
process CPU **80.3 s**. Sampling `go test` itself undercounts the child
binary; do not use the wrapper RSS.

Reproduce: `python tools/run_informal_benches.py` and
`go test -c -o compat.test.exe .` then run that exe with
`-test.bench=. -test.benchmem -test.count=1` while watching its
`WorkingSet64` / `CPU`. Receipt: `INFORMAL_RUN.json`.

The corpus is a small temp tree (about 400 wide files for raw walk, 80
groups for regex selection, 4000 names for `DirScanner`). This is not a
10k/100k/1M campaign.

## Method-matched timings

| Case | Treestamp | Competitor | Note |
| --- | --- | --- | --- |
| Raw walk, serial callback | 386 µs, 172 KiB, 1241 allocs | fastwalk 502 µs, 150 KiB, 1652 allocs; godirwalk 365 µs, 227 KiB, 2031 allocs | Same ignore-nothing listing |
| Raw walk, parallel callback | 288 µs, 174 KiB, 1252 allocs | fastwalk 502 µs, 150 KiB, 1652 allocs | Same syscall floor |
| Regex filename selection | 2.63 ms, 101 KiB, 1339 allocs | gocodewalker 3.24 ms, 137 KiB, 1667 allocs | Files-first listing |
| Cached regular `Stat` | 266 µs, 174 KiB, 1245 allocs | fastwalk 265 µs, 146 KiB, 1641 allocs; `os.Stat` 20.3 ms, 496 KiB, 2854 allocs | Callback cache, not a full scan |
| Lazy `DirScanner` | 1.55 ms, 648 KiB, 8152 allocs | godirwalk `Scanner` 2.06 ms, 564 KiB, 12008 allocs | Wide directory; Treestamp does not `Stat` each name |
| Scratch `ReadDirents` | 2.29 ms, 736 KiB, 8021 allocs | godirwalk `ReadDirents` 1.84 ms, 955 KiB, 12023 allocs | Treestamp used fewer allocs, more wall time on this run |
| `WalkFS` vs `fs.WalkDir` | 2.10 ms, 227 KiB, 2558 allocs | std `WalkDir` 1.88 ms, 228 KiB, 2678 allocs | `fstest.MapFS` only |

`Scan` vs `ScanWith` on a one-file temp tree: 358 µs / 80 KiB / 155 allocs
versus 389 µs / 82 KiB / 159 allocs. Facade overhead is a few allocations,
not a second walk.

Treestamp is in the same band as fastwalk/godirwalk on raw listing. It is
not 2× faster. The large win versus naive `os.Stat` is the cached callback
`Stat`. Gaps from this run: scratch directory listing and `WalkFS` were
slightly slower than the comparator despite fewer or similar allocations.

## Contract benefits these benches do not measure

Treestamp still provides surfaces the walkers do not: normalized relatives,
SHA-256, descriptor v2, typed skips, cache v2, watch-plan apply, path
confinement, and same-pass visit manifests. Those stay capability rows, not
speed rows.
