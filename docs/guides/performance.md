# Informal performance notes

Official B01–B14 first-campaign receipts are `MEASURED` in
`compat/results/official-benches.json`. This page is the informal Windows
listing campaign only.

## Versions used on this receipt

| Item | Version |
| --- | --- |
| Host | Windows/amd64, Intel Core Ultra 7 255U, 20 September 2026 remasure |
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

Last Windows run after persistable listing entries (no per-callback
`Clone`), lazy parallel workers, and listing-ready `FileInfo`:

- count=3 medians: raw serial 365 µs / 1248 allocs; regex select 4.89 ms; cached Stat 346 µs / 1252 allocs
- `ReadDirentsScratch` 1.88 ms vs godirwalk 2.10 ms
- cached `Stat` 164 KiB / 346 µs vs fastwalk 140 KiB / 405 µs
- compiled binary peak RSS 56.1 MiB; process CPU 129 s (`-test.count=1`)

## Remaining limits

- Serial raw walk still uses more `B/op` than fastwalk v1.0.14
  (365 µs / 158 KiB / 1248 allocs vs 400 µs / 144 KiB / 1652 allocs).
  Wall times are host-local. This is not a ranking claim.
- Official 10k/100k/1M sizes are not measured.
- Linux/macOS informal medians are not in this receipt.

WalkDirs-matched godirwalk rows (not this table):
[`bench/go-compat/WALKDIRS_G05.md`](../../bench/go-compat/WALKDIRS_G05.md).

Full table: [`bench/go-compat/BENEFITS.md`](../../bench/go-compat/BENEFITS.md).
