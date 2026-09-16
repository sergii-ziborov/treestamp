# Update a Merkle snapshot

Task: apply one changed record and keep the previous snapshot.

Canonical example: `ExampleTreeSnapshot_Apply`.

```text
go test -count=1 -run ExampleTreeSnapshot_Apply ./dx
```

Expected: `true` then `false` — `tree2:` prefix, revisions differ.

`TreeSnapshot` is a persistent in-memory structure over supplied
path/hash/size records. It is not a disk index and not a full incremental
lifecycle. `ScanSession` still stores a normal report. `tree2:` does not
replace the flat `sha256:` revision, descriptor v2, or event coverage.
