# Walk an fs.FS

Task: list files in a small `fstest.MapFS`.

Canonical example: `ExampleWalkFS`.

```text
go test -count=1 -run ExampleWalkFS ./dx
```

Expected: `a.txt,sub/b.txt` in lexical DFS order.

This is the walker only. `MapFS` does not imitate native file identity,
symlinks, or repository scanning. Child symlinks are not followed, matching
`fs.WalkDir`.
