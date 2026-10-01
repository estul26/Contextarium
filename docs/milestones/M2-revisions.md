# M2 — Revision Engine + Minimal Mutation Audit

## Current implementation authorization

The owner subsequently authorized scoped M2 implementation, tests, local
validation, commits, a branch push, and a draft PR from approved documentation
checkpoint `05907846c59d909990d0c6159edce1da88dea7c6` on 2026-09-30.
D1–D5 remain the implementation specification. Final acceptance is pending;
D6/R3 remains OPEN. This authorization does not permit merging, deployment,
real personal data, M3, or provisioning/running a new R3 environment.
The earlier documentation-only approval record below is retained as history;
its implementation restriction has been superseded only for this scoped work.

Current implementation evidence is tracked in the [T01–T30 matrix](../m2-evidence.md).

## Historical documentation approval record


## Design status

**D1–D5 APPROVED — DOCUMENTATION BASELINE ONLY**

**D6 / R3 OPEN — M2 IMPLEMENTATION NOT AUTHORIZED**

The [M2 temporary contract](../m2-contract.md) and [M2 acceptance test plan](../m2-test-plan.md)
record the design against merged M1 checkpoint
`14dc96ad18810202f63d5ac590822f117a787e82`. The owner approved D1–D5 and the
consistent-snapshot observer clarification on 2026-09-30, authorizing a
documentation-only merge of PR #5. D6/R3 remains unresolved. These documents
do not authorize implementation or claim that acceptance tests have run.

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

Acceptance evidence must cover the linked test matrix: create/update/no-op head
consistency, all mutation preconditions, concurrent conflicts, replay after later
edits/restart, restore content and exact schemas/numbers, immutable-history guards,
bounded revision pagination, four-part transactional rollback (including replay),
M1 adoption/legacy keys, interrupted upgrades, and old-binary refusal. M0/M1
loopback, Host/Origin, privacy, probe, and shutdown protections remain covered.

Distinguish graceful shutdown, abrupt process termination, and simulated storage/
power-loss recovery. The M0-deferred power-loss rehearsal remains an open M2 gate
until executed in an appropriate isolated disposable environment; SIGTERM/SIGKILL
alone do not pass it. Missing environment/approval/evidence must be reported, not
silently waived. This planning pass runs no such rehearsal.

## Rollback
Never downgrade in a way that silently discards revision or required mutation-audit history.
