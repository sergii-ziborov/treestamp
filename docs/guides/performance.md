# Informal performance notes

Official B01–B14 remain `NOT_RUN`. Do not treat this page as that campaign.

## Reproduce

```text
set CGO_ENABLED=0
set GOTOOLCHAIN=local
python tools/run_informal_benches.py
cd bench/go-compat
go test -c -o compat.test.exe .
.\compat.test.exe -test.bench=. -test.benchmem -test.count=1
```

On Windows, sample the compiled `compat.test.exe` working set and CPU, not
the `go test` wrapper. JSON receipt: `bench/go-compat/INFORMAL_RUN.json`.

Last committed Windows run (16 September 2026, Intel Core Ultra 7 255U,
Go 1.26.5):

- count=3 medians: raw serial 386 µs; regex select 2.63 ms; cached Stat 266 µs
- compiled binary peak RSS 54.7 MiB; process CPU 80.3 s (`-test.count=1`)
- `ScanWith` vs `Scan`: +4 allocs, +1.5 KiB, same order of time

## Gaps this run showed

- Scratch `ReadDirents` was slower than godirwalk `ReadDirents` (2.29 ms vs
  1.84 ms) while using fewer allocations.
- `WalkFS` was slightly slower than `fs.WalkDir` on `MapFS`.
- Cached `Stat` uses more `B/op` than fastwalk (174 vs 146 KiB) and fewer
  allocs.
- Official 10k/100k/1M sizes are not measured.
- Linux/macOS informal medians are not in this receipt.

Full table: [`bench/go-compat/BENEFITS.md`](../../bench/go-compat/BENEFITS.md).
