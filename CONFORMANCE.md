# Conformance

Pinned source: weavatrix-scan 0.5.2 at
`29c003a6ad541c9a10faf30505235375fa78b9d8`
(tree `108c59e66c90c3b649b3ff867360a6c0e9ff7f6e`).

Crate-root exports: 108 names in `src/lib.rs` (107 always visible, plus
`RayonExecutor` behind `feature = "rayon"`). See `compat/inventory.json`.
That file is a regex extraction. Finishing a reviewed member list is still a
P0 condition for calling the inventory complete.

## Contracts

`compat/contracts.json` lists T01–T35. After P1:

| IDs | Status |
| --- | --- |
| T01, T02, T03, T07, T08 | Implemented (serial only) |
| T11–T15 | Partial platform/path/symlink |
| T04–T06, T09–T10, T16–T35 | Not implemented |

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

Controlled clocks and fake filesystems belong to deterministic cancel and
mutation cases. A live writer racing two engines cannot be required to match
partial snapshots.

Git is an ignore oracle, not a runtime dependency.
