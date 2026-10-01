# M2 implementation evidence

**IMPLEMENTATION CANDIDATE — ACCEPTANCE PENDING; D6/R3 OPEN**

Starting checkpoint: `05907846c59d909990d0c6159edce1da88dea7c6` (approved D1–D5).
`origin/main` matched that checkpoint when implementation started. Work is on
`m2-implementation`; earlier checkouts and local artifacts were preserved.
The containing implementation commit identifies this source/test tree. The draft
PR records its exact candidate SHA and the subsequent local/Linux validation
results, avoiding a self-referential commit hash in this file.

## Execution boundary

Validation date: 2026-09-30. Local platform: macOS / darwin arm64, Go 1.26.2,
CGO enabled; go-sqlite3 v1.14.52, bundled SQLite 3.53.4. Observed settings:
foreign_keys=1, journal_mode=wal, synchronous=2 (FULL), busy_timeout=1000 ms.
The existing one-connection/private-cache/IMMEDIATE policy is retained.
Only synthetic disposable databases and test-owned processes are used.

Commands used for the matrix:

- **G**: `go test -count=1 ./...` — PASS locally.
- **R**: `go test -race -count=1 ./...` — PASS locally.
- **B**: `python3 scripts/m2-binary-check.py --binary <candidate-binary> --m1-binary <M1-binary> --m0-binary <M0-binary>` — PASS locally, with actual compiled binaries and newly allocated temporary databases.
- **V**: `gofmt -l .`, `git diff --check`, `go mod verify`, `go vet ./...`, and `go build -o <temporary-candidate-binary> ./cmd/contextarium` — PASS locally.
- **S**: `go test -v -count=1 ./internal/storage -run TestM2MigrationMetadataAndConnectionEvidence` — PASS; reports the SQLite/settings evidence above.

The M1 compatibility binary is built from
`14dc96ad18810202f63d5ac590822f117a787e82`; the M0 binary from
`3b6be6e989aa8f9e75c1ef2ea7f589db77d58329`. Linux CI retains the existing workflow
unchanged; its exact candidate/run/result is recorded in the draft PR. Ordinary
Linux CI is not R3 evidence. The Python compiled-binary rehearsal is a local
check, not an added CI job.

The rehearsal initially failed because its HTTP client did not encode Unicode as
UTF-8, then because its Python observer context manager did not close SQLite
connections before a file copy. Both harness defects were corrected and the full
rehearsal rerun successfully. It now explicitly closes observers and refuses a
closed-file copy if WAL content remains. No existing runtime data was involved.

## T01–T30 matrix

Names below are actual tests, not proposed coverage. `G/R` execute all named Go
tests and their subcases. `B` exercises real binary boundaries. A PASS applies to
the stated software/ordinary-process cases; it does not substitute for T30.

| Case | Status | Actual coverage and command | Remaining gap |
| --- | --- | --- | --- |
| T01 | PASS | `TestM2AllMetadataPathsAndCreateDomains`, `TestM2SnapshotsRestoreAndReplay`, `TestTwoIndependentDomainsAndPersistence`, `TestM2HTTPContractAndSafety` (G/R/B): nullable/full metadata, two domains, head 1, snapshot/event/key/actor links. | None for this case. |
| T02 | PASS | `TestTwoIndependentDomainsAndPersistence`, `TestM2SnapshotsRestoreAndReplay` (G/R): whole-field replacement, omission/null behavior, retained earlier snapshots. | None. |
| T03 | PASS | `TestM2NoopKindsAndFullRestore`, `TestM2PreconditionsAttributionAndFingerprints` (G/R): status/data/metadata no-ops append, matching replay does not, empty/base-only bodies reject. | None. |
| T04 | PASS | `TestM2PreconditionsAttributionAndFingerprints`, `TestM2HTTPContractAndSafety`, existing strict JSON tests (G/R): missing/null/string/bool/zero/negative/fraction/exponent/overflow/duplicate/case/If-Match; unchanged stores. | None. |
| T05 | PASS | `TestM2ControlledPoolRaces`, `TestM2IndependentProcessRaces`, `TestConcurrentPatchesPreserveOmittedFields` (G/R): distinct same-base PATCH/PATCH and PATCH/restore; one winner, one stale loser, explicit rebase. | None. |
| T06 | PASS | `TestM2AllMetadataPathsAndCreateDomains`, `TestM2PreconditionsAttributionAndFingerprints`, `TestTwoIndependentDomainsAndPersistence` (G/R): missing/stale/current bases across data, key, sensitivity, provenance, archive/unarchive, combined and archived-record changes. | None. |
| T07 | PASS | `TestM2SnapshotsRestoreAndReplay`, `TestM2HTTPContractAndSafety` (G/R), B: create/PATCH/restore original result after later edits/restart, fresh HTTP request IDs, unchanged stores. | None. |
| T08 | PASS | `TestM2PreconditionsAttributionAndFingerprints` (G/R): whitespace/key order, number spelling, changed base/field/restore target, scope independence, reusable failed key. | None. |
| T09 | PASS | `TestM2ControlledPoolRaces`, `TestM2IndependentProcessRaces`, existing retry concurrency tests (G/R): same scoped record request across pools/processes, one revision/event/effect. | None. |
| T10 | PASS | `TestM2SnapshotsRestoreAndReplay`, `TestM2NoopKindsAndFullRestore` (G/R/B): multiple states, full/null metadata, historical status/data/schema, stable identities, new actor/time/request/linkage. | None. |
| T11 | PASS | `TestM2SnapshotsRestoreAndReplay`, `TestM2PreconditionsAttributionAndFingerprints` (G/R/B): unarchive preserves current data, historical/current/identical restore appends, stale/missing bases reject, replay preserved. | None. |
| T12 | PASS | `TestM2RestoreCorruptionAndLimitFixtures`, `TestSchemaPinningAndImmutableRegistry` (G/R): incompatible v2, missing target, isolated corrupt pair/identity/schema/data fail closed. | Corrupt fixtures are test-only; ordinary paths retain all guards. |
| T13 | PASS | `TestM2ExactJSONAcrossHistory`, `TestM2AdoptionAndLegacyReplay`, existing numeric/schema-bound tests (G/R/B): exact large integer, decimal/exponent spelling, negative zero, bounded extremes, unknown arrays/Unicode across history/restore/replay/upgrade/restart. | None. |
| T14 | PASS | `TestM2RestoreCorruptionAndLimitFixtures`, `TestM2LinkageGuards`, `healthy` oracle (G/R): uniqueness/range/max overflow, controlled limit fixture, contiguous chains, current snapshot/event linkage. | Limit fixture intentionally bypasses guards only during isolated setup. |
| T15 | PASS | `TestM2ImmutableSQLHistory`, `TestM2HTTPContractAndSafety` (G/R): UPDATE/DELETE/REPLACE/upsert guards, immutable revision field, GET-only routes/405, absent audit API, unchanged state after reopen. | None. |
| T16 | PASS | `TestM2LinkageGuards`, `TestM2ImmutableSQLHistory` (G/R): missing references/events, wrong pair/ID, duplicate event, invalid base/source/actor/range, head mismatch including null, deletion/replacement, deferred commit rejection. | None within the approved application/guard boundary; privileged schema editing is outside it. |
| T17 | PASS | `TestM2HTTPContractAndSafety`, `TestM2PreconditionsAttributionAndFingerprints`, `TestM2NoopKindsAndFullRestore`, `TestM2SnapshotsRestoreAndReplay`, existing privacy tests (G/R/B): explicit actor, spoof rejection, event actions/minimal columns and no payload leakage. | Attribution remains development/test, never authentication. |
| T18 | PARTIAL | `TestM2AtomicFailureBoundaries`, `TestM2SQLWriteAndDeferredCommitFailures`, `TestM2SerializationFailureRollsBack`, `TestM2CommittedResponseLossReplay`, `TestM2CancellationBeforeCommit` (G/R): create/data/metadata/archive/unarchive/restore, F0–F6, deferred-constraint F7, postcommit F8 and lost/truncated response. | Actual storage write/sync failure during commit is not executed; it belongs to open R3. SQL errors and cancellation do not close that gap. |
| T19 | PASS | `TestM2RevisionPagination`, `TestM2HTTPContractAndSafety` (G/R): 205+ revisions, limits default/1/100, changed limit, append and restore between pages, fixed end, invalid cursors/queries/numbers, missing/empty, unchanged reads. | None. |
| T20 | PASS | `TestM2MigrationInterruptionAndRetry`, existing fresh/M0 migration tests (G/R/B): fresh/M0/empty M1 and M1 metadata/keys without records; ledger retained, no invented adoption. New record creation is exercised on fresh and upgraded stores. | None. |
| T21 | PASS | `TestM2AdoptionAndLegacyReplay` (G/R), B with actual M1 binary: active/archived records, multiple M1 edits/replays, verbatim current bytes/times, one adoption each, explicit boundary. | None. |
| T22 | PASS | `TestM2AdoptionAndLegacyReplay` (G/R), B: old create/PATCH bodies without base replay exact M1 objects after later edits/restart, contract tag m1, altered body conflict, unknown missing base rejection, subject/schema replay and original stored rows retained. | None. |
| T23 | PASS | `TestM2MigrationInterruptionAndRetry`, `TestM2MigrationCancellationAndCommitFailure`, `TestM2MigrationMidAdoptionAndDeadline`, `TestM2UpgradeCommitConstraintRollback`, `TestM2MigrationAbruptRecovery`, `TestM2StartupDeadlineDuringStorageWait` (G/R): U0–U5, mid-backfill, deferred commit failure, cancellation/inherited startup deadline, repeat/reopen. | Storage/power failure during migration is reserved for T30. Deadline tests use a shorter inherited deadline. |
| T24 | PASS | `TestM2AdoptionAndLegacyReplay` ledger-prefix checks (G/R); B actual M0/M1 refusal before listener/ready, unchanged ledger/history, separate closed-copy rollback retaining M2 evidence and explicitly omitting later M2 edits. | Connection PRAGMAs may initialize before refusal, as documented in M0. |
| T25 | PASS | `TestM2ContentionCancellationAndReplay`, existing unavailable-error tests (G/R): writer lock for new/replay requests, bounded unavailable outcome, release/retry, stale conflict, exhausted pool, cancellation/deadline, no leakage. | None. |
| T26 | PASS | Existing storage/config/schema tests plus S (G/R): bounded/replacement connections, FK/WAL/FULL/busy timeout, ledger/path/symlink rejection, retained M1 identity/schema guards. | No driver/connection policy change. |
| T27 | PASS | `TestM2HTTPContractAndSafety`, `TestAPILoopbackHostForms`, existing strict/privacy/config tests (G/R/B): new-route Host/Origin/Sec-Fetch, IPv4/IPv6, malformed ports/hosts, media/body/query limits, safe headers/errors/logs, absent deferred APIs. | None. |
| T28 | PASS | `TestM2ShutdownWithCommittedMutation`, `TestM2CommittedResponseLossReplay`, `TestM2CancellationBeforeCommit`, existing probe/drain/forced-close/startup tests (G/R/B): readiness withdrawal, drain/force-close, committed response loss and retry, resource release, graceful SIGTERM. | Graceful shutdown is not abrupt/power-loss evidence. |
| T29 | PASS | `TestM2AbruptChildRecovery`, `TestM2MigrationAbruptRecovery` (G/R): test children killed at transaction/adoption precommit and postcommit barriers, WAL retained, reopen integrity/linkage/whole-store checks and same-key retry. | SIGKILL is R2 process-crash evidence only. |
| T30 | BLOCKED | R3 not run; no isolated fault environment is provisioned or authorized. | Owner approval, harness implementation/verification, execution and durable-storage evidence required. |

## Failure points and observation

F0 precedes transaction acquisition; F1 follows validation; F2 follows revision
insertion; F3 follows the current-state write; F4 follows required audit; F5 precedes
result serialization; F6 follows replay insertion and precedes commit; F8 follows
commit before returning to the HTTP adapter. Required-write triggers inject SQL
errors during each store write; a deferred FK injects F7 commit failure. Invalid
response JSON exercises serialization failure. Internal barriers have no HTTP or
production environment control. Response writers model loss before/partway through
delivery; real loopback forced-close tests cover committed work racing disconnect.

U0 precedes upgrade acquisition; U1 follows new structures before adoption; U2 is
between revision and event backfill; U3 follows guards/backfill validation;
U4_before_ledger/U4 bracket ledger insertion before commit; U5 is after commit.
A trigger aborts partway through multi-record adoption; another creates a deferred
commit error after the ledger insertion. Cancellation/deadline tests retain no
partial upgrade. Repeating/reopening successful adoption does not relabel history.

The `observe` helper and binary rehearsal use separate physical connections with
explicit deferred `BEGIN` for all four tables. They never request the writer lock
while a paused writer owns it. Rollback checks compare complete ordered contents;
recovery checks use integrity/FK checks, chain/head/snapshot/audit linkage and replay
results. Counts alone are not used as proof of atomicity. No test disables guards
on ordinary paths; impossible-state/limit fixtures are explicitly isolated.

## Open R3 gate and proposed setup

Proposed method: a dedicated test executable with a test-only SQLite VFS wrapping
the same bundled SQLite 3.53.4 build. Run it inside an isolated disposable Linux
VM with a dedicated scratch volume. The VFS maintains a deterministic virtual
persistent image and volatile writes; controls cover xWrite, xSync, xTruncate,
WAL/checkpoint writes, injected IOERR/FULL, and discard/reorder/torn writes at
selected barriers. Recovery runs in a fresh process against the modeled durable
image, with the normal WAL/FULL/FK settings. Keep an external acknowledgement
ledger so acknowledged effects must survive, while ambiguous effects may be wholly
present or wholly absent. Compare every store, exact JSON and schema pair, history
chains and linkage; include migration, commit and checkpoint schedules. Validate
the harness using known fault schedules before relying on it.

This is a concrete proposed method, not an implemented or approved harness. It
requires separate owner approval to provision/use the isolated VM and scratch
storage, build the test-only VFS integration, and run the fault matrix. It must
never fill a real disk, touch personal/runtime databases, reboot the user's
machine, or treat process kill/SQL-error tests as proof of durable media behavior.
No setup or R3 execution occurred in this change. T18 remains partial, T30 blocked,
and overall M2 acceptance remains pending until the required evidence exists.

## Review boundary

Architecture deviations: **NONE**. D1–D5 are retained. Migration 001/002, dependencies,
accepted ADRs and CI behavior are unchanged. Malformed Host port forms are rejected
consistently while valid loopback forms remain supported. There is no M3+ feature,
authenticated actor, public audit query, deployment, or real personal data.
No currently known application defect is being waived; the open acceptance gap is
storage/power-loss evidence. The draft PR is not a request to merge.
