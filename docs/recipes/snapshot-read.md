# Bounded snapshot reread

Task: open a selected file again and refuse oversized reads.

Canonical example: `ExampleSnapshotContentProvider_ReadBounded`.

```text
go test -count=1 -run ExampleSnapshotContentProvider_ReadBounded ./dx
```

Expected: `abcdef true` after a 2-byte limit fails and a 64-byte limit
succeeds with SHA-256 evidence.

This reopens the file on disk and checks the snapshot. Bytes are not stored
inside the manifest. `ContentProvider.Open` returns loaded content, not an
`io.Reader`.

A changed file fails closed (stale). See [troubleshooting](../troubleshooting.md).
