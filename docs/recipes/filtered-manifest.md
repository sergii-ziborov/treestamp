# Filtered manifest

Task: hash only `*.go` files using current `Options`.

Canonical example: `ExampleNewScanner_withExtensions`.

```text
go test -count=1 -run ExampleNewScanner_withExtensions ./dx
```

Expected: `1 true true` — one file, complete, no termination.

Check `Complete` and `Termination` after a nil error. Policy skips stay on
`Skipped` and are not failures.

Or use `ScanWith(ctx, root, WithExtensions("go"))` for the same profile
without constructing `Options` by hand.
