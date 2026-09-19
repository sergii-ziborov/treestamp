# Changelog

## Unreleased

`verify` and `diff` print every changed path in the human card, not a
preview of eight. `--null` writes the same `+` / `-` / `~` / rename
records separated by NUL. `scan` lists selected names when there are
20 or fewer. `scan --format ndjson` emits `scan_begin`, each
`file_committed`, then `scan_end` from `ScanInto` instead of printing
after a full in-memory manifest. `--profile artifact` hashes binaries,
drops the 1.5MiB cap, and skips gitignore files while keeping standard
skips. Default `repo` stays the Weavatrix source profile. This is not
a hashdeep codec.

## Library 0.1.4 / CLI 0.1.4 (2026-09-19)

High-level scan now reports incompleteness on every facade: `Plan.Scan`,
`Plan.Files`, `ScanIntoErr`, and `EachFile` share the same finish check.
A `MaxEntries=0` walk on a non-empty tree yields `ErrPartial` instead of
a silent end. A deliberate iterator break or `ErrStop` is still not a
failure. `EachFile` uses the same `ScannedFile` converter as `Scan` and
counts hashed bytes in the visit summary.

CLI `Load` requires a second JSON decode to `EOF`, so a trailing `]`,
`}`, second document, or garbage after a valid manifest is rejected.
Contradictory baselines (`complete` plus timeout/cancel/required
failures, unknown evidence/profile, mismatched counts) fail at load.
`AsReport` keeps `Termination`. Text-only `scan` no longer
`MarshalIndent`s a manifest that will not be written. CI runs the CLI
module with `GOWORK=off` against the declared require.

CLI human output from 0.1.3 stays: empty rows and leftover `completion`
are gone; `config` has no `show` subcommand. Tags `v0.1.3` and
`cmd/treestamp/v0.1.3` stay immutable.

## Library 0.1.3 / CLI 0.1.3 (2026-09-17)

Minimum Go is **1.21.0** again, so every toolchain from 1.21 through
current can compile. `Plan.Files` no longer imports `iter` (1.23+ can
still range over the yield func). `golang.org/x/sys` is pinned to
v0.30.0, the last line that does not require Go 1.23. CI compiles
1.21.x–1.26.x. The CLI README shows committed-subtree baselines,
`verify` vs `diff`, and `paths --null` into another tool.

## Library 0.1.2 / CLI 0.1.2 (2026-09-17)

pkg.go.dev pages describe the product first. That tag declared Go 1.23.2;
use `@v0.1.3` for the 1.21 floor. Tags `v0.1.0` and `cmd/treestamp/v0.1.1`
stay immutable.

## Full-port gate (2026-09-17)

T01–T35 closed. Official B01–B14 first campaign recorded on a 1000-file
tree (`compat/results/official-benches.json`). Optional `watch/` module
uses fsnotify outside the main `go.mod`. GitHub can render `docs/cli/*.svg`
(no unescaped `&`). README and `--require-full` match that state.

## CLI 0.1.1 (2026-09-17)

CLI tag `cmd/treestamp/v0.1.1` requires published library `v0.1.0`.
Install with `@v0.1.1`. The earlier `@v0.1.0` CLI still depends on
library `v0.1.0-alpha.1`.

## Library 0.1.0 (2026-09-17)

First non-alpha library tag: `v0.1.0`. Still not a full Weavatrix Scan port.

- `EachFile` / owned content now hash and copy bytes from one open. A
  successful callback has `hash(data) == file.ContentHash`.
- Parallel inspect reserves a 64KiB ready lease, not the whole file size.
  A single item larger than the ready-byte cap fails immediately instead
  of waiting. `Group.Wait` errors are returned; reserved bytes are dropped
  on every path.
- CLI baseline: `raw_b64` is real base64; empty hashed trees stay
  `evidence=sha256`; manifest size is checked before allocating the body;
  declared hashes and trailing data are validated. `max_file_bytes` from
  config is applied. `paths --absolute` resolves the root first. `verify`
  reports inferred renames. Human stdout errors are no longer dropped.

## CLI 0.1.0 (2026-09-16)

First non-alpha CLI tag: `cmd/treestamp/v0.1.0`. That tag still required
library `v0.1.0-alpha.1`. Install with `@v0.1.0`, not `@cmd/treestamp/v0.1.0`.

## Library 0.1.0-alpha.1 (2026-09-16)

Method-complete native scanner surfaces plus a nested CLI module.
Still not a full port.

- First user CLI: `scan`, `paths`, `explain`, `diff`, `verify`, plus
  `version`, `doctor`, `config show`, and `schema`. Nested module
  `github.com/sergii-ziborov/treestamp/cmd/treestamp`. Cobra stays out
  of the library `go.mod`. Manifest schema `treestamp.manifest/v1`.
- `--scope` is `Filters.LocationInclude`. It does not reuse override
  include rules. `--output` refuses a path inside the scan root.
- Follow canonicalizes the walk root and strips Windows `\\?\` prefixes
  so relative escape links stay resolvable. If a followed link cannot be
  opened, confinement uses the link text. Compat `relPath` keeps symlink
  names; it only resolves the root when `Rel` would escape.
- Unsorted directory listing (`File.ReadDir`, no `os.ReadDir` sort), lazy
  callback `Info`, and `WalkFS` = `fs.WalkDir`. Informal Windows medians
  now beat godirwalk `ReadDirents` and use less `B/op` than fastwalk
  cached `Stat`. Official B01–B14 stay `NOT_RUN`.
- Task-first docs, executable `Example` recipes, `examples/docquickstart`,
  and a docs/consumer CI job. Official B01–B14 stay `NOT_RUN`.
- High-level facade: `ScanWith`, `ScanPathsWith`, `EachFile`, `Compile` /
  `Plan`, `Describe`, `Explain`, `WithLogger`, `WithExtensions`,
  `WithExcludeGlobs`. Existing `Scan(ctx, root)` / `ScanPaths(ctx, root)`
  signatures are unchanged. `Options{}` is not reinterpreted as
  `DefaultOptions()`.
- New facade `err == nil` means selected work finished; unread selected
  files return `ErrPartial`. `EachFile` returns callback errors and `ErrStop`.
- `toScanOptions` defaults `Walk.MaxOpen` only; it no longer replaces the
  rest of `Walk`. Snapshot content provider freezes a file index once.

- Ignore parser with nested sources, overrides, standard skips, 265 named types.
- `Scan`, `ScanCompact`, `ScanPaths`, `ScanCached`, `ScanIncremental`, `ScanInto`.
- `ScanPaths` stays path-only: no hashes, descriptor, revision, or `max_file_bytes`.
- `ScanInto` inspects under sink backpressure and does not retain the manifest.
- Compact inspect no longer materializes a full `ScanReport` first.
- Snapshot reads verify SHA-256 or file version; stale and bounded reads fail closed.
- `ToCompact` / `IntoScanReport` / `AbsolutePath`, `DescriptorFromOptions`, fluent options.
- `SelectionMatcher.Matched` / `Refresh`, ordered parallel pull, `FileWalker`.
- SHA-256, `fp128` fingerprints, revision, descriptor v2 (policy feed, not JSON).
- `ParallelWalker` visit/collect/pull, `Walk` / `WalkUnsorted` / `ErrSkipFiles`.
- Cache v2, sessions, watch plans, typed rescan reasons, content visit.
- Pinned Rust fixture differential covers raw walks, `ScanPaths`, `Scan`, and
  `ScanCompact`; Windows/NTFS and Linux/overlayfs campaigns pass exact hashes,
  descriptor v2, revision, skip order, and ignore-source evidence. Linux also
  passes a follow-links raw walk against the Rust oracle.
- Isolated fastwalk/gocodewalker/godirwalk functional comparators; no
  competitor dependency was added to the runtime module.
- Fixed parallel `WalkWithConfig` callback-error propagation, deterministic
  sorted configuration, genuinely unsorted `CollectParallel`, descriptor v2
  file-type framing, repository-relative ignore evidence, and compact skip
  ordering.
- Fixed parallel symlink root-follow, path-escape, ancestor-loop, and dangling
  target handling, plus Unix path/file identity lookup.
- Added selective directory-symlink traversal through `ErrTraverseLink`,
  `WalkTraverseLink`, and `Walker.TraverseCurrentSymlink`, retaining all
  Treestamp safety guards.
- Added deterministic arbitrary-`fs.FS` traversal through `WalkFS` and the
  pull-based `NewFSWalker`, with `fs.WalkDir` callback parity.
- Added fastwalk-compatible callback `DirEntry`, `StatDirEntry`, and
  `DirEntryDepth`; target metadata and target errors are cached and reused by
  selective traversal.
- Added a godirwalk-class lazy `DirScanner` and reusable scratch-buffer
  `ReadDirentsScratch` / `ReadDirnames`. Official B01–B14 remain `NOT_RUN`.
- Optional `.gitmodules` handling via `WithGitModules` (off by default; not
  added to the first ignore preset).
- Minimum Go is 1.23.0.
- `VisitContentManifest` now returns the same-pass visit manifest.
- Strict reads bind opened identity/mtime; snapshot reads confine canonical
  paths and compare snapshot mtime; cache reuse follows the current policy;
  mid-file `ContentQuit` no longer commits the file.
- Optional `.gitmodules` handling via `WithGitModules`; default stays off.
- Minimum Go is 1.23.0.
- `VisitContentManifest` now returns the same-pass compact manifest.
- Strict reads bind opened identity/mtime; snapshot reads confine through
  intermediate directory links and compare snapshot version before hashing.
- Content visit commits a file only after `FileEnd`; mid-chunk `Quit` does not.
- Cache reuse requires the current hash/binary policy plus cache root/format.
- Functional parity now includes cache reuse and watch-plan sequences against
  the pinned Rust driver, and the campaign runs in the Windows/Linux/macOS CI
  matrix. Official B01–B14 benches remain `NOT_RUN`.
- Optional fsnotify module and official benches remain open.
- `ScanPaths` lists from directory names and loads ignore files only when
  they appear in that listing; `DirScanner` batches reads and skips eager
  `Stat`. Informal go-compat medians are in `bench/go-compat/BENEFITS.md`.
- Path-only discovery walks files first then directories, restores ignore
  state only after a listing actually loaded rules, and skips matcher work
  when the ignore engine is idle.
- `VisitChangedContent` visits only `WatchPlan.Changed`.
- Watch apply confines `..`, records Lstat failures as skips, stamps candidate
  versions, and replaces skip/warning evidence under changed prefixes.
- `Runtime.Run` / `Group.Go` wait for admission; caller-runs is opt-in via
  `OverflowCallerRuns`. `ParallelWalker` admits workers through the executor.
- Public `encoding/json` goldens for `ScannedFile` and `ScanCache`.
- `Runtime.WithAdmitTimeout` / `AdmitWait`; multi-root scans share a dedicated worker budget.
- Declarative `Filters` (name/dir/regex/location), full `FileWalker` gocodewalker-class fields, `FindRepositoryRoot`, `WalkDirs` hooks, parallel files-first/dirs-first.
- Informal competitor benches and a method table live in `bench/go-compat/BENEFITS.md` and `COMPETITORS.md`. Official B01–B14 remain `NOT_RUN`.
- Benchmark campaign remains `NOT_RUN`.
- Fail-closed regex filters, literal `.gitmodules` paths, honest watch
  completeness, ancestor ignore on changed/watch paths, and snapshot identity
  checks.
- Compile override includes into `MayContainMatch` prefixes; `ScanPaths` /
  `FileWalker` prune unrelated subtrees when skip lists are off.
- `FileWalker` streams paths during discovery; content reuse is keyed by
  filesystem identity.
- `Scanner.Explain` reports the winning ignore/override source, pattern, and
  line.
- Shared `runtime.Budget` for roots/directory/metadata/content and ready
  count+bytes. Ordered parallel pull lists directories concurrently and emits
  serial DFS order.
- Persistent keyed Merkle `TreeSnapshot` / `TreeRevision` (`tree2:`). Legacy
  flat `sha256:` revision is unchanged.

## 0.1.0-alpha.0 (2026-09-14)

Personal public repository `sergii-ziborov/treestamp` started.

- Pinned weavatrix-scan 0.5.2 at `29c003a6ad541c9a10faf30505235375fa78b9d8`.
- Recorded 108 crate-root exports and T01–T35 contracts.
- Implemented an iterative serial walker and serial `WalkBuilder`.
