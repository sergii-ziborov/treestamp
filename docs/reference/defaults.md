# Defaults

`DefaultOptions()` (and therefore `ScanWith` / `Compile`):

- `max_file_bytes=1_500_000`
- hash and binary detection on
- complete evidence
- standard skips on, hidden skip off
- ignore files: `.gitignore`, `.ignore`, `.weavatrixignore`
- cache Fast, content Strict, streaming discovery
- whole-scan entry/byte/time limits off

`MaxFileBytes == 0` stays unlimited for legacy `Options`. A true zero-byte
cap is `WithReadLimit(ZeroBytes())`.

`Options{}` is not `DefaultOptions()`.

`.treestampignore` is not in the first preset.

Descriptor v2 is a canonical byte feed, not a JSON hash. Cache format is 2.
