# Troubleshooting

| Symptom | Check | Safe action |
| --- | --- | --- |
| File exists but is missing from the result | `Explain`, defaults, ignore/override | Read the winning source/line/pattern. Do not disable all ignores as a first step. |
| File has no hash | Operation was `ScanPaths` or `MetadataOnly` | Use `Scan` / `ScanWith` if you need hashes. |
| `err == nil` but the list looks short | `Complete`, `Termination`, typed skips | Inspect `Skipped` and `Summary()`. |
| Snapshot read fails after an edit | File version / hash evidence | Restore the file or rescan. Do not skip verification. |
| Cache did not speed up | `ReusedHashes`, `ContentReads`, `Rebuilt` | Confirm the cache root/format and that files were unchanged. |
| `tree2:` ≠ `sha256:` | Different structures | Compare within one format only. |
| Consumer stopped, process waits | Channel / sink lifecycle | `FileWalker.Start` closes the queue; `Terminate` cancels. |
| `WalkDirs` looks parallel after `Unsorted` | `Unsorted` only drops sort | Set `NumWorkers` for parallel callbacks. |
| File root: `cannot Walk non-directory` | godirwalk default | Set `AllowNonDirectory`. |
| `SkipDir` on a file dropped siblings | That is `SkipDir` | Use `SkipThis` to skip one node. |
| `compat/godirwalk` slower than `WalkDirs` | Extra `Dirent` wrappers | Prefer `WalkDirs` unless you need the old types. |
| Hardlink shown twice | Path-only dedup | Wrap with `IgnoreDuplicateFiles`. Same bytes ≠ same object. |
| `compat/fastwalk` `SkipAll` is an error | fastwalk v1.0.14 | Native `Walk` treats `SkipAll` as a successful stop unless `KeepSkipAll`. |
| Need an import switch, not a speed claim | Existing fastwalk module | Change the import to `compat/fastwalk`; see `consumer/`. |
| Parallel walk became serial after `Sort` | Legacy `Config.Sort` bool | Use `SortMode` for local order. `Sort` is global DFS. |
| Ordered pull hung after `Close` | Waiters missed cancel | `Close` cancels the pull context and wakes cond waiters. |
| `TryIntoIterOrderedBounded` failed | Executor admitted no workers | That is an admission error, not a silent one-goroutine fallback. |
| Darwin or Windows walk ≠ Linux timing | Different directory backends | Do not publish a Linux getdents number as a Darwin/Windows result. |
| `FileVersion.Identity` nil on Windows | FindFirstFile has no file index | Use `PathIdentity` or follow/same-filesystem when an index is required. |
| What to attach to an issue | Versions, options, tiny fixture | No file contents, no absolute host paths. |

`Explain` does not diagnose binary or oversize skips. Those appear as typed
`Skipped` entries on a content scan.
