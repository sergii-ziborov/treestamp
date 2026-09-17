# Choose an API

Minimum Go: 1.23.2. `CGO_ENABLED=0` for the intended runtime.

| Need | Call | Does not do |
| --- | --- | --- |
| Selected relative paths | `ScanPaths` / `ScanPathsWith` | Hash, binary detect, `max_file_bytes` |
| Manifest with hashes | `ScanWith` or `Scanner.Scan` | Watcher, search, parse |
| Owned file bytes for an indexer | `EachFile` | Replay after a callback error |
| Why a path was ignored | `Explain` / `Scanner.Explain` | Content, size, or binary verification |
| Stream without retaining files | `ScanInto` / `ScanIntoErr` | A complete in-memory manifest |
| Reuse hashes | `ScanCached` + `ToCache` | A promised speedup |
| Raw directory walk | `Walk`, `WalkFS`, `DirScanner` | Repository ignore unless you add it |
| Incremental events | `ApplyWatchPlan` | Recursive fsnotify (not in this module) |

`Scan(ctx, root)` and `ScanPaths(ctx, root)` keep two-argument signatures.
`Options{}` is the legacy zero value and is not reinterpreted as
`DefaultOptions()`. The facade (`ScanWith`, `Compile`) starts from
`DefaultOptions()`.

`internal/selection.Compile` is an ignore-prefix plan. It is not
`treestamp.Compile` → `Plan`.
