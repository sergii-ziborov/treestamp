# Consumer migration

This module is an external Treestamp user. It walks through
`compat/fastwalk` and the public scan facade. It does not require
`github.com/charlievieth/fastwalk`, `karrick/godirwalk`,
`boyter/gocodewalker`, or `fsnotify`.

```text
- import "github.com/charlievieth/fastwalk"
+ import fastwalk "github.com/sergii-ziborov/treestamp/compat/fastwalk"
```

```text
go test .
```

Keep `KeepSkipAll` / `Follow` semantics from the compatibility entry.
Native `treestamp.Walk` treats `SkipAll` as a successful stop unless
you set `KeepSkipAll`. Details: [MIGRATING.md](../MIGRATING.md).

This is a compile-and-behavior migration, not a published speed win.
The library tag stays `v0.1.4` until a new tag is cut. Do not retag it.
