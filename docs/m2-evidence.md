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
| T30 | BLOCKED | The fourth dedicated job passed the wrapper and full prerequisite gates, including all five negative controls, then failed with an AssertionError before any application fault schedule/case was logged. | All four dedicated slots are consumed. No application fault-matrix coverage exists; the assertion's exact cause is unretained. Further execution requires separate authorization. |

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

**Original two-job harness validation result: FAIL; T18 PARTIAL; T30/R3 BLOCKED.**
Both authorized jobs have been consumed. The R3 workflow now has an unconditional
false gate; no third job or rerun is authorized. No application acceptance fault
matrix ran, and no R3 durability claim is made.

| Run | Harness SHA | Executed result |
| --- | --- | --- |
| [36807842932](https://github.com/estul26/Contextarium/actions/runs/36807842932) | `658fdebba839b0f6a8d19eac366b19ee98bddc43` | FAIL during build, 26 seconds. The module directory was queried before downloading dependencies, leaving the pinned SQLite header unavailable. No validation or fault execution. |
| [36808063489](https://github.com/estul26/Contextarium/actions/runs/36808063489) | `47924bace3c5b5defa7243ffc13e26ee251484fa` | Build/vet/module checks passed after fixing download ordering. Validation then failed, 32 seconds total: the instrumented application baseline child exited without a result/barrier. No acceptance matrix started. |

Run 2's actually executed checks:

| Check | Result / boundary |
| --- | --- |
| Independent in-memory storage model | PASS: live visibility, successful/failed sync, truncation, create/delete namespace and crash reconstruction assertions. |
| Incorrect-sync negative control | PASS: the validator rejected a deliberately broken model that ignored successful syncs. |
| Native raw-VFS probe | PASS: write `abcdef` at offset 0, sync, overwrite `XYZ` at offset 2, truncate to length 4, sync and syncDir delete. Direct reads confirmed visibility. |
| Native raw-VFS failed-sync probe | PASS: injected IOERR at the first `xSync` on `probe.db`, phase `model-validation` (the deterministic probe's occurrence 3; sync has no byte range). The target acknowledgement and trace match were required; the failed sync did not establish modeled durability. |
| Actual application with normal VFS | PASS: create response and complete oracle checked on the candidate's real engine/storage paths. |
| Actual application with R3 VFS, no injected fault | FAIL: worker exited before returning a response/barrier. Its exit cause is unresolved. |
| Lost-acknowledgement and broken revision/audit/idempotency negative controls | NOT RUN: the instrumented baseline failed before these controls. |
| Application migration/mutation/commit/checkpoint fault matrix | NOT RUN: the prerequisite harness gate did not pass. |

Seeds 17, 29 and 101 were used only in the small model's deterministic reconstruction
checks. They were merely listed in the acceptance manifest; **no application
retain/discard/reorder/torn schedule was executed**. The raw probe's IOERR is harness
validation, not proof of T18's application commit-failure coverage.

The public run-2 build manifest records Go 1.26.8 linux/amd64, GCC 13.3.0, Ubuntu
image `20260927.320.1`, kernel `6.17.0-1022-azure`, 4 CPUs and `16373452 kB` total RAM.
It records candidate/harness IDs and source hashes, including:

- Bundled amalgamation SHA-256: `eb023455154c8da14a9920dbe44f4f9e732871ef8d8a058cafb84d18d6a2de00`.
- Bundled header SHA-256: `4e7d1523cf95991f7e4c08c576e2232e063da6f10067e5c03c9bf9f904b1cf5f`.
- VFS C source SHA-256: `238edd41657a04511f3527f4dc940e1c66f192dcbddc78f0a96e5ed3598eec0a`.
- Controller SHA-256: `ce1b0ba655b606ccf8452eda160aec8ec3ff56b3291f30b7b1fce794403b1dc5`.

The pinned header declares SQLite 3.53.4 and source ID
`2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`.
This is a source declaration, not a retained R3 runtime observation.

The worker queried SQLite source ID/options and PRAGMAs during successful preliminary
operations, but the controller did not print those transient settings messages
before failure. Those runtime values, the failing child's stderr/exit detail and
full validation traces were not retained in job logs or artifacts. This diagnostic
retention gap must be repaired before any renewed execution; do not substitute
previous M2 settings evidence for missing R3 runtime evidence. No application trace
manifest or recovery-state hash exists because the matrix never started.

No artifacts or caches were uploaded. The two jobs used verified standard
public-repository hosted runners; no paid runner/storage feature was requested or
used ($0 additional compute/artifact cost under that service's public-repository
pricing). Full test-owned files existed only on the disposable runner; GitHub job
logs retain the build manifest and the results above. No personal database, host
reboot, real-disk filling, local VM, or production fault switch was used.

Ordinary Linux CI passed for both harness commits:
[36807844653](https://github.com/estul26/Contextarium/actions/runs/36807844653) and
[36808068288](https://github.com/estul26/Contextarium/actions/runs/36808068288).
These runs do not validate the explicitly tagged R3 harness or close T18/T30.

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

The implemented but UNEXECUTED bounded matrix selects the first/middle/last observed positions for each
application phase, file role and semantic I/O category, logs the exact mandatory
positions, and uses discard/retain plus reorder/torn seeds 17, 29 and 101. It covers
create, data/metadata PATCH, archive/unarchive, no-op, restore, explicit TRUNCATE
checkpoint, default automatic checkpoint/WAL reset, fresh initialization, empty-M1 upgrade and actual
M1-binary adoption. Missing targets, trace drift, untraced file changes, failed
negative controls, timeout or resource exhaustion fail the gate. This is bounded
sampling, not exhaustive enumeration of every possible storage schedule.

The acknowledgement ledger is outside the modeled fault domain and is flushed
only after complete successful response receipt. Test children are terminated
without cleanup; only the modeled image is materialized for recovery. The first
recovery connection uses the bundled SQLite and inspects state before migration
retry, then the full oracle and repeated same-key requests check survival and no
duplicate effects. No normal parent close/checkpoint is used as crash evidence.

## Separately authorized one-shot no-fault diagnostic

The owner clarified that the original two-job R3 allowance remained exhausted.
One additional diagnostic job only was authorized: standard `ubuntu-24.04`, at
most 15 minutes including setup/build/reporting, no rerun or replacement, and no
fault probes, negative controls, crash schedules or acceptance matrix. A setup
failure would also have consumed this slot. This was not another R3 allowance.

[Run 36816413358](https://github.com/estul26/Contextarium/actions/runs/36816413358)
**PASS**, attempt 1, one job, 25 seconds total
(`2026-10-01T04:43:11Z`–`04:43:36Z`). The slot is now consumed. Execution stopped.

- Workflow commit: `fa86676e26b6b91eed09b3042287a4ae1e8f4f2a`.
- Reviewed worker/controller checkout: `43e382e43010b7a35f73cc2c001902a5f85fbad7`.
- Application candidate: `b6f63a7564977555faffe6f9ca6b1a9c22910d43`.
- Worker binary SHA-256: `5f4562da123091a309a9f3a9ce62582c7743add3309b4a76cf46f2c0b23d8c9f`.
- Controller SHA-256: `9148314cff94dbe98d45055cd67778940fab8a7655cb19ea0c36c46a5043c0df`.

The separate workflow checks out the exact reviewed source in a clean checkout;
the uncommitted negative-control patch was preserved and excluded. Its push/path,
parent-commit, commit-message, attempt and run-count gates admit only this slot.
The original R3 workflow's unconditional false gate is unchanged. No manual
dispatch or rerun occurred, and no production source, migration or dependency
changed. The only diagnostic command was:

```sh
python3 scripts/r3-check.py --mode no-fault-diagnostic --worker "$R3_DIAG_BUILD/worker"
```

Both `diagnostic-native` and `diagnostic-instrumented` passed: actual application
open, incremental settings, one successful create/result, integrity/FK checks and
consistent-state inspection of the application stores. The oracle checked exact
content and record/revision/audit/idempotency linkage. The instrumented control
also compared actual files with the traced model. This is a no-fault diagnostic,
not recovery or fault-model validation.

Runtime settings in both controls: SQLite 3.53.4; source ID
`2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`;
WAL, synchronous=2 (FULL), foreign_keys=1, busy_timeout=1000,
wal_autocheckpoint=1000, page_size=4096 and max_open_connections=1.
Native mmap_size=0; instrumented mmap_size was explicitly unsupported with reason
`vfs-control-notfound-no-row`. This confirms the no-result-row mechanism in this
reviewed harness. It does not recover the historical failed child's missing
stderr/exit details or prove that run's root cause.

The instrumented trace reported 110 reads, five mmap-control rejections and zero
fetches/null-fetches; policy was `always-null-no-native-delegation`. No mapped read
was observed. With no xFetch call, its guard path was not dynamically exercised.
Compile options were logged, but three entries were sanitized as `redacted`;
their exact runtime values are not retained. Successful execution also did not
exercise EOF/timeout/malformed-output failure-diagnostic paths.

Instrumented evidence SHA-256 values retained in job logs:

| Evidence | SHA-256 |
| --- | --- |
| Independent acknowledgement ledger | `a484c37c5823e243361fbabd0578815ea8139048633989d3bc2f15aa91fda962` |
| Successful response | `145deb4b062b61fcefc6ba2702bb3e3a5323932080c3065f9a11baf0516be87f` |
| Consistent state | `c43b5780df7c82115de26b575a698c653f365887dc9b5db98b7d1e9bac61b664` |
| VFS trace | `c08c62007b8b3955c517b4d065224304ee57ba43573038a18895b3b3584d4f84` |

The build manifest also retains all four reviewed harness source hashes, bundled
amalgamation/header hashes and dependency-file hashes. Environment: Go 1.26.8,
GCC 13.3.0, Ubuntu image `20260920.314.1`, x86_64 kernel `6.17.0-1022-azure`,
4 CPUs and 16373452 kB RAM. The 4 GiB modeled-data cap and 2 GiB free-space reserve
checks passed; no peak-usage measurement was recorded. Only synthetic fixtures
and test-owned runner temporary paths were used. No personal/runtime database,
host reboot, real-disk filling, VM or production fault switch was used.

No artifacts or caches were uploaded (run artifacts API count: zero). Sanitized
settings, manifests, hashes and results survive in job logs; full test-owned
database/trace files were temporary on the disposable runner. Only standard
public-repository hosted compute was used, with no paid service requested or
used ($0 additional compute/artifact cost under public-runner pricing).

[Ordinary Linux CI 36816417481](https://github.com/estul26/Contextarium/actions/runs/36816417481)
passed at workflow commit `fa86676e26b6b91eed09b3042287a4ae1e8f4f2a`.
It is separate from the diagnostic and supplies no R3 evidence.

**Diagnostic PASS only. Last full harness validation remains FAIL.** No fault
targets, seeds/schedules, negative controls or acceptance matrix ran in this job.
T18 remains **PARTIAL**, T30 **BLOCKED**, D6/R3 **OPEN**, M2 acceptance **PENDING**,
and PR #6 **DRAFT**. Successful no-fault operation does not establish write/sync
failure handling, simulated persistence-loss survival or physical durability.
Further execution requires separate owner authorization; there is no remaining
R3 or diagnostic slot.

## Fourth dedicated job: reviewed wrapper validation and conditional R3

The owner separately authorized one new standard `ubuntu-24.04` job, first attempt
only, with a 60-minute total limit and $0 paid usage. Setup/build failure would
consume the slot. The original two R3 jobs and completed diagnostic remained
exhausted. Only new one-shot workflow wiring was added; both older workflows,
the reviewed harness/controller, application, migrations and dependencies were
unchanged. There was no local substitute execution.

[Run 36818758680](https://github.com/estul26/Contextarium/actions/runs/36818758680)
**FAIL**, attempt 1, one job, 36 seconds total
(`2026-10-01T05:13:42Z`–`05:14:18Z`). The allowance is consumed; no repair,
rerun, replacement or further execution occurred. PR #6 remains DRAFT.

| Identity | Exact checkpoint |
| --- | --- |
| Application tested | `b6f63a7564977555faffe6f9ca6b1a9c22910d43` |
| Reviewed harness/controller checked out | `ce6755148f9f4b66d0d9ad49208a8550c0bb0ed1` |
| Workflow | `7c37d62ad1a83dac544cb16833d0e05b80a26dfe` |
| Actual M1 fixture binary source | `14dc96ad18810202f63d5ac590822f117a787e82` |

Build/source checks passed: clean exact checkout, application-source equality,
Go formatting, pinned module download before SQLite header resolution,
`go mod verify`, tagged vet and worker build, and M1 fixture-binary build. The
worker links bundled go-sqlite3 v1.14.52 / SQLite 3.53.4, not system SQLite.
Worker SHA-256: `936cde2fca275b0e0cd7179ce389f138ab21a7c9c1d773b5cad89bcb3883cb5d`.
Controller SHA-256: `b931738a0e5b8f72930c1865ccf12e566b77f0f44df8ac3a9daafac9c0f87837`.
The build manifest retains all harness, dependency, amalgamation and header hashes.

### Executed prerequisite evidence

**Wrapper behavioral checks: PASS (9/9).** The workflow loaded the reviewed
`expect_reject` without invoking controller main, using controlled callbacks.
It verified the exact expected assertion produces the intended PASS record;
different assertions, a worker-failure assertion and no rejection fail without
such a record; TimeoutExpired, CalledProcessError, JSONDecodeError, KeyError and
StopIteration propagate as the same exception objects. These checks validate
the wrapper, not application durability or real worker-failure diagnostics.

**Full prerequisite harness validation: PASS.** Executed model checks covered
visibility, successful/failed sync, truncation, namespace operations and
deterministic reconstruction. The native raw-VFS probe and its first-sync IOERR
control passed. Native and instrumented application controls, preliminary
fresh-process inspection/recovery, same-key replay and second-replay equality
all returned through their required checks. The positive image was freshly
inspected, matched its expected state and passed the oracle before corruption;
deep copies were corrupted and the positive input passed again afterwards.

All five required negative controls logged their exact rejection:

| Negative control | Observed rejection | Result |
| --- | --- | --- |
| Incorrect sync semantics | `successful sync persistence` | PASS |
| Lost acknowledged mutation | `acknowledged mutation lost/changed` | PASS |
| Broken audit linkage | `event/revision pair` | PASS |
| Broken revision linkage | `head/snapshot mismatch` | PASS |
| Broken idempotency linkage | `idempotency revision absent` | PASS |

These oracle controls corrupt observed in-memory input copies; they do not prove
on-disk corruption recovery. The retained gate record states harness validation
PASS and matrix STARTING only after every prerequisite returns. The wrapper
checks and this gate were independently reconciled against the pinned source and
actual individual log records; no final PASS line was used as acceptance proof.

Runtime settings were retained for both application controls: SQLite 3.53.4,
source ID `2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`,
WAL, synchronous=2 (FULL), foreign_keys=1, busy_timeout=1000,
wal_autocheckpoint=1000, page_size=4096, max_open_connections=1. Native mmap_size=0;
instrumented mmap_size reported unsupported with `vfs-control-notfound-no-row`.
Three compile-option entries per control were sanitized as `redacted`; their
exact runtime values are not retained.

### Failure and acceptance reconciliation

The controller subsequently emitted FINAL FAIL / `AssertionError` and exited 1.
No matrix `settings`, `schedule` or `case` record appeared: **zero application
fault schedules, target acknowledgements, completed cases or matrix recoveries**
are evidenced. The source proceeds from the prerequisite gate through M1 fixture
preparation before the first matrix baseline. The retained output does not
identify the exact assertion or preserve that fixture child's HTTP response,
stderr or exit detail, so no particular fixture failure or application defect is
established. No assertion, source, oracle or storage model was changed to proceed.

Seeds 17, 29 and 101 executed only in the small-model deterministic checks;
retain/17 also reconstructed the preliminary instrumented application image.
No application discard/reorder/torn fault schedule ran. The raw probe's first-sync
IOERR was prerequisite evidence; its separate target/trace details were not
retained after later controls reused the trace file. It does not close T18.

T18 remains **PARTIAL**: F7 application storage write/sync failures for the required
mutation paths remain unexecuted. T30/R3 remains **BLOCKED**: mutation, adoption,
fresh/empty-M1 initialization, commit, checkpoint and WAL reset/truncation fault
coverage is absent. D6/R3 is **OPEN** and M2 acceptance **PENDING**. The fourth job
failed even though its prerequisite gate passed. The earlier no-fault diagnostic
at `43e382e` remains PASS; historical failed runs remain unchanged above.

### Retained evidence and resource boundaries

Sanitized job logs retain 9 wrapper results, 5 exact-reason negative-control
results, 4 prerequisite validation records, 44 incremental settings records,
the gate, failing FINAL and consumed-slot report. The controller log manifest
reports 9,229 bytes, zero malformed records, zero matrix cases/recoveries and
SHA-256 `0d0a9d1199810abdaf11a848a2d41eba9940f16f3bf5f5a748ace817263c16f9`.

The failure reporter hashed remaining test-owned evidence without printing pages,
SQL, request bodies or raw stderr:

| Evidence remaining after failure | Bytes | SHA-256 |
| --- | --- | --- |
| VFS trace | 475658 | `a08074195edeb386144f48cef1038699686caf5e3abcd643b5a91c908a51f047` |
| External acknowledgement ledger | 48735 | `35272f260fe0f5744c1a793d9b40afec35aa2685ba5cea52fbd64edc9251e45c` |
| Instrumented worker stderr | 26534 | `f1d4ac6d1833d76b1511318e7451dc1e80f7811206f736511d806ece24fe074b` |

The last retained VFS row is a post record, seq=116, rc=0, applied=1352; its row
does not identify role/operation/range/phase. These remaining files belong to the
earlier instrumented control, not a demonstrated failing M1 fixture or matrix
fault target. Hashes preserve identity but cannot recover missing content after
runner disposal. No full trace/database/ledger or fixture-child log was uploaded.

Environment: standard public-repository runner, Ubuntu image `20260920.314.1`,
Go 1.26.8, GCC 13.3.0, kernel `6.17.0-1022-azure`, x86_64, 4 CPUs, 16373452 kB RAM.
Free space was 92,137,967,616 bytes before execution and 92,136,939,520 afterwards;
remaining modeled scratch was 868,721 bytes. The reviewed controller's 4 GiB cap
and 2 GiB reserve checks were retained. Peak usage was not measured; the retained
measurements establish the boundaries at those checkpoints, not continuous peak
telemetry. Only marked runner-owned temporary resources and synthetic data were
used; no personal VM/database, host reboot, real-disk filling or production switch.

Artifacts API count is zero; there are no artifact/cache upload steps. Only
standard public-repository hosted compute was used, with $0 additional usage
under [GitHub's public-runner pricing](https://docs.github.com/en/billing/concepts/product-billing/github-actions).
[Ordinary CI 36818761528](https://github.com/estul26/Contextarium/actions/runs/36818761528)
passed at workflow SHA `7c37d62ad1a83dac544cb16833d0e05b80a26dfe`; it is separate
and is not R3 validation. All four dedicated execution slots are consumed.
Reporting pushes cannot satisfy the new workflow's exact predecessor/message/path
gates; reruns fail its first-attempt gate. There is no remaining execution allowance.

## Review boundary

Architecture deviations: **NONE**. D1–D5 are retained. Migration 001/002, dependencies,
accepted ADRs and CI behavior are unchanged. Malformed Host port forms are rejected
consistently while valid loopback forms remain supported. There is no M3+ feature,
authenticated actor, public audit query, deployment, or real personal data.
No application defect is established by the historical or fourth-job failures.
The later prerequisite controls passed, but application storage/power-loss evidence
remains absent. This gap is not waived or represented as application acceptance.
The draft PR is not a request to merge.
