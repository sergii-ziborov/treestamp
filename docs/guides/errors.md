# Errors and ownership

`ScanWith` / `Plan.Scan`: `err == nil` means selected work finished.
Unread selected files return the report plus `errors.Is(err, ErrPartial)`.
Policy skips are not errors.

`EachFile`: callback errors wrap as `CodeCallback`. `ErrStop` is deliberate.
Bytes passed to the callback are owned by the caller.

`ScanIntoErr`: the sink function's error is `CodeCallback`. `ScanInto` itself
only returns control codes.

Cancellation and deadline become `CodeCancelled` / `CodeTimeout`.
`CodePermission` and `CodeUnavailable` map `fs.ErrPermission` /
`fs.ErrNotExist`. Admission pressure is `CodeAdmission`.

Do not log file contents or absolute paths by default. `SafeCause` keeps the
basename. `WithLogger` stays off unless set; applications may still wrap
`Summary()` in their own slog.
