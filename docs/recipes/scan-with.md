# One-shot facade

Task: scan from `DefaultOptions()` without building `Options` by hand.

Canonical example: `ExampleScanWith`.

```text
go test -count=1 -run ExampleScanWith ./dx
```

Expected: `1 keep.go` after excluding `*_test.go`.

`ScanWith` / `ScanPathsWith` / `EachFile` / `Compile` do not change
`Scan(ctx, root)`. `WithLogger` is silent unless you pass a logger. Progress
has no percent field.
