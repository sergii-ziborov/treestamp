# Conformance

Pinned source: weavatrix-scan 0.5.2 at
`29c003a6ad541c9a10faf30505235375fa78b9d8`
(tree `108c59e66c90c3b649b3ff867360a6c0e9ff7f6e`).

Crate-root exports: 108 names in `src/lib.rs` (107 always visible, plus
`RayonExecutor` behind `feature = "rayon"`). See `compat/inventory.json`.
That file is a regex extraction. Finishing a reviewed member list is still a
P0 condition for calling the inventory complete.

## Contracts

`compat/contracts.json` lists T01–T35. After P2–P6 method work:

| IDs | Status |
| --- | --- |
| T01–T10, T16–T29, T31–T33 | Implemented or partial with method surface present |
| T11–T15 | Partial platform/path/symlink/reparse |
| T24 | Default scan differential passes on Windows and Linux; Linux follow-links walk also passes; broader option/CI matrix remains open |
| T30 | Executors present; admission timeout not wired |
| T34 | Optional fsnotify module not started |
| T35 | Native tests exist; official benches/fuzz/rust diffs remain open |

`python3 tools/audit.py --require-full` must fail while any contract is open.

## Language adaptation

Go does not copy Rust syntax. It must copy behavior.

- `context.Context` on scan entry points
- errors with codes and `Unwrap`
- `io.EOF` ends a pull/walk stream
- `Close` releases handles
- Rayon is not a Go runtime dependency; external pools replace it later
- Node/npm delivery is not a runtime dependency

## Exact rules that are easy to get wrong

1. `scan_repository_paths` is path-only: sorted relatives, no full report,
   hashes, descriptor, or revision. `max_file_bytes` does not apply. A full
   Scan plus path extraction is a different function.
2. Raw walk is a separate API without repository selection.
3. Descriptor v2 hashes explicit ignore sources in author order. Scheduling
   and cancellation are excluded. Do not hash a JSON struct.
4. Rescan reasons and stable labels are part of the API:
   `Incremental`, `FullRescan:PolicyChanged`,
   `FullRescan:IgnoreInputChanged`, `FullRescan:StructuralChange`,
   `FullRescan:IncompletePreviousState`.
5. Incremental work is not wholly `O(k)`. Retained `N` records and revision
   stay in the measured cost.
6. The first compatible ignore preset keeps `.gitignore`, `.ignore`, and
   `.weavatrixignore`. Brand is not a silent format change.
7. Compact reports must not allocate a full report first. A streaming mode
   that drops the manifest cannot secretly keep a validation report in the
   same memory session.

## Tests that cannot wait until the end

Compare real records and event sequences, not only counts. Unordered streams
are multisets. Ordered streams check order. Full and compact normalize to one
logical report.

Cross-platform normalization is specified in `bench/protocol.md` before a
failing test invents it. Native-ID caches are not portable between machines.

The Windows/NTFS and Linux/overlayfs Docker functional campaigns compare raw
serial/sorted walks, `ScanPaths`, `Scan`, and `ScanCompact` against the pinned
Rust process, including exact SHA-256, content fingerprints, descriptor v2,
revision, typed skip order, and ignore-source evidence. Linux also compares a
follow-links raw walk containing an internal alias, file link, ancestor loop,
and root escape. The same campaign checks equivalent fastwalk, godirwalk,
filepath, and gocodewalker behavior through the isolated `bench/go-compat`
module. Results are `compat/results/windows-ntfs-functional.json` and
`compat/results/linux-overlayfs-functional.json`.

These are local two-platform results, not the required CI matrix. They close
neither cache/watch-sequence, mutation-race, fuzz/leak, nor performance work.
B01–B14 remain `NOT_RUN`.

Controlled clocks and fake filesystems belong to deterministic cancel and
mutation cases. A live writer racing two engines cannot be required to match
partial snapshots.

Git is an ignore oracle, not a runtime dependency.
