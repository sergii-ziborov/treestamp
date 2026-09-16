# Informal competitor benches

These are developer benches in `bench/go-compat`. They are **not** official
B01–B14 rows and must not be copied into README as published Treestamp
results. Official B01–B14 stay `NOT_RUN`.

Pinned comparators: fastwalk v1.0.14, gocodewalker v1.5.1, godirwalk v1.17.0.

Recorded 16 September 2026 on Windows/amd64, Intel Core Ultra 7 255U,
`go test -bench … -benchmem -count=3`. Values are medians of three runs.
The corpus is a small temp tree (about 400 wide files for raw walk, 80
groups for regex selection, 4000 names for `DirScanner`). This is not a
10k/100k/1M campaign.

## Method-matched timings

| Case | Treestamp | Competitor | Note |
| --- | --- | --- | --- |
| Raw walk, serial callback | 327 µs, 175 KiB, 1241 allocs | fastwalk 322 µs, 153 KiB, 1652 allocs; godirwalk 378 µs, 233 KiB, 2031 allocs | Same ignore-nothing listing |
| Raw walk, parallel callback | 282 µs, 177 KiB, 1252 allocs | fastwalk (default workers) 322 µs | Same syscall floor |
| Regex filename selection | 5.83 ms, 104 KiB, 1339 allocs | gocodewalker 6.96 ms, 154 KiB, 1746 allocs | Files-first listing like gocodewalker; ignore files load only when present in the directory listing |
| Cached regular `Stat` | 298 µs, 178 KiB, 1245 allocs | fastwalk 295 µs, 149 KiB, 1641 allocs; `os.Stat` 12.1 ms | Callback cache, not a full scan |
| Lazy `DirScanner` | 1.48 ms, 663 KiB, 8152 allocs | godirwalk `Scanner` 1.84 ms, 577 KiB, 12008 allocs | Wide directory; Treestamp does not `Stat` each name |

Treestamp is in the same band as fastwalk/godirwalk on raw listing and
cached `Stat`. It is not 2× faster. Regex `ScanPaths` now follows
gocodewalker’s files-first listing and skips idle ignore work; on this
machine it is ahead on time and allocations. The large win versus naive
`os.Stat` is the cached callback `Stat`.

## Contract benefits these benches do not measure

Treestamp still provides surfaces the walkers do not: normalized relatives,
SHA-256, descriptor v2, typed skips, cache v2, watch-plan apply, path
confinement, and same-pass visit manifests. Those stay capability rows, not
speed rows.
