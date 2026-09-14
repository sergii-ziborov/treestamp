# Treestamp

Public personal repository of [Sergii Ziborov](https://github.com/sergii-ziborov).
Module path: [`github.com/sergii-ziborov/treestamp`](https://github.com/sergii-ziborov/treestamp).

This is **not** a Weavatrix organization repository and **not** an EdgeHawk
repository. Weavatrix Scan is the pinned Rust oracle being ported.

Treestamp is a native Go library for verifiable repository scanning: walk,
select, read, and produce a deterministic manifest. It is not a parser, search
engine, graph, embedder, secret scanner, MCP server, web service, or daemon.

**Status on 14 September 2026:** design, bootstrap, and a serial walker.
The scanning API is not implemented. Benchmarks are `NOT_RUN`. This is not a
finished scanner and not a full port.

## What exists now

| Surface | Status |
| --- | --- |
| Iterative serial `Walker`, `WalkBuilder` | Implemented (P1) |
| Depth, `max_open`, error policy, file root | Implemented (P1) |
| `Scan`, `ScanCompact`, `ScanPaths`, `NewScanner` | Not implemented |
| Ignore, named types, matchers | Not implemented |
| Reports, hashes, descriptor, revision | Not implemented |
| Parallel executors, cache v2, incremental, watch adapter | Not implemented |
| Official B01–B14 campaign | `NOT_RUN` |

Do not treat `filepath.WalkDir` usage elsewhere as this library. The serial
walker is iterative, bounds open directory handles, and is not a WalkDir
wrapper renamed as a port.

## Requirements

- Go 1.25.0 or newer
- `CGO_ENABLED=0` for the intended runtime
- Rust is optional and only for developers who run the pinned oracle and
  differential drivers

```text
go get github.com/sergii-ziborov/treestamp@latest
```

```go
package main

import (
    "fmt"
    "io"
    "log"

    "github.com/sergii-ziborov/treestamp"
)

func main() {
    walker, err := treestamp.NewWalker(".")
    if err != nil {
        log.Fatal(err)
    }
    defer walker.Close()
    for {
        entry, err := walker.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            log.Println(err)
            continue
        }
        fmt.Println(entry.Path())
    }
}
```

The functions below compile and return a typed not-implemented error. They
are the intended scan contract, not a working scanner:

```go
_, err := treestamp.Scan(ctx, root)
_, err = treestamp.ScanCompact(ctx, root)
_, err = treestamp.ScanPaths(ctx, root)
```

## What “full” means later

A full port repeats the Weavatrix Scan contract in Go: serial and parallel
walkers, ignore selection, full and compact reports, path-only scan without
full-scan I/O, verified content, cache v2, incremental rescan reasons, and
honest benches. It does not wrap the Rust crate, Node, a Git executable, or
another process at runtime.

Default scan options in the oracle are not “every resource limited”:
`max_file_bytes=1_500_000`, hashing and binary detection on, complete
evidence, standard skips on, hidden skipping off, repository-local ignores
`.gitignore` / `.ignore` / `.weavatrixignore`, cache Fast, content Strict,
streaming discovery. Whole-scan limits are off. The first compatible ignore
preset keeps those three file names. `.treestampignore` is added only by an
explicit or versioned preset.

Descriptor v2 is a canonical byte feed, not a JSON hash. Cache format is 2.

Pinned oracle: weavatrix-scan **0.5.2**, commit
`29c003a6ad541c9a10faf30505235375fa78b9d8`, tree
`108c59e66c90c3b649b3ff867360a6c0e9ff7f6e`.

## Documents

- [AGENTS.md](AGENTS.md) — how to continue the port
- [ARCHITECTURE.md](ARCHITECTURE.md)
- [CONFORMANCE.md](CONFORMANCE.md)
- [COMPETITORS.md](COMPETITORS.md)
- [BENCHMARKS.md](BENCHMARKS.md)
- [THREAT_MODEL.md](THREAT_MODEL.md)
- [RELEASE.md](RELEASE.md)
- Machine-readable plan: [compat/](compat/) and [bench/](bench/)

## Check the bootstrap

```text
python3 tools/audit.py
python3 -m unittest discover -s tools -p "test_*.py"
python3 tools/audit.py --require-full
```

`--require-full` must fail until every T01–T35 contract is closed with
evidence. That failure is correct.

```text
go test ./...
```

## License

MIT. Copyright (c) 2026 Sergii Ziborov. The upstream weavatrix-scan notice of
Sergii Ziborov is preserved.
