# Informal performance notes

Official B01–B14 first-campaign receipts are `MEASURED` in
`compat/results/official-benches.json`. This page is the informal Windows
listing campaign only.

## Versions used on this receipt

| Item | Version |
| --- | --- |
| Host | Windows/amd64, Intel Core Ultra 7 255U, 20 September 2026 remasure (0.1.5) |
| Go | 1.26.5 (`GOTOOLCHAIN=local`, `CGO_ENABLED=0`) |
| Module line | 1.21.0 |
| fastwalk | v1.0.14 |
| gocodewalker | v1.5.1 |
| godirwalk | v1.17.0 |

## Reproduce

```text
set CGO_ENABLED=0
set GOTOOLCHAIN=local
python tools/run_informal_benches.py
```

On Windows, sample the compiled `compat.test.exe` working set and CPU, not
the `go test` wrapper. JSON receipt: `bench/go-compat/INFORMAL_RUN.json`.

Last Windows run after persistable WalkDirs, `Readdirnames`, listing
`FileInfo`, ordered ready reserve, and compat `FollowOutside`:

- count=3 medians: raw serial 571 µs / 1248 allocs; regex select 5.60 ms; cached Stat 424 µs / 1252 allocs
- `ReadDirnames` 1.93 ms / 4020 allocs vs godirwalk 1.81 ms / 8022 allocs
- `ReadDirentsScratch` 2.95 ms vs godirwalk 3.80 ms
- cached `Stat` 164 KiB / 424 µs vs fastwalk 139 KiB / 409 µs
- compiled binary peak RSS 55.0 MiB; process CPU 145 s (`-test.count=1`)

## Remaining limits

- Serial raw walk still uses more `B/op` than fastwalk v1.0.14
  (571 µs / 158 KiB / 1248 allocs vs 327 µs / 143 KiB / 1652 allocs).
  Wall times are host-local. This is not a ranking claim.
- Official 10k/100k/1M sizes are not measured.
- Linux/macOS informal medians are not in this receipt.

WalkDirs-matched godirwalk rows (not this table):
[`bench/go-compat/WALKDIRS_G05.md`](../../bench/go-compat/WALKDIRS_G05.md).

Full table: [`bench/go-compat/BENEFITS.md`](../../bench/go-compat/BENEFITS.md).
