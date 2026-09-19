# Choose an API

Minimum Go: 1.21.0. `CGO_ENABLED=0` for the intended runtime.

| Need | Call | Does not do |
| --- | --- | --- |
| Selected relative paths | `ScanPaths` / `ScanPathsWith` | Hash, binary detect, `max_file_bytes` |
| Manifest with hashes | `ScanWith` or `Scanner.Scan` | Watcher, search, parse |
| Owned file bytes for an indexer | `EachFile` | Replay after a callback error |
| Why a path was ignored | `Explain` / `Scanner.Explain` | Content, size, or binary verification |
| Stream without retaining files | `ScanInto` / `ScanIntoErr` / `Plan.Files` | A complete in-memory manifest |
| Reuse hashes | `ScanCached` + `ToCache` | A promised speedup |
| Manifest over `fs.FS` | `ScanFS` / `EachFileFS` / `ScanPathsFS` | OS identity, symlink follow, cache reuse |
| Combined multi-root digest | `MultiScanReport.Revision` | A merged file list across roots |
| Raw directory walk | `Walk`, `WalkFS`, `DirScanner` | Repository ignore unless you add it |
| Child names only | `ReadDirnames` | Types, metadata, or a `[]Record` on Linux |
| godirwalk-shaped walk | `WalkDirs` | Treat `Unsorted` as parallel; a borrowed `DirEntry` |
| godirwalk import alias | `compat/godirwalk` | A runtime dependency on karrick/godirwalk |
| Incremental events | `ApplyWatchPlan` | Native watches (`watch.Open` is recursive; closed is `ErrClosed`) |
| Path list with a cap | `ScanPathsWith` + `WithMaxEntries` | Silent truncation; the prefix returns with `ErrPartial` |
| Stream file bytes | `VisitContentStreaming` | Discovery still buffers candidates; hashed files are not retained |

`Scan(ctx, root)` and `ScanPaths(ctx, root)` keep two-argument signatures.
`Options{}` is the legacy zero value and is not reinterpreted as
`DefaultOptions()`. The facade (`ScanWith`, `Compile`) starts from
`DefaultOptions()`.

`internal/selection.Compile` is an ignore-prefix plan. It is not
`treestamp.Compile` → `Plan`.
