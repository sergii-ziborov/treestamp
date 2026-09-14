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

P0 is in place with a reviewed-gap inventory. P1 serial walker is implemented.
P2–P8 remain mandatory scope. Ledger: `compat/ledger.json`.

## Next implementation work

1. Finish the public-member review of `compat/inventory.json` against rustdoc
   at the pinned commit. No member may be dropped.
2. Keep the independent Rust driver honest: no scanner-algorithm edits.
3. Start P2: ignore parser, sources, precedence, types, overrides, matchers.
4. Close each stage in the ledger with tests actually run and contracts still
   open.

## Local checks

```text
python3 tools/audit.py
python3 -m unittest discover -s tools -p "test_*.py"
python3 tools/audit.py --require-full
go test ./...
```

`--require-full` must fail until the port is actually complete.

## GitHub

```text
python3 tools/create_github_repo.py --visibility public
python3 tools/create_github_repo.py --visibility public --execute
```

The helper creates `sergii-ziborov/treestamp` only. It refuses another account
or an organization. It does not overwrite an existing remote and does not
force-push.
