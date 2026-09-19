# Replace godirwalk

Task: walk a native tree with the godirwalk callback shape. Saved
entries keep their names. `SkipThis` drops one node, not file siblings.

Canonical example: `ExampleWalkDirs`.

```text
go test -count=1 -run ExampleWalkDirs ./dx
```

Expected: `keep.go`.

Default order is lexical DFS. `Unsorted` only drops that sort; callbacks
stay serial. `NumWorkers` is the parallel opt-in. `PostChildrenCallback`
runs after children, including the root. Paths keep the cleaned root
form (`tree/a.txt`, not an forced absolute). A file root needs
`AllowNonDirectory`.

```go
import godirwalk "github.com/sergii-ziborov/treestamp/compat/godirwalk"
```

That import is the same engine, not `karrick/godirwalk`. Prefer
`WalkDirs` when you do not need the old type names. `ReadDirnames` and
`DirScanner` list one directory; they do not walk the tree.
