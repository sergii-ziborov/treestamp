# Repeat a scan with cache v2

Task: reuse content hashes when the tree is unchanged.

Canonical example: `ExampleScanReport_ToCache`.

```text
go test -count=1 -run ExampleScanReport_ToCache ./dx
```

Expected: `true true` — same revision, at least one reused hash.

Equal revisions do not prove a speedup. Record `Cache.ReusedHashes` and
`ContentReads`. An incompatible cache is dropped; `Cache.Rebuilt` means a
full content pass ran.

`WithRequireCache` fails that rebuilt pass with `CodeCache`.
