# treestamp-driver

Fixture protocol driver for Treestamp parity tests. It is **not** the
product CLI and not a published Go module.

| Want | Use |
| --- | --- |
| User command `treestamp` | [`../treestamp`](../treestamp) — `go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.4` |
| Go library | [`github.com/sergii-ziborov/treestamp`](https://pkg.go.dev/github.com/sergii-ziborov/treestamp) |

This binary reads JSON operations on stdin (scan, session, watch-plan
apply) so `tools/run_functional_parity.py` can compare Treestamp to the
pinned Weavatrix Scan oracle. It lives in the **library** module, not in
`github.com/sergii-ziborov/treestamp/cmd/treestamp`.

```text
go build -o treestamp-driver ./cmd/treestamp-driver
```

Do not `go install` this path. Do not attach it to a GitHub CLI release.
There is no `cmd/treestamp-driver/v…` tag.
