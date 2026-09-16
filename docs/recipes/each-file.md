# Hand owned bytes to an indexer

Task: receive verified file bytes that the callback owns.

Canonical example: `ExampleEachFile`.

```text
go test -count=1 -run ExampleEachFile ./dx
```

Expected: `a.go true`.

The `[]byte` is transferred to the caller. A callback error is not retried
and is not replayed on the next file. Return `ErrStop` to halt. Concurrent
modification is retried once before the callback runs.
