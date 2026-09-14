# Rust reference driver

Independent oracle process for the pinned weavatrix-scan commit
`29c003a6ad541c9a10faf30505235375fa78b9d8`.

It does not change scanner algorithms. It reads the fixture-protocol JSON from
stdin and writes JSON to stdout.

```text
cargo run --release --manifest-path reference/rust-driver/Cargo.toml
```

Rust is a developer dependency. Treestamp runtime users do not need it.
