# Benchmarks

**Status: `NOT_RUN`.** This file is the measurement policy. It does not contain
Treestamp timings. Do not copy weavatrix-scan percentages or millisecond
figures into README or release notes as if they were measured here.

## Cases

See `bench/cases.json`. B01–B14 stay separate classes.

## Sizes and shapes

Required sizes: 10k / 100k / 1M files, plus 10 / 100 / 1k for overhead.
Shapes: balanced, wide, deep, skewed, ignore-heavy.
Contents: empty, 1KiB, 64KiB, 1MiB, binary, mixed, `max_file_bytes` edges.

Real public checkouts use fixed commits. Do not install `node_modules` or
build `target` unless generated trees are the point of the case.

A content run is not one million one-byte files. A memory run is not only
`B/op`; RSS is required. Interactive latency is not mean bulk throughput.

## Incremental

`N` in {10k, 100k, 1M}, `k` in {0, 1, 10, 100, 1%, bulk}. Include rename,
subtree delete, and ignore edits. Record files actually reread, filesystem
calls, reused entries, retained-report cost, revision cost, requested
serialization, peak memory, and the actual update reason. After every
quiescent batch, compare to a full rescan. That is a correctness gate.

## How Go and Rust are compared

Two native executables, one corpus, one machine. JSON request names the
operation. Result data and timing metadata are split. Compile and parse
outside the timer. The timer ends when the requested result exists.

Application run: fresh process → configure → operate → output → close. Both
languages pay startup. Also run a resident sequence with cache reuse, cleanup,
and GC/allocator cost inside the interval. A fresh process is not a cold
filesystem cache.

Order AB/BA or randomized blocks. At least three warmups. Target 20–30
measured pairs. Report median, spread, sample count, and a paired interval.
p95 needs a long series (target 100 observations). Expensive 1M sets may use
fewer repeats without fake tail precision.

Two resource tracks: matched workers/`GOMAXPROCS`/CPU limits, and default-auto
with resolved counts recorded. Headline Go runs use ordinary GC.
Race/sanitizers/profilers are diagnostic binaries. Do not give PGO or CPU
affinity to only one participant.

Windows/NTFS, Linux/ext4, and macOS/APFS are separate campaigns.

`speedup = T_reference / T_treestamp`. Above 1 is a speedup. Memory is its
own column. States: `NOT_RUN` / `UNSUPPORTED` / `PARITY_FAIL` /
`EXECUTION_FAIL` / `MEASURED`.

The engineering aim is to approach the Rust oracle and the best equivalent Go
baseline, then look for wins. Speed does not authorize weaker checks. A
correct full port may ship behind, if the loss is stated.
