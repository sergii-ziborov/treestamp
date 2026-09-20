# Changelog

## Library 0.1.5 (2026-09-20)

Walk and listing close-out after published `v0.1.4`. Library tag is
`v0.1.5`. Official B01–B14 JSON is unchanged. This is not a claimed
15–25% or 50% win over fastwalk v1.0.14.

`Walk` / `WalkDirs` keep persistable listing entries (WalkDirs uses one
`[]Entry`). Parallel unsorted walk streams those entries and attaches
listing `FileInfo` without `Stat` on a Linux lazy dent. Workers start
only when more than one directory remains; an explicit `NumWorkers` is
not clipped to the native default cap of 8. `ReadDirnames` uses
`Readdirnames(-1)`. Local `SortMode` includes `SortDirsFirst` (directory,
regular file, other) and serial `Walk` honors it. Ordered pull reserves
a ready count slot before listing, then adds bytes; `ParallelWalkIter.Err`
surfaces a failed `IntoIterOrderedBounded` admit. `Options.IgnoreRules`
and `FileWalker.CustomIgnorePatterns` are ignore patterns, not inverted
overrides. Empty `IgnoreRules` keep the oracle descriptor v2 hash.
`compat/fastwalk` keeps a relative root relative and always sets
`FollowOutside` so `ErrTraverseLink` can leave the root. Informal
`bench/go-compat` medians were remasured on this tree and written into
README.

Serial `Walk` / `WalkDirs` now keep persistable listing entries instead of
`Clone` on every callback. Parallel unsorted walk attaches listing
`FileInfo` and starts workers only when more than one directory remains.
Informal `bench/go-compat` medians were remasured on this tree and written
into README. Official B01–B14 first-campaign JSON is unchanged. This is
not a claimed 15–25% win over fastwalk v1.0.14.

Windows serial listings now keep FindFirstFile `FileInfo` on `Record` and
`Scanner`, so callback `Stat` does not `Lstat` every name. Informal
`bench/go-compat` medians were remasured on 20 September 2026 and written
into README. Official B01–B14 first-campaign JSON is unchanged. This is
not a claimed 15–25% win over fastwalk v1.0.14.

Comparative release notes pin the existing official B01–B14 receipt and
the 20 September informal remasurement. Official B01–B14 JSON is not rewritten.
The README leads with a walk, then those receipts, then scan/CLI.
`compat/fastwalk` is the import migration; `consumer/` walks that
import without a competitor require. The main module still requires
only `golang.org/x/sys`. This is not a claimed 15–25% win over
fastwalk v1.0.14. `v0.1.4` is not retagged.

`CollectMetadata` takes size, mtime, hidden, and identity from the
enumeration `FileInfo` when that value already carries them. It does
not open a second handle only to fill `FileVersion.Identity`. Windows
FindFirstFile/Lstat has no file index, so identity stays unset there
until a real handle path (`PathIdentity`, follow, same-filesystem).
Content inspect copies identity from the already-open hash handle so
hardlink reuse still works on Windows.
Darwin and Windows keep the stdlib `ReadDir` batch path; Linux
getdents is not claimed on those OS. Native walk worker default stays
capped at 8 on every OS and is not the fastwalk Darwin 4/6/10 table.
`go 1.21.0` is unchanged. This is not a claimed speed win.

`IntoIterOrderedBounded` no longer starts a bypass goroutine when
the executor rejects every worker; `TryIntoIterOrderedBounded`
returns that admission error. Consumed directory listings are
dropped after emit, and ready-byte credits are released on
consume. Cancel wakes cond waiters, closes the budget, and waits
for workers. `Close` is safe on a failed iterator. A single
listing larger than the ready-byte cap fails instead of growing
past the limit. This is not a claimed speed win.

Parallel unsorted callback walk streams a getdents or `ReadDir`
batch into an owned persistable entry and invokes the callback
without a `[]Record` copy, a second `[]DirEntry` slice, or a
pool `Clone`. `SortMode` still buffers that directory so it can
reorder it. This is not a claimed speed win over fastwalk.

`compat/fastwalk` is an import-compatible entry for charlievieth/fastwalk
v1.0.14 on this engine: `Walk(*Config, root, fn)`, `SortMode`,
`DefaultNumWorkers`, `DefaultToSlash`, `Config.Copy`, `DirEntry`,
`EntryFilter`, and the `Ignore*` wrappers. `fs.SkipAll` is returned as
an error there. `Follow` may leave the original root. Native
`Config.Sort` (bool) is still serial global DFS; `Config.SortMode` is
local and may stay parallel. Native `Walk()` is unchanged (serial).
`KeepSkipAll` and `FollowOutside` are the explicit engine knobs.

Walk `IgnoreDuplicateFiles` / `IgnoreDuplicateDirs` now key on native
file identity (device+inode, or Windows volume/index), not path+mtime.
`IgnoreDuplicateDirs` still shows a directory alias and requests
`ErrTraverseLink`; it does not walk the same object twice. A second
hardlink is one object; two files with the same bytes stay two files.
`NewEntryFilter` is the reusable form. Identity errors are not treated
as proof of a duplicate. `listwalk.Entry` caches Info/Stat success and
error under an atomic cache; `Clone` does not copy sync objects.
Parallel `ErrSkipFiles` reads the skip map under the same mutex as
writes.

README, GitHub About, and the CLI release notes now name the library,
`cmd/treestamp`, and `cmd/treestamp-driver` separately, with install
commands and a pkg.go.dev link. The driver is documented as a fixture
binary, not the product CLI.

Library scan is a Weavatrix Scan port with Go-side additions. `ScanFS` /
`ScanPathsFS` / `EachFileFS` apply the same ignore, filter, skip, hash,
and binary rules over an `fs.FS` without OS identity or cache reuse.
Content reads stop at the discovered size (and `MaxFileBytes` when set);
an extra byte is `concurrent_modification` or `oversized`. `size==0` with
an unlimited byte cap still reads to EOF. Hashing also stops when
`Limits.Timeout` is reached. `MultiScanReport.Revision` hashes ordered
`(root, revision, complete)` tuples and does not smash colliding
relatives into one report.

`ScanPaths` / `ScanPathsFS` now return the selected prefix with
`ErrPartial` when `MaxEntries` hits or discovery is incomplete.
`--jobs` sets content workers and walk admission. `VisitContent`
calls `factory(worker)` and does not retain hashed files in
`VisitStreaming`. `inspectCompact` uses the same parallel inspect
path. `BuildParallelOrdered` pulls from `IntoIterOrderedBounded`.
`WalkParallel` keeps the first callback error under a mutex.
`watch.Open` adds every directory and returns `ErrClosed` after
`Close`. `--profile artifact` is `artifact-v2` (VCS skips only);
`artifact-v1` snapshots keep generated-directory skips.
`verify` / `diff --null` write raw paths. Official B01–B14 numbers
stay the first-campaign receipts; the harness now calls compact,
mutates before cache reuse, and uses two roots.

`WalkDirs` is the first godirwalk replacement step: default order is
lexical DFS, `Unsorted` only drops that sort (callbacks stay serial),
saved `DirEntry` values keep their names after return, and
`PostChildrenCallback` fires for the root after its children. Explicit
`NumWorkers` still opts into parallel callbacks. `SkipThis` skips one
node without dropping file siblings. `ErrorCallback` now sees both OS
and user-callback errors (`nil` continues, a non-nil value halts).
`ScratchBuffer` and `AllowNonDirectory` are on `DirWalkOptions`.
Callback paths keep the cleaned root form instead of forcing
absolute. `PostChildrenCallback` works with `FollowSymbolicLinks`.
`compat/godirwalk` is an import-compatible entry on this engine, not
a wrapper over karrick/godirwalk. Serial walk no longer treats a zero
dirent type as missing metadata (that is a regular file) and only
lstats `UnknownType`. `ReadDirnames` on Linux reads names from
getdents without a `[]Record`. `WalkDirs` keeps owned records instead
of a second `[]os.DirEntry` copy. Unsorted serial walk streams children
from `DirScanner` so the first callback can run before the rest of the
directory is listed; `SkipAll`, `SkipDir` on a file, and `Context`
close the listing without draining it. Nested lazy reads keep at most
`MaxOpen` directory FDs (default 64). Sorted and contents-first walks
still buffer a directory so they can reorder it. Informal WalkDirs
benches against godirwalk v1.17.0 live in `bench/go-compat/WALKDIRS_G05.md`;
they are not official B01–B14 rows and do not mix sorted with unsorted.
CI on stable Go uploads `count=1` WalkDirs logs for Linux, Windows, and
macOS. `python tools/run_informal_benches.py --walkdirs` refreshes the
Windows receipt without rewriting `INFORMAL_RUN.json`. That receipt
now covers Walk, SkipThis, PostChildren, ReadDirents, ReadDirnames,
DirScanner, and NewDirent; `TestGodirwalkAllMethodsParity` checks
those against godirwalk v1.17.0. Docs name `WalkDirs`, `SkipThis`,
`ReadDirnames`, `DirScanner`, and `compat/godirwalk` from README,
`MIGRATING.md`, and `docs/recipes/walk-dirs.md`.

`verify` and `diff` print every changed path in the human card, not a
preview of eight. `scan` lists selected names when there are 20 or
fewer. `scan --format ndjson` emits `scan_begin`, each
`file_committed`, then `scan_end` from `ScanInto` instead of printing
after a full in-memory manifest. Default `repo` stays the Weavatrix
source profile. This is not a hashdeep codec. Human `scan` lists
selected names above `Dropped`, hides the revision unless `--output`
is set, and drops the save hint. `explain` says `Would keep` when the
path is not on disk. `paths` / `explain` / `config` no longer
advertise scan-only flags. Human `scan` names dropped files under
`Dropped`. Default `config` omits `Hash contents` when hashing is on.
README CLI cards match that human output; examples show CI, `explain`,
`paths`, and artifact scans.

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
