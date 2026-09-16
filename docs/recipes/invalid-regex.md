# Invalid regex configuration

Task: fail closed on a broken filename pattern.

Canonical example: `ExampleNewScanner_invalidRegex`.

```text
go test -count=1 -run ExampleNewScanner_invalidRegex ./dx
```

Expected: `invalid` (`CodeInvalid` via `errors.As`).

`Filters` now stores the compile error. `NewScanner` and `Compile` surface it
instead of ignoring the pattern.
