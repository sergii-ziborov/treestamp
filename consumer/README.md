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
The library tag is `v0.1.5`. Do not retag an immutable version.
