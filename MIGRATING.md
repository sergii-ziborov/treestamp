# Migrating from Go walkers

These are method-matched listing notes, not a claim that Treestamp is a
drop-in speed replacement. Official B01–B14 stay `NOT_RUN`.

## fastwalk

Use `Walk` / `WalkUnsorted` / `WalkParallel`. Callback entries implement
`DirEntry` with cached `Stat()` and `Depth()`. `ErrTraverseLink` selects one
directory symlink. `WalkFS` covers arbitrary `fs.FS`.

## gocodewalker

Use `ScanPaths` or `ScanPathsWith` plus `Filters` / `WithExtensions` /
`WithExcludeGlobs`. That listing does not hash. For a hashed manifest use
`ScanWith`. `FileWalker` streams paths during discovery.

## godirwalk

Use `DirScanner`, `ReadDirentsScratch`, and `WalkDirs`.
`PostChildrenCallback` requires serial, non-follow walks.

## Standard library

`filepath.WalkDir` is not this library. The serial walker is iterative and
bounds open directories.
