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
| `filepath.WalkDir` | Required stdlib baseline | Go 1.25+ (use WalkDir, not only the older Walk) |
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

## Headline policy

There is no single “overall fastest” number. Paths-only, metadata, Git
selection, hashing, full report, compact report, streaming, cache, and
incremental are different cases (B01–B14).
