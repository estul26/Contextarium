# M2 implementation evidence

**IMPLEMENTATION CANDIDATE — ACCEPTANCE PENDING; D6/R3 OPEN**

Starting checkpoint: `05907846c59d909990d0c6159edce1da88dea7c6` (approved D1–D5).
`origin/main` matched that checkpoint when implementation started. Work is on
`m2-implementation`; earlier checkouts and local artifacts were preserved.
The containing implementation commit identifies this source/test tree. The draft
PR records its exact candidate SHA and the subsequent local/Linux validation
results, avoiding a self-referential commit hash in this file.

Latest executed R3 evidence: [sixth dedicated job](#sixth-dedicated-job--targeting-tests-pass-matrix-stopped-before-mismatched-injection).
All 26 targeting tests, 16 fixture tests and prerequisites passed. The matrix failed
before a mismatched injection after 379 cases / 1,895 recoveries. T18 PARTIAL,
T30 BLOCKED, D6/R3 OPEN; all six slots consumed. Earlier source-review and run
records below describe their historical status, not the current execution result.

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
| T18 | PARTIAL | `TestM2AtomicFailureBoundaries`, `TestM2SQLWriteAndDeferredCommitFailures`, `TestM2SerializationFailureRollsBack`, `TestM2CommittedResponseLossReplay`, `TestM2CancellationBeforeCommit` (G/R): create/data/metadata/archive/unarchive/restore, F0–F6, deferred-constraint F7, postcommit F8 and lost/truncated response. | Fifth- and sixth-job WAL write/sync fault cases passed for create/data/metadata/archive/unarchive, with no-op partial; restore and full required boundary coverage remain missing. T18 is not closed by this partial matrix or SQL-only errors. |
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
| T30 | BLOCKED | Sixth job: 26 targeting tests, 16 fixture tests, nine wrapper checks, prerequisites, five negative controls and both real M1 fixtures PASS; 379 cases / 1,895 recovery checks before `noop-s512-b39-cut-after` failed its pre-injection descriptor check. Fifth-run 386 / 1,930 remain separate historical evidence. | No-op incomplete; restore, migration, fresh/empty initialization, checkpoint/WAL reset/truncate and new acknowledged-effect fault survival remain unexecuted. All six slots consumed. See detailed evidence below. |

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

## M1 fixture source correction — runtime validation NOT RUN

This source-only correction starts from reporting checkpoint
`2e6d3dc170dc5eca733af2d23d900a5e56aa3b24`. The last executed harness remains
`ce6755148f9f4b66d0d9ad49208a8550c0bb0ed1`; no additional execution slot was
authorized or used. The fourth-job failure record above remains unchanged.

Source and Python symbol-table inspection confirm that the nested `http` helper
shadowed the imported package: its `http.client.HTTPConnection` expression used
the enclosing function binding rather than the module. Renaming the helper and
all calls to `request_http` makes `http` resolve globally. This establishes a
source defect; it does not recover the missing historical exception chain or
prove which cleanup assertion produced the retained final AssertionError.

Two further source defects are corrected: the old finally block could replace a
primary failure, and the empty fixture returned before the WAL postcondition.
The patch preserves a primary exception and traceback, reports cleanup failures
separately, and makes cleanup-only failure fatal. Both fixture kinds now reach
the same zero-exit and WAL-absence checks. Intended HTTP methods, paths, request
bodies, idempotency keys and 200/201 success checks are retained.

Fixture diagnostics identify launch/readiness/subject/schema/record/update/close,
fixed error codes, HTTP status and allowlisted API error codes. Child output is
captured in exclusive fixture-owned files, with sizes/hashes and at most eight
allowlisted records from each 64 KiB tail. Any returned/partial communicate output
is also summarized, rather than discarded. Natural exit and controller SIGTERM/
SIGKILL are reported separately; timeout, signal, capture and close failures stay
secondary when a primary exists. HTTP reads are bounded at 128 KiB plus the
overflow sentinel; arbitrary response bodies, child messages, environment values,
SQL, paths and exception text are not published. Top-level failure records add
fixed error codes and controller function/line locations, with an error identifier
tied to those locations in the reported harness source.

[Focused regression source](../scripts/test_r3_fixture.py) prepares 16 fake-only
tests: module resolution and intended requests; primary plus process/HTTP cleanup
failure; cleanup-only failure; readiness timeout/early exit; populated and empty
clean-close/WAL checks; safe API status/code reporting; bounded sensitive-output
redaction; launch failure and diagnostic-sink failure. **All 16 tests are NOT RUN.**
They are not wired into ordinary CI. No controller/test import, harness build,
fixture binary, probe, negative control or fault schedule was executed for this
patch, and no Actions job was manually dispatched or rerun.

Static checks cover AST parsing, before/after symbol resolution, whitespace,
scope/public-content review, and unchanged storage model, oracle, replay,
negative-control and main-function ASTs. Application code, migrations,
dependencies, approved contracts, ordinary CI and every existing workflow gate
remain unchanged. Runtime behavior of the correction, the prepared regressions,
real M1 fixture closure and all outstanding application fault coverage remain
unverified. T18 **PARTIAL**, T30 **BLOCKED**, D6/R3 **OPEN**, M2 acceptance
**PENDING**, PR #6 **DRAFT**, additional execution slots **ZERO**.

## Fifth dedicated job — prerequisites PASS; matrix stopped on missing target

The owner authorized one new standard-hosted job, first attempt only, with a
60-minute total limit and no replacement, rerun or local substitute. This was
**the fifth dedicated job historically**; the four earlier allowances remained
exhausted. [Run 36823608907](https://github.com/estul26/Contextarium/actions/runs/36823608907),
job `110244300469`, attempt 1, **FAILED** after **105 seconds**
(`2026-10-01T06:13:09Z`–`06:14:54Z`). Execution stopped without source repair or
retry. All five slots are consumed; additional execution slots: **ZERO**.

| Identity | Exact commit |
| --- | --- |
| Application, verified unchanged | `b6f63a7564977555faffe6f9ca6b1a9c22910d43` |
| Reviewed harness/controller/16-test source checked out and executed | `305931711ff79ac32853794fb165fe32a6d45f90` |
| Genuine M1 fixture-binary source | `14dc96ad18810202f63d5ac590822f117a787e82` |
| Separate one-shot workflow | `3b031db317161cebf2c93d6239f36078bacd7ba0` |

Only `.github/workflows/r3-fifth-once.yml` was added for execution. It pins the
reviewed checkout, exact predecessor and push message, public repository,
standard `ubuntu-24.04`, first attempt and single workflow-run count. It has no
manual trigger; reporting pushes do not match its path gate and reruns fail its
attempt gate. Original `r3.yml` false gate, consumed diagnostic/fourth-job wiring,
ordinary CI, production source, migrations, dependencies and reviewed harness
are unchanged. Local dirty work and other worktrees were preserved.

### Executed stages

1. **Fixture regressions: PASS, exactly 16/16.** Before Go setup/builds,
   `python3 scripts/test_r3_fixture.py -v` executed once. Actual summary:
   `Ran 16 tests in 0.160s`, then `OK`; 16 individual `ok` lines, zero skips,
   expected failures or unexpected successes; process/log-capture exits both 0.
   The workflow independently checked count and outcome from the captured output
   without reimporting/rerunning tests. These fake HTTP/process tests cover the
   prepared resolution, primary/cleanup, timeout/exit, populated/empty closure,
   WAL, API-code and sensitive-output cases; they are not durability evidence.
2. **Build/source checks: PASS.** Clean HEAD and production-source equality to
   the application candidate, harness gofmt, `go mod download` before header
   lookup, `go mod verify`, pinned go-sqlite3 `v1.14.52`, tagged vet/build against
   its bundled header, and genuine M1 archive/build all passed. No system SQLite
   substitution. The manifest records source/tree/dependency/header/binary hashes.
3. **Wrapper behavior: PASS, 9/9.** Exact intended assertion accepted;
   different assertion, no rejection and worker assertion rejected without a
   PASS record; TimeoutExpired, CalledProcessError, JSONDecodeError, KeyError and
   StopIteration propagated unchanged. These controlled callbacks exercise the
   reviewed wrapper; they do not simulate application durability.
4. **Prerequisite validation: PASS.** Independent model, raw native VFS and
   first-sync IOERR control, actual native/instrumented application controls,
   fresh-process inspection and same-key/second replay passed. Fresh positive
   oracle inputs passed before copied corruption and remained unchanged after it.
   All five negative controls emitted the required exact rejection below.
5. **Both genuine M1 fixtures: PASS.** Populated fixture reached readiness,
   subject/schema/record/update; empty fixture reached readiness. Both received
   controller SIGTERM, exited 0, and passed the required WAL-absence check before
   any matrix schedule. Neither exited naturally before cleanup. No cleanup
   error was reported. Both safe child logs contain ready/stopped codes; their
   raw stderr is not published. These are fixture R1 closure checks, not R3.
6. **Acceptance invocation: FAIL / incomplete.** The unchanged command ran once:
   `python3 scripts/r3-check.py --mode acceptance --worker "$R3_BUILD/worker" --m1-binary "$R3_BUILD/m1-binary"`.
   Its step lasted 79 seconds, including prerequisites and fixtures. The matrix
   stopped at the first unexpected failure, detailed below.

| Negative control | Exact observed rejection | Result |
| --- | --- | --- |
| Incorrect sync semantics | `successful sync persistence` | PASS |
| Lost acknowledged mutation | `acknowledged mutation lost/changed` | PASS |
| Broken audit linkage | `event/revision pair` | PASS |
| Broken revision linkage | `head/snapshot mismatch` | PASS |
| Broken idempotency linkage | `idempotency revision absent` | PASS |

### Completed matrix evidence and failure

| Operation | Sector sizes | Completed fault cases | Completed recovery checks |
| --- | --- | --- | --- |
| Create | 512, 4096 | 72 | 360 |
| Data PATCH | 512, 4096 | 72 | 360 |
| Metadata PATCH | 512, 4096 | 72 | 360 |
| Archive | 512, 4096 | 72 | 360 |
| Unarchive | 512, 4096 | 72 | 360 |
| No-op PATCH | 512 only, incomplete | 26 | 130 |
| Restore, checkpoint, autocheckpoint/WAL reset, migration/adoption, fresh and empty-M1 initialization | NOT RUN | 0 | 0 |
| **Total** | | **386** | **1,930** |

Every completed case has five recovered images: discard/17, retain/17,
reorder-torn/17, reorder-torn/29 and reorder-torn/101. The bounded selection is
first/middle/last per phase/file-role/semantic group, not exhaustive sampling.
Completed modes: cut-before 86, cut-after 86, IOERR 86, FULL 64 and partial write
64. All 386 completed targets were WAL operations: 323 writes and 63 syncs,
covering headers/frames/commit markers and syncs. No matrix truncate or database
checkpoint-write fault executed. The earlier raw-VFS truncate probe is separate.

Independent log reconciliation matched all completed case identities, target
phase/role/operation/offset/length/occurrence, five schedules/seeds and result
hashes to the 11 announced schedules. There were 86 distinct completed
operation/sector/occurrence targets out of 88 announced, and 386 completed cases
out of 396 announced combinations. All case rows were retained and their combined
manifest hash matches the runner's manifest. Each case returned through fresh
process inspection before retry, integrity/FK checks, complete store/ledger/schema
and history consistency, exact JSON/schema retention, and same-key second replay.
Recovery outcomes were 1,557 wholly absent and 373 wholly present.

**All 386 completed fault cases had zero newly acknowledged responses.** Their
oracle checks retain previously durable history and permit only whole ambiguous
outcomes. They do not establish survival of newly acknowledged effects under
these faults. Preliminary/no-fault retain-image controls are separate evidence;
post-acknowledgement checkpoint fault coverage did not run.

The next attempted case, **`noop-s512-47-full`**, failed at
`fault-target-verification`: `target_reached=false`. Stable error identifier:
`controller-assertion:execute:600`, with `main:1129` and `require:295` in the
reviewed controller. Line 600 asserts `requested fault not reached`. The schedule
expected a WAL commit-marker write at occurrence 47, offset 86,552, length 24,
phase `mutation-noop`, with FULL injection. The retained last VFS pre/post instead
ends at occurrence 44: WAL write, offset 74,240, length 4,072, rc=0/applied=4,072.
Why this execution's operation sequence differed from its baseline is unresolved;
no source repair, scheduling relaxation or repeat was attempted.

That worker delivered a complete done record and one acknowledged response, then
the controller performed its planned SIGKILL. Diagnostics report 54 complete
protocol records, no malformed record or partial bytes, no natural exit observed,
and signal 9 after controller termination. The primary assertion was retained;
cleanup reported no secondary error. **No recovery/oracle/replay check completed
for this failed case or its acknowledgement.** Its target is not counted as
reached, FULL is not counted as injected, and its outcome is not PASS. Nine further
announced no-op/512 combinations were not attempted. No-op/4096 and all later
operation families remain unexecuted. No application corruption or durability defect is
established by this missing-target failure.

T18 stays **PARTIAL**: some actual WAL write/sync failures now have passing recovery
evidence, but restore and the complete required mutation/boundary coverage remain
missing. T30/R3 stays **BLOCKED** by the failed mandatory target and incomplete
matrix; completed subsets are retained, not promoted to milestone acceptance.
D6/R3 **OPEN**, M2 acceptance **PENDING**, PR #6 **DRAFT**. Prerequisite validation
PASS is distinct from this full execution's FAIL.

### Fidelity, retention and resource limits

Runtime SQLite: `3.53.4`, source ID
`2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`.
Reported WAL/FULL (synchronous=2), foreign_keys=1, busy_timeout=1000,
wal_autocheckpoint=1000, page_size=4096 and max_open_connections=1 remain intact.
Native mmap_size=0; instrumented mmap diagnostic is explicitly unsupported with
`vfs-control-notfound-no-row`. All 11 matrix baseline no-mmap guards passed.
Three compile-option entries are redacted per report; their exact values are not
retained. Software VFS write-back results do not prove physical-device power-loss
behavior or devices that falsely report successful sync; sampling is bounded.

| Retained identity/evidence | Bytes where applicable | SHA-256 |
| --- | --- | --- |
| Reviewed controller | | `35b9877373fa7a89617f27ca09dfbbdabfa2dcd3831b80be2f70da97eabf334e` |
| Reviewed regression source | | `4326a6930d06467ed3a7a0d41037e1c6c45bfcaa103e5f2c46181fdb0d5ea433` |
| Worker binary | | `b18e0e118a793b81d7db13ac4ecf45bd48f14057a02cdcef90c27edf7f993d29` |
| Genuine M1 binary | | `e5da0705c51d9c424d4658fb62f5a966e71a6e747fcc05b59f66ed2342d006cb` |
| SQLite amalgamation | | `eb023455154c8da14a9920dbe44f4f9e732871ef8d8a058cafb84d18d6a2de00` |
| SQLite bundled header | | `4e7d1523cf95991f7e4c08c576e2232e063da6f10067e5c03c9bf9f904b1cf5f` |
| Actual fixture test output | 2153 | `cc377c51bce5831f43e759928354887ced23fa7c19e6a3ca2864787306132c13` |
| Controller result log | 1997822 | `b81fc466b23cd8e1a14bb34e6657ba2dbc1f23669ed42c3529d9a9e300f477e9` |
| Completed-case manifest | | `e64601ddd389ac5763d7bc297f49d71b8bccd9296f9cbc1ceb6bf54033c6ff64` |
| Failed-case remaining VFS trace | 164939 | `78510bb9887e901195c5447cd5beeec31c6348f25fe2f8434adbd9b967dd6e50` |
| Failed-case external acknowledgement ledger | 12794 | `d0eb57c19e6c174275270cb2dbb0025fc90b53343abcab4a4ce9429e9ae27ada` |
| Failed-case worker stderr | 11371 | `d5fefe39aafbec0e08def2549e27611c8c259e9574a412ec2f657fe400c7d12e` |
| Populated fixture stderr | 167 | `5bb64cd8c40a16cfb7567e1b9a7804c7d03f9394abc452a452a34d29476f9e82` |
| Empty fixture stderr | 168 | `e3e1d62ae642a613f6c8c0e12dfe895d75c2b09940e4f6b6ff1ba61fe5ac4c23` |

The full build manifest in job logs additionally retains module and other harness
source hashes. Raw-log export was reconciled after removing a log-chunk BOM:
all 8,800 settings-query rows and the controller byte count/hash match the
runner's saved result log exactly. There is no detected controller-log truncation.
The independent case manifest also matches. No raw pages, SQL or response bodies,
credentials, full environments or private data were published. Raw transient
trace/page/ledger content was not uploaded; hashes cannot recover it after runner
disposal. Sanitized diagnostics, target metadata and per-case outcomes remain in
job logs; local copies are reporting evidence only.

Standard public-repository `ubuntu-24.04`, image `20260927.320.1`, kernel
`6.17.0-1022-azure`, x86_64, 4 CPUs, 16373452 kB RAM, Go 1.26.8 and GCC 13.3.0.
Free space: 92,413,218,816 bytes before tests, 92,207,689,728 before execution,
92,199,628,800 after failure. Remaining scratch: 849,863 bytes. The reviewed 4 GiB
modeled-scratch cap and 2 GiB reserve checks were retained; no resource exhaustion
or timeout occurred. Peak usage was not measured; these observations do not claim
continuous peak telemetry. Only marked runner-owned temporary paths and synthetic
test-owned processes were used. No personal database/VM, host reboot, disk filling
or production fault switch.

Artifact count: **0**; no artifact/cache upload wiring. **$0 additional paid usage**
under [standard public-runner pricing](https://docs.github.com/en/billing/concepts/product-billing/github-actions),
with no paid runner/service or storage requested.
[Ordinary Linux CI 36823613198](https://github.com/estul26/Contextarium/actions/runs/36823613198)
passed at workflow SHA `3b031db317161cebf2c93d6239f36078bacd7ba0`; it excludes the
tagged harness and is not R3 evidence. No merge, deployment or M3; architecture
deviations: **NONE**. Any further execution needs a new explicit allowance.

## Targeting correction — source review only; runtime NOT RUN

This correction starts from reporting checkpoint
`0441d9c597f0591ccba14af6ed80456416940c25`. No intervening remote commits were
present. The last executed harness remains
`305931711ff79ac32853794fb165fe32a6d45f90`; the application remains
`b6f63a7564977555faffe6f9ca6b1a9c22910d43`. No new execution allowance was granted.
The fifth run's **386 cases / 1,930 recoveries** above retain their original
source, schedules and limitations; they do not validate this correction.

### Diagnosis and bounded targeting change

Source confirms that discovery copied a global sequence number into the next
worker. The VFS `before` hook compared only that number; descriptor comparison
occurred after injection. Global sequence includes other file operations, so
inserting/removing an earlier operation can shift it without identifying the
intended fault operation. The fifth-run evidence proves the planned occurrence
47 was not reached and that the worker completed through occurrence 44 with one
acknowledgement. It does not retain the full raw trace or establish why the
operation sequence changed. Entropy, timestamps or SQLite layout variation are
possible explanations, not diagnosed causes; production entropy/time is unchanged.

Proposed correction: **a synchronous pre-I/O decision barrier**, confined to the
tagged test VFS and controller. For fault runs, each pending write/sync/truncate
publishes bounded metadata and waits. The controller matches phase, file name,
role, operation, semantic meaning, offset, length and flags. It verifies the
ordered descriptor prefix within the selected phase/role/semantic group, then
checks the still-pending private pre-trace row, including WAL commit-marker bytes,
**before** sending an injection decision. Raw bytes stay private. The VFS requires
a reply naming this live sequence; malformed/stale/EOF replies fail without
performing that pending I/O. Cut-after is armed before the native call and pauses
after its post row; the existing IOERR/FULL/partial-write behavior is unchanged.

The global `-target` ordinal input is removed. Global sequence numbers remain
monotonic trace identities and diagnostics. Case IDs explicitly label discovery
sequence with `b`; successful reports include both planned descriptor/discovery
sequence and actual reached sequence. Later descriptor checks remain additional
assertions, not the first protection against a wrong injection.

**Selection mapping:** the same no-fault discovery groups still select positions
0, floor(N/2) and N-1. When positions coincide, one target retains every applicable
first/middle/last label. Group count, position, labels, descriptor and prefix hash
are logged. Other groups may change length without shifting the selected group.
Repeated identical descriptors are distinct ordered group-prefix positions; the
Nth planned occurrence is explicit. Another decision after selection is rejected.
A selected-group insertion/reordering or changed descriptor fails before injection;
a shortened group or absent target fails at completion. There is no nearby-target
substitution, retry or skip. An indistinguishable repeated descriptor is resolved
by that documented group position, not a global ordinal.

First/middle/last continue to mean positions in the recorded no-fault discovery
trace, not a counterfactual suffix after a fault changes execution. This correction
does not guarantee that a variable selected group can be reproduced: strict
prefix/range checks can still block the matrix. Runtime feasibility and handshake
cost remain unverified. No production entropy/time seam or persistence-model
change is introduced to force reproducibility. The protocol keeps its existing
byte/time bounds and separately caps decision records at 32,768; reaching a bound
fails with incomplete coverage. No-fault runs do not use the decision protocol.

Missing/mismatched-target diagnostics retain planned descriptor, baseline sequence,
position/count/labels, prefix hash, matched-prefix count, up to eight observed
safe descriptors, selected live sequence, trace-verification/decision status and
acknowledgement count. Existing primary-error/child/cleanup diagnostics remain.
No payload, page hex, arbitrary exception text or private path is added to logs.

### Prepared tests and acceptance checks

[Targeting regression source](../scripts/test_r3_targeting.py) contains **26 prepared
fake-trace/dependency tests, all NOT RUN**. Cases cover unrelated earlier events;
wrong phase/role/semantic operation; changed ranges/prefix order; private-trace and
live-sequence verification before the command; missing-target planned/observed
privacy; duplicate descriptors/decisions; first/middle/last and collapsed labels;
missing required groups; external-ledger ordering/failure and incomplete responses;
commit-marker-not-acknowledgement; absent families/sectors, missing cases,
incomplete recovery schedules, zero acknowledged-effect coverage and a positive
synthetic completeness control. They do not launch a binary, SQLite or a fault.
The previously executed 16 fixture regressions are unchanged and were not rerun.
These prepared tests do not yet validate the C/Go/Python handshake at runtime.

A new final completeness guard requires all 12 existing operation families at
both sector sizes, every planned target/fault combination exactly once, and all
five existing recovery schedules/seeds for each completed case. Complete no-op,
restore, migration/adoption, fresh/empty-M1 initialization and checkpoint families
cannot disappear while the final result says PASS. Existing discovery requirements
for checkpoint database writes/sync, WAL truncation and automatic WAL reset remain.

The guard additionally requires completed recovery after **newly acknowledged
mutation responses** at checkpoint database write/sync and WAL-truncate boundaries,
and automatic-checkpoint database write/sync and WAL-reset boundaries, for both
sector sizes. The controller adds an acknowledgement to its visible list only
after a complete successful response has been parsed and written/flushed/fsynced
to the external ledger. Its count is captured before authorizing the relevant
fault; a later response or WAL commit marker cannot satisfy that count. The
unchanged full oracle verifies those acknowledgements after each recovered image.
A zero count cannot pass final acceptance. Historical completed cases are not
loaded into this new execution's coverage accounting.

**Static checks only:** Python AST parsing and structural/order inspection; Go
formatting; C `-fsyntax-only` against the cached pinned go-sqlite3 v1.14.52 header;
whitespace, scope and public-content review. AST comparison preserves `Model`,
`oracle`, consistency/replay, positive/negative controls, M1 fixture/cleanup,
no-fault diagnostic, required settings/no-mmap checks and resource limits.
Successful-sync guarantees, recovery schedules and seeds are unchanged.
No controller/test import, regression execution, tagged build, fixture binary,
probe, negative control, fault schedule, manual Actions launch or rerun occurred.
Ordinary PR CI may run automatically; it is not R3 evidence.

Remaining acceptance work includes validating this correction and its prepared
tests, complete no-op and restore coverage, migration/initialization faults,
checkpoint/database-write/truncation/reset faults, and survival of newly
acknowledged effects. Source inspection supplies none of that runtime evidence.
T18 **PARTIAL**, T30 **BLOCKED**, D6/R3 **OPEN**, M2 acceptance **PENDING**, PR #6
**DRAFT**, additional execution slots **ZERO**. Application/migrations/dependencies,
D1–D5, ordinary CI and every existing workflow gate are unchanged. Architecture
deviations: **NONE**; the test-selection change above is proposed for source review.

## Sixth dedicated job — targeting tests pass; matrix stopped before mismatched injection

The owner authorized one additional standard `ubuntu-24.04` job, the sixth
historically, with 60 minutes total, $0 paid usage and no rerun, replacement or
local substitute. The five previous allowances stayed exhausted. This job used
the reviewed source unchanged; only separate one-shot wiring was added. Its
push path, exact predecessor/message, first-attempt and single-run checks exclude
reporting pushes and reruns. Every earlier workflow gate remains unchanged.

- Application: `b6f63a7564977555faffe6f9ca6b1a9c22910d43`.
- Reviewed harness/controller/tests: `6fbdc611672f493626ab758dfc7ce1e9682307a1`.
- Genuine M1 source: `14dc96ad18810202f63d5ac590822f117a787e82`.
- Workflow: `aea4746d6d497ffbb1d05f1c621d9fa775bc67de`.
- [Run 36868756735](https://github.com/estul26/Contextarium/actions/runs/36868756735),
  job `110390788451`, attempt **1**, **FAIL**. Job duration **184 seconds**,
  2026-10-01 13:27:07–13:30:11 UTC, including setup/build/reporting.

**The sixth allowance is consumed. Additional execution slots: ZERO.** No source
repair, retry, replacement or second acceptance invocation followed the failure.
The original dirty controller patch and all unrelated worktrees/artifacts remain
preserved. The reporting checkout does not include that old uncommitted patch.

### Executed stages

| Stage, in execution order | Actual result |
| --- | --- |
| `python3 scripts/test_r3_targeting.py -v` | **PASS: exactly 26 tests**, `Ran 26 tests in 0.497s`, `OK`; no skips or expected failures. |
| `python3 scripts/test_r3_fixture.py -v` | **PASS: exactly 16 tests**, `Ran 16 tests in 0.309s`, `OK`; no skips or expected failures. |
| Source equality, formatting, pinned module download/verification, tagged vet/worker build, genuine M1 build | PASS. Modules downloaded before resolving the bundled header; no system SQLite substitution. |
| Controlled rejection-wrapper checks | **PASS: nine**, including exact rejection, different/no rejection and propagation of worker/timeout/malformed/missing-field/lookup failures. |
| Model/VFS, native and instrumented application, recovery/replay and positive-oracle prerequisites | PASS. |
| Five exact-reason negative controls | PASS: incorrect sync, lost acknowledgement, broken audit/revision/idempotency linkage; positive oracle still passed afterward. |
| Real populated and empty M1 fixtures | **Both PASS**: controller SIGTERM, exit 0, WAL absence, no secondary cleanup errors; safe ready/stopped logs retained. These closures are R1, not R3 evidence. |
| One acceptance invocation and conditional matrix | **FAIL / incomplete** after **379 completed fault cases / 1,895 fresh-process recovery checks**. |

The negative-control reasons remain exactly `successful sync persistence`,
`acknowledged mutation lost/changed`, `event/revision pair`,
`head/snapshot mismatch` and `idempotency revision absent`. The wrapper and Python
suites validate harness behavior; they are not application durability evidence.

### Completed sampling and the failure

| Application family | Sector 512 cases | Sector 4096 cases | Coverage in this run |
| --- | ---: | ---: | --- |
| Create | 36 | 36 | All announced samples completed |
| Data PATCH | 36 | 36 | All announced samples completed |
| Metadata PATCH | 36 | 36 | All announced samples completed |
| Archive | 36 | 36 | All announced samples completed |
| Unarchive | 36 | 36 | All announced samples completed |
| No-op | 19 of 41 | 0 | Required target mismatch; incomplete |
| Restore, checkpoint, automatic checkpoint, adoption, fresh initialization, empty-M1 upgrade | 0 | 0 | NOT RUN |

There were 11 announced family/sector plans, 89 sampled targets and 401 planned
fault combinations. Of these, 379 completed, one failed before injection and 21
later combinations were not attempted. Each completed case ran **discard/17,
retain/17, reorder-torn/17, reorder-torn/29 and reorder-torn/101**. Outcomes were
1,525 wholly absent and 370 wholly present. Completed faults: cut-before **85**,
cut-after **84**, IOERR **84**, FULL **63**, partial write **63**; targets were **316
WAL writes and 63 WAL syncs**. Initial WAL-header writes do not establish the
unexecuted automatic-checkpoint WAL-reset coverage. No database checkpoint write,
database sync or WAL truncation case completed.

Independent log reconciliation checked every completed case against its announced
full descriptor/position/prefix hash and compatible mode, unique case identity,
all five recovery schedules, state/image hashes, sample labels and aggregate
manifest. All successful live sequence numbers happened to equal their discovery
numbers; real shifted-sequence success is therefore **not** established by this
run (the fake-trace test covers that branch). First/middle/last labels still refer
to discovery positions. Sampling is bounded, not exhaustive. Historical fifth-run
386 cases / 1,930 recoveries remain separate at their original source/schedules;
they are not substituted into this run's completeness accounting.

**Failed case: `noop-s512-b39-cut-after`.** The plan selected the first of three
`mutation-noop` WAL commit-marker writes, discovery sequence 39, offset **70072**,
length **24**, flags 0. The live sequence 39 at that offset was an ordinary WAL
frame header. The next observed commit marker was sequence **45**, offset
**82432**, length 24. It did not match the planned descriptor/prefix.

The controller stopped at `pre-injection-descriptor-check`, stable identifier
`controller-assertion:observe:596`; that exact source line requires
`required target group prefix changed`. Diagnostics show **no selected sequence,
no trace-verification authorization, no injection decision and no reached target**.
There were 43 I/O decision records, no acknowledgement, no done record, no partial
protocol bytes and no malformed record. The last completed I/O was write 44
(offset 78336, length/applied 4096, rc 0); write 45 remained pending. The worker was
still running at primary failure, then controller cleanup sent SIGKILL to that
test-owned child; there were no secondary cleanup errors. No recovery for this
failed case is counted.

This demonstrates rejection of the wrong pending descriptor before injection. It
also confirms that strict selected-group reproducibility still blocks full
execution. It does **not** establish the cause of the changed commit-marker
layout, recover missing historical trace data or prove an application defect.
Production entropy/time, storage model and reviewed source were not changed.

**All 379 completed cases had zero newly acknowledged responses**, both overall
and before the selected fault. Their full oracle checked previously durable
history and all-or-none ambiguous effects, exact data/schema, migration state,
head/revision/audit/idempotency linkage, integrity/FKs and same-key replay in fresh
processes. They do not prove survival of newly acknowledged effects under the
required post-response checkpoint faults. The external acknowledgement ledger
remains required; commit-marker detection is not client acknowledgement.

### Fidelity, retained evidence and limits

The runner verified production-source equality against the application pin.
Observed: go-sqlite3 **v1.14.52**, bundled SQLite **3.53.4**, source ID
`2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`;
WAL, synchronous **2/FULL**, foreign keys **1**, busy timeout **1000 ms**, one open
connection, page size **4096**, auto-checkpoint **1000**. Both modeled sector sizes
were used. `mmap_size` was explicitly unsupported with the VFS-control/no-row
reason, not fabricated as zero. Eleven baseline no-mmap checks recorded five
control rejections each, 26 reads for the two create baselines and 29 for the
other nine, with zero fetches throughout; the always-null/no-delegation fetch
policy was required. The logs retain compile options with **three entries
redacted by the existing allowlist**; their runtime values are not recovered here.

Essential SHA-256 evidence:

| Evidence | SHA-256 |
| --- | --- |
| Controller result stream, 2,125,881 bytes | `bcfdead4b048e451c86b0e16a5795b2e321a7a8f886b7729fc4e8e63e4f458f0` |
| Canonical completed-case manifest | `9a2a6b7ff37ebb7ff0e4c438b17c714ff14b178d1c1abb54af7fd3dd3e28347f` |
| Targeting test output, 3,599 bytes | `bda73272562fd77039c97b498637abd0a331ab78dd9bdd416a30ab7c7efe8821` |
| Fixture test output, 2,153 bytes | `2a0eaa9b8d4b3655a15d8ad5b2e4640d7b84797b538f49c5d89140fed9fd35ac` |
| Worker binary | `b79325aa476cf677ed10643b26ac81866270c77cbde27196b74ca52ba93d7543` |
| Genuine M1 binary | `c0bcab916eee59c2423ac1fb23ec6c231e8071e1e0564487579649a162bd525d` |
| SQLite amalgamation | `eb023455154c8da14a9920dbe44f4f9e732871ef8d8a058cafb84d18d6a2de00` |
| SQLite header | `4e7d1523cf95991f7e4c08c576e2232e063da6f10067e5c03c9bf9f904b1cf5f` |
| Failed-case trace, 173,381 bytes | `967a40c7df60a41a851867a7b244a2e5e11c99d7ff59a92ae67c1a09a94a09a7` |

The complete downloaded controller stream matched the runner's byte count/hash,
all 8,646 settings-query records and zero malformed result records; no truncated
result stream was observed. Source/dependency hashes, planned/reached descriptors,
per-case trace/ledger/image/state hashes, safe failure/cleanup diagnostics and
runner manifests remain in job logs. Raw database pages, full traces, responses
and stderr were not uploaded; hashes cannot reconstruct their contents. Failed
trace/stderr bytes existed only on the disposable runner. An independent full
raw-trace audit is unavailable after runner disposal.

Runner: public repository, standard `ubuntu-24.04`, image `20260927.320.1`, 4 CPUs,
`MemTotal: 16373452 kB`, x86_64 kernel `6.17.0-1022-azure`, Go 1.26.8, GCC 13.3.0.
Free bytes: **92,411,924,480** before tests, **92,206,338,048** before execution,
**92,200,083,456** after; remaining scratch **849,697 bytes**. The unchanged
controller checked its **4 GiB** scratch bound and **2 GiB** free reserve at case
boundaries; no resource failure occurred. Peak usage was **not measured**, so
these observations are not a continuous peak/reserve measurement.

No artifact/cache upload or paid resource was used; API artifact count is zero.
The verified public standard-runner execution is **$0** under
[GitHub's billing policy](https://docs.github.com/en/billing/concepts/product-billing/github-actions),
and job logs do not count as artifact storage. No billing/capacity change was
made. Synthetic data and marked runner-owned paths were used throughout.

### Acceptance and publication status

- Prerequisite harness validation: **PASS**; full validation/matrix: **FAIL**.
- T18: **PARTIAL**; no-op incomplete and restore/storage-boundary coverage missing.
- T30/R3: **BLOCKED**; required target mismatch, remaining families and newly
  acknowledged-effect fault recovery are incomplete.
- D6/R3: **OPEN**; M2 acceptance **PENDING**; PR #6 **DRAFT**.
- Additional execution slots: **ZERO**. No repair, rerun, merge, deployment or M3.
- Architecture deviations: **NONE**. This failure grants no new design decision.

Ordinary [CI 36868760789](https://github.com/estul26/Contextarium/actions/runs/36868760789)
passed at the workflow commit; it is separate from R3 evidence. This publication
changes only the new one-shot workflow and this evidence document. Every reviewed
source file, migration, dependency, approved contract, ordinary CI and previous
workflow remains unchanged. The subsequent reporting commit is identified in PR
#6, avoiding a self-referential SHA here.

## Review boundary

Architecture deviations: **NONE**. D1–D5 are retained. Migration 001/002, dependencies,
accepted ADRs and ordinary CI behavior are unchanged. Malformed Host port forms are rejected
consistently while valid loopback forms remain supported. There is no M3+ feature,
authenticated actor, public audit query, deployment, or real personal data.
No application defect is established by the historical failures, fifth-job missing
target or sixth-job pre-injection descriptor mismatch. Prerequisite controls and
bounded subsets of WAL fault cases passed; remaining storage/power-loss coverage
is incomplete.
This gap is not waived or represented as application acceptance.
The draft PR is not a request to merge.
