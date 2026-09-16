# Portable report

Task: export relatives and hashes without the host root.

Canonical example: `ExampleScanReport_ToPortable`.

```text
go test -count=1 -run ExampleScanReport_ToPortable ./dx
```

Expected: `a.txt true`.

Portable is not anonymous. Relative names can still identify a project.
Case-sensitive names stay case-sensitive. Do not attach file contents or
absolute paths to an issue by default.
