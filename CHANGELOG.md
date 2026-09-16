# Changelog

## CLI 0.1.0 (2026-09-16)

First non-alpha CLI tag: `cmd/treestamp/v0.1.0`. The library it requires
is still `v0.1.0-alpha.1`. Install with `@v0.1.0`, not `@cmd/treestamp/v0.1.0`.

## Unreleased — library 0.1.0-alpha.1 (2026-09-16)

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
