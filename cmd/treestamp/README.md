# treestamp

Command-line app for the Treestamp library. Scan a tree, save the policy
with the result, explain a skip, then verify what changed.

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.2
```

Requires **Go 1.23.2** or newer. `CGO_ENABLED=0`. This is not a second
scanner and not a `find` or `ripgrep` replacement.

Library: [`github.com/sergii-ziborov/treestamp`](https://pkg.go.dev/github.com/sergii-ziborov/treestamp)
(tag `v0.1.2`). People without Go should use a release binary.

```text
treestamp scan . --ext go --json --output ../baseline.tstamp.json
treestamp paths . --ext go --null
treestamp explain src/generated/model.go --root .
treestamp diff ../before.tstamp.json ../after.tstamp.json
treestamp verify ../baseline.tstamp.json --root .
```

`scan` and `paths` default `ROOT` to `.`. `verify` requires `--root`.
A bare `treestamp` prints help and does not hash the current directory.

`--scope` narrows selection. It does not cancel ignore rules.
`--output` must sit outside the scan root. Fast cache is never treated as
content proof for `verify`.

Exit `0` is a completed operation. `1` is a finished difference
(`verify`, or `diff --exit-code`). `2` is usage. `3` is partial. `4` is
impossible. `5` is a publish failure.

Guide: [docs/guides/cli.md](../../docs/guides/cli.md).
