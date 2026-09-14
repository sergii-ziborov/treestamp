# watch

Optional later Go module for an fsnotify event adapter.

It must not become a required dependency of `github.com/sergii-ziborov/treestamp`.
fsnotify is an event source, not a scanner, and its public API does not
recursively watch a whole tree by itself.
