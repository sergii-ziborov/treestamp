# Treestamp CLI

The CLI is a thin process on the public library. It does not import
`internal/` scanner packages and does not grow a second engine.

Install after the published tag:

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.0
```

## First three commands

Scan a checkout and keep the baseline outside the tree:

```text
treestamp scan . --ext go --json --output ../baseline.tstamp.json
```

List selected paths without hashing:

```text
treestamp paths . --ext go
```

Ask why a path was dropped:

```text
treestamp explain generated/model.go --root .
```

An excluded path is still exit `0`. The command explained the decision.

## Baseline workflow

```text
treestamp scan . --json --output ../before.tstamp.json
# ...edit the tree...
treestamp scan . --json --output ../after.tstamp.json
treestamp diff ../before.tstamp.json ../after.tstamp.json --exit-code
treestamp verify ../before.tstamp.json --root .
```

`diff` reads two documents only. `verify` re-applies the saved policy to
`--root` and looks for new selected paths. Matching size and mtime is not
enough. The command does not reuse Fast cache as content proof.

## Flags that stay honest

| Flag | Meaning |
| --- | --- |
| `--scope GLOB` | Narrow the tree. Ignore rules still apply. |
| `--exclude GLOB` | Subtract matching relative paths. |
| `--ext` | Extension filter. Dots are optional. |
| `--json` | One versioned document on stdout. |
| `--output FILE` | Atomic write. File must be outside the scan root. |
| `--null` | `paths` only. Exact names, NUL separated. |
| `--color` | `auto`, `always`, or `never`. `NO_COLOR` wins. |

Do not redirect `scan --json` into a file inside `.`. The shell creates that
file before Treestamp starts. Write `--output ../baseline.tstamp.json`.

## What this release does not do

No `watch`, `--exec`, TUI, server, MCP, hidden cache, or plugin loader.
`cmd/treestamp-driver` stays the fixture protocol driver.

## Versions

`treestamp version --json` prints CLI, core, and manifest schema. Nested
module tags use the `cmd/treestamp/v…` Git prefix. `go install` still
takes `@v0.1.0`.
