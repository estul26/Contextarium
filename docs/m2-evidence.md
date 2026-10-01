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
- **B**: `python3 scripts/m2-binary-check.py --binary <candidate-binary> --m1-binary <M1-binary> --m0-binary <M0-binary> --reviewed-m2-binary <reviewed-M2-binary>` — PASS locally, with actual compiled binaries and newly allocated temporary databases.
- **V**: `gofmt -l .`, `git diff --check`, `go mod verify`, `go vet ./...`, and `go build -o <temporary-candidate-binary> ./cmd/contextarium` — PASS locally.
- **F**: `go test -v -count=1 ./internal/engine -run 'TestM2(ControlledActionClock|ColdRestartRecovery|SnapshotsRestoreAndReplay)$'` and `go test -v -count=1 ./internal/storage -run TestM2ReviewedCandidateChecksumRefusal` — PASS locally; G/R also execute these cases.
- **S**: `go test -v -count=1 ./internal/storage -run TestM2MigrationMetadataAndConnectionEvidence` — PASS; reports the SQLite/settings evidence above.

The M1 compatibility binary is built from
`14dc96ad18810202f63d5ac590822f117a787e82`; the M0 binary from
`3b6be6e989aa8f9e75c1ef2ea7f589db77d58329`. The reviewed M2 compatibility binary is
built from an archive of `3da25a78e5100d58fb75b196bc91a85e606f0b78`. Linux CI retains
the existing workflow unchanged; its exact candidate/run/result is recorded in the draft PR. Ordinary
Linux CI is not R3 evidence. The Python compiled-binary rehearsal is a local
check, not an added CI job.

The rehearsal initially failed because its HTTP client did not encode Unicode as
UTF-8, then because its Python observer context manager did not close SQLite
connections before a file copy. Both harness defects were corrected and the full
rehearsal rerun successfully. It now explicitly closes observers and refuses a
closed-file copy if WAL content remains. No existing runtime data was involved.

## PR #6 review corrections

Review of `3da25a78e5100d58fb75b196bc91a85e606f0b78` confirmed all three findings.
Its earlier passing commands did not establish cold-restart or controlled-clock
coverage. The affected rows were held PARTIAL while corrections were in progress;
they return to PASS below only after the named regressions, uncached ordinary/race
suites and compiled-binary rehearsal executed. The approved contract is unchanged.

1. **Restore action:** the engine, migration 003 action constraint/linkage trigger,
   and regression expectations now use the approved `revision.restored`.
   `TestM2SnapshotsRestoreAndReplay`, `TestM2ControlledActionClock`, and
   `TestM2ColdRestartRecovery` assert the actual event action. Old spelling remains
   only in fixtures/checks for the incompatible reviewed candidate.
2. **Cold restart:** `TestM2AbruptChildRecovery` remains live-observer coverage,
   not proof of a sole-owner cold restart. New `TestM2ColdRestartRecovery` runs
   create/PATCH/restore at F6 and F8. Setup captures the complete four-store oracle
   and closes its pool before the mutation child opens SQLite. A JSON barrier
   acknowledges the exact point and PID; F8 also captures the committed oracle in
   the owning child. The parent verifies WAL bytes, sends SIGKILL to that child,
   verifies its signal exit and unchanged crash-left WAL/main-file bytes, then
   starts a fresh recovery process as the first SQLite opener. No parent/diagnostic
   SQLite connection, clean close or checkpoint occurs between kill and recovery.
   Recovery checks full store contents, integrity/FKs, head/snapshot/chain linkage,
   exact content, attribution, audit, original replay and one-effect deltas.
3. **Controlled clock:** an unexported record-action clock defaults to `time.Now`.
   `TestM2ControlledActionClock/{equal,regressing,advancing}` supplies four known
   times for create, PATCH, fresh no-op and restore. It checks every revision/event
   time, numeric ordering, stale PATCH/restore conflicts, new restore attribution,
   and byte-identical replay without reading the clock or changing any store.
   Unconditional successive-timestamp inequality assertions were removed; the
   separate advancing case establishes distinct action times deterministically.

The cold fixtures use large synthetic records and test-connection-only cache
spilling/autocheckpoint settings to ensure WAL is present at the barriers. WAL/FULL,
foreign keys and production connection policy are unchanged. Before recovery, F6
has mutation frames but zero commit markers; F8 has a final commit marker and a
valid complete checksum chain. Both check WAL format/header checksum/frame layout.
The first focused run exposed a harness assumption: SQLite may defer frame salts
and checksums while rewriting uncommitted spilled pages. The WAL inspector was
corrected against the bundled SQLite implementation to recognize those pending
headers at F6; every focused case and full suite then passed. This is R2 only,
not a storage/power-loss simulation. Cold F7 storage/sync interruption is untested.

### Unmerged migration 003 compatibility

Correcting 003 changes its SHA-256 checksum:

- Reviewed candidate: `31865ddf5b43c6d64b741d8be40c46da191fd01d77681b88c4aecf9ebbae01cf`.
- Corrected candidate: `9e6bfd1718cdaeaed92b7ac028bb90894d4a1ca3467d7ca02e6ba8460dbb5b9d`.

Existing disposable databases that applied the reviewed 003 are incompatible and
are refused by the corrected binary. There is no automatic old-003 conversion.
Preserve those files and ledgers; use a fresh database or a separately prepared
M1 fixture for this candidate. `TestM2ReviewedCandidateChecksumRefusal` and B's
actual reviewed binary create separate disposable old-003 fixtures and prove
refusal without rewriting their ledger/history; B compares all four mutation
stores and verifies refusal before readiness/listening. No existing candidate
database was opened or modified. Migration 001/002 remain byte-for-byte unchanged.

## T01–T30 matrix

Names below are actual tests, not proposed coverage. `G/R` execute all named Go
tests and their subcases. `B` exercises real binary boundaries. A PASS applies to
the stated software/ordinary-process cases; it does not substitute for T30.

| Case | Status | Actual coverage and command | Remaining gap |
| --- | --- | --- | --- |
| T01 | PASS | `TestM2AllMetadataPathsAndCreateDomains`, `TestM2SnapshotsRestoreAndReplay`, `TestTwoIndependentDomainsAndPersistence`, `TestM2HTTPContractAndSafety` (G/R/B): nullable/full metadata, two domains, head 1, snapshot/event/key/actor links. | None for this case. |
| T02 | PASS | `TestTwoIndependentDomainsAndPersistence`, `TestM2SnapshotsRestoreAndReplay` (G/R): whole-field replacement, omission/null behavior, retained earlier snapshots. | None. |
| T03 | PASS | `TestM2ControlledActionClock` (F/G/R): fresh no-op appends under equal/regressing/advancing action clocks; `TestM2NoopKindsAndFullRestore`, `TestM2PreconditionsAttributionAndFingerprints` (G/R): status/data/metadata no-ops append, matching replay does not, empty/base-only bodies reject. | None. |
| T04 | PASS | `TestM2PreconditionsAttributionAndFingerprints`, `TestM2HTTPContractAndSafety`, existing strict JSON tests (G/R): missing/null/string/bool/zero/negative/fraction/exponent/overflow/duplicate/case/If-Match; unchanged stores. | None. |
| T05 | PASS | `TestM2ControlledPoolRaces`, `TestM2IndependentProcessRaces`, `TestConcurrentPatchesPreserveOmittedFields` (G/R): distinct same-base PATCH/PATCH and PATCH/restore; one winner, one stale loser, explicit rebase. | None. |
| T06 | PASS | `TestM2AllMetadataPathsAndCreateDomains`, `TestM2PreconditionsAttributionAndFingerprints`, `TestTwoIndependentDomainsAndPersistence` (G/R): missing/stale/current bases across data, key, sensitivity, provenance, archive/unarchive, combined and archived-record changes. | None. |
| T07 | PASS | `TestM2ControlledActionClock` (F/G/R): original timestamps replay without clock reads or new effects; `TestM2SnapshotsRestoreAndReplay`, `TestM2HTTPContractAndSafety` (G/R), B: create/PATCH/restore original result after later edits/restart, fresh HTTP request IDs, unchanged stores. | None. |
| T08 | PASS | `TestM2PreconditionsAttributionAndFingerprints` (G/R): whitespace/key order, number spelling, changed base/field/restore target, scope independence, reusable failed key. | None. |
| T09 | PASS | `TestM2ControlledPoolRaces`, `TestM2IndependentProcessRaces`, existing retry concurrency tests (G/R): same scoped record request across pools/processes, one revision/event/effect. | None. |
| T10 | PASS | `TestM2ControlledActionClock` (F/G/R): historical content with new action time/actor under all three clocks; `TestM2SnapshotsRestoreAndReplay`, `TestM2NoopKindsAndFullRestore` (G/R/B): multiple states, full/null metadata, historical status/data/schema, stable identities, new actor/time/request/linkage. | None. |
| T11 | PASS | `TestM2ControlledActionClock` (F/G/R): stale restore checks remain based on the head revision under all three clocks; `TestM2SnapshotsRestoreAndReplay`, `TestM2PreconditionsAttributionAndFingerprints` (G/R/B): unarchive preserves current data, historical/current/identical restore appends, stale/missing bases reject, replay preserved. | None. |
| T12 | PASS | `TestM2RestoreCorruptionAndLimitFixtures`, `TestSchemaPinningAndImmutableRegistry` (G/R): incompatible v2, missing target, isolated corrupt pair/identity/schema/data fail closed. | Corrupt fixtures are test-only; ordinary paths retain all guards. |
| T13 | PASS | `TestM2ExactJSONAcrossHistory`, `TestM2AdoptionAndLegacyReplay`, existing numeric/schema-bound tests (G/R/B): exact large integer, decimal/exponent spelling, negative zero, bounded extremes, unknown arrays/Unicode across history/restore/replay/upgrade/restart. | None. |
| T14 | PASS | `TestM2ControlledActionClock` (F/G/R): equal/regressing clocks cannot change numeric revision ordering or conflict checks; `TestM2RestoreCorruptionAndLimitFixtures`, `TestM2LinkageGuards`, `healthy` oracle (G/R): uniqueness/range/max overflow, controlled limit fixture, contiguous chains, current snapshot/event linkage. | Limit fixture intentionally bypasses guards only during isolated setup. |
| T15 | PASS | `TestM2ImmutableSQLHistory`, `TestM2HTTPContractAndSafety` (G/R): UPDATE/DELETE/REPLACE/upsert guards, immutable revision field, GET-only routes/405, absent audit API, unchanged state after reopen. | None. |
| T16 | PASS | `TestM2LinkageGuards`, `TestM2ImmutableSQLHistory` (G/R): missing references/events, wrong pair/ID, duplicate event, invalid base/source/actor/range, head mismatch including null, deletion/replacement, deferred commit rejection. | None within the approved application/guard boundary; privileged schema editing is outside it. |
| T17 | PASS | `TestM2ControlledActionClock` (F/G/R): approved restore action and event time/actor linkage under all three clocks; `TestM2HTTPContractAndSafety`, `TestM2PreconditionsAttributionAndFingerprints`, `TestM2NoopKindsAndFullRestore`, `TestM2SnapshotsRestoreAndReplay`, existing privacy tests (G/R/B): explicit actor, spoof rejection, event actions/minimal columns and no payload leakage. | Attribution remains development/test, never authentication. |
| T18 | PARTIAL | `TestM2AtomicFailureBoundaries`, `TestM2SQLWriteAndDeferredCommitFailures`, `TestM2SerializationFailureRollsBack`, `TestM2CommittedResponseLossReplay`, `TestM2CancellationBeforeCommit` (G/R): create/data/metadata/archive/unarchive/restore, F0–F6, deferred-constraint F7, postcommit F8 and lost/truncated response. | Actual storage write/sync failure during commit is not executed; it belongs to open R3. SQL errors and cancellation do not close that gap. |
| T19 | PASS | `TestM2RevisionPagination`, `TestM2HTTPContractAndSafety` (G/R): 205+ revisions, limits default/1/100, changed limit, append and restore between pages, fixed end, invalid cursors/queries/numbers, missing/empty, unchanged reads. | None. |
| T20 | PASS | `TestM2MigrationInterruptionAndRetry`, existing fresh/M0 migration tests (G/R/B): fresh/M0/empty M1 and M1 metadata/keys without records; ledger retained, no invented adoption. New record creation is exercised on fresh and upgraded stores. | None. |
| T21 | PASS | `TestM2AdoptionAndLegacyReplay` (G/R), B with actual M1 binary: active/archived records, multiple M1 edits/replays, verbatim current bytes/times, one adoption each, explicit boundary. | None. |
| T22 | PASS | `TestM2AdoptionAndLegacyReplay` (G/R), B: old create/PATCH bodies without base replay exact M1 objects after later edits/restart, contract tag m1, altered body conflict, unknown missing base rejection, subject/schema replay and original stored rows retained. | None. |
| T23 | PASS | `TestM2MigrationInterruptionAndRetry`, `TestM2MigrationCancellationAndCommitFailure`, `TestM2MigrationMidAdoptionAndDeadline`, `TestM2UpgradeCommitConstraintRollback`, `TestM2MigrationAbruptRecovery`, `TestM2StartupDeadlineDuringStorageWait` (G/R): U0–U5, mid-backfill, deferred commit failure, cancellation/inherited startup deadline, repeat/reopen. | Storage/power failure during migration is reserved for T30. Deadline tests use a shorter inherited deadline. |
| T24 | PASS | `TestM2AdoptionAndLegacyReplay` ledger-prefix checks and `TestM2ReviewedCandidateChecksumRefusal` (G/R); B corrected-binary refusal of reviewed 003; B actual M0/M1 refusal before listener/ready, unchanged ledger/history, separate closed-copy rollback retaining M2 evidence and explicitly omitting later M2 edits. | Connection PRAGMAs may initialize before refusal, as documented in M0. |
| T25 | PASS | `TestM2ContentionCancellationAndReplay`, existing unavailable-error tests (G/R): writer lock for new/replay requests, bounded unavailable outcome, release/retry, stale conflict, exhausted pool, cancellation/deadline, no leakage. | None. |
| T26 | PASS | Existing storage/config/schema tests plus S (G/R): bounded/replacement connections, FK/WAL/FULL/busy timeout, ledger/path/symlink rejection, retained M1 identity/schema guards. | No driver/connection policy change. |
| T27 | PASS | `TestM2HTTPContractAndSafety`, `TestAPILoopbackHostForms`, existing strict/privacy/config tests (G/R/B): new-route Host/Origin/Sec-Fetch, IPv4/IPv6, malformed ports/hosts, media/body/query limits, safe headers/errors/logs, absent deferred APIs. | None. |
| T28 | PASS | `TestM2ShutdownWithCommittedMutation`, `TestM2CommittedResponseLossReplay`, `TestM2CancellationBeforeCommit`, existing probe/drain/forced-close/startup tests (G/R/B): readiness withdrawal, drain/force-close, committed response loss and retry, resource release, graceful SIGTERM. | Graceful shutdown is not abrupt/power-loss evidence. |
| T29 | PASS | `TestM2ColdRestartRecovery` (F/G/R): sole-child create/PATCH/restore at F6/F8, raw crash-left WAL verification, fresh-process recovery and exact same-key replay; `TestM2AbruptChildRecovery` retains live-observer F0–F6/F8 coverage; `TestM2MigrationAbruptRecovery` covers U0–U5. | R2 only. Cold mutation coverage is specifically F6/F8; F7 storage/sync and power loss remain R3/T30, not claimed here. |
| T30 | BLOCKED | Bounded R3 harness and standard-runner execution now authorized; validation and matrix not yet run. | Harness gates and exact executed storage-fault evidence are still required. |

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

Live multi-table observers use separate physical connections and the `observe`
helper or binary rehearsal's explicit deferred `BEGIN` for all four tables. The
cold-restart test has no live parent observer; its sole owner observes at F8 only
after commit, when the mutation transaction has released the connection. No
observer requests the writer lock while a paused writer owns it. Rollback checks compare complete ordered contents;
recovery checks use integrity/FK checks, chain/head/snapshot/audit linkage and replay
results. Counts alone are not used as proof of atomicity. No test disables guards
on ordinary paths; impossible-state/limit fixtures are explicitly isolated.

## Bounded R3 authorization and execution status

The owner authorized a test-only VFS harness against application candidate
`b6f63a7564977555faffe6f9ca6b1a9c22910d43`, with at most two sequential standard
public-repository GitHub-hosted Ubuntu 24.04 jobs, 60 minutes each, 4 GiB modeled
scratch and a 2 GiB free-space reserve. Paid usage is limited to $0. No artifacts
or caches are uploaded; sanitized manifests, hashes and results stay in job logs.
This replaces the earlier proposal's unapproved VM/scratch-volume setup with the
existing hosted-runner service. No personal VM or host storage faults are used.

The harness is implemented under `internal/r3` and requires the explicit `r3`
build tag. The ordinary application has no VFS controls. A wrapper intercepts the
pinned bundled SQLite's actual write/sync/truncate operations while the test
worker calls the existing engine and migration paths. A separate Python controller
reconstructs modeled durable images from the I/O trace and checks complete state
and replay in fresh worker processes. Exact candidate-source equality is checked
in the workflow; the harness SHA, amalgamation/header hashes, build environment,
SQLite source ID/options, settings, target positions and seeds are logged.

**Current result: NOT RUN. Harness validation is pending; T18 remains PARTIAL and
T30 remains BLOCKED pending validation and executed coverage.** No application
acceptance run may start unless the storage-model, native-VFS, native-application
control and deliberate corruption controls all pass. No result below is inferred
from compiling the harness.

The declared `write-back-v1` model maintains live and durable file images. Successful
file sync persists that file's bytes/length and creation; syncDir deletion persists
the namespace removal. It does not silently sync another file. Unsynced writes can
be retained, discarded, reordered within namespace/truncation epochs, or torn
within their addressed byte range; no collateral corruption of bytes outside that
write is modeled. Earlier syncs cannot be undone except by later modeled writes.
Returned IOERR/FULL/partial-write errors are a separate schedule axis. Shared-memory
indexes are volatile and rebuilt. Profiles report 512/4096-byte sectors and no
atomic-write/safe-append/powersafe-overwrite capabilities. Results cannot establish
physical hardware, filesystem or dishonest-flush durability.

The bounded matrix selects the first/middle/last observed positions for each
application phase, file role and semantic I/O category, logs the exact mandatory
positions, and uses discard/retain plus reorder/torn seeds 17, 29 and 101. It covers
create, data/metadata PATCH, archive/unarchive, no-op, restore, explicit TRUNCATE
checkpoint, default automatic checkpoint/WAL reset, fresh initialization and actual
M1-binary adoption. Missing targets, trace drift, untraced file changes, failed
negative controls, timeout or resource exhaustion fail the gate. This is bounded
sampling, not exhaustive enumeration of every possible storage schedule.

The acknowledgement ledger is outside the modeled fault domain and is flushed
only after complete successful response receipt. Test children are terminated
without cleanup; only the modeled image is materialized for recovery. The first
recovery connection uses the bundled SQLite and inspects state before migration
retry, then the full oracle and repeated same-key requests check survival and no
duplicate effects. No normal parent close/checkpoint is used as crash evidence.

## Review boundary

Architecture deviations: **NONE**. D1–D5 are retained. Migration 001/002, dependencies,
accepted ADRs and CI behavior are unchanged. Malformed Host port forms are rejected
consistently while valid loopback forms remain supported. There is no M3+ feature,
authenticated actor, public audit query, deployment, or real personal data.
No currently known application defect is being waived; the open acceptance gap is
storage/power-loss evidence. The draft PR is not a request to merge.
