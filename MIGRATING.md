# Migrating from Go walkers

These are method-matched listing notes, not a claim that Treestamp is a
drop-in speed replacement. Official first-campaign rows are `MEASURED` in
`compat/results/official-benches.json`; they are not a 1M ranking.

## fastwalk

Switch the import. Do not add charlievieth/fastwalk to the Treestamp
runtime module. Worked external consumer: [`consumer/`](consumer).

```text
- import "github.com/charlievieth/fastwalk"
+ import fastwalk "github.com/sergii-ziborov/treestamp/compat/fastwalk"
```

| Topic | fastwalk v1.0.14 | Native `Walk` / `WalkWithConfig` | `compat/fastwalk` |
| --- | --- | --- | --- |
| `SkipAll` | returned as an error | successful stop unless `KeepSkipAll` | error (`KeepSkipAll`) |
| `Follow` | may leave the original root | stays inside unless `FollowOutside` | `FollowOutside` follows `Follow` |
| `Sort` | local `SortMode`, walk stays parallel | bool `Sort` is serial global DFS | `Config.Sort` is `SortMode` |
| Dedup | device+inode (Windows volume/index) | `NewEntryFilter` / `IgnoreDuplicate*` | same wrappers |
| Default workers | Darwin 4/6/10; else clamp 4–32 | cap 8 on every OS | competitor table |
| Runtime require | that module | `golang.org/x/sys` only | none of the competitor modules |

Use `Walk` / `WalkUnsorted` / `WalkParallel`. Callback entries implement
`DirEntry` with cached `Stat()` and `Depth()`. `ErrTraverseLink` selects one
directory symlink. `WalkFS` covers arbitrary `fs.FS`.

`IgnoreDuplicateFiles` / `IgnoreDuplicateDirs` follow native identity
(device+inode, or Windows volume/index), not path+mtime. Two hardlinks
are one object; two files with the same bytes are not. Directory aliases
are still delivered, then `ErrTraverseLink` / `SkipDir` so children run
once. `NewEntryFilter` is the reusable form. Identity errors are not
treated as duplicates on the native filter.

`Config.Sort` (bool) is still the legacy **serial global DFS** switch.
`Config.SortMode` is local directory order and may stay parallel
(`SortNone`, `SortLexical`, `SortFilesFirst`, `SortDirsFirst`). Do not
set `Sort` when you only want `SortMode`. Native `Walk()` is serial
(`NumWorkers == 0`); `fs.SkipAll` stops successfully unless
`KeepSkipAll` is set. `Follow` stays inside the root unless
`FollowOutside` is set.

```go
import fastwalk "github.com/sergii-ziborov/treestamp/compat/fastwalk"
```

That import matches charlievieth/fastwalk v1.0.14 names on this engine.
It is not a runtime dependency on that module. `Walk` stats a missing
root before the callback, uses `DefaultNumWorkers` when `NumWorkers <= 0`,
keeps `SkipAll` as an error, and lets `Follow` leave the original root.
Prefer `treestamp.WalkWithConfig` for new code. Parallel unsorted
walk now delivers owned callback entries from the listing block;
local `SortMode` still buffers one directory.

`TryIntoIterOrderedBounded` fails when the executor admits no
workers. `Close` unblocks a pull that is waiting on a listing.
A directory that cannot fit the ready-byte budget returns
`ErrReadyLimit` instead of buffering it unbounded.

`CollectMetadata` no longer calls `PathIdentity` for every file.
Identity is copied from `FileInfo.Sys` when the platform left a
device/inode there. On Windows that field is absent, so
`FileVersion.Identity` stays nil unless follow or same-filesystem
opens a handle. A content hash still records identity from that
open handle, so hardlink reuse does not need a walk-time CreateFile.
Native default workers stay cap-8; Darwin defaults in
`compat/fastwalk` stay the competitor heuristic.

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
