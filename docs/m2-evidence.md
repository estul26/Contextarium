# M2 implementation evidence

**IMPLEMENTATION CANDIDATE — ACCEPTANCE PENDING; D6/R3 OPEN**

Starting checkpoint: `05907846c59d909990d0c6159edce1da88dea7c6` (approved D1–D5).
`origin/main` matched that checkpoint when implementation started. Work is on
`m2-implementation`; earlier checkouts and local artifacts were preserved.
The containing implementation commit identifies this source/test tree. The draft
PR records its exact candidate SHA and the subsequent local/Linux validation
results, avoiding a self-referential commit hash in this file.

Latest source-review proposal: [bounded sampling v4](#bounded-sampling-v4--source-review-only).
Its 79 targeting/preflight, 26 classifier and 16 fixture test methods are prepared
but **NOT RUN** for this candidate. No controller, bridge, fixture binary, probe,
fault schedule or dedicated job was executed in this source-only pass. The eighth
run below remains the latest executed evidence; no execution allowance was added.

Latest executed R3 evidence: [eighth dedicated job](#eighth-dedicated-job--prerequisites-pass-bounded-matrix-times-out).
The reviewed `627e4c0` source passed all 26 classifier, 52 targeting and 16 fixture
tests, nine wrapper checks, prerequisites, five negative controls and both genuine
M1 fixtures. The bounded acceptance invocation timed out after 1,560 completed
cases / 7,800 recoveries. Explicit checkpoint completed for both sector models;
automatic checkpoint remains partial. Newly acknowledged effects survived all
7,025 recoveries in the 1,405 completed cases with a pre-fault acknowledgement,
but required acknowledgement boundaries and later families are still missing.
**T18 PARTIAL; T30 BLOCKED; D6/R3 OPEN; M2 acceptance PENDING. All eight dedicated
slots are consumed; additional execution slots ZERO.**

The earlier source-only [grouping v3](#target-grouping-v3--source-review-only) and
[WAL classifier](#wal-format-classifier--source-review-only) records retain their
historical NOT RUN status at publication. Their tests were subsequently executed
only at the exact eighth-job source below. Historical fifth/sixth/seventh cases
are not included in this candidate's completeness accounting. The old classifier's
semantic labels still require re-evaluation; no historical relabelling occurred.

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
| T18 | PARTIAL | `TestM2AtomicFailureBoundaries`, `TestM2SQLWriteAndDeferredCommitFailures`, `TestM2SerializationFailureRollsBack`, `TestM2CommittedResponseLossReplay`, `TestM2CancellationBeforeCommit` (G/R): create/data/metadata/archive/unarchive/restore, F0–F6, deferred-constraint F7, postcommit F8 and lost/truncated response. | Fifth-, sixth- and seventh-job mutation cases remain historical partial evidence. The eighth job adds completed checkpoint and partial automatic-checkpoint faults only; required mutation/restore F7 storage/sync coverage is still incomplete. No historical cases fill the eighth candidate's missing matrix. |
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
| T30 | BLOCKED | Eighth job: 26 classifier + 52 targeting + 16 fixture tests, nine wrappers, prerequisites, five negative controls and genuine M1 fixtures PASS. Bounded invocation timed out after 1,560 cases / 7,800 recoveries; explicit checkpoint complete in both sectors; 1,405 cases establish pre-fault acknowledged-effect survival. | Automatic checkpoint incomplete; later ten operation families unexecuted for this source; five required post-acknowledgement boundaries missing. All eight slots consumed. See the eighth-job reconciliation below; no final acceptance PASS. |

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

## Target identity refinement — source review only, runtime NOT RUN

This source-only refinement starts at reporting checkpoint
`d40704f97a3716f5e404b37b045e1ce8999fd0ee`. The last executed harness remains
`6fbdc611672f493626ab758dfc7ce1e9682307a1`; the application remains
`b6f63a7564977555faffe6f9ca6b1a9c22910d43`. **ZERO additional execution slots.**
No controller/test import or execution, tagged build, fixture binary, probe,
negative control, fault schedule, dispatch or rerun is part of this pass.

The sixth run's **379 completed cases / 1,895 recoveries** remain historical
partial evidence at their original source and schedules. In
`noop-s512-b39-cut-after`, the baseline first `mutation-noop` WAL commit-marker
write was sequence 39, offset 70072, length 24. The live first marker was sequence
45, offset 82432, length 24, after additional ordinary frame writes. Full-descriptor
comparison rejected it before injection. No target was selected/reached and no
recovery result exists for that failed case. The differing layout's cause remains
**unproven**. A prepared fake-trace test uses the retained metadata; it does not
replay the old run or convert that failure into passing evidence.

### Selector identity versus recorded geometry

The proposed policy is explicitly versioned
`semantic-position-wal-geometry-v2`. Full diagnostic descriptors retain
phase/name/role/op/meaning/offset/length/flags, with sequence recorded alongside.
`TARGET_FIELDS` still includes offset. Matching projects a separate selector;
all classes require exact phase, file name, role, operation, semantic meaning,
length and flags, plus the explicit position in the selected semantic group.

| Operation class | Additional selector identity | Observed geometry |
| --- | --- | --- |
| WAL append frame/commit-marker write | Positive offset required; exact write length and flags retained. Commit-marker shape requires 24 bytes and unchanged private-byte semantic classification. | Exact positive append offset may differ from discovery; global sequence may differ. Both baseline and live values are recorded. |
| WAL header reset | Offset **0**, exact length/flags; a nonzero header-reset claim is rejected. | Sequence only; no offset relaxation. |
| Truncate | Exact requested size (the VFS `offset` field), length and flags. | Sequence only; a different size fails. |
| Sync | Exact offset/length and sync flags, including FULL/data-only distinctions. | Sequence only; flags are never dropped. |
| Database/journal write | Exact physical offset, length and flags. | Sequence only; no WAL relaxation. |

**Database/journal decision for review:** retain physical offset as identity.
These writes address database pages or journal content; this harness has no
reviewed semantic page identity to replace the address safely. They can still
fail reproducibility if layout changes. That remains an explicit possible block,
not permission to inject at another page or relax the requirement later.

No production entropy/time or C/Go protocol change is needed. The same test VFS
blocks before native write/sync/truncate; the controller chooses against the live
operation. The private pre-trace check remains **exact** against that live
message, including its actual offset, sequence, length, flags and byte-derived
semantic category. Allowing discovery-to-live WAL geometry differences never
allows private-trace-to-live disagreement. After execution, the trace must still
equal the selected full live descriptor. Wrong phase/file/role/op/category cannot
receive injection; a missing required target remains a failure.

### Ordered prefix and evidence

The matched prefix now contains stable selectors/shapes. A separate baseline
prefix preserves full descriptors and sequence numbers. Each selected-group
member must match the selector at that position before the counter advances;
changed shape, strict offset or distinguishable ordering fails before an inject
reply. Events in other groups do not advance this counter.

**Indistinguishable-member rule:** members with identical stable selectors are
resolved solely by their Nth group position. This includes WAL append writes that
differ only in positive physical offset. Inserting/removing an indistinguishable
member is not detectable as a different logical page/transaction; it counts toward
the explicit position. A distinguishable prefix change fails; a shortened prefix
fails if the required position is never reached. There is no search ahead for a
nearby target, retry until a trace matches, or skip-to-PASS.

The unchanged discovery policy selects first/middle/last positions within each
recorded no-fault phase/role/semantic group, retaining collapsed labels. These are
discovery positions, not a claim that the selected live occurrence is necessarily
the midpoint/last of a changed counterfactual trace. Required families, modes,
sector sizes and recovery schedules remain mandatory.

Every permitted prefix offset change emits allowlisted `target-geometry` metadata
with its baseline/live descriptor and sequence and explicit group position before
a reply. Its stage says selector match **before private-trace verification**;
it is not an injection/recovery PASS record. Successful case reports retain
`planned`, full `observed` descriptor/sequence and `geometry_changed`; failure
diagnostics retain selected observations and verification/decision flags. The
versioned `prefix_sha256` now hashes selectors; `baseline_prefix_sha256` separately
hashes baseline descriptors. Historical manifests are not rewritten or interpreted
using the new policy. No page bytes, SQL, payloads or private paths are added.

### Prepared regression source and static checks

The targeting suite now contains **41 prepared tests, NOT RUN** (15 added; existing
range/prefix tests revised for this explicit policy). Coverage includes moved first
commit marker, baseline/live diagnostics and reports, wrong phase/file/role/op and
semantic category, incompatible length/flags, nonzero WAL header, zero append
position, changed truncate size, strict database/journal ranges, stable-prefix
shape/order, indistinguishable members and explicit positions, stale/malformed or
geometry-disagreeing private trace, verification before injection, missing targets,
first/middle/last, and the existing completeness/acknowledgement gates. The previous
blanket range-failure test now asserts both allowed WAL geometry and rejected
database/journal movement; it was not deleted. The 16 fixture tests are unchanged
and were not rerun. Earlier 26/16 passing summaries remain evidence only for the
sixth-run source.

**Static checks PASS:** Python AST parsing/counts and structural comparison; Go
formatting (`gofmt -l internal/r3`); C syntax only (`cc -fsyntax-only` against the
cached pinned go-sqlite3 v1.14.52 bundled header); whitespace, scope, links and
public-content review. No executable was built or test/controller imported.
AST comparison retains `Model`, `oracle`, consistency/replay, successful-sync
semantics, private-trace verification, selection, fault modes, all five schedules,
`AcceptanceCoverage`, external-ledger acknowledgement ordering, prerequisites,
negative controls, fixtures, no-fault mode, settings and resource limits unchanged.

New runtime feasibility and regression outcomes are **NOT RUN**. Mandatory remaining
acceptance includes complete no-op, restore, adoption/migration, fresh and empty-M1
initialization, checkpoint database writes/sync, WAL truncate/reset, both sector
sizes, every planned fault mode/all five recovery schedules, and recovery of new
acknowledged effects recorded after a complete successful response. Fifth/sixth
partial evidence cannot count as execution of this refined harness.

Only the controller, focused targeting test source and this evidence note change.
Production, migrations, dependencies, approved D1–D5, C/Go harness files, ordinary
CI and every R3 workflow/gate remain unchanged. Ordinary PR CI may run automatically;
the original false-gated R3 workflow may record a skipped run with zero steps.
Neither is R3 validation. No new workflow, permission or execution slot is added.

**T18 PARTIAL · T30 BLOCKED · D6/R3 OPEN · M2 acceptance PENDING · PR #6 DRAFT ·
Execution slots ZERO.** Architecture deviations: **NONE**; this test-selection
refinement is explicitly proposed for source review. No merge, deployment or M3.

## Seventh dedicated job — geometry case passes; stable-prefix mismatch stops matrix

The owner authorized one additional standard public-repository `ubuntu-24.04`
job, the seventh historically, with 60 minutes total, first attempt only, $0 paid
usage and no rerun, replacement, second invocation or local substitute. All six
prior allowances remained exhausted. Only separate one-shot wiring was added;
all earlier gates and all reviewed source remained unchanged.

- Reviewed harness/controller/tests: `9be4ed2d87abbfce968dcb737338dc2554878229`.
- Application: `b6f63a7564977555faffe6f9ca6b1a9c22910d43`.
- Genuine M1 source: `14dc96ad18810202f63d5ac590822f117a787e82`.
- Workflow: `06658f5ecc3b39e1088a4c7405a5898e569ba14c`.
- [Run 36880298702](https://github.com/estul26/Contextarium/actions/runs/36880298702),
  job `110430060383`, attempt **1**, **FAIL**. Job duration **181 seconds**,
  2026-10-01 14:56:16–14:59:17 UTC, including setup, builds and reporting.

**The seventh allowance is consumed. Additional execution slots: ZERO.** The
reviewed controller, tests, VFS, matching policy, application, migrations,
dependencies, oracle and persistence model were not repaired or changed. No
retry, replacement or second acceptance invocation followed the first failure.

### Gate results in execution order

| Stage | Actual result |
| --- | --- |
| `python3 scripts/test_r3_targeting.py -v` | **PASS: exactly 41/41**, `Ran 41 tests in 0.833s`, `OK`; zero failures/errors/skips/expected failures/unexpected successes. |
| `python3 scripts/test_r3_fixture.py -v` | **PASS: exactly 16/16**, `Ran 16 tests in 0.319s`, `OK`; no skips or expected failures. |
| Existing rejection-wrapper behavioral checks | **PASS: 9/9**, before setup/build; unexpected errors propagated instead of producing PASS. |
| Clean pinned source, production equality, Go formatting, pinned module download/verification, tagged vet/worker build and genuine M1 build | PASS. Modules were downloaded before resolving the bundled SQLite header. |
| Storage-model validation, native VFS probe, prerequisite IOERR, native/instrumented application and recovery/replay controls | PASS. |
| Five exact-reason negative controls and unchanged positive oracle inputs | PASS. |
| Genuine populated and empty M1 fixtures | **Both PASS**: controller SIGTERM, exit 0, WAL absence and no secondary cleanup failures. |
| Conditional application matrix | **FAIL / incomplete** after **399 cases / 1,995 fresh-process recovery checks**. |

The unchanged acceptance entry owns its prerequisite gates. One invocation of
`python3 scripts/r3-check.py --mode acceptance --worker "$R3_BUILD/worker" --m1-binary "$R3_BUILD/m1-binary"`
ran those gates and fixtures once before entering the matrix. Nothing bypassed or
repeated them. The five exact rejections were `successful sync persistence`,
`acknowledged mutation lost/changed`, `event/revision pair`,
`head/snapshot mismatch` and `idempotency revision absent`. The 41/16 suites and
nine wrapper checks establish harness behavior, not application durability.

### Completed operation/sector/fault coverage

Counts below are completed cases, not discovered targets or attempted failures.

| Operation | Sector | Cut-before | Cut-after | IOERR | FULL | Partial write | Total |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Create | 512 | 8 | 8 | 8 | 6 | 6 | 36 |
| Create | 4096 | 8 | 8 | 8 | 6 | 6 | 36 |
| Data PATCH | 512 | 8 | 8 | 8 | 6 | 6 | 36 |
| Data PATCH | 4096 | 8 | 8 | 8 | 6 | 6 | 36 |
| Metadata PATCH | 512 | 8 | 8 | 8 | 6 | 6 | 36 |
| Metadata PATCH | 4096 | 8 | 8 | 8 | 6 | 6 | 36 |
| Archive | 512 | 8 | 8 | 8 | 6 | 6 | 36 |
| Archive | 4096 | 8 | 8 | 8 | 6 | 6 | 36 |
| Unarchive | 512 | 8 | 8 | 8 | 6 | 6 | 36 |
| Unarchive | 4096 | 8 | 8 | 8 | 6 | 6 | 36 |
| No-op | 512 | 9 | 9 | 9 | 6 | 6 | 39 |
| **Total** | | **89** | **89** | **89** | **66** | **66** | **399** |

There were 11 announced family/sector plans, 89 sampled targets and 401 planned
fault combinations. No-op/512 completed **39 of 41**: `noop-s512-b44-full` failed
before injection and `noop-s512-b44-partial` was not attempted. No-op/4096,
restore, migration/adoption, fresh initialization, empty-M1 initialization,
explicit checkpoint and automatic checkpoint were **NOT RUN** at either
applicable sector size. Real fixture preparation does not count as migration
fault coverage.

Completed targets were **333 WAL writes and 66 WAL syncs**; database checkpoint
writes/syncs and WAL truncation/reset boundaries remain missing. Initial WAL
header writes do not substitute for automatic-checkpoint WAL-reset coverage.
Each completed case ran **discard/17, retain/17, reorder-torn/17,
reorder-torn/29 and reorder-torn/101**. Recovered outcomes were 1,593 wholly absent
and 402 wholly present. Reconciliation matched every case to its announced plan,
selector, compatible fault, sample labels and unique identity, verified full live
geometry against the selected policy, required all five schedules and matched
the aggregate case manifest. This is bounded sampling, not exhaustive coverage.

### Observed geometry change and first failure

One `target-geometry` record and one completed changed-geometry case were retained:
**`noop-s512-b39-cut-after`**, first WAL commit-marker occurrence in
`mutation-noop`. Discovery sequence **39**, offset **70072**, length **24**, flags
0 mapped to live sequence **45**, offset **82432**, with the same selector/shape
and group position **1**. The case passed its exact private-trace/reached-target
checks and all five recovery/replay checks (all five recovered wholly absent).
Its trace SHA-256 was
`942db8c356c2e4cdab49d5e36d6ae5ac2af104ea4e837c4251b4bd063c29bbe9`.
This is new seventh-run evidence at `9be4ed2...`; it does **not** alter the sixth
run's failed case with the same case name. That earlier failure still has no
reached target or recovery result.

**First failure: `noop-s512-b44-full`.** Stable diagnostic
`controller-assertion:observe:639`, stage `pre-injection-descriptor-check`.
The planned target was the last WAL-frame group member, position **36 of 36**,
discovery sequence **44**, offset **74240**, length **4072**, flags 0. The matcher
had accepted **34** prior members, then rejected the next pending member at live
sequence **39**, offset **70072**, length **24**, flags 0, semantic `wal-frame`.
The exact source assertion compares that member's stable selector to its expected
prefix entry. **The expected intermediate entry is hashed but not printed in
retained logs**, so the planned final target's 4072-byte length must not be treated
as the proven expectation at this earlier failing position. Which selector field
differed there, and why the layout changed, remain unproven from retained evidence.

No target was selected/reached; private-trace authorization and injection-decision
flags were false. There were 37 I/O decision records, zero acknowledgements, no
done record, no malformed record and no partial protocol bytes. The last completed
I/O was write 38 at offset 65976, length/applied 4096, rc 0; write 39 was pending.
The worker was still running at primary failure; controller cleanup then sent
SIGKILL to that test-owned child, with no secondary errors. No FULL injection or
recovery result is counted for this failed case. Matching was not relaxed and no
source change followed the failure. No application defect is established.

### Acknowledgements and acceptance gaps

**All 399 completed fault cases had zero new acknowledgements**, both overall and
before the fault decision. The ledger for the failed case was also empty. Full
recovery oracles checked integrity/FKs, subjects/schemas, records, revisions,
audit/idempotency/migration linkage, contiguous history, prior durable content,
exact JSON/schema retention and same-key replay in fresh processes. These passing
ambiguous-effect cases do not establish newly acknowledged-effect durability.

None of the twelve required operation/sector/role/semantic post-acknowledgement
combinations completed: explicit and automatic checkpoint database write/sync,
explicit WAL truncation and automatic WAL reset, for both sectors. A response must
still be complete and recorded by external-ledger write/flush/fsync before the
fault decision. No commit marker or later response is substituted for that rule.

The full completeness guard did not succeed; the matrix stopped earlier. **T18
PARTIAL**, **T30/R3 BLOCKED**, **D6/R3 OPEN**, **M2 acceptance PENDING**, **PR #6 DRAFT**.
Complete no-op, restore, adoption/initialization, checkpoint/reset/truncation and
newly acknowledged-effect coverage remain missing. Fifth-run 386 cases / 1,930
recoveries and sixth-run 379 / 1,895 remain separate historical records; they were
not loaded into seventh-run accounting.

### Fidelity, evidence retention, resources and cost

Production-source equality against the application pin passed. Recorded SQLite:
go-sqlite3 **v1.14.52**, bundled SQLite **3.53.4**, source ID
`2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`;
WAL, FULL/synchronous 2, foreign keys 1, busy timeout 1000 ms, one connection,
page size 4096 and auto-checkpoint 1000. `mmap_size` remained explicitly unsupported
with the VFS-control/no-row reason. Eleven no-mmap reports recorded five control
rejections each, 26 reads for the two create baselines and 29 for the other nine,
zero fetches, and the required always-null/no-delegation fetch policy.
Compile options remain logged with three allowlist-redacted values not recovered
here. No system SQLite or changed amalgamation was substituted.

| Retained evidence | SHA-256 |
| --- | --- |
| Complete controller stream, 2,458,053 bytes | `89efe8dac1f9c9d3686669141e272af68ae8fd82cbb5d97ea7dee0c54ab1f7dc` |
| Canonical completed-case manifest | `4c97e8b68e18f45849182817ce979a55d41ee2476fbdf8e35a2e02fd62e87e67` |
| Targeting output, 5,656 bytes | `77ceb1b5415461be14b8330ac1c57ee292fd9fd7c7600f709d450fc8e2bf4c0a` |
| Fixture output, 2,153 bytes | `b8947b24fac1c90baf7feabf8e2b80d3b4f23804f779c617951e274b11ec4279` |
| Worker binary | `7a30b99b66096ded1b62939062ec0d507cc62fef2231987479fcad3b82d3a67f` |
| Genuine M1 binary | `014b209f296c9c3c22c804ec327f9d3ff2e0a525ee946422360158117ac8a531` |
| SQLite amalgamation | `eb023455154c8da14a9920dbe44f4f9e732871ef8d8a058cafb84d18d6a2de00` |
| SQLite header | `4e7d1523cf95991f7e4c08c576e2232e063da6f10067e5c03c9bf9f904b1cf5f` |
| Failed-case trace, 147,515 bytes | `5ea2f4d513c6498d9bc8863daf1cb87fbb1ad6747993769924b0496b85ba2c18` |

The downloaded controller stream matched its exact byte count/hash, all 9,086
settings-query records and zero malformed result records. No truncated result
stream was observed. Job logs retain source/dependency/binary manifests,
planned/live descriptors, geometry changes, per-case trace/ledger/image/state
hashes, primary/cleanup diagnostics and fixture closure evidence. Raw traces,
pages, responses and stderr were not uploaded; their hashes do not reconstruct
the bytes after runner disposal. The missing intermediate baseline prefix detail
limits diagnosis, even though the result stream itself is complete.

Environment: public repository, standard `ubuntu-24.04`, image `20260927.320.1`,
4 CPUs, `MemTotal: 16373452 kB`, x86_64, kernel `6.17.0-1022-azure`, Go 1.26.8,
GCC 13.3.0. Free bytes were **92,412,993,536** before tests,
**92,207,386,624** before execution and **92,197,261,312** after. Remaining scratch
was **810,235 bytes**. Existing **4 GiB** scratch and **2 GiB** free-reserve checks
passed at case boundaries; no resource failure occurred. Peak usage was **not
measured**, so this is not a continuous peak/reserve measurement.

No artifact or cache upload occurred in the dedicated job; its artifact API
returned zero. Standard
public GitHub-hosted execution is **$0** under
[GitHub's billing policy](https://docs.github.com/en/billing/concepts/product-billing/github-actions);
no paid service, larger runner, billing change or capacity purchase was used.
Synthetic databases and marked runner-owned temporary paths were used throughout.

Only the new seventh one-shot workflow and this evidence update are published.
The reporting push cannot trigger the consumed workflow. The original dirty patch,
local approval records, binaries, databases and unrelated worktrees remain
preserved. Ordinary [CI 36880305839](https://github.com/estul26/Contextarium/actions/runs/36880305839)
passed at the workflow commit; it is separate from R3 evidence. The reporting SHA
is recorded in PR #6. Architecture deviations: **NONE**. No owner acceptance,
merge, deployment or M3. **Additional execution slots: ZERO.**

## Target grouping v3 — source review only

Prepared from reporting checkpoint `8b9327d4692319e46cfee9608646c168bef5ead0`.
The containing commit and PR identify the new source-review candidate; it has
**not** been executed. Application source remains equal to
`b6f63a7564977555faffe6f9ca6b1a9c22910d43`. Last executed harness remains
`9be4ed2d87abbfce968dcb737338dc2554878229`.

Source confirms that the previous discovery key `(phase, role, meaning)` could
combine multiple distinct selectors (including different lengths or flags),
while the matcher compared their ordered selector prefix. This grouping mismatch
is corrected below. The seventh run's **399 cases / 1,995 recoveries** remain
historical partial evidence with their original source, schedules and hashes.
`noop-s512-b44-full` remains **failed before injection**: no injection/recovery
result exists for it. The expected intermediate prefix member was not retained;
the final target's 4,072-byte descriptor cannot establish which field differed
at that intermediate position. Neither that field nor the source of historical
trace variation is proven by this source change.

### One selector class per group

Policy identifier: `stable-selector-group-nth-v3`. Both discovery and live
matching call `target_group`, defined as the sorted key/value tuple of the
**same existing `target_selector` projection** used to authorize injection:

| Operation class | Stable group identity |
| --- | --- |
| Positive WAL append write (`wal-frame` or `wal-commit-marker`) | phase, name/file, role, op, meaning, length, flags, offset_policy=`positive-wal-append`; positive physical offset excluded |
| All strict classes | phase, name/file, role, op, meaning, length, flags, offset_policy=`exact`, exact offset |

Strict classes include WAL header reset at offset zero, truncate's exact
requested size in `offset`, database/journal writes at exact offsets, and sync's
exact offset/length/flags. Commit markers still require 24 bytes. No global
length, flag or offset relaxation is introduced. A mixed-class discovery plan
is rejected. The C change only updates the decision-protocol comment.

Discovery retains first/middle/last at positions `1`, `floor(count/2)+1`, and
`count` **within every stable selector class**, coalescing duplicate positions
while retaining every applicable label. Splitting the former coarse groups can
increase the selected target count; no family or first/middle/last label is
removed. This is a documented change to bounded sampling, not exhaustive
coverage or a promise that a future execution will finish within an allowance.

The matcher selects the **Nth live member of the planned selector group**.
Other shapes receive only a continue reply, do not advance N, and cannot cause
a prefix mismatch. Missing N still fails. Indistinguishable members have only
ordered position as identity; insertion/removal within the exact same class can
change which occurrence occupies N. There is no search ahead, retry, nearest
address or skip-to-PASS. Baseline sequence, full geometry, group count and sample
labels remain evidence, not a constraint on the live suffix.

Before an inject reply, the exact selector and Nth position must match, followed
by the unchanged private pre-trace check against the **live** sequence, actual
offset, length, flags and byte-derived meaning. The post-trace still checks the
selected live descriptor exactly. A permitted WAL offset difference is recorded
with both full baseline and live descriptors; it does not bypass private-trace
verification.

Failure logs retain the planned selector, full baseline descriptor/sequence,
position/count/labels, matching-live count, last eight candidate descriptors,
selected descriptor and trace/decision/reached flags. Relevant alternative
shapes (same phase **or** file) retain at most eight selector/count/last-descriptor
entries, with a total event count and explicit unlisted-event overflow count.
The existing outer failure record retains the complete acknowledgement count;
target diagnostics also retain acknowledged-before-fault count. Values are
allowlisted labels and integers, without page bytes, SQL, request bodies,
arbitrary exception text, credentials or private paths. Prefix hashes no longer
stand in for observable selector fields.

### Future order and unchanged acceptance

After the existing prerequisites and both real M1 fixtures, future matrix order
is: **explicit checkpoint → automatic checkpoint/WAL reset → restore → M1
migration/adoption → fresh initialization → empty-M1 initialization → no-op →
create → data PATCH → metadata PATCH → archive → unarchive**. Prerequisite
create controls and request objects remain unchanged. Ordering rejects missing
or duplicate families.

Final PASS still requires all 12 operation families, both 512/4,096 sector
models, every selected target/compatible fault mode, and all five recovery
schedules: discard/17, retain/17, reorder-torn/17, /29 and /101. The existing
newly acknowledged-effect guards remain unchanged. A complete successful
response must be received and fsynced in the external ledger before the relevant
fault; a commit marker is not an acknowledgement. Each invocation starts fresh
accounting; fifth/sixth/seventh results cannot fill future candidate coverage.
Model, successful-sync guarantees, oracle, replay and completeness semantics
are unchanged.

### Prepared regression source and static checks

The targeting file contains **52 test methods, NOT RUN** at this candidate,
including separate frame-header/page-data/commit groups, positive WAL geometry,
strict length/flags/database/journal/sync/truncate/header identities, mixed-plan
rejection, Nth/missing-N selection, exact private pre-trace checks, bounded safe
selector diagnostics, first/middle/last labels and matrix order. The existing
acknowledgement/completeness test methods remain unchanged. The 16 fixture tests
are unchanged and also **NOT RUN** in this source-only pass.

Executed static checks only:

- Python `ast.parse` on controller, targeting tests and unchanged fixture tests;
  structural comparison against the reporting checkpoint confirmed protected
  model/oracle/replay/selector/trace/ledger/acceptance definitions unchanged.
- `gofmt -l internal/r3`: no output.
- `cc -fsyntax-only` for `internal/r3/vfs/vfs.c`, using the cached pinned
  go-sqlite3 v1.14.52 bundled SQLite header (SHA-256
  `4e7d1523cf95991f7e4c08c576e2232e063da6f10067e5c03c9bf9f904b1cf5f`).
- Whitespace, scope and public-content review; production source equality,
  dependencies, contracts and all workflow gates unchanged.

No tests, controller import/execution, harness build, fixture binary, probe,
negative control, fault schedule or dedicated Actions job ran. Ordinary PR CI
may run automatically and supplies no R3 validation. The original disabled
workflow and every consumed one-shot workflow are unchanged.

Remaining gaps include complete no-op coverage, restore, migration/adoption,
fresh/empty-M1 initialization, checkpoint/database writes/sync/truncation/WAL
reset and survival of newly acknowledged effects. All 399 seventh-run completed
cases had zero acknowledgements; that remains an acceptance gap. V3 grouping,
prepared tests and revised ordering require separately authorized execution.
**T18 PARTIAL; T30 BLOCKED; D6/R3 OPEN; M2 acceptance PENDING; PR #6 DRAFT;
additional execution slots ZERO.** Architecture deviations: **NONE**.

## WAL format classifier — source review only

Prepared from `8f7a469a3d20507e8cfe6fbe940523e6d45a5071`; the containing commit
and PR identify the correction. No execution was authorized or performed.
Application remains `b6f63a7564977555faffe6f9ca6b1a9c22910d43`; last executed
harness remains `9be4ed2d87abbfce968dcb737338dc2554878229`.

### Source-supported defect and format rule

Both old classifiers treated a 24-byte positive-offset write with nonzero bytes
4–7 as a commit marker without checking frame alignment. At page size 4,096,
offset **56** is page data, so this rule can misclassify an ordinary page fragment.
This is a static counterexample, **not** a reconstruction of the seventh run's
missing expected prefix member or proof of its historical root cause.

The pinned go-sqlite3 v1.14.52 amalgamation (SQLite 3.53.4, SHA-256
`eb023455154c8da14a9920dbe44f4f9e732871ef8d8a058cafb84d18d6a2de00`)
defines the format in its WAL-file comment, `walFrameOffset`, `walIndexRecover`,
`walEncodeFrame`, `walWriteOneFrame` and `walWriteToLog`. It agrees with the
[SQLite WAL format](https://www.sqlite.org/fileformat2.html#walformat): 32-byte
WAL header, then frames of 24-byte header plus database-page-size bytes. Header
starts are `32 + N * (page_size + 24)`; page data starts 24 bytes later. The
bundled `walWriteToLog` can split a write at a sync point, so length alone cannot
identify either boundary.

The test-only C helper and independent Python classifier now validate WAL-header
magic, version, power-of-two database page size **512–65,536**, and the header
checksum in the byte order selected by its magic. A marker label additionally
requires exact frame-header alignment, a complete 24-byte header, nonzero page
number, matching current WAL salts and nonzero database-size field. A complete
aligned non-commit header is `wal-frame`; page-contained data/fragments are
`wal-page-data`. Modeled device sector size is not an input to this calculation.

This labels the **requested complete header write**, not a verified complete
frame/checksum chain, a successful transaction or client acknowledgement. A
partial fault on that request still records the actual applied bytes/error;
it does not become evidence that all 24 bytes or the page were persisted.
The existing recovery oracle and external acknowledgement ledger retain their
separate roles.

### Geometry context, reset and unsupported shapes

Each WAL file handle begins with unknown geometry. The VFS observes only bytes
already returned by SQLite's existing successful header reads or applied by
its intercepted writes. It adds no reads, recursive SQLite calls or settings
queries. Fresh initialization obtains pending reset geometry from the complete
32-byte header being written; successful application installs that context for
following frames. Existing WAL files can establish context through SQLite's
own full header read. WAL reset/reuse replaces the stored header/page size/salts.
A new file handle never inherits the old handle's context.

A header-overlapping partial observation invalidates the context; split header
parts are **not** assembled speculatively. A write applying zero bytes leaves
context unchanged. Successful truncate below 32 bytes invalidates it; a retained
complete header keeps it. Failed header reads clear the context. If no validated
context is observed before a frame write, its label is `wal-unknown`, never a
commit marker. This conservative path can leave required semantic coverage
missing and must fail acceptance; no runtime confirmation of all paths exists.

Split frame-header writes are `wal-frame-fragment`, partial WAL headers are
`wal-header-fragment`, and mixed/cross-boundary/invalid shapes are `wal-raw`.
They remain selectable **raw write fault** targets, with exact offsets. They
cannot satisfy the required complete commit-marker target. Supporting a future
unsupported shape would require a reviewed correction, not a skip or relabeling.

Private pre-trace rows retain the current validated header bytes alongside the
pending operation; the public protocol carries only the bounded page-size
integer. Python independently validates those header bytes and recomputes the
semantic label from exact live geometry and private write bytes before injection.
`wal_page_size` is an additional exact descriptor/selector field, preventing
cross-page-size matching. V3 grouping from the selector, Nth-member resolution,
exact live pre/post trace checks, length/flags and strict offsets are retained.
Validated page-data writes inherit the positive-WAL-offset geometry policy;
unknown, mixed and partial-header shapes stay strict. No bytes/salts are added
to public diagnostics.

### Prepared tests and static evidence

**78 classifier/targeting tests prepared, NOT RUN:** 26 new methods in
`test_r3_wal_classifier.py` plus the 52 targeting methods with corrected format
fixtures. The unchanged 16 fixture tests are also NOT RUN in this pass
(**94 methods** across the three suites, not an executed count).

Prepared cases cover valid commit/non-commit headers, offset 56 and other
24-byte page fragments, all supported page sizes, independent sector sizes,
both header checksum byte orders, invalid header fields/checksum, frame/page
splits, mixed writes, unknown context, reset/reuse, partial/zero-applied header
writes, truncate/reopen/read context, old salts and exact pre-injection checks.
The new suite, if separately authorized later, builds a small bridge to the
same pure C helper used by the VFS. Both C and Python must match explicit
format-derived expectations; agreement between them alone is insufficient.
That bridge was **not built or loaded** in this pass. Synthetic fixture builders
supply valid headers, salts and first-frame checksums; later-frame grouping
fixtures assert geometry and do not pretend to establish checksum-chain history.
Existing acknowledgement/completeness assertions are retained.

Static checks actually performed:

- Python AST parsing of controller, both targeting/classifier suites, synthetic
  builders and unchanged fixture suite; no imports or test execution.
- AST comparison confirms model, oracle, replay, Nth matcher, pre/post trace
  verification, acknowledgement recording, acceptance guards, prerequisites,
  checkpoint-first matrix, fault modes and recovery schedules unchanged.
- `gofmt -l internal/r3`: no output. C `-std=c99 -Wall -Wextra -Werror
  -fsyntax-only` on the VFS with the pinned bundled SQLite header and on the
  literal regression bridge; no executable/shared library or tagged build.
- Whitespace, scope, public-content and production-source equality review;
  every workflow gate, ordinary CI, dependencies, migrations and D1–D5 unchanged.

### Historical qualification and open gates

All historical counts, outcomes, manifests and failed-case records remain
intact, including seventh-run **399 cases / 1,995 recoveries** and failed
`noop-s512-b44-full` with **no injection/recovery result**. **Semantic labels
potentially affected by the old classifier need re-evaluation.** No cases are
retroactively relabeled without the necessary retained bytes/context. Raw
fault/recovery observations and claims of semantic commit-boundary coverage are
not interchangeable; the complete historical runtime cause remains unproven.

The new classifier, updated fixtures and regressions have no execution evidence.
Complete no-op, restore, migration/adoption, initialization, checkpoint/WAL-reset
and newly acknowledged-effect coverage remain open. Checkpoint-first ordering,
every family/sector/selected fault mode, all five recoveries, successful-sync
guarantees and complete-response external-ledger acknowledgement gates remain
unchanged. Historical cases cannot fill a future candidate's completeness count.
Ordinary automatic PR CI is not R3 validation. No dedicated job, test, tagged
harness, fixture binary, probe, negative control or fault schedule was executed.
**T18 PARTIAL; T30 BLOCKED; D6/R3 OPEN; M2 acceptance PENDING; PR #6 DRAFT;
additional execution slots ZERO.** Architecture deviations: **NONE**.

## Review boundary

Architecture deviations: **NONE**. D1–D5 are retained. Migration 001/002, dependencies,
accepted ADRs and ordinary CI behavior are unchanged. Malformed Host port forms are rejected
consistently while valid loopback forms remain supported. There is no M3+ feature,
authenticated actor, public audit query, deployment, or real personal data.
No application defect is established by the historical failures, fifth-job missing
target, sixth-job pre-injection descriptor mismatch or seventh-job stable-prefix
mismatch. Prerequisite controls and
bounded subsets of WAL fault cases passed; remaining storage/power-loss coverage
is incomplete.
This gap is not waived or represented as application acceptance.
The draft PR is not a request to merge.

## Eighth dedicated job — prerequisites pass; bounded matrix times out

**Executed 2026-10-01. Prerequisite harness validation PASS; bounded application
matrix INCOMPLETE / job FAIL (timeout). T18 PARTIAL; T30 BLOCKED; D6/R3 OPEN;
M2 acceptance PENDING; PR #6 DRAFT; additional execution slots ZERO.**

- [Run 36896062479](https://github.com/estul26/Contextarium/actions/runs/36896062479),
  [job 110483242949](https://github.com/estul26/Contextarium/actions/runs/36896062479/job/110483242949),
  push event, attempt 1, one dedicated job: **55m 58s**, 16:59:33–17:55:31 UTC.
- Tested harness/controller/tests: `627e4c0fc392410910ec71ed7d522d1a7e27eefe`.
- Unchanged application: `b6f63a7564977555faffe6f9ca6b1a9c22910d43`.
- Genuine M1 source: `14dc96ad18810202f63d5ac590822f117a787e82`.
- Workflow commit: `1834d51e356a12674d178ae9c2cdc008c6343675`, separately recorded
  from the checked-out harness. Only `.github/workflows/r3-eighth-once.yml` was
  added for this authorization; prior workflows and ordinary CI were unchanged.
- The 60-minute job used one acceptance invocation with a 3,300-second process
  bound, leaving reporting time. At 17:55:22 UTC the bounded command ended with
  exit **137**, exactly 55 minutes after its step began. The reporting step passed.
  No repair, rerun, replacement, second acceptance invocation or local harness
  execution followed. This is a time-bound exhaustion, not evidence of a failed
  durability invariant in a completed case.

The workflow checks the repository is public, the runner is standard GitHub-hosted
Linux, the exact predecessor/branch/commit message, attempt 1 and a single workflow
run. Its path filter excludes reporting pushes and it has no dispatch trigger.
The final API census contains one run, attempt 1, and zero artifacts. Previous
allowances remain consumed; the original false gate remains unchanged.

### Executed gates and commands

Each command below executed once, in order, without skipped, expected-failure or
unexpected-success test cases. The temporary C bridge built and loaded successfully
against the reviewed `wal_geometry.h`; the classifier suite was not Python-only.

| Gate | Actual result |
| --- | --- |
| C compiler availability | PASS; `cc` and `gcc` available, GCC 13.3.0 |
| `python3 scripts/test_r3_wal_classifier.py -v` | PASS: `Ran 26 tests in 1.663s`; `OK` |
| `python3 scripts/test_r3_targeting.py -v` | PASS: `Ran 52 tests in 1.252s`; `OK` |
| `python3 scripts/test_r3_fixture.py -v` | PASS: `Ran 16 tests in 0.357s`; `OK` |
| Nine rejection-wrapper checks | PASS, 9/9: exact assertion; different assertion; no rejection; worker assertion; timeout; worker exit; malformed data; missing field; failed lookup |
| Source/dependency/build checks | PASS: clean exact checkout, production-source equality, formatting, `go mod download`, `go mod verify`, tagged vet and worker build; genuine M1 build from its pinned archive |
| Independent storage model | PASS: visibility, successful/failed sync, truncate, namespace and deterministic reconstruction |
| Real intercepted VFS checks | PASS: visibility/read/write/sync/truncate/create/delete and reached failed-sync probe |
| Native/instrumented application controls | PASS: actual mutation, positive consistency oracle and replay controls |
| Five exact-reason negative controls | PASS; reasons below |
| Genuine populated and empty M1 fixtures | PASS: applicable HTTP preparation, controller-requested graceful SIGTERM, exit 0, clean-close/WAL postconditions; no cleanup errors |

The **94 test methods** and nine wrapper checks are harness-validation evidence,
not simulated application durability results. The acceptance command was exactly:

```sh
python3 scripts/r3-check.py --mode acceptance --worker "$R3_BUILD/worker" --m1-binary "$R3_BUILD/m1-binary"
```

This single entry performed the prerequisites and real fixtures before beginning
the matrix. Positive oracle inputs remained unchanged. Negative controls rejected
only for their expected reasons:

| Negative control | Recorded rejection |
| --- | --- |
| incorrect-sync-semantics | `successful sync persistence` |
| lost-acknowledged-mutation | `acknowledged mutation lost/changed` |
| broken-audit-linkage | `event/revision pair` |
| broken-revision-linkage | `head/snapshot mismatch` |
| broken-idempotency-linkage | `idempotency revision absent` |

### Reconciled application coverage

An independent reader of the downloaded job log (without importing/executing the
harness) checked each completed case against its announced plan: unique identity,
all selector fields, validated page size, permitted positive-WAL offset geometry,
exact live sequence/descriptor, compatible fault mode and first/middle/last labels.
Every counted case records all five recoveries, state/image hashes and replay
success. Counts and the canonical case hash match the runner's retained manifest.
The complete 10,179,133-byte controller stream also matches its hash; zero malformed
records were reported. Private trace bytes are no longer available for independent
reclassification; their hashes bind the runtime verification records. There is no controller `FINAL` record because of the kill.
A complete retained stream does not mean a complete acceptance matrix.

| Family | Sector | Completed / announced cases | cut-before | cut-after | IOERR | FULL | partial write | Recoveries |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Explicit checkpoint | 512 | 345 / 345 | 71 | 71 | 71 | 67 | 65 | 1,725 |
| Explicit checkpoint | 4096 | 345 / 345 | 71 | 71 | 71 | 67 | 65 | 1,725 |
| Automatic checkpoint | 512 | 870 / 7,843 | 175 | 175 | 174 | 173 | 173 | 4,350 |
| Automatic checkpoint | 4096 | NOT RUN / not discovered | 0 | 0 | 0 | 0 | 0 | 0 |
| Restore, migration/adoption, fresh initialization, empty-M1 initialization, no-op, create, data PATCH, metadata PATCH, archive, unarchive | Both | NOT RUN / not discovered | 0 | 0 | 0 | 0 | 0 | 0 |
| **Total completed** | | **1,560** | **317** | **317** | **316** | **307** | **303** | **7,800** |

For every completed case, schedules were exactly `discard/17`, `retain/17`,
`reorder-torn/17`, `reorder-torn/29`, `reorder-torn/101`: **1,560 recoveries per
schedule**. Outcomes were 7,141 wholly present and 659 wholly absent; no partial
cross-store outcome passed. Required consistency includes integrity/FKs, schema
and subject retention, migration ledger, records/revisions/audit/idempotency,
contiguous heads/snapshots, exact JSON tokens/schema pair, prior durable history
and same-key replay in fresh processes. There are 6,973 uncompleted combinations
in the already announced automatic-checkpoint plan, plus undiscovered families.
The reviewed checkpoint-first order, compatible modes and completeness guard
were unchanged; historical cases are not counted toward these totals.

Reached operations: WAL writes 330, WAL syncs 21, database writes 1,187,
database syncs 6, database truncates 8 and WAL truncates 8. Format-derived WAL
write labels were: header reset 15, non-commit frame header 40, complete
commit-marker header 25, page data 240 and partial frame header 10. The ten partial
headers remain **raw write-fault coverage**, not complete commit markers. No
unknown or unsupported write was relabelled to satisfy semantic coverage. The
actual database page size was 4096 in both device-sector models. A complete marker
header label alone establishes neither durable commit nor client acknowledgement.
Earlier runs' old-classifier semantic labels remain qualified and unchanged.

**New acknowledged effects:** 1,405 completed cases had complete successful
mutation responses in the independent ledger before injection (284 per explicit
checkpoint sector; 837 automatic-checkpoint/512). All **7,025** corresponding
recoveries retained their acknowledged effects and passed replay without duplicate
effects. The recorded pre-fault count ranged from 1 to 19 per case; these are
repeated disposable scenarios, not a claim of that many unique production effects.
The unchanged controller writes, flushes and fsyncs the ledger before exposing an
acknowledgement to the fault decision. All 155 other completed cases had zero
acknowledgements and were allowed wholly present/absent outcomes.

The required post-ack database write/sync and WAL truncate checks completed for
explicit checkpoint in both sectors. Automatic checkpoint/512 covered acknowledged
database writes but still lacks acknowledged database sync and WAL-header-reset
coverage. Automatic checkpoint/4096 lacks all three required categories. Those
**five missing acknowledgement categories** prevent final acceptance independently
of the missing operation families. Model successful-sync guarantees were unchanged.

### Timeout evidence and limits

Last completed case: `autocheckpoint-s512-b2262-cut-after`. The next in-flight
case was `autocheckpoint-s512-b2262-ioerr`; its last public settings query completed
`max_open_connections=1`. Its plan targeted `autocheckpoint/store.db/database/write`,
meaning `write`, exact offset **688128**, length **4096**, flags **0**, WAL context
**0**, N=1 of 1, baseline sequence **2262**, labels first/middle/last. There is no
completed injection/recovery result for that case; it is not included in counts.

The last retained private-trace metadata is only `stage=post`, sequence **5162**,
`rc=0`, `applied=3776`. This post row lacks the operation, offset, page context and
selector fields needed to bind it to the planned target. It is not proof that the
planned IOERR was or was not injected. No controller exception/worker-failure or
cleanup-error diagnostic survived the hard timeout; natural child exit, exact
in-flight stage, pre-injection verification/reached flags and partial recovery
progress are unestablished. The log retains file hashes/sizes, not the private
trace/ledger/stderr contents. Do not invent a mismatch, failed SQLite invariant,
missing stderr chain or exact slow stage from this limited evidence.

The 55-minute command bound expired before the reviewed matrix could complete.
The partial automatic-checkpoint plan alone announces 7,843 cases for sector 512;
its remaining work was not skipped, narrowed or retried. Timing evidence does not
establish an application defect. Closing T18/T30 still requires the missing
mutation/restore, migration/initialization, automatic-checkpoint/reset and full
acknowledged-effect evidence under a separately reviewed and authorized execution.
No additional execution is authorized by this report.

### Fidelity, manifests, resource and retention evidence

Standard GitHub-hosted `ubuntu-24.04`, image `20260927.320.1`, x86_64,
4 logical CPUs, `MemTotal: 16373452 kB`, kernel `6.17.0-1022-azure`,
Go `1.26.8`, GCC `13.3.0`. Application-source equality was verified before building.
Modules were downloaded before resolving the bundled header; no system SQLite was
selected. go-sqlite3 is `v1.14.52`; actual SQLite is `3.53.4`, source ID
`2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`.

Observed settings: foreign_keys=1, journal_mode=wal, synchronous=2/FULL,
busy_timeout=1000, max_open_connections=1, page_size=4096, wal_autocheckpoint=1000.
`mmap_size` was explicitly unsupported (`vfs-control-notfound-no-row`); each
executed family/sector baseline passed the no-mmap bypass checks. Compile options
were logged through the existing allowlist; three entries were redacted, so the
public list is not an unredacted compiler-options transcript.

| Source/binary | SHA-256 from the executed build manifest |
| --- | --- |
| `go.mod` | `c7023abb4aec085d90899acecd2a4c2cbe4bc9081d781ad9f64a897ca9d236a1` |
| `go.sum` | `1de26858cca5c22e551d270f2788c128530c149e3f2a321d80902480831a9ca2` |
| `internal/r3/vfs/vfs.c` | `8d8e6964bb24b5464f73dd3b94a1bf5e2938ebaeeefda1c803d148ab2e983a61` |
| `internal/r3/vfs/vfs.go` | `4ee3d9df8c5dedc0dde712c8f206615a84c6222cbae2f368dae4b59f03eb15dc` |
| `internal/r3/vfs/wal_geometry.h` | `b9c4c78fd70a3e71245907c011944b30c60249d1989e988b563eac495e7dbf8a` |
| `internal/r3/worker/main.go` | `1f3943bb36af9071d3d52dd9028e174973a49dceda5f836e74cc05617db128d5` |
| `m1-binary` | `25c0afbdf927c39bceff7558a295d82baa2cf63760f1f846b9d681d5b38345c0` |
| `m1/go.mod` | `c7023abb4aec085d90899acecd2a4c2cbe4bc9081d781ad9f64a897ca9d236a1` |
| `m1/go.sum` | `1de26858cca5c22e551d270f2788c128530c149e3f2a321d80902480831a9ca2` |
| `scripts/r3-check.py` | `deab5ec4d2a1d2941779baf1af0a23dcf4dc78ebd0098619b881d26b187561c9` |
| `scripts/r3_wal_test_vectors.py` | `0e8330960d6523d271d6508c00982d71a33b1e0edc0fd576bd776697c202aeb4` |
| `scripts/test_r3_fixture.py` | `4326a6930d06467ed3a7a0d41037e1c6c45bfcaa103e5f2c46181fdb0d5ea433` |
| `scripts/test_r3_targeting.py` | `928b0f2a61b91ad6e29b2c459cf9b5209ee43800b5919682fd8dd09360687b0d` |
| `scripts/test_r3_wal_classifier.py` | `cf14904dad3e370d49a07a058f697e1fb58f28c2fbb3e139ed61566716c9cdf9` |
| `sqlite3-binding.c` | `eb023455154c8da14a9920dbe44f4f9e732871ef8d8a058cafb84d18d6a2de00` |
| `sqlite3-binding.h` | `4e7d1523cf95991f7e4c08c576e2232e063da6f10067e5c03c9bf9f904b1cf5f` |
| `worker` | `decd353da3a4df50af7e0d3746dce69ca9787eee190d7a76263cef281849f020` |

Retained manifests (SHA-256):

| Evidence | Bytes | SHA-256 |
| --- | ---: | --- |
| controller JSONL | 10,179,133 | `61ad3a68e90fd1d16c1f33e05b7bad1f8a48d133e34082c27c237f1142e15108` |
| Canonical completed-case manifest | — | `88ead355d3d4f4e68a67e3a730d38e11fcd4b811157a61997bcd6a4b277cd12d` |
| `classifier-tests.log` | 3,855 | `7d93dd82277c2c166e50bee752755a0735db13aa619b3a2da61080ffa5d333fb` |
| `targeting-tests.log` | 7,287 | `cd0a35c241d81d93ceae98b292ab52d3b872421589c8f1d80baf0d80fba9a233` |
| `fixture-tests.log` | 2,153 | `1cc7774c93e98e35d1c5fe66c8c23401338ad0e33d574d759227ea6ddf20e9ed` |
| `wrapper-results.json` | 633 | `4d96fb05b495895e539fc9a59ad3ca5589ba358eca5c6523b41b100e97c0810d` |
| `trace.jsonl` | 25,969,427 | `7fc446995397019ba63475e19c16e2c5eef8c507cf85a8cdda64a654f6186e45` |
| `acks.jsonl` | 3,880,356 | `17e94da4392223bdb2412906f4a9e5d61b4aca5724caf28ef7f03a397c724e78` |
| `worker-stderr.txt` | 1,107,073 | `8f956cf682d01cbb723c4a4a3cbbb87f614976171e6f46e5e33f9d53342fbbdc` |

The job logs additionally retain each case's planned/live descriptor, trace and
ledger hashes, schedule/seed, recovered image/state hashes, sanitized unit-test
summaries, fixture close records and source/build manifests. No artifact or cache
uploads were configured in the R3 job; the artifact API returned zero. Private
page traces, raw worker output, database images and ledgers remained on the
marked disposable runner and were not retained after runner disposal. Local
report reconciliation used the downloaded public job logs only.

Modeled scratch cap: 4,294,967,296 bytes; free-space reserve: 2,147,483,648 bytes.
Observed free space before acceptance: **92,207,206,400 bytes**; after:
**92,149,215,232 bytes**. Remaining marked scratch: **40,136,603 bytes**. The reviewed
controller sampled its cap/reserve checks at case entry; no resource-bound error
was reported. Continuous peak usage was **not measured**, so endpoint observations
must not be presented as a measured high-water mark or an OS-enforced quota.
Only synthetic/test-owned runner paths and processes were used. No real disk
filling, host reboot, personal databases, production fault switches or paid
services were used. Paid usage for this standard public-runner job is **$0** under
[GitHub's public-repository runner policy](https://docs.github.com/en/billing/concepts/product-billing/github-actions);
no billing was enabled or purchased. Ordinary PR CI remained separate and passed
at the workflow candidate ([36896068963](https://github.com/estul26/Contextarium/actions/runs/36896068963));
it is not R3 evidence.

Workflow publication checks were YAML parsing, Bash syntax, embedded Python AST
parsing, scope/privacy review and whitespace checks. No reviewed harness source,
application code, migrations, dependencies, model/oracle/replay semantics, D1–D5,
previous workflow gate or ordinary CI behavior was changed. The original dirty
patch and unrelated worktrees/artifacts were preserved. Architecture deviations:
**NONE**. Evidence publication does not grant owner acceptance or merge readiness.

## Bounded sampling v4 — source review only

**SOURCE REVIEW ONLY — runtime NOT RUN. T18 PARTIAL; T30 BLOCKED; D6/R3 OPEN;
M2 acceptance PENDING; PR #6 DRAFT; execution slots ZERO.**

Prepared from reporting checkpoint `cbe6dda9b859325f00ad242e93e6d8727390c0a1`.
Last executed harness remains `627e4c0fc392410910ec71ed7d522d1a7e27eefe`; production
application remains `b6f63a7564977555faffe6f9ca6b1a9c22910d43`. Only the controller,
targeting/preflight test source and this evidence document change. No workflow
wiring or gate is changed, and no new execution is authorized.

The eighth run is preserved exactly: checkpoint/512 **345/345**, checkpoint/4096
**345/345**, autocheckpoint/512 **870 completed**, total **1,560 cases / 7,800
recoveries**, including **1,405 pre-fault acknowledged cases / 7,025 acknowledged
recoveries**. Its timeout and interrupted `autocheckpoint-s512-b2262-ioerr` remain
incomplete. Previous semantic-label qualifications, manifests and historical
counts are unchanged. No historical case contributes to a future coded
completeness check.

### Coverage buckets versus exact injection checks

Policy identifier: `bounded-coverage-bucket-nth-v4`. `coverage_bucket` controls
sampling and Nth live-member selection. Its common key fields are **phase, name,
role, operation, semantic meaning, flags and validated WAL page size**. Meaning
comes from the unchanged format classifier; unknown context remains zero, not an
invented page size. Additional key fields are:

| I/O class | Additional sampling identity |
| --- | --- |
| Database/journal write (each phase separately) | `shape=write`, exact write length; physical offset excluded |
| Complete WAL header/reset | `shape=wal-header-reset`, length 32, offset 0 |
| Complete non-commit frame header | `shape=wal-frame`, length 24 |
| Complete commit-marker header | `shape=wal-commit-marker`, length 24 |
| Complete WAL page data | `shape=wal-page-full`; valid page-data start and length equal to the validated database page size |
| WAL page-data fragment | `shape=wal-page-fragment`; incidental offset/length excluded |
| WAL frame-header fragment | `shape=wal-frame-fragment`; incidental offset/length excluded |
| WAL header fragment | `shape=wal-header-fragment`; incidental offset/length excluded |
| Unsupported raw / unknown-context WAL write | Separate `wal-raw` / `wal-unknown` shapes; incidental offset/length excluded |
| Sync, including WAL sync | `shape=sync`, exact offset/length; common flags remain exact |
| Truncate, including WAL truncate | `shape=truncate`, exact requested size in offset and exact length |

WAL append offsets are excluded from coverage buckets, except the mandatory
header-reset offset zero. Different page sizes, flags, files, phases, meanings
and full-page/fragment shapes remain different classes. Raw and fragment classes
never satisfy the required complete commit-marker class. Database/journal length
variation still makes separate buckets; this is not a global relaxation of write
length or flags.

Discovery samples first, middle (`count//2+1`), and last positions in each bucket,
coalescing duplicate positions while retaining all labels. Execution selects the
Nth **LIVE** member of that bucket; other buckets do not advance it. Physical
baseline/live offsets, fragment lengths, sequence, flags, page context and meaning
remain in planned/live evidence. The original `target_selector` projection is
retained for baseline/live evidence, not reused as the sampling key or a physical
address search. Changing an in-bucket live geometry does not claim it is the same
logical page or transaction as the baseline member.

Before an inject reply, the selected live candidate must pass the unchanged
`verify_pending_trace`: exact live sequence plus the entire descriptor, including
actual offset/length/flags, validated page size and byte-derived classification.
Post-trace verification against that selected live descriptor is unchanged.
Missing N fails; there is no search ahead, nearest address, retry, substitution or
skip-to-PASS. Diagnostics retain planned bucket/position/count/labels and full
baseline/live descriptors, bounded alternate-bucket counts, and decision/trace
flags. No page bytes or request payloads are added to public evidence.

### Full plan and execution-budget preflight

`application_cases` yields nothing until all these steps complete:

1. `compile_application_plans` discovers and validates every **12-family x
   2-sector** no-fault baseline in checkpoint-first order. Genuine M1 fixtures
   and the existing prerequisite probes/negative controls still precede this
   stage; they are separate from injected application matrix cases.
2. `preflight_application_plans` validates complete family/sector identity,
   unique targets, bucket linkage, all first/middle/last positions and labels,
   compatible modes and count/recovery arithmetic. It emits one `application-plan`
   summary with each family's targets/cases/recoveries, per-bucket member counts
   and sampled positions, required acknowledgement categories and five schedules.
3. It checks **MAX_PLANNED_FAULT_CASES=1000** and
   **MAX_PLANNED_RECOVERIES=5000**. Oversized plans fail before any application
   injection, with no accepted-plan gate and no partial matrix execution.
4. Only after both checks pass are all plans registered in fresh completeness
   accounting, followed by every selected case in the unchanged family order.

The summary logs at most 512 bucket rows, ordered by contribution on overflow;
it reports total/omitted rows and hashes the full census. Every accepted plan fits
in this summary: at least three compatible modes per target imply at most 333
buckets under the 1,000-case cap. An oversized plan retains bounded contributor
information and fails; only diagnostic rows may be bounded, never the actual
coverage plan. The status remains PENDING until the subsequent plan gate passes.

Expected boundedness is source-level arithmetic, not a measured application plan:
**1,477** otherwise identical database writes at different offsets form one
bucket, selecting positions **1/739/1477**, at most **3 targets / 15 fault cases /
75 recoveries**. Repeated WAL fragment lengths likewise contribute at most three
samples per shape/context/flags bucket. Sync and truncate buckets contribute at
most 9 and 12 fault cases respectively. Checkpoint/autocheckpoint sampling thus
scales with I/O classes rather than the number of written database pages or
incidental fragment lengths. The actual 24-family/sector total and runtime remain
unknown: distinct lengths/flags/context can still exceed the limits, and a
case-count bound does not promise completion within a future time allowance.
No application discovery or plan has been executed for v4.

The limits protect one invocation; they do not replace acceptance criteria or
permit skipping a selected case. Final acceptance still requires all 12 families,
both sectors, every selected target and compatible fault, all five schedules,
complete integrity/consistency/replay checks and every newly acknowledged-effect
category. The acknowledgement categories are moved unchanged into a shared helper
for preflight reporting and final checking. Complete-response receipt and external
ledger write/flush/fsync ordering remain unchanged. The matrix order remains:
checkpoint, autocheckpoint, restore, migration, fresh, empty-M1, no-op, create,
data PATCH, metadata PATCH, archive, unarchive.

### Prepared regressions and static evidence

**79 targeting/preflight + 26 classifier + 16 fixture = 121 test methods; all
NOT RUN for this source candidate.** The 52 existing targeting methods are retained
and adjusted where v3 sampling expectations intentionally changed; 27 methods are
added. Classifier/fixture suites and the synthetic WAL vector builder are unchanged.
New coverage source includes:

- 1,477 database offsets forming one bucket and exact three samples; database and
  journal live-offset evidence plus private-trace mismatch refusal; separate
  database lengths/flags.
- Separate full-page, fragment, complete frame/commit/reset, raw and unknown WAL
  classes; many varying fragment/raw lengths remaining bounded; exact live
  fragment geometry before injection and preserved page-size/flag distinctions.
- Wrong-bucket rejection, missing N failure, bucket tampering rejection, retained
  first/middle/last labels and exact pre/post trace checks.
- All 24 discoveries preceding the first yielded fault; late discovery failure
  yielding none; actual main-loop wiring through the preflight generator.
- Oversized case/recovery plans rejected independently, inclusive bounds, exact
  arithmetic/mode counts, bounded oversized summaries, missing families/sectors,
  duplicate targets and missing samples.
- Existing checkpoint-first ordering, fresh completeness, all recovery schedules,
  acknowledgement categories and external-ledger ordering tests retained.

Performed locally: Python AST parsing and structural/count checks only; `gofmt -l`
reported no changes; C `-fsyntax-only` passed against the cached pinned
`go-sqlite3@v1.14.52` bundled header; whitespace and scope/privacy review passed.
The header SHA-256 is
`4e7d1523cf95991f7e4c08c576e2232e063da6f10067e5c03c9bf9f904b1cf5f`.
Static AST comparisons confirm unchanged classifier, exact private-trace verifier,
Model, consistency/oracle/replay, execution and fixture functions, prerequisite
controls, fault modes and matrix ordering. C/Go VFS code, WAL geometry header,
worker, production, migrations, dependencies, D1–D5 and every workflow are unchanged.
No controller import/execution, Python test, bridge build/load, tagged build,
fixture binary, fault/probe/negative-control schedule or dedicated job was run.
Ordinary PR CI may run automatically; it does not execute these Python tests and
is not R3 validation.

Remaining gaps: v4 tests and complete no-fault plan preflight have no runtime
evidence; the real plan may exceed the budget; no future v4 fault/recovery case
has executed. The eighth run's missing families, acknowledged-effect categories
and timeout diagnostic limits remain open. No acceptance gate is waived. Existing
worktrees, the original dirty patch, local approval records, binaries and databases
are preserved. Architecture deviations: **NONE**; this is a test-only sampling
policy change proposed for review, not application or persistence redesign.
