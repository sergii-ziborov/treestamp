# Informal performance notes

Official B01–B14 first-campaign receipts are `MEASURED` in
`compat/results/official-benches.json`. This page is the informal Windows
listing campaign only.

## Versions used on this receipt

| Item | Version |
| --- | --- |
| Host | Windows/amd64, Intel Core Ultra 7 255U, 20 September 2026 |
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

Last Windows run after F01–F06 and Windows enumeration `FileInfo`:

- count=3 medians: raw serial 212 µs; regex select 3.25 ms; cached Stat 231 µs
- `ReadDirentsScratch` 1.30 ms vs godirwalk 1.61 ms
- cached `Stat` 156 KiB / 231 µs vs fastwalk 139 KiB / 221 µs
- compiled binary peak RSS 20.2 MiB; process CPU 89.8 s (`-test.count=1`)

## Remaining limits

- Serial raw walk is a few percent faster here and still uses more `B/op`
  than fastwalk v1.0.14 (212 µs / 153 KiB vs 240 µs / 142 KiB). Parallel
  raw walk and cached `Stat` are not faster on this run.
- Official 10k/100k/1M sizes are not measured.
- Linux/macOS informal medians are not in this receipt.

WalkDirs-matched godirwalk rows (not this table):
[`bench/go-compat/WALKDIRS_G05.md`](../../bench/go-compat/WALKDIRS_G05.md).

Full table: [`bench/go-compat/BENEFITS.md`](../../bench/go-compat/BENEFITS.md).
