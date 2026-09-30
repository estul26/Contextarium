# M2 — Revision Engine + Minimal Mutation Audit

## Scope
- immutable per-record revisions
- unique `(record_id, revision_number)`
- current-revision relationship
- base-revision checks inside the mutation transaction
- restore-as-new-revision
- atomic current-state + revision mutation
- minimum append-only mutation audit event persisted in the same transaction
- explicit development/test actor attribution before M3 authentication exists

## Non-goals
No proposal workflow, client permissions, client credential authentication, audit query API, denied-attempt audit policy, or full audit reporting.

## Safety gate
M2 data remains development/pre-release data. The revision and required mutation-audit invariants become mandatory for all revision-bearing writes from this milestone onward.

M2 audit events identify a development/test actor from the local harness/internal call context. They must not claim authenticated client attribution. Authenticated client attribution becomes mandatory when M3 introduces authentication; earlier events retain their original attribution.

## Data changes
Revision storage, current-revision constraints, and the minimal mutation-audit storage needed for atomic guarantees.

## API changes
Revision read/restore endpoints. Any implemented endpoint must have explicit request/response/error semantics before code is accepted.

## Security
Reject malformed/stale revision identifiers. A failed required audit write must roll back the mutation. Development/test actor attribution must be explicit and distinguishable from later authenticated-client attribution.

## Tests
- monotonic revisions
- unique revision/head constraints
- simultaneous stale writes
- restore preserving history
- required audit linkage
- explicit M2 development/test actor attribution
- no M2 event falsely labeled as authenticated
- transaction rollback on record/revision/audit failure
- contention and crash-recovery behavior appropriate to SQLite

## Acceptance
No accepted revision-bearing mutation can silently erase history or commit without its required mutation audit event, and M2 audit attribution truthfully reflects that client authentication does not yet exist.

## Rollback
Never downgrade in a way that silently discards revision or required mutation-audit history.
