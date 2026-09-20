# Agent consumer guide

Version: development checkout at the commit that last updated
`docs/evidence.lock.json`. This is not a published module tag unless that
file says so.

Use only the public module `github.com/sergii-ziborov/treestamp`.

Working entry points: `ScanPathsWith`, `ScanWith`, `EachFile`, `ScanFS`,
`EachFileFS`, `Compile`, `Explain`, `NewScanner`, `Walk`, `WalkDirs`,
`WalkFS`, `WalkWithConfig`, `ReadDirnames`, `DirScanner`, `compat/fastwalk`,
`Options.IgnoreRules`.
External import switch: [`consumer/`](../consumer).

Recipes: [index.md](index.md). Examples: `dx/example_docs_test.go`.

Do not:

- import `internal/` from an application
- treat `internal/selection.Compile` as `treestamp.Compile`
- invent official 10k/100k/1M rankings; first-campaign `MEASURED` rows live in `compat/results/official-benches.json`
- log file contents or absolute paths
- invent `.treestampignore` as a default
- call `ScanPaths` a hashed scan
- say `Explain` verifies content
- say `TreeSnapshot` is an on-disk index
