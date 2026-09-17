# First complete scan

Task: list Go files in a checkout, print a summary, and explain the first
selected path.

Minimum Go: 1.21.0.

Full program: [`examples/docquickstart/main.go`](../../examples/docquickstart/main.go).

```text
go run ./examples/docquickstart -root .
```

Expected: a `files=` line, `complete=true` or a typed termination, and an
`explain` line. Check `err` before reading the report. A nil error means the
selected work finished under the chosen policy; typed skips are not errors.

Cost: this walks the tree and hashes selected files up to 1.5 MiB. Use
`ScanPathsWith` if you only need names.

Limits: parent ignore files are off unless you enable them. `.treestampignore`
is not in the first preset. Official first-campaign benches are `MEASURED`
on a 1000-file tree.
