# List selected paths

Task: get ignore-aware relative names without hashing.

Canonical example: `ExampleScanPaths` in
[`dx/example_docs_test.go`](../../dx/example_docs_test.go).

```text
go test -count=1 -run ExampleScanPaths ./dx
```

Expected output: `true false` — `keep.go` is listed, `skip.bin` is not.

Cost: directory listing and ignore matching only. `ScanPaths` does not apply
`max_file_bytes` and does not hash. It is not `Scan` with the report stripped.

Limit: explicit `WithHashContents` / `WithDetectBinary` / `WithMaxFileBytes`
on `ScanPathsWith` returns `CodeUnsupported`.
