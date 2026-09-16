# API contract template

Use this next to exported symbols in Go comments. Link types with
`[Scanner]`, `[ScanReport]`, `[context.Context]`.

| Question | Write |
| --- | --- |
| Paths | Root interpretation; native vs slash |
| Order | Sorted, DFS, completion, or unspecified |
| Concurrency | Can the callback run on several goroutines? |
| Ownership | May the caller keep bytes/entries after return? |
| Close | Who closes queues; is Close idempotent? |
| Errors | Partial, cancel, stop, consumer failure |
| Resources | What stays in memory; which limits apply |

Example of a surprising name: `[SnapshotContentProvider.Open]` loads bytes.
It does not return an `io.Reader`.
