# Revision Model

## Invariants

- Record IDs are stable.
- Revision numbers increase monotonically per record.
- `(record_id, revision_number)` is unique.
- Each record has exactly one current revision pointer once created.
- Historical revisions are immutable.
- Accepted mutations create new revisions.
- Restoring old state creates a new revision.
- Conflicting base revisions never silently overwrite current state.
- Each revision pins the exact schema ID/version used to validate that authoritative state.

## Example

```text
rec_01JEXAMPLE

revision 1
    |
revision 2
    |
revision 3
    |
revision 4 <- current
```

## Optimistic concurrency

A client that read revision 4 may submit:

```json
{
  "base_revision": 4,
  "patch": {
    "data.status": "complete"
  }
}
```

If the current revision is already 5, the server returns `REVISION_CONFLICT`.

The base-revision predicate is evaluated inside the same transaction that persists the accepted mutation.

## Transaction boundary

A revision-bearing accepted change atomically persists:

- verification of the expected current revision or create precondition
- the new immutable revision
- the new current record state/current-revision pointer
- the required mutation audit event
- the proposal decision/linkage when the mutation originated from a proposal

No partial combination may become visible.

## Restore

Restoring historical state creates a new current revision. It does not rewrite or delete history.

The historical revision's exact schema ID/version is part of the restored state unless the caller explicitly performs a separately defined schema migration.

## Content integrity

Revisions may include a canonical content hash for corruption detection or verification. Canonicalization and hash rules must be explicitly defined before they become compatibility guarantees.

## Retention

Revision retention is indefinite by default in v0.1. Schema definitions referenced by retained revisions must also be retained.

Pruning or compaction requires a future ADR.
