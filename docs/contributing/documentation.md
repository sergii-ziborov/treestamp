# Maintaining documentation

Canonical code is `dx/example_docs_test.go`. Markdown recipes name the
`Example` and the test command. Do not hand-edit code blocks that claim to
be the example.

```text
python tools/docs_check.py
go test -count=1 -run '^Example' ./dx
```

A changed public behavior needs: implementation, example, recipe, and a
changelog line in the same change. Future API stays in `design/` until it
builds and a test runs it.

`AGENTS.md` is for people changing Treestamp. `docs/agent-guide.md` is for
agents that only consume the library.
