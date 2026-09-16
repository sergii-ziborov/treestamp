# Why a file was excluded

Task: print the winning ignore rule.

Canonical example: `ExampleScanner_Explain`.

```text
go test -count=1 -run ExampleScanner_Explain ./dx
```

Expected: `excluded ignore_rule .gitignore 2 generated/**`.

This is selection, not content verification. The public `Explain` path does
not pass file size into the selection query and does not re-run binary or
hash checks. Do not write that it explains every reason `Scan` omitted a
file.

`Scanner.Explain` and `treestamp.Explain` share that boundary.
