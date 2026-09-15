# Go functional comparators

This isolated developer module pins fastwalk v1.0.14, gocodewalker v1.5.1,
and godirwalk v1.17.0. The main Treestamp module does not depend on them.

```text
go test -count=1 -v ./...
```

Tests compare only equivalent work: raw path/type sets, sorted DFS,
skip/stop control, configured repository selection, and symlink follow/error
policies. Arbitrary `fs.FS` traversal is compared directly with `fs.WalkDir`
using `fstest.MapFS` and `os.DirFS`. These tests do not pretend that a walker
provides Treestamp reports, hashes, descriptors, cache, or incremental updates.

The three current capability differentials are explicit:

- selective directory-link follow versus fastwalk `ErrTraverseLink`;
- arbitrary `fs.FS` traversal versus `fs.WalkDir`;
- cached callback target `Stat` and depth versus fastwalk `DirEntry`.

Platform-specific competitor behavior is asserted and logged instead of being
normalized away. Directory-symlink tests skip when the current account cannot
create symlinks. The Linux matrix covers no-follow, in-root follow, root
escape, ancestor loop, dangling target, symlink-root behavior, and selective
`ErrTraverseLink` parity in serial and parallel Treestamp walkers, including
the direct `WalkTraverseLink` visitor control.
