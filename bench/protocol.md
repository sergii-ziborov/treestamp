# Driver and fixture protocol

This is the machine contract between Treestamp, the pinned Rust oracle, and later competitor adapters. It is not a scanner.

## Process shape

Two independent native executables share one corpus root on one machine.

1. Control JSON is parsed **outside** the timer.
2. The timer starts when the requested operation begins.
3. The timer ends when the **requested result** is available.
4. Raw result data and timing metadata are separate objects.
5. If serialized output was requested, that cost is either a separate row or included explicitly. Lazy export that hides work is forbidden.

The Rust adapter must not change weavatrix-scan algorithms. An experimental Rust hash adapter, if added later, is a different row.

## Request

```json
{
  "op": "raw_walk_serial",
  "root": "C:/corpus/tiny",
  "options": {
    "min_depth": 0,
    "max_depth": null,
    "max_open": 64,
    "same_file_system": false,
    "follow_links": false,
    "collect_metadata": false,
    "error_policy": "continue",
    "root_symlink_policy": "follow",
    "sort_by_file_name": false,
    "contents_first": false
  }
}
```

Supported now:

| `op` | Engine | Status |
| --- | --- | --- |
| `raw_walk_serial` | Treestamp walker / Rust `Walker` | Implemented |
| `raw_walk_sorted` | `WalkBuilder` + sort by file name | Implemented |
| `scan` | Treestamp `Scan` | Not implemented |
| `scan_compact` | Treestamp `ScanCompact` | Not implemented |
| `scan_paths` | Treestamp `ScanPaths` | Not implemented |

## Response

```json
{
  "data": {
    "entries": [
      {
        "relative": "a.txt",
        "depth": 1,
        "is_file": true,
        "is_dir": false,
        "is_symlink": false,
        "bytes": null,
        "skip_reason": null
      }
    ]
  },
  "timing": null,
  "error": null
}
```

`timing` stays `null` unless a measurement harness filled it. A fixture comparison ignores `timing`.

Relative paths use `/` only **after** comparison normalization. Drivers emit native relatives; the compare tool converts separators and does not rewrite names.

## Comparison rules

- Ordered ops: compare sequences.
- Unordered ops: compare multisets and fail on duplicates that the other side does not have.
- Errors, native identities, and timestamps are not required to share a byte layout across operating systems. Normalization is specified here, not invented after a failed run.
- A cache that stores native IDs is not portable between machines just because both sides have a JSON codec.
- A live writer racing the walk cannot be required to produce the same partial snapshot. Check invariants and incompleteness marks.

## Git oracle (P2+)

Temporary repository, controlled `HOME` / `XDG_CONFIG_HOME` / `core.excludesFile` / `core.ignoreCase`, and `GIT_CONFIG_NOSYSTEM=1`.

```
git check-ignore --no-index --stdin -z --verbose --non-matching
```

Account for negate rules, NUL fields, and exit status 1. Git is not a Treestamp runtime dependency.

## Campaign records

Each measured row must store Go/Rust/dependency versions, OS, filesystem, CPU, RAM, storage, power mode, antivirus/indexing background, warm/cold protocol, seed, corpus fingerprint, policies, and worker counts. Windows/NTFS, Linux/ext4, and macOS/APFS are separate campaigns.

States: `NOT_RUN`, `UNSUPPORTED`, `PARITY_FAIL`, `EXECUTION_FAIL`, `MEASURED`.

`speedup = T_reference / T_treestamp`. A value above 1 is a speedup. Missing support is not infinite speedup.
