# Treestamp CLI

A small command-line app on the Treestamp library. It is not a second scanner
and not a `find` or `ripgrep` replacement.

The useful story is: select a tree by policy, save the rules with the result,
explain a decision, then verify what changed.

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.0-alpha.1
```

Go 1.23.0+, `CGO_ENABLED=0`. The Git tag for this nested module is
`cmd/treestamp/v0.1.0-alpha.1`. The version you pass to `go install` is
`v0.1.0-alpha.1`.

People without Go should use a release binary. The library import path is
separate: `github.com/sergii-ziborov/treestamp`.

## Commands

```text
treestamp scan .
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
Library: [the module README](../../README.md).
