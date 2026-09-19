# Treestamp documentation

Start with the task, then open the matching recipe. Signatures live in Go
doc comments. Copy code from [`dx/example_docs_test.go`](../dx/example_docs_test.go),
not from remembered snippets.

| Task | Page |
| --- | --- |
| Choose an API | [choose-an-api.md](choose-an-api.md) |
| First complete scan | [tutorials/first-scan.md](tutorials/first-scan.md) |
| List selected paths | [recipes/scan-paths.md](recipes/scan-paths.md) |
| Build a filtered manifest | [recipes/filtered-manifest.md](recipes/filtered-manifest.md) |
| Ask why a file was skipped | [recipes/explain.md](recipes/explain.md) |
| Reuse a cache | [recipes/cache.md](recipes/cache.md) |
| Reread snapshot bytes with a limit | [recipes/snapshot-read.md](recipes/snapshot-read.md) |
| Walk an `fs.FS` | [recipes/walk-fs.md](recipes/walk-fs.md) |
| Replace godirwalk | [recipes/walk-dirs.md](recipes/walk-dirs.md) |
| Update a Merkle snapshot | [recipes/tree-snapshot.md](recipes/tree-snapshot.md) |
| Export a portable report | [recipes/portable.md](recipes/portable.md) |
| Handle a bad regex | [recipes/invalid-regex.md](recipes/invalid-regex.md) |
| Stop a sink early | [recipes/scan-into-stop.md](recipes/scan-into-stop.md) |
| One-shot facade | [recipes/scan-with.md](recipes/scan-with.md) |
| Hand owned bytes to an indexer | [recipes/each-file.md](recipes/each-file.md) |
| Symptom lookup | [troubleshooting.md](troubleshooting.md) |
| Defaults and limits | [reference/defaults.md](reference/defaults.md) |
| Revisions and snapshots | [guides/snapshots.md](guides/snapshots.md) |
| Errors and ownership | [guides/errors.md](guides/errors.md) |
| Informal benches | [guides/performance.md](guides/performance.md) |
| Command-line app | [guides/cli.md](guides/cli.md) |
| Agent consumer index | [agent-guide.md](agent-guide.md) |

Existing design notes stay where they are:
[ARCHITECTURE.md](../ARCHITECTURE.md),
[CONFORMANCE.md](../CONFORMANCE.md),
[BENCHMARKS.md](../BENCHMARKS.md),
[THREAT_MODEL.md](../THREAT_MODEL.md),
[COMPETITORS.md](../COMPETITORS.md).

Official B01–B14 first-campaign rows are `MEASURED` in
[`compat/results/official-benches.json`](../compat/results/official-benches.json)
(1000-file tree). Informal Windows medians stay in
[guides/performance.md](guides/performance.md).
