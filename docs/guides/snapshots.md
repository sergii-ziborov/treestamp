# Revisions and snapshots

Three different values:

| Value | Meaning |
| --- | --- |
| `report.Revision` | Flat `sha256:` of the selected manifest |
| `TreeSnapshot.TreeRevision` | `tree2:` Merkle root over path/hash/size |
| Descriptor v2 | Policy feed, not a JSON hash |

`SnapshotFromFiles` builds a persistent data structure from records you
already have. It does not watch the disk. `Apply` returns a new snapshot and
leaves the previous one unchanged.

`ContentProvider` rereads files and checks version or hash. That is not a
free extract from the report.

`ScanSession` still holds a `ScanReport`. Merkle apply on the session updates
the tree when a snapshot is present; it does not make every session method
O(k).
