# Migrating from Go walkers

These are method-matched listing notes, not a claim that Treestamp is a
drop-in speed replacement. Official first-campaign rows are `MEASURED` in
`compat/results/official-benches.json`; they are not a 1M ranking.

## fastwalk

Use `Walk` / `WalkUnsorted` / `WalkParallel`. Callback entries implement
`DirEntry` with cached `Stat()` and `Depth()`. `ErrTraverseLink` selects one
directory symlink. `WalkFS` covers arbitrary `fs.FS`.

## gocodewalker

Use `ScanPaths` or `ScanPathsWith` plus `Filters` / `WithExtensions` /
`WithExcludeGlobs`. That listing does not hash. For a hashed manifest use
`ScanWith`. `FileWalker` streams paths during discovery.

## godirwalk

Use `WalkDirs` with `DirWalkOptions`. Default order is lexical DFS.
`Unsorted` only drops that sort. `SkipThis` skips one node; on a file it
does not drop siblings. `ErrorCallback` sees OS and callback errors
(`nil` continues). Callback paths keep the cleaned root form. A file
root needs `AllowNonDirectory`. Unsorted serial walk streams children
from `DirScanner` so `SkipAll` / `Context` can close the listing early.

```go
import godirwalk "github.com/sergii-ziborov/treestamp/compat/godirwalk"
```

`Walk`, `Options`, `Dirent`, `Scanner`, `ReadDirents`, and `ReadDirnames`
run on this engine. There is no runtime dependency on
`karrick/godirwalk`. The alias allocates extra `Dirent` wrappers;
`WalkDirs` is the faster native call. `PostChildrenCallback` works with
`FollowSymbolicLinks` on the serial path. Parallel still needs explicit
`NumWorkers`.

## Standard library

`filepath.WalkDir` is not this library. The serial walker is iterative and
bounds open directories.
