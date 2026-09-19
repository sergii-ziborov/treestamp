# Errors and ownership

`ScanWith` / `Plan.Scan`: `err == nil` means selected work finished.
Unread selected files, a hit scan limit, or another incomplete
termination return the report plus `errors.Is(err, ErrPartial)`.
Policy skips are not errors.

`Plan.Files` uses the same finish check after the iterator ends. A
deliberate `yield` false / `ScanSinkStop` is not incompleteness.

`EachFile`: callback errors wrap as `CodeCallback`. `ErrStop` is deliberate.
Bytes passed to the callback are owned by the caller. Evidence fields
match `Scan` (`Version`, fingerprint, `BinaryChecked`).

`ScanIntoErr`: the sink function's error is `CodeCallback`. After a
successful sink, the same finish check reports an incomplete stream.

Cancellation and deadline become `CodeCancelled` / `CodeTimeout`.
`CodePermission` and `CodeUnavailable` map `fs.ErrPermission` /
`fs.ErrNotExist`. Admission pressure is `CodeAdmission`.

Do not log file contents or absolute paths by default. `SafeCause` keeps the
basename. `WithLogger` stays off unless set; applications may still wrap
`Summary()` in their own slog.
