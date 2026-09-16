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
| What to attach to an issue | Versions, options, tiny fixture | No file contents, no absolute host paths. |

`Explain` does not diagnose binary or oversize skips. Those appear as typed
`Skipped` entries on a content scan.
