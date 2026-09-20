This is the **user CLI** (`treestamp`). It is not the Go library and not
`cmd/treestamp-driver`.

## Install with Go

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.5
```

Requires Go 1.21 or newer. `CGO_ENABLED=0`.

## Install without Go

Download a `treestamp-*` binary attached to this release. Checksums are
in `SHA256SUMS.txt`. Rename the file to `treestamp` (or
`treestamp.exe` on Windows) and put it on `PATH`.

## Library

The CLI is a nested module on the published library:

- Import: https://pkg.go.dev/github.com/sergii-ziborov/treestamp
- Library tag: `v0.1.5` (same number, different Git tag)
- CLI docs: https://github.com/sergii-ziborov/treestamp/blob/main/cmd/treestamp/README.md

`cmd/treestamp-driver` is a fixture protocol binary for tests. Do not
install it as the product CLI.
