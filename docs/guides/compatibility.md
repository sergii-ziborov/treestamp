# Compatibility

Pinned oracle: weavatrix-scan 0.5.2, commit
`29c003a6ad541c9a10faf30505235375fa78b9d8`.

Go 1.21.0 is the minimum (`log/slog`, `min`/`max`). That is the oldest
toolchain that can compile the public API. CI compiles 1.21.x through
1.26.x on Linux, and 1.21.x plus the current stable on Windows/macOS.
`GOTOOLCHAIN=local` and empty `GOEXPERIMENT` keep generic aliases off.
`Plan.Files` is a yield callback so 1.21 does not need the `iter` package;
Go 1.23+ can still range over it. `golang.org/x/sys` stays on v0.30.0
(last line whose own `go` directive is below 1.23).

A local consumer uses `replace` and `GOWORK=off`. That does not prove a
released tag. GitHub Releases was empty at the last check; do not write
`@latest` as a certified install until a tag exists.

Competitor walkers are isolated in `bench/go-compat` (Go 1.23+; gocodewalker
declares 1.23.0). They are not main-module dependencies. That module is
tested with `GOWORK=off` and is not in `go.work`.
