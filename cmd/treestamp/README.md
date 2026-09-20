# treestamp

This directory is the **user CLI**. The Go **library** is the repository
root, not this folder.

| | |
| --- | --- |
| Library import | [`github.com/sergii-ziborov/treestamp`](https://pkg.go.dev/github.com/sergii-ziborov/treestamp) |
| Library tag | [`v0.1.5`](https://github.com/sergii-ziborov/treestamp/releases/tag/v0.1.5) |
| This CLI | `go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.5` |
| Binaries | [CLI v0.1.5](https://github.com/sergii-ziborov/treestamp/releases/tag/cmd/treestamp/v0.1.5) |
| Not this | [`../treestamp-driver`](../treestamp-driver) — fixture protocol, do not install |

Same scanner as `go get github.com/sergii-ziborov/treestamp@v0.1.5` — not
a second engine. Requires **Go 1.21** or newer. `CGO_ENABLED=0`.

A bare `treestamp` prints help and does not hash the current directory.

## When to use this, not the library

Write Go against `ScanWith` / `EachFile` / `Explain`. Use this binary
when the caller is CI, a shell, PowerShell, or a person at a terminal.

| Situation | Command | Do not |
| --- | --- | --- |
| Fail a job if selected sources change | `verify --root` | Treat exit `3` as clean |
| Commit that gate next to the subtree | `scan --output` on a subtree | Redirect JSON into the scan root |
| Two jobs already wrote manifests | `diff --exit-code` | Expect `diff` to open the tree |
| Pipe names into `gofmt` / `xargs` | `paths --null` | Assume `paths` skipped binaries |
| “Why is this file missing?” | `explain` | Treat excluded as a failed command |
| Hash `dist/` as shipped | `scan --profile artifact` | Change default `repo` scans |
| Watch hashes in a log | `scan --format ndjson` | Parse it as a finished manifest |

```text
treestamp scan ./examples/docquickstart --ext go
```

![treestamp scan](../../docs/cli/scan.svg)

```text
treestamp explain testdata/generated/model.go --root .
```

![treestamp explain](../../docs/cli/explain.svg)

```text
treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp
```

![treestamp verify](../../docs/cli/verify.svg)

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
changed. The human card lists every one of those paths (`+`, `-`, `~`,
`old → new`). Matching size and mtime is not enough. Fast cache is
never treated as proof.

Pipe raw changed paths into another tool (`verify --null` writes the
path only, not the human `+` / `-` / `~` prefix):

```text
treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp --null
```

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

```yaml
- uses: actions/setup-go@v5
  with:
    go-version: "1.21.x"
- run: go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.5
- run: treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp --json
```

Exit `0` keeps the job green. Exit `1` is a finished difference. Exit
`3` is a partial walk — do not allow that as success.

## Compare two CI artifacts without opening the tree

After two jobs each wrote a manifest:

```text
treestamp diff ./baselines/before.tstamp.json ./baselines/after.tstamp.json --json --exit-code
```

`diff` only reads the two documents. Use it when the trees are gone and
you still need added/removed/changed/renamed. `--exit-code` is exit `1`
when a completed comparison finds a difference.

## Watch files as they hash

```text
treestamp scan ./cmd/treestamp --ext go --format ndjson
```

Each line is one JSON object: `scan_begin`, then `file_committed` as
soon as that file is hashed, then `scan_end`. This is not a dump of a
finished manifest.

## Hash binaries and ignore gitignore

Default `scan` is the repo profile: `.gitignore` applies, NUL-containing
files are skipped, and files over 1.5MiB are skipped. For a release
tree or other artifact that should be hashed as-is:

```text
treestamp scan ./dist --profile artifact --json --output ../dist.tstamp.json
treestamp verify ../dist.tstamp.json --root ./dist
```

`--profile artifact` (stored as `artifact-v2`) hashes binaries and
`node_modules` / `vendor` / `dist`. It still skips VCS directories
(`.git`, `.hg`, `.svn`). Saved `artifact-v1` snapshots keep the old
generated-directory skips. This is not a hashdeep file.
`--metadata-only` is refused. Default `repo` still skips binaries.

## Feed selected names to another tool (no hashing)

`paths` applies the same ignore and filter rules as `scan` and does not
read file bytes. It can list a binary that `scan` later drops. `--null`
is for names that contain spaces or newlines:

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
  "profile": "repo",
  "extensions": ["go"],
  "scope": ["cmd/treestamp"],
  "exclude": ["testdata"]
}
```

```text
treestamp config --config policy.json
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
