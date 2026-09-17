# Agent instructions

Treestamp is the personal public repository of Sergii Ziborov:

- GitHub: `https://github.com/sergii-ziborov/treestamp`
- Module: `github.com/sergii-ziborov/treestamp`
- Owner account: `sergii-ziborov` only

Do not create or push this project under the Weavatrix or EdgeHawk
organizations. Weavatrix Scan is the pinned upstream oracle, not the host of
this repository.

## Product rules

- Full native Go port. No CGO, WASM, or Rust subprocess in the runtime library.
- Rust is for developers: oracle, reference driver, and benchmarks.
- Do not add a parser, content search, graph, embeddings, MCP, secret scanner,
  web service, or daemon.
- Do not rename a `filepath.WalkDir` wrapper as a full port.
- Do not add `.treestampignore` to the first compatible ignore preset.
- Do not claim default whole-scan resource limits. They are off in the oracle.
- Do not hash JSON and call it descriptor v2.
- Do not implement `ScanPaths` by running a full `Scan` and dropping fields.
- Do not put `fsnotify` in the main module’s required dependencies.
- Do not publish Rust timing percentages as Treestamp results.
- Commits must not add `Co-authored-by` trailers.
- one file ≤ 600 lines
- one method ≤ 60 lines
- ≤ 6 parameters, else wrap in an object
- one folder ≤ 6 .go files (including tests), else split into speaking-named subfolders

## Pinned oracle

```text
crate:              weavatrix-scan
cargo version:      0.5.2
commit:             29c003a6ad541c9a10faf30505235375fa78b9d8
tree:               108c59e66c90c3b649b3ff867360a6c0e9ff7f6e
descriptor version: 2
cache format:       2
```

Before moving the pin, diff contracts, refresh goldens, and rerun tests.
Following live `main` as the oracle destroys reproducibility.

## Current stage

P2–P6 method surfaces are implemented: ignore, scan, parallel/pull,
content visit, cache v2, incremental watch apply. Go-market extensions include
selective links, arbitrary `fs.FS`, cached callback target `Stat`, a
lazy single-directory `DirScanner` with reusable scratch buffer, optional
`.gitmodules` skipping, changed-only content visit, confined watch-plan apply,
executor-owned parallel admission, compiled override-scope pruning,
`Explain`, identity-keyed content reuse, a shared admission budget,
ordered-parallel directory pull, and a persistent Merkle `TreeSnapshot`.
`ScanSession` applies Merkle deltas when a `TreeSnapshot` is present.
The recommended facade is `ScanWith` / `EachFile` / `Compile` without
changing the two-argument `Scan` / `ScanPaths` signatures. The user CLI
is `cmd/treestamp` (separate Go module). `cmd/treestamp-driver` stays the
fixture driver. Task docs live in `docs/`. Optional `watch/` is a nested
module (never a main-module fsnotify require). Official B01–B14
first-campaign receipts live in `compat/results/official-benches.json`.
Functional parity, including cache/watch sequences, is in the
Windows/Linux/macOS CI matrix. Ledger: `compat/ledger.json`.

## Next implementation work

1. Larger official sizes (10k / 100k / 1M) against fastwalk, gocodewalker,
   and the rust oracle when a dedicated host is available.
2. Library tag is `v0.1.3`. CLI tag is `cmd/treestamp/v0.1.3` and must
   require that published library. Do not retag an immutable version.

## Local checks

```text
python3 tools/audit.py
python3 -m unittest discover -s tools -p "test_*.py"
python3 tools/run_functional_parity.py
python3 tools/audit.py --require-full
go test ./...
cd cmd/treestamp && go test ./...
cd bench/go-compat && GOWORK=off go test ./...
```

`--require-full` must pass while T01–T35 are implemented, B01–B14 first-campaign
rows are MEASURED, and `inventory_complete` is true.

## GitHub

```text
python3 tools/create_github_repo.py --visibility public
python3 tools/create_github_repo.py --visibility public --execute
```

The helper creates `sergii-ziborov/treestamp` only. It refuses another account
or an organization. It does not overwrite an existing remote and does not
force-push.
