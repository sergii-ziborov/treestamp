# Treestamp CLI

The CLI is a thin process on the public library. It does not import
`internal/` scanner packages and does not grow a second engine.

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.3
```

Nested-module docs:
[pkg.go.dev/github.com/sergii-ziborov/treestamp/cmd/treestamp](https://pkg.go.dev/github.com/sergii-ziborov/treestamp/cmd/treestamp).

## Baseline you can commit

`--output` must sit outside the scan root. To keep a manifest in the
same Git repository, scan a subtree:

```text
treestamp scan ./cmd/treestamp --ext go --json --output ./baselines/cli.tstamp.json
treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp
```

A whole-tree baseline belongs beside the checkout:

```text
treestamp scan . --ext go --json --output ../repo.tstamp.json
```

Do not write `treestamp scan --json > ./baseline.tstamp.json`. The shell
creates that file first; Treestamp would then select it.

`verify` always needs `--root`. The manifest does not choose the tree.
Matching size and mtime is not enough. Fast cache is never treated as
content proof. An incomplete or `--metadata-only` baseline cannot be
verified.

## CI

Gate the job on the process exit code:

```text
treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp --json
```

Exit `1` is a finished difference. Exit `3` is a partial walk; do not
treat that as “clean”.

When two jobs already produced manifests and the trees are gone:

```text
treestamp diff ./baselines/before.tstamp.json ./baselines/after.tstamp.json --json --exit-code
```

`diff` does not open the repository. `verify` does.

## Selected paths without hashing

Same ignore and filter rules as `scan`:

```text
treestamp paths . --ext go --null | xargs -0 gofmt -l
```

`--json` is the PowerShell-friendly form. `--absolute` when the next
command cannot run from the scan root. `--null` and `--json` cannot be
combined. `--metadata-only` is invalid here: `paths` is already
content-free.

## Explain a skip with the same policy

```text
treestamp explain skip.txt --root . --ext go
```

An excluded path is still exit `0`. `--json` reports `outcome`,
`source`, `line`, and `pattern`. Bytes and binary detection are not
checked.

## Shared policy file

Schema `treestamp.policy/v1`. Flags override empty slices from the file.

```text
treestamp config show --config policy.json
treestamp scan . --config policy.json --json --output ../out.tstamp.json
```

`--scope` does not cancel ignore rules. `--no-ignore` drops ignore files
and keeps standard skips. `--exclude` matches a substring of the
relative path.

## Flags that stay honest

| Flag | Meaning |
| --- | --- |
| `--scope GLOB` | Narrow the tree. Ignore rules still apply. |
| `--exclude SUBSTR` | Subtract relative paths that contain this string. |
| `--ext` | Extension filter. Dots are optional. |
| `--json` | One versioned document on stdout. |
| `--output FILE` | Atomic write. File must be outside the scan root. |
| `--null` | `paths` only. Exact names, NUL separated. |
| `--absolute` | `paths` only. Print absolute paths. |
| `--metadata-only` | `scan` only. No content hashes; `verify` will refuse. |
| `--color` | `auto`, `always`, or `never`. `NO_COLOR` wins. |

## What this release does not do

No `watch`, `--exec`, TUI, server, MCP, hidden cache, or plugin loader.
`cmd/treestamp-driver` stays the fixture protocol driver.

## Versions

`treestamp version --json` prints CLI, core, and manifest schema. Nested
module tags use the `cmd/treestamp/v…` Git prefix. `go install` still
takes `@v0.1.3` for the current CLI tag. The library import is
`github.com/sergii-ziborov/treestamp@v0.1.3`.
