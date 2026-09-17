# Informal performance notes

Official B01–B14 first-campaign receipts are `MEASURED` in
`compat/results/official-benches.json`. This page is the informal Windows
listing campaign only.

## Versions used on this receipt

| Item | Version |
| --- | --- |
| Host | Windows/amd64, Intel Core Ultra 7 255U, 16 September 2026 |
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

Last Windows run after the listing fixes:

- count=3 medians: raw serial 375 µs; regex select 2.36 ms; cached Stat 197 µs
- `ReadDirentsScratch` 1.82 ms vs godirwalk 2.26 ms
- cached `Stat` 126 KiB vs fastwalk 146 KiB
- compiled binary peak RSS 54.2 MiB; process CPU 93.9 s (`-test.count=1`)
- `ScanWith` vs `Scan`: +4 allocs, about +2 KiB

## Remaining limits

- Parallel raw walk can still lose to fastwalk v1.0.14 on a ~400-file tree
  (633 µs vs 520 µs on this receipt).
- Official 10k/100k/1M sizes are not measured.
- Linux/macOS informal medians are not in this receipt.

Full table: [`bench/go-compat/BENEFITS.md`](../../bench/go-compat/BENEFITS.md).
