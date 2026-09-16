# Changelog

## Unreleased — 0.1.0-alpha.2 (2026-09-14)

Method-complete native scanner surfaces. Still not a full port.

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

## 0.1.0-alpha.0 (2026-09-14)

Personal public repository `sergii-ziborov/treestamp` started.

- Pinned weavatrix-scan 0.5.2 at `29c003a6ad541c9a10faf30505235375fa78b9d8`.
- Recorded 108 crate-root exports and T01–T35 contracts.
- Implemented an iterative serial walker and serial `WalkBuilder`.
