# WalkDirs G05 informal evidence

This is **not** an official B01–B14 campaign. It does not replace the
informal ranking in `BENEFITS.md` / `INFORMAL_RUN.json`.
`compat/results/official-benches.json` is unchanged.

## Work

`TestGodirwalkAllMethodsParity` compares Treestamp and
`compat/godirwalk` to karrick/godirwalk v1.17.0 on the same tree:

| Method | Result |
| --- | --- |
| Sorted `Walk` / `WalkDirs` | Same lexical DFS paths |
| `SkipThis` on a file and a directory | Same remaining nodes |
| `PostChildrenCallback` including root | Same post order |
| `ReadDirents` / `ReadDirnames` / `Scanner` | Same names and mode types |
| `NewDirent` flags | Same `Name`, `IsRegular`, `IsDir`, `IsSymlink`, `IsDevice`, `ModeType` |
| File root without `AllowNonDirectory` | Both reject with `cannot Walk non-directory` |
| `ErrorCallback` Halt / SkipNode | Halt returns the callback error; SkipNode continues |
| `MinimumScratchBufferSize` | Same floor |
| `FollowSymbolicLinks` | Same relatives when the host can create a symlink |
| Unsorted competitor `Walk` | Skipped on Windows (`EOF`); not rewritten as sorted |

## Speed contract

Each row is one order, one ownership, one concurrency:

- Sorted `WalkDirs` vs sorted godirwalk v1.17.0. Owned persistable
  `DirEntry` versus godirwalk's borrowed `Dirent`.
- Unsorted `WalkDirs` vs unsorted godirwalk only where godirwalk's
  `Unsorted` succeeds. On Windows, godirwalk v1.17.0 `Unsorted` returns
  `EOF`; that row is skipped, not rewritten as sorted.
- `SkipAll` after the first file is a Treestamp control (early close).
  It is not compared to godirwalk Halt, which returns a callback error.
- `SkipThis` and `PostChildrenCallback` are matched against godirwalk.
- Listing methods use a 4000-name wide directory. Walk methods use 400
  files + `sub`.
- `compat/godirwalk` is the same engine as `WalkDirs`, not a wrapper
  over `karrick/godirwalk`. The alias pays a `Dirent` wrapper.

Corpus host class is Windows/amd64, Intel Core Ultra 7 255U. These
WalkDirs rows are from the same 20 September remasurement as
`INFORMAL_RUN.json`. Do not treat them as a claimed ranking over
godirwalk v1.17.0. Official B01–B14 stay the 17 September first campaign.

Reproduce:

```text
set CGO_ENABLED=0
set GOWORK=off
python tools/run_informal_benches.py --walkdirs
```

Linux and macOS `count=1` logs are CI artifacts `walkdirs-g05-<os>-stable`.
Those are smoke timings, not medians of three.

## Windows medians (`count=3`, same INFORMAL_RUN remasure)

Walk (400 files + `sub`):

| Case | Treestamp | godirwalk v1.17.0 | Note |
| --- | --- | --- | --- |
| Sorted `WalkDirs` | 368 µs, 208 KiB, 1252 allocs | 419 µs, 138 KiB, 1631 allocs | Same lexical DFS. Owned entries cost bytes. |
| Unsorted `WalkDirs` | 348 µs, 158 KiB, 1249 allocs | not compared | Competitor `Unsorted` is `EOF` on Windows |
| Unsorted `SkipAll` | 188 µs, 46 KiB, 534 allocs | not compared | Listing stops; not Halt-vs-Halt |
| `SkipThis` on `sub` | 832 µs, 226 KiB, 1259 allocs | 707 µs, 156 KiB, 1635 allocs | Same remaining nodes |
| `PostChildrenCallback` | 373 µs, 214 KiB, 1259 allocs | 447 µs, 227 KiB, 2033 allocs | Same post order |
| `compat/godirwalk` Walk | 486 µs, 265 KiB, 2057 allocs | — | Import switch; Dirent wrappers |

Listing (4000 names):

| Case | Treestamp | godirwalk v1.17.0 | Note |
| --- | --- | --- | --- |
| `ReadDirnames` | 2.09 ms, 802 KiB, 8022 allocs | 1.68 ms, 800 KiB, 8022 allocs | Alloc-for-alloc on Windows |
| Scratch `ReadDirents` | 1.88 ms, 736 KiB, 8021 allocs | 2.10 ms, 956 KiB, 12023 allocs | P02 name path is Linux getdents |
| `DirScanner` | 1.71 ms, 648 KiB, 8152 allocs | 2.01 ms, 564 KiB, 12008 allocs | Lazy one-directory enumerator |
| `NewDirent` | 21 µs, 464 B, 4 allocs | 22 µs, 480 B, 4 allocs | One `Lstat` |

`official_benches` stays `NOT_RUN` in `WALKDIRS_G05.json`.
