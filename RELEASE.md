# Release

Owner: Sergii Ziborov, personal public repository
`github.com/sergii-ziborov/treestamp`.

Do not publish this project from the Weavatrix or EdgeHawk organizations.

## Current

- Published library: `v0.1.4` → `go get github.com/sergii-ziborov/treestamp@v0.1.4`
  ([pkg.go.dev](https://pkg.go.dev/github.com/sergii-ziborov/treestamp))
- This checkout: `Version` `0.1.5` (untagged). Do not retag `v0.1.4`.
- Published CLI: `cmd/treestamp/v0.1.4` → `go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.4`
- Driver: `cmd/treestamp-driver` is not tagged and not installed

Those published tags stay immutable. A later library bump is a new tag. The published
CLI `go.mod` must not contain a `replace` of the library. GitHub “Latest
release” is the CLI binary tag; the published library is the `v0.1.4` release on the
same repository until `v0.1.5` is tagged.

## Full-port gate

All of the following must be true:

- Reviewed public-member inventory at the pinned commit; no dropped member
- T01–T35 closed with evidence
- Required language adaptations covered
- Rust, Go, and Git oracle checks passed
- Native platform tests plus fuzz, race, and leak work executed
- Real benchmarks published with raw artifacts
- Sources, licenses, and versions pinned

An alpha may be partial if the notes say so. “Full port” is not a marketing
shortcut.

## Each stage PR

State the upstream commit, contract IDs, tests actually run, unsupported
cases, and only numbers that were measured.

## Versioning

Go module versions follow semver. Breaking scan-contract changes need a new
major or a documented profile. Compatible ignore presets are versioned
separately from the product name.

## Creating the GitHub repository

```text
python3 tools/create_github_repo.py --visibility public
python3 tools/create_github_repo.py --visibility public --execute
```

The helper uses the logged-in `gh` account, requires `sergii-ziborov`, creates
a public repository, and refuses organization targets.
