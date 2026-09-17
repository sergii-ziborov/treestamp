# treestamp

Command-line app for the Treestamp library. Same scanner as
`go get github.com/sergii-ziborov/treestamp` — not a second engine.

```text
go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.3
```

Requires **Go 1.21** or newer. `CGO_ENABLED=0`. People without Go should
use a GitHub release binary.

Library: [`github.com/sergii-ziborov/treestamp`](https://pkg.go.dev/github.com/sergii-ziborov/treestamp)
(tag `v0.1.3`).

A bare `treestamp` prints help and does not hash the current directory.

## Keep a baseline Git can store

`--output` inside the scan root is refused. Redirecting `scan --json`
into a file under `.` is worse: the shell creates that file before
Treestamp starts, so the scan would see its own output.

Scan a subtree and write the manifest next to it:

```text
treestamp scan ./cmd/treestamp --ext go --json --output ./baselines/cli.tstamp.json
```

The file stores the policy with the content hashes. Later, on another
machine or in CI:

```text
treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp
```

Exit `0` means the same policy still selects the same bytes. Exit `1`
means a selected file was added, removed, renamed, or its contents
changed. Matching size and mtime is not enough. Fast cache is never
treated as proof.

Whole-repo baseline lives *beside* the checkout, not inside it:

```text
treestamp scan . --ext go --json --output ../repo.tstamp.json
treestamp verify ../repo.tstamp.json --root .
```

## Fail CI when selected files move

```text
treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp --json
```

Use the JSON document in logs. The process exit code is what the job
should gate on. `verify` re-walks `--root` with the policy saved in the
manifest. It does not take the root from the file (so a copied artifact
cannot silently point at the wrong tree).

## Compare two CI artifacts without opening the tree

After two jobs each wrote a manifest:

```text
treestamp diff ./baselines/before.tstamp.json ./baselines/after.tstamp.json --json --exit-code
```

`diff` only reads the two documents. Use it when the trees are gone and
you still need added/removed/changed/renamed. `--exit-code` is exit `1`
when a completed comparison finds a difference.

## Feed selected names to another tool (no hashing)

`paths` applies the same ignore and filter rules as `scan` and does not
read file bytes. `--null` is for names that contain spaces or newlines:

```text
treestamp paths . --ext go --null | xargs -0 gofmt -l
```

PowerShell (no `xargs -0`):

```text
(treestamp paths . --ext go --json | ConvertFrom-Json).paths
```

`--absolute` when the next tool cannot be run from the scan root.

## Why this file is missing from the baseline

`explain` is selection only. It does not re-read contents. Use the same
`--ext` / `--scope` / `--exclude` / `--config` as the scan that produced
the manifest, or you will explain a different policy.

```text
treestamp explain internal/generated/model.go --root . --ext go
```

Exit `0` even when the path is excluded. The command answered the
question; it did not fail the build. For a script, `--json` prints
`outcome`, `source`, `line`, and `pattern`.

## One policy file for scan, paths, and explain

```text
{
  "schema": "treestamp.policy/v1",
  "extensions": ["go"],
  "scope": ["cmd/treestamp"],
  "exclude": ["testdata"]
}
```

```text
treestamp config show --config policy.json
treestamp scan . --config policy.json --json --output ../cli.tstamp.json
treestamp paths . --config policy.json
treestamp explain cmd/treestamp/main.go --root . --config policy.json
```

`--scope` narrows the tree. It does not cancel `.gitignore`.
`--no-ignore` drops ignore files and still keeps standard skips (VCS
directories, oversized files). `--exclude testdata` is a substring of
the relative path, not a second ignore engine.

## Do not verify a name-only inventory

```text
treestamp scan . --metadata-only --json --output ../names.tstamp.json
treestamp verify ../names.tstamp.json --root .
```

The second command refuses. There are no content hashes, so a match
would be a lie.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Completed operation (including “explained, and it is excluded”) |
| 1 | Finished difference (`verify`, or `diff --exit-code`) |
| 2 | Usage |
| 3 | Partial scan |
| 4 | Impossible (missing file, incomplete baseline, no hashes to verify) |
| 5 | Publish failure (could not write the manifest) |

Guide: [docs/guides/cli.md](../../docs/guides/cli.md).
