# Stop a sink

Task: emit one file and halt without claiming a full walk.

Canonical example: `ExampleScanner_ScanInto_stop`.

```text
go test -count=1 -run ExampleScanner_ScanInto_stop ./dx
```

Expected: `true 1`.

`Stopped` means the consumer asked to halt. It is not a complete scan
and is not reported as `ErrPartial`. Callback errors use `ScanIntoErr`
and `CodeCallback`. `EachFile` returning `ErrStop` is the owned-bytes
equivalent. After a sink that did not stop, `ScanIntoErr` and
`Plan.Files` yield the same incompleteness error as `Plan.Scan`.
