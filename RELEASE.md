# Release

Owner: Sergii Ziborov, personal public repository
`github.com/sergii-ziborov/treestamp`.

Do not publish this project from the Weavatrix or EdgeHawk organizations.

## Current

Still not a full port:

- Library: `v0.1.0` → `go get github.com/sergii-ziborov/treestamp@v0.1.0`
- CLI: `cmd/treestamp/v0.1.0` → `go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.0`

Do not tag `v1` or write “full port” in a release title. The published CLI
`go.mod` must not contain a `replace` of the library.

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
