# Walk a tree (fastwalk-shaped)

Task: walk a native tree with a `WalkDirFunc`. Parallel callbacks are an
explicit `NumWorkers`. Saved entries keep their names.

Canonical example: `ExampleWalkWithConfig`.

```text
go test -count=1 -run ExampleWalkWithConfig ./dx
```

Expected: `a.go,b.go`.

`Walk` is serial. `Config.Sort` is legacy serial global DFS.
`SortMode` is local directory order and may stay parallel. `fs.SkipAll`
stops successfully unless `KeepSkipAll` is set.

```go
import fastwalk "github.com/sergii-ziborov/treestamp/compat/fastwalk"
```

That import matches charlievieth/fastwalk v1.0.14 names on this engine.
There is no runtime dependency on that module. Prefer `WalkWithConfig`
for new code. A worked external module: [`consumer/`](../../consumer).
Semantic table: [MIGRATING.md](../../MIGRATING.md).

This recipe is not a claimed speed win.
