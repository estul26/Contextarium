# M2 revision and mutation-audit acceptance plan

## Current implementation authorization

The owner subsequently authorized scoped M2 implementation, tests, local
validation, commits, a branch push, and a draft PR from approved documentation
checkpoint `05907846c59d909990d0c6159edce1da88dea7c6` on 2026-09-30.
D1–D5 remain the implementation specification. Final acceptance is pending;
D6/R3 remains OPEN. This authorization does not permit merging, deployment,
real personal data, M3, or provisioning/running a new R3 environment.
The earlier documentation-only approval record below is retained as history;
its implementation restriction has been superseded only for this scoped work.

Current implementation evidence is tracked in the [T01–T30 matrix](m2-evidence.md).

## Current D6/T30 policy approval

**representative-r3-v1 — OWNER APPROVED**, 2026-10-01, from reviewed head
`ae5e4db31274d84eb6088330f6ff85915bfbee48`. The owner adopted the explicitly
enumerated **78 fault cases × 3 recovery classes = 234 recovery executions**
and the demonstrated standard GitHub-hosted runner/test-only VFS environment
class. See [R3 below](#r3--simulated-storage-failure-and-power-loss) and the
[approved decision record](m2-r3-representative-proposal.md).

The policy approval authorized documentation alignment, validation, commit and push
to Draft PR #6 only. It did not authorize harness implementation, R3 execution,
a new execution slot, production/migration changes, merge, deployment or M3.
**T18 PARTIAL; T30 BLOCKED; D6/R3 OPEN; M2 acceptance PENDING; PR #6 DRAFT;
additional execution slots ZERO.** Environment selection is resolved; the new
representative harness runtime validation and acceptance execution evidence
remain missing.
F0–F8, U0–U5, R1/R2 and all unrelated T01–T30 requirements remain intact.

## Subsequent representative harness source-only authorization

On 2026-10-01 the owner authorized the scoped next step: representative harness
and necessary test-source adaptation, static review, commit and push to existing
Draft PR #6. See the [source review record](m2-r3-representative-source-review.md).
This supersedes the policy-adoption implementation restriction only for this
source-only work. No tests, classifier bridge, compilation/build, fixture/probe,
no-fault discovery, R3 or fault/recovery schedule is executed or authorized here.
Production code, migrations, VFS/classifier, dependencies and workflows stay
unchanged. No execution slot is requested or consumed. T18 PARTIAL, T30 BLOCKED,
D6/R3 OPEN, M2 acceptance PENDING and PR #6 DRAFT remain unchanged.

## Historical documentation approval record


**D1–D5 APPROVED — ACCEPTANCE PLAN ONLY**

**D6 / R3 OPEN — M2 IMPLEMENTATION NOT AUTHORIZED**

Owner approval of D1–D5 and the consistent-snapshot observer clarification was
recorded on 2026-09-30 for the documentation-only merge of PR #5. Implementation
remains unauthorized. R3/T30 remains open; no acceptance gate is waived.

Companion to the [M2 contract](m2-contract.md), based on merged M1
`14dc96ad18810202f63d5ac590822f117a787e82`. This is a test **plan**, not test evidence.
All M2 cases below are **NOT RUN / NOT IMPLEMENTED** during this documentation pass.
Existing test coverage was read, not rerun or treated as proof of M2 behavior.

## 1. Test boundary and evidence rules

Use synthetic subjects/schemas/records and disposable temporary databases only.
Do not open existing runtime data, overwrite evidence/artifacts, use personal data,
deploy remotely, reboot/power-cycle a user's machine, or provision infrastructure.
Future test helpers remain internal/test-only; no HTTP failure-injection switches,
environment-controlled production fault mode, or public audit API is required.

For each later run, record exact source commit, test IDs, command, OS/architecture,
Go/driver/bundled SQLite versions, actual connection PRAGMAs, scenario/seed,
observed result, and limitations. Public evidence must omit private paths, machine
identifiers, request payloads/credentials, approval transcripts, and raw local logs.
Record unavailable/manual gates separately from passing automated checks.

Shared transaction oracle: compare current records, all snapshots, mutation events,
idempotency rows, and migration ledger before/after, using independent connection
reads and restart where indicated. Compare complete contents/linkage, not only row
counts. After recovery run `PRAGMA integrity_check`, `PRAGMA foreign_key_check`, and
application checks for head=max revision, contiguous numbers, snapshot equality,
one correct event per revision, and response-to-revision agreement for M2 record-
mutation keys. Subjects/schemas do not have revision linkage.
Legacy M1 keys are explicitly exempt from response-to-revision agreement; their
stored bytes must match their pre-upgrade values.

**Consistent-snapshot observer rule.** Each live multi-table consistency
observation must use one explicit read transaction on a separate physical
connection, or an equivalent single-statement consistent snapshot. Comparisons
across separate autocommit reads that straddle a writer commit must not be treated
as evidence of a partial commit. The observer must not acquire the writer lock
while the writer is paused at a test barrier.

## 2. Invariant-to-test acceptance matrix

IDs are proposed stable acceptance labels, not assertions that test functions exist.
E = engine/application service; H = HTTP; S = storage/migration; P = subprocess.

| ID | Invariant / concrete scenario | Required assertions / layer |
| --- | --- | --- |
| T01 | Create in two independent synthetic domains, with nullable/full metadata | 201/current head 1; complete snapshot; base/source null; one `record.created` event; one matching replay row; exact actor/time/linkage. No hard-coded domain behavior. E/H/S |
| T02 | Sequential replacement PATCHes | Every fresh success increments exactly once; omitted fields persist; supplied data/provenance replace whole fields; null clears only key/provenance. Earlier rows remain byte-for-byte unchanged. E/H |
| T03 | Valid no-op and empty patch | Correct-base unchanged status/data/metadata with fresh keys append a revision/event and new action timestamp; same-key replay does not. Base-only/empty body rejects without consuming a key. E/H |
| T04 | Missing/invalid bases | Exercise absent, null, string, bool, zero, negative, fraction, exponent, overflow, duplicate/differently cased fields, alternate `If-Match`; distinct expected 400 codes, unchanged four mutation stores. E/H |
| T05 | Same-base race, distinct requests | Synchronize two writers on different pools at the same database, same record/base, distinct keys/bodies. Exactly one succeeds in a controlled lock schedule; loser is `REVISION_CONFLICT` after lock release. No lost update, extra revision/event/key, or number gap. Repeat PATCH/PATCH and PATCH/restore, including independent processes. E/S/P |
| T06 | Metadata/status cannot bypass concurrency | Parameterize data, key, sensitivity, provenance, archive, unarchive, combined update, and edits of archived records. Each missing/stale base fails; correct base appends. Re-read/rebase a losing request with a new key and verify omitted fields from the winning head survive. E/H |
| T07 | Replay after edits and restart | Save create/PATCH/restore response; accept later edits; close/reopen; retry original key/body with its now-stale base. Original object/status/revision returned; fresh envelope request ID; no new row/time/event/head. E/H/P |
| T08 | Fingerprint and key scope | Reorder object keys/whitespace: replay. Change data number spelling, base, or any submitted field: conflict. Same restore key for another target: conflict. Same key in a different permitted operation/record scope remains independent. Failure keys remain reusable. E/H |
| T09 | Concurrent identical replay | Race identical scoped key/body across pools/processes. All completed successes identify the same record/revision/event; any contention outcome is bounded/retryable. Retry all failures; there is exactly one effect. E/S/P |
| T10 | Restore historical content | Build at least three distinct states including null/full metadata and archived status; restore an older one with current base. New head contains historical data/key/sensitivity/provenance/status and exact pair; stable identities/created_at; new action time/actor/request/event, correct base/source. Old rows unchanged. E/H |
| T11 | Restore versus unarchive/no-op restore | Status-only unarchive keeps latest data; historical restore chooses target content/status. Restore current or identical snapshot still appends. Verify stale/missing-base restore rejection and same-key replay. E/H |
| T12 | Schema/identity guards during restore | Publish incompatible schema v2 while record stays on v1; restoring v1 still uses v1. Missing target 404. Controlled invalid pair/identity/schema fixtures fail closed (no guard removal in ordinary paths); cross-pair restore 409, identity corruption internal failure, missing retained schema unavailable. No general schema write API. E/H/S |
| T13 | Exact JSON and retained schemas | Preserve `9007199254740993`, `0.30`, `1e2`, `-0`, bounded large/exponent numbers, accepted unknown properties, arrays/Unicode through create, metadata-only update, revision read/list, restore, replay, upgrade, restart. Exact token/text and value assertions; no float normalization. Existing invalid JSON/schema bounds continue to reject. E/H/S |
| T14 | Number uniqueness/range/head | Duplicate record/revision identity fails; separate records can both have revision 1. Reject zero/fraction/overflow; at maximum reject new mutation without wrap/key consumption. Use a controlled isolated near-limit fixture rather than billions of rows. Verify no gaps, dangling head, cross-record pointer, stale pointer, or snapshot mismatch after every supported mutation. E/S |
| T15 | Immutable history | Through supported services/HTTP, no revision/audit edit/delete route or mutable revision field exists; GET-only revision paths return 405 with Allow, audit path remains 404. Direct SQL UPDATE/DELETE and replacement/upsert attempts against revisions/events fail with guards enabled. Snapshot fields, attribution, and linkage remain unchanged after failure/reopen. E/H/S |
| T16 | Database linkage protections | Direct inserts/updates attempt missing record/subject/schema, incorrect event pair/ID, missing event, duplicate event, invalid predecessor/source, invalid actor kind, mismatched head content, skipped/regressed head, and record deletion/replacement. Assert the specified guard rejects at statement or commit; rollback any open transaction. Exercise null-safe snapshot comparisons and cyclic deferred foreign keys. S |
| T17 | Explicit actor and minimal event | Local HTTP actions use only `local-http-adapter`; direct harness calls pass distinct synthetic actors/request IDs. Missing/invalid/internal authenticated-kind context rejects. Spoof actor/client/request headers and provenance: cannot alter trusted fields. No payload, schema, provenance text, key header, or secret in events/logs/errors. Exactly one event with proper action for combined/status/no-op paths. E/H/S |
| T18 | Atomic failure at every write boundary | For create, data/metadata PATCH, archive, unarchive, and restore, run F0–F8 below. Revision/current/audit/replay/serialization/commit failures never leave a partial accepted mutation. Controlled required-audit failure rolls back all earlier writes. E/S/H |
| T19 | Revision pagination | At least 205 revisions; default/1/100 limits; sorted contiguous coverage without duplicates; empty next cursor at end. Append/restore between pages: original traversal stops at captured head; new traversal sees new head. Changed limit allowed. Reject oversize/foreign-record/resource/malformed/version-mismatched cursors, duplicate/unsupported queries and invalid path numbers. Missing record/revision 404. Bounded queries/response work, no total history scan, no writes on reads. E/H/S |
| T20 | Fresh/empty upgrades | Fresh DB, M0 DB, M1 empty DB, and M1 with subjects/schemas/keys but no records. Correct ledger 001–003, no adoption events without records; immutable existing rows and migration checksums/times retained. First future record starts at 1. S |
| T21 | Populated M1 adoption | Prepare a genuine 001/002 disposable DB with active/archived records, exact numbers/nulls and multiple earlier edits/replay snapshots. Adopt current state only as revision 1; original record bytes/timestamps unchanged; new adoption actor/time/event/boundary. No synthetic earlier history or conversion of replay snapshots. Verify all records adopted in the same transaction. S/E/H |
| T22 | Legacy replay after upgrade | Replay old record-create and PATCH keys with original M1 body, including missing base and historical response different from adopted state, after later edits/restart. Return stored object without revision, same status, result_contract m1; no effects. Added base/changed body under old key conflicts. Unknown key missing base fails. Retain subject/schema replays and all original hashes/scopes/response bytes. E/H/S |
| T23 | Interrupted/repeated upgrade | Inject failures at U0–U5 below on fresh/empty/populated DBs, cancellation and startup deadline. Restart yields either entire old state or entire adopted state. Retry once, reopen repeatedly: no duplicate adoption/event, no timestamp changes or ledger rewrites. S/P |
| T24 | Old-binary refusal and explicit rollback | Actual M1 checkpoint binary and ledger-prefix unit test refuse schema 003 before listener/ready log; M0 likewise rejects. They do not edit ledger/history (allow documented connection-PRAGMA caveat). On separate closed synthetic copies, return to preserved M1 baseline and retain M2 evidence; explicitly account for omitted later changes. S/P |
| T25 | Contention versus conflict | Hold IMMEDIATE writer lock in second pool; fresh mutation and replay return bounded 503 if acquisition fails, not 409. Release lock and same-key retry succeeds/replays. Also exhaust sole pool connection and cancel/deadline requests. Retry stale inputs after availability yields 409. Check no key/event/revision leakage. E/H/S |
| T26 | Connection/migration regressions | One open/idle connection, replacement connections retain FK ON/WAL/FULL/busy timeout/private cache/IMMEDIATE. Unknown ledger versions, gaps, altered checksums/names, nonempty unowned DBs, invalid paths/symlinks fail closed. M1 schema immutability/retention and identity/schema-pin trigger still reject writes. S |
| T27 | Loopback, browser, privacy regressions | Numeric loopback listener only; unsafe config rejected before DB creation. Host/Origin/Sec-Fetch-Site rejections on all new paths; accepted IPv4/IPv6 with/without port, rejected malformed/zoned/nonloopback forms. Safe headers, strict body/media/query/key limits, no raw SQL/paths/bearer/body/untrusted request IDs in errors/logs; audit/proposals/search/MCP remain absent. H/P |
| T28 | Probes and graceful lifecycle | Exact GET/HEAD/Allow/probe bodies, read-only probes, bounded readiness, no readiness before migration/after withdrawal, unavailable DB stays unready. Startup failure releases resources. SIGTERM/cancellation and forced-close deadline resolve in-flight work without partial effects, including a commit racing with disconnect; DB closes after handlers. See R1. H/P |
| T29 | Abrupt process recovery | Controlled test child exits without defers or receives SIGKILL at precommit/postcommit barriers; reopen with WAL sidecars intact and use shared oracle plus same-key retry. No partial mutation or duplicate committed effect. See R2; not power-loss evidence. P/S |
| T30 | Simulated storage/power-loss recovery | **representative-r3-v1 — OWNER APPROVED:** exactly 78 enumerated fault cases and 234 recovery executions across all 12 families, both sectors and the three approved recovery classes; actual pinned SQLite/VFS mutation/migration/commit/checkpoint paths, acknowledged-effect survival, all-or-none ambiguous effects, integrity/FKs, complete linkage/exact data and duplicate-free replay. See R3 and the approved decision record. **Environment class APPROVED; T30 BLOCKED; representative implementation/execution evidence absent; execution slots ZERO.** |

T12's impossible-state fixtures must be constructed in isolated test setup or via
an internal read seam, then discarded; supported writes may not weaken production
guards to manufacture cross-schema history. T14's overflow fixture is likewise
test-only and does not excuse gaps in normal committed histories. T16 tests the
specific planned database guards; full application postconditions additionally
cover arbitrary unattached insertions, as specified in the contract.

Use a controlled clock to distinguish historical/adoption/new action times. Also
exercise equal or regressing wall-clock values: ordering/conflict decisions still
depend exclusively on revision numbers, and replay never regenerates action time.

## 3. Defined mutation failure boundaries

Use a deterministic internal seam or temporary synthetic rejection trigger for
write failures, with a separate child-process barrier for crash scenarios. Never
expose these controls to ordinary HTTP clients. For a failpoint to count as tested,
the harness must prove it was reached; sleeps alone are not sufficient evidence.

| Point | Injection location | Expected result |
| --- | --- | --- |
| F0 | Before BEGIN / during connection or lock acquisition | Bounded unavailable/cancelled result; no new effect. |
| F1 | After key lookup and base/schema validation, before revision insertion | Error/cancellation leaves all mutation stores unchanged. |
| F2 | During revision insertion and immediately after it | Revision failure or later abort leaves no new revision/current/audit/key. |
| F3 | During current-record insert/conditional head update and immediately after it | Old head and history retained; a zero-row predicate result is handled, not accepted. |
| F4 | During required audit insertion and immediately after it | No accepted record/revision without the event; full rollback. |
| F5 | During response serialization and replay insertion, and immediately after replay insertion | No mutation can survive an absent/invalid saved result. |
| F6 | After all writes/postconditions, immediately before calling COMMIT | Injected error or abrupt exit recovers the complete old state; same key can subsequently succeed once. |
| F7 | During COMMIT: constraint failure and storage/sync failure variants | Known precommit rejection rolls back. Ambiguous storage/commit outcome is resolved after reopen; all four stores agree. Never equate a generic commit error to proof of no commit. |
| F8 | After successful COMMIT, before any HTTP success is delivered (also mid-response disconnect) | Simulate dropped delivery or child termination. Reopen and retry same key/body: return committed saved result once; no second revision/event. |

Test event/revision/current/idempotency failures independently, not just one generic
transaction abort. Serialization should use a test seam rather than introduce an
otherwise invalid production payload. Ensure the failure affects the intended
record operation, not fixture creation. After removing a reversible injected fault,
retry the original key and assert exactly one accepted effect. Concurrent readers,
using the consistent-snapshot observer rule in section 1, must never observe
a partial four-store combination at any barrier.

## 4. Defined upgrade failure boundaries

| Point | Injection location | Required recovery |
| --- | --- | --- |
| U0 | Before migration 003 DDL / while acquiring migration lock | Original ledger and data unchanged. |
| U1 | After new tables/columns exist in transaction, before adoption | No partial schema/ledger visible after rollback/restart. |
| U2 | Mid-adoption between records or between a snapshot and event | All records remain M1, or entire batch ultimately commits; never a visible mixed boundary. |
| U3 | After backfill, during new guards/link checks and legacy response tagging | Abort preserves original record/replay bytes and 001/002 ledger. |
| U4 | Before ledger insert, after ledger insert, and during/before COMMIT | Either complete pre-upgrade or complete M2 state; resolve commit ambiguity by reopening. |
| U5 | After migration commit, before listener/readiness | Complete M2 state; later startup validates without adopting again. |

Run upgrade failure cases against fresh, empty M1, and populated M1 fixtures.
Failing from zero may leave an empty filesystem database, as in M0; it must not
leave a partial migration ledger. Cancellation/backfill timeout never turns into
silent partial acceptance. Corrupt rows/foreign keys must fail instead of skipping
records. Old ledger checksums and release migrations remain unchanged.

## 5. Recovery classes and the remaining acceptance gap

### R1 — Graceful shutdown

Use cancellation and SIGTERM against only a test-owned child, with barriers around
active handlers. Verify readiness withdrawal, bounded drain, forced-close deadline,
transaction resolution, listener closure, then pool closure. Restart and inspect
state/replay consistency. SIGTERM exercises handled shutdown; it does not simulate
loss of kernel buffers, filesystem writes, or power.

### R2 — Abrupt process termination

Use a test-owned child and explicit barriers F2–F8/U1–U5; kill only that recorded
child PID or exit without deferred cleanup. Reopen the same disposable database
with its SQLite-managed WAL/sidecars intact. Before commit, no new effect should
survive; after acknowledged commit, the full effect and replay result survive.
An interrupted commit can recover either whole outcome, never a partial one.
Record which cases were actually exercised. SIGKILL alone leaves the operating
system/storage stack running and is **not proof of power-loss durability**.

### R3 — Simulated storage failure and power loss

#### Adopted representative-r3-v1 acceptance policy

**representative-r3-v1 — OWNER APPROVED** on 2026-10-01. The exact
[boundary/fault matrix](m2-r3-representative-proposal.md#6-approved-fault-matrix)
is authoritative: **78 fault cases**, each with **discard/17, retain/17 and
reorder-torn/17**, yielding **234 recovery executions**. Its assigned CB/CA/E/F/P
faults are acceptance requirements; every other technically compatible mode is
not automatically added. CB=cut-before, CA=cut-after, E=IOERR, F=FULL,
P=partial write. Both device-sector profiles, 512 and 4096, remain required;
device-sector size is not database page size or an atomic-write guarantee.

Every family has an explicit assignment. Combined PATCH represents `patch` only;
it does not replace metadata/archive/unarchive/no-op. Each listed mode in the
decision record creates one case per sector and three recoveries per case:

| Required application family | Approved matrix IDs | Faults per sector | Both sectors | Recovery executions |
| --- | --- | ---: | ---: | ---: |
| create | C1–C2 | 8 | 16 | 48 |
| patch | P1–P2 | 2 | 4 | 12 |
| metadata | V1 | 1 | 2 | 6 |
| archive | V2 | 1 | 2 | 6 |
| unarchive | V3 | 1 | 2 | 6 |
| noop | V4 | 1 | 2 | 6 |
| restore | R1–R2 | 4 | 8 | 24 |
| checkpoint | K1–K4 | 8 | 16 | 48 |
| autocheckpoint | A1–A3 | 3 | 6 | 18 |
| migration/adoption | U1–U3 | 6 | 12 | 36 |
| fresh initialization | I1–I2 | 2 | 4 | 12 |
| empty-M1 initialization | I3–I4 | 2 | 4 | 12 |
| **Total** | | **39** | **78** | **234** |

The only mandatory recovery classes per fault are discard/seed 17, retain/seed 17
and reorder-torn/seed 17. The latter must be validated to demonstrate the intended
reorder/torn behavior on a suitable multi-write probe rather than degenerating
into discard or retain. Post-sync cases may legitimately have no pending writes.
Seeds 29/101 remain historical/supporting or optional exploratory coverage, not
mandatory M2 acceptance. No extra seed, universal first/middle/last expansion,
incidental WAL fragment length or physical database-page offset automatically
enlarges these 78 cases. Additional acceptance requirements need separate owner
approval; optional exploratory work grants no execution permission.

**D6 environment class — OWNER APPROVED:** standard GitHub-hosted `ubuntu-24.04`
runner, public repository/$0 paid usage, test-only VFS, the candidate's exact
pinned go-sqlite3/bundled SQLite, synthetic/disposable databases and isolated
runner-owned temporary storage. No personal database, owner-machine reboot/power
cycle, real-disk filling or production fault surface. This resolves environment
selection; D6/R3 remains OPEN until representative-r3-v1 is implemented, executed
and accepted. There is no new slot or permission to run/setup a job.

All twelve post-acknowledgement category combinations remain mandatory:

| Sector profile | Family | File role / storage boundary | Approved matrix IDs |
| --- | --- | --- | --- |
| 512 | checkpoint | database write | K1–K2 |
| 512 | checkpoint | database sync | K3 |
| 512 | checkpoint | WAL truncate | K4 |
| 512 | autocheckpoint | database write | A1 |
| 512 | autocheckpoint | database sync | A2 |
| 512 | autocheckpoint | WAL reset/header | A3 |
| 4096 | checkpoint | database write | K1–K2 |
| 4096 | checkpoint | database sync | K3 |
| 4096 | checkpoint | WAL truncate | K4 |
| 4096 | autocheckpoint | database write | A1 |
| 4096 | autocheckpoint | database sync | A2 |
| 4096 | autocheckpoint | WAL reset/header | A3 |

K/A require complete successful application responses written, flushed and
fsynced to the independent external acknowledgement ledger before fault
authorization. A WAL commit marker alone is not an acknowledgement. Every prior
acknowledged effect and replay result must survive; these assignments supply
22 acknowledged fault cases / 66 corresponding recovery executions minimum.

The fundamental oracle and storage model are unchanged: actual bundled SQLite
WAL/FULL I/O, exact reached/live/private-trace targeting, correct successful-sync
semantics, all-or-none ambiguous transactions, unchanged earlier durable history,
full integrity/FK/revision/current/audit/replay/ledger/exact-data checks, and
repeated same-key replay in fresh processes. Check recovered schema/data before
migration retry; never let startup hide a partial upgrade. A verified committing
WAL sync retains its complete durable commit even before response receipt;
generic sync errors remain unavailable/ambiguous and must not become success.

Contextarium owns its transaction/adoption structure, configuration, acknowledgement
handling, replay, oracle and adherence to SQLite durability assumptions. SQLite
owns exhaustive internal WAL/page-layout algorithm testing. Hardware/storage
that falsely reports successful sync completion is outside the guarantee.
The claim is representative modeled storage recovery, not physical VPS power-loss
verification. SQLite remains authoritative; no architecture or ADR is changed.

F0–F8, U0–U5, ordinary/process-level tests and R1/R2 are preserved. The
representative matrix supplies their relevant storage-level dimension; it does
not replace unrelated acceptance requirements. All approved IDs, sectors,
assigned modes, three recoveries, acknowledgement categories and prerequisite/
oracle gates must complete in fresh accounting. Missing targets, incomplete
recoveries, timeouts and resource failures cannot pass. Historical runs are
supporting evidence only, never automatic future completeness credit.

#### Policy change history and preserved original planning text

The original R3 plan below predates the implemented harness/environment. Later
harness policies, including source-only v4 at the reviewed head, expanded sampling
to first/middle/last per I/O bucket, all compatible faults and five schedules.
Their runtime results and failures remain in [M2 evidence](m2-evidence.md).
The 2026-10-01 owner decision replaces those broader acceptance expansions with
the enumerated representative-r3-v1 policy above. The original requirement for
multiple deterministic seeds/positions is refined to the approved positions and
three recovery classes. Original environment-absent statements below are
historical, not the current D6 selection status. No historical result is erased,
relabelled or promoted by this amendment.

The [M0 runtime](m0-runtime.md) explicitly leaves a full crash/power-loss durability
rehearsal to M2. Repository inspection found no existing runnable WAL power-loss
harness. This documentation pass does not establish that an appropriate external
environment is available; it does not install or run one.

Proposed acceptance method: a test-only storage/VFS fault harness against the same
bundled SQLite version and WAL/FULL settings, or an already approved isolated
disposable environment whose storage model demonstrably drops unsynchronized
writes. It must record write/sync/checkpoint fault points, simulate interrupted or
reordered unsynced writes within a stated model, and reopen the resulting database
image. Plain injected SQL errors test rollback, not this storage fault model.
Likewise killing a VM/container process is insufficient unless its cache/virtual
disk behavior actually models the stated storage loss.

Exercise record mutations and upgrade at precommit, WAL commit/sync, postcommit,
and checkpoint boundaries, using multiple deterministic seeds/positions. Include
I/O-error and disk-full simulation without filling the user's real disk. A successful
durably acknowledged commit must retain its entire effect and replay result; an
unacknowledged commit may recover wholly present or absent. Already committed
history cannot disappear or change. Validate SQLite integrity/foreign keys and
the shared application oracle after every recovered image. A reported sync error
is a separate unavailable/ambiguous outcome, never an acknowledged success.

The model must state what a successful sync guarantees. A device that lies about
flush completion is outside that guarantee and cannot support a durability claim.
SQLite's [WAL documentation](https://www.sqlite.org/wal.html#performance_considerations)
explains FULL commit syncing. Its [atomic-commit crash tests](https://www.sqlite.org/atomiccommit.html#crashtest)
describe a modified VFS and reopening to verify all-or-none outcomes; the rollback-
journal discussion is not itself evidence for this application's WAL path.

**Current gap and impact:** no R3 run, harness, or approved environment evidence is
available in this pass. M2 durability acceptance remains **OPEN / NOT PASSED** even
if unit/race/process-crash checks later pass. Before implementation acceptance, the
owner must select an appropriate isolated environment and authorize any separate
setup it requires. If unavailable, document the block; do not waive it or claim
M2 fully accepted. Any relaxation needs an explicit approved milestone/design
change. Never reboot or power-cycle the owner's machine to satisfy this test.

## 6. Existing regression anchors and later execution gates

Preserve and extend the relevant existing tests, including:

- Engine: `TestTwoIndependentDomainsAndPersistence`,
  `TestSchemaPinningAndImmutableRegistry`,
  `TestInvalidMutationsLeaveStateAndRetryKeyUntouched`,
  `TestRetryAtomicityAndConcurrentRequests`,
  `TestBoundedOrderedPaginationAndExactFilters`,
  `TestDataNumbersAndUnknownFieldsSurviveStorage`,
  `TestMutationContentionReturnsUnavailableAndRecovers`, and JSON/schema validation.
  M1's `TestConcurrentPatchesPreserveOmittedFields` currently expects both different
  patches to succeed without bases: replace that expectation with T05/T06's conflict
  and explicit rebase behavior, retaining omitted-field preservation coverage.
- HTTP: `TestHTTPRecordLifecycle`, `TestAPIRejectsMalformedAndUnboundedRequests`,
  `TestAPIBrowserBoundaryAndErrorPrivacy`, `TestAPILoopbackHostForms`. Update only
  the now-intentional revisions-route absence case; other deferred routes stay absent.
- Storage: lifecycle/connection policy, repeat/failed/cancelled migration, unknown
  ledger/unmanaged DB rejection, bounded busy waiting, invalid paths, and
  `TestM0UpgradeAndOldBinaryRefusal`. Update expected table/ledger inventory for 003;
  add genuine M1 fixtures without rebuilding them using M2 migrations.
- Server/app/config/main: probe methods/bodies/privacy, unavailable/readiness bounds,
  graceful/forced shutdown, unsafe config before DB creation, startup cancellation,
  newer-schema refusal before readiness, startup-resource release, and safe logging.

Later implementation should run the existing Linux CI checks without weakening
them: formatting, `go vet ./...`, `go test ./...`, `go test -race ./...`, and build
with Go 1.26.x and CGO. These commands are future execution gates, not commands run
as part of this documentation-only pass. Source review plus passing compilation
cannot stand in for executed migration/concurrency/crash/fault tests.

M2 acceptance requires approved contract decisions, all applicable T01–T30 evidence,
unchanged M0/M1 protections, and an explicit recorded R3 result. An infrastructure
choice or fault tool requiring approval is handled separately after the concrete
plan is reviewable; no CI or infrastructure change is proposed in this pass.
