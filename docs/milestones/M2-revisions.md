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

## Current D6/T30 policy approval

**representative-r3-v1 — OWNER APPROVED**, 2026-10-01, from reviewed head
`ae5e4db31274d84eb6088330f6ff85915bfbee48`. The exact
[78-fault/234-recovery matrix](../m2-r3-representative-proposal.md#6-approved-fault-matrix)
and demonstrated standard GitHub-hosted `ubuntu-24.04`/test-only VFS environment
class are adopted for D6/T30. Environment selection is resolved, while
**T18 PARTIAL; T30 BLOCKED; D6/R3 OPEN; M2 acceptance PENDING; PR #6 DRAFT;
execution slots ZERO**. Only documentation alignment/validation/commit/push is
authorized. No harness implementation, R3 execution/new slot, production or
migration changes, merge, deployment or M3. Earlier design status is historical.

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
alone do not pass it. The approved environment class is a standard GitHub-hosted
`ubuntu-24.04` runner for a public repository/$0 paid usage, test-only VFS against
the exact pinned candidate go-sqlite3/bundled SQLite, synthetic/disposable databases
and isolated runner-owned temporary storage. No personal database, owner-machine
reboot/power cycle, real-disk filling or production fault surface is permitted.
Environment selection is resolved; representative implementation, execution and
acceptance evidence are still missing. No execution slot is authorized.

The [R3 acceptance plan](../m2-test-plan.md#r3--simulated-storage-failure-and-power-loss)
explicitly maps create, patch, metadata, archive, unarchive, noop, restore,
checkpoint, autocheckpoint, migration/adoption, fresh initialization and empty-M1
initialization to their assigned matrix cases under both 512/4096 sector profiles.
Exactly 78 faults × discard/17, retain/17 and validated reorder-torn/17 = 234
recovery executions are required. All 12 post-acknowledgement combinations remain:
for each sector, checkpoint database write/sync and WAL truncate; autocheckpoint
database write/sync and WAL reset/header. A complete response counts only after
external-ledger write/flush/fsync before fault, never from a WAL marker alone.

The 2026-10-01 policy amendment replaces broader universal I/O-bucket positions,
compatible-mode multiplication and mandatory reorder seeds 29/101; historical
results/failures/timeouts and semantic qualifications remain supporting evidence,
without automatic new completeness credit. Additional acceptance expansion needs
separate owner approval. F0–F8, U0–U5 and ordinary/process-level tests are intact.

Contextarium proves application transaction/migration atomicity, correct WAL/FULL
and SQLite durability use, acknowledgements, recovery oracle, integrity/FKs and
revision/audit/replay linkage, plus replay without duplication. SQLite owns its
upstream exhaustive internal WAL/page-layout crash testing; dishonest successful
sync reports are outside the guarantee. SQLite remains authoritative and the
architecture is unchanged. This documentation adoption runs no rehearsal and
cannot close any gate or accept M2.

## Rollback
Never downgrade in a way that silently discards revision or required mutation-audit history.
