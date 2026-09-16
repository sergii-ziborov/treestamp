# Compatibility

Pinned oracle: weavatrix-scan 0.5.2, commit
`29c003a6ad541c9a10faf30505235375fa78b9d8`.

Go 1.23.0 is the minimum. CI compiles the public package on 1.23.2.
`GOTOOLCHAIN=local` and empty `GOEXPERIMENT` keep generic aliases off.

A local consumer uses `replace` and `GOWORK=off`. That does not prove a
released tag. GitHub Releases was empty at the last check; do not write
`@latest` as a certified install until a tag exists.

Competitor walkers are isolated in `bench/go-compat`. They are not main-module
dependencies.
