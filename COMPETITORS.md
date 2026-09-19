# Competitors

Treestamp is compared as a future scanner, not as a published winner. Observed
versions below were checked on 14 September 2026. Pages can lag. P0 may pin
exact tags or commits; a newer stable release is added as a new line, not by
editing history.

This repository is personal (`sergii-ziborov`). Competitor libraries stay
third-party oracles. None of them is hosted under this account as a fork
required for the port.

## Participants

| Participant | Role | Pin observed 2026-09-14 |
| --- | --- | --- |
| `filepath.WalkDir` | Required stdlib baseline | Go 1.23+ (use WalkDir, not only the older Walk) |
| `fs.WalkDir` | `io/fs` baseline | Same real filesystem adapter; do not score MemFS against disk |
| Manual iterative `File.ReadDir(n)` | Minimum-overhead control | Not a product |
| [charlievieth/fastwalk](https://github.com/charlievieth/fastwalk) | Main parallel Go comparator | **v1.0.14** (2025-09-10). Latest tagged release found. Measure unordered callbacks and collect+sort separately. |
| [boyter/gocodewalker](https://github.com/boyter/gocodewalker) | Main repository-selection Go comparator | **v1.5.1** on pkg.go.dev. Later untagged commits exist (including 2026-06-25 global ignore work). Do not treat a pseudo-version as the stable pin without writing a new line. Match ignore sources and scope before comparing. |
| [karrick/godirwalk](https://github.com/karrick/godirwalk) | Extra Go comparator | **v1.17.0**. No newer tagged release found. Measure sorted and unsorted. |
| weavatrix-scan 0.5.2 | Required Rust oracle and performance baseline | commit `29c003a6ad541c9a10faf30505235375fa78b9d8` |
| Rust `ignore` | Selection baseline | **0.4.33** from the pinned Cargo.toml. Lock before each run. |
| Rust `jwalk` | Traversal control | **0.9.0** |
| Rust `dirwalk` | Traversal control | **1.1.1** |
| Rust `walkdir` | Traversal control | **2.5.0** |
| Node/Bun `fdir` | Cross-language control | Paths and paths+metadata separately |

Do not reuse old published timings for newer versions of `ignore`, `jwalk`, or
anyone else.

Published receipts stay source-pinned. Official B01–B14:
`compat/results/official-benches.json` (first campaign, 17 September 2026).
Informal listing: `bench/go-compat/INFORMAL_RUN.json` (20 September 2026).
Do not rewrite either file without a new host run. They are not a 15–25%
claim over fastwalk v1.0.14.

## Not competitors

| Tool | Why it is listed at all |
| --- | --- |
| Git | Ignore-correctness oracle. Not a native-library speed rival. Not a runtime dependency. |
| fsnotify | Event source. Public API does not recursively watch a whole tree by itself. That work belongs to an optional adapter. |

## Composed rows

A foreign walker may appear in a full-pipeline table only with an independent
report/hash layer. Label the row `COMPOSED`. Time the whole layer. Missing
scanner guarantees stay marked. The competitor must not call Treestamp under
the hood.

## What they do not provide

fastwalk, gocodewalker, godirwalk, walkdir, jwalk, dirwalk, and fdir do not
ship the Weavatrix/Treestamp contract: normalized relatives, SHA-256,
aggregate revision, descriptor v2, typed skip evidence, portable reports,
cache v2, or incremental rescan reasons. Capability gaps stay visible even
when a walker is faster on raw listing.

## Functional differential

The reproducible developer-only module `bench/go-compat` pins fastwalk
v1.0.14, gocodewalker v1.5.1, and godirwalk v1.17.0 without adding them to the
Treestamp runtime module. It compares normalized path/type sets, sorted DFS,
directory skip, callback stop, repository ignores, extensions, standard
directories, binary selection, symlink policies, and arbitrary-`fs.FS`
traversal against `fs.WalkDir`. Callback target-`Stat` caching and depth are
compared directly with fastwalk's `DirEntry` and helper functions.

The 15 September 2026 Windows/NTFS and Linux/overlayfs Docker runs passed every
equivalent check available on each platform. The Windows run records:

- gocodewalker v1.5.1 uses the Windows hidden attribute; Treestamp follows the
  Rust oracle and also treats dot-prefixed names as hidden when that option is
  enabled.
- godirwalk v1.17.0 `Unsorted` returns `EOF` after a successful Windows
  enumeration. Equivalent set comparison therefore uses its sorted mode on
  Windows and records the unsorted behavior separately.
- gocodewalker v1.5.1 `SetConcurrency` does not change its package-level
  eight-worker semaphore. A matched-worker benchmark must record this as
  unsupported rather than claim that the requested count was applied.
- gocodewalker include/exclude fields overwrite earlier decisions in several
  combinations. Ignore, hidden, allowlist, and denylist families must be
  measured separately instead of composing a misleading “same policy”.

Windows symlink cases are `SKIP`: fixture setup itself fails with
`A required privilege is not held by the client`, before Treestamp is called.
The same cases run in a Linux container on overlayfs and establish these
contract differences:

- Treestamp and fastwalk both support callback-selected directory symlinks
  through `ErrTraverseLink`. Treestamp still applies root-escape, ancestor-loop,
  depth, and filesystem guards; fastwalk assigns cycle prevention to the caller
  for this selective mode.
- Treestamp rejects a followed directory link that escapes the root and emits
  `path_escape`; fastwalk and godirwalk traverse outside the root.
- Treestamp emits `symlink_loop`; fastwalk stops ancestor recursion without
  typed evidence, while godirwalk has no ancestor-loop guard.
- Treestamp and godirwalk report a dangling followed target; fastwalk silently
  leaves it as a link entry.
- With no child-follow option, Treestamp's default root policy still follows a
  symlink root while preserving link type in the compatibility callback.
  fastwalk reports that root as a directory; godirwalk rejects it.

Treestamp's serial `raw_walk_follow` output matches the pinned Rust oracle
exactly on the Linux fixture. See
`compat/results/windows-ntfs-functional.json` and
`compat/results/linux-overlayfs-functional.json`. These are functional
records, not timing results.

## Method surface versus pinned Go walkers

| Method family | Treestamp | fastwalk v1.0.14 | gocodewalker v1.5.1 | godirwalk v1.17.0 |
| --- | --- | --- | --- | --- |
| Callback walk / skip / stop | `Walk`, `WalkDirs`, `ErrTraverseLink` | `Walk`, `ErrTraverseLink` | channel `FileWalker` | `Walk` + `Callback` |
| Post-children callback | `DirWalkOptions.PostChildrenCallback` | no | no | `PostChildrenCallback` |
| Files-first / dirs-first | `Config.ContentsFirst` / `DirsFirst` | OS order | files then dirs | `Unsorted` only |
| Scratch / lazy dir scan | `DirScanner`, `ReadDirentsScratch` | no | no | `Scanner`, `ReadDirents` |
| Cached callback `Stat` | `StatDirEntry` | `DirEntry.Stat` | no | per-call `Stat` |
| `.gitmodules` | `WithGitModules` (off by default) | no | `IgnoreGitModules` inverted | no |
| Regex / name / dir filters | `Filters`, `FileWalker` fields | no | include/exclude fields; later include-regex overwrites name exclude | no |
| Find repo root | `FindRepositoryRoot(start)` walks up from `start` | no | `FindRepositoryRoot` walks `Getwd()` | no |
| Terminate / error handler | `Terminate`, `Walking`, `SetErrorHandler`, `ErrTerminateWalk` | cancel via callback error | `Terminate`, `SetErrorHandler` | callback error |
| Report / hash / cache / watch | yes | no | no | no |

Informal timing for the walk/select rows lives in
`bench/go-compat/BENEFITS.md`. WalkDirs-matched godirwalk rows are
`bench/go-compat/WALKDIRS_G05.md` and do not replace that table or
official B01–B14. Official first-campaign rows are `MEASURED`
in `compat/results/official-benches.json`.

## Headline policy

There is no single “overall fastest” number. Paths-only, metadata, Git
selection, hashing, full report, compact report, streaming, cache, and
incremental are different cases (B01–B14).
