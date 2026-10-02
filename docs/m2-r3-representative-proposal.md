# Representative R3 acceptance-policy decision record

**representative-r3-v1 — OWNER APPROVED**

The owner adopted this D6/T30 acceptance-policy refinement on 2026-10-01,
following the documentation-only proposal reviewed against
`ae5e4db31274d84eb6088330f6ff85915bfbee48`. This record is the canonical enumerated
matrix referenced by the [M2 test plan](m2-test-plan.md#r3--simulated-storage-failure-and-power-loss),
[contract](m2-contract.md#current-d6t30-policy-approval) and
[milestone](milestones/M2-revisions.md#current-d6t30-policy-approval).

Owner permission covers documentation alignment, documentation validation,
commit and push to existing `m2-implementation`/Draft PR #6 only. It does not
authorize representative harness implementation, R3 execution or a new slot,
production code, migrations, merge, deployment or M3. Approval is a policy
decision, not an acceptance result. The unchanged harness still implements the
earlier broader sampling/completeness policy.

| Current gate | Unchanged status |
| --- | --- |
| T18 | PARTIAL |
| T30 | BLOCKED |
| D6/R3 | OPEN |
| M2 acceptance | PENDING |
| PR #6 | OPEN, DRAFT |
| Authorized additional R3 execution slots | ZERO |

Originally reviewed on 2026-10-01 against remote branch `m2-implementation`, head
`ae5e4db31274d84eb6088330f6ff85915bfbee48`. PR #6's head and draft state were
checked read-only. Application candidate:
`b6f63a7564977555faffe6f9ca6b1a9c22910d43`. Genuine M1 source:
`14dc96ad18810202f63d5ac590822f117a787e82`. A read-only tree comparison found no
changes between the application candidate and reviewed head in production Go
directories, migrations/storage, dependencies, or the API specification.

During the original review the primary checkout was at initial commit `0720796`;
authoritative M2 files were read from the pinned Git object. Only this standalone
proposal was added; no test/build/harness execution, commit or push occurred in
that review. Documentation adoption is subsequently published from a separate
ordinary checkout of the reviewed head, preserving primary untracked files and
old worktree metadata. This adoption changes only the five M2 documentation
files listed in section 8, without tests/builds/R3/bridge/fault execution.

## 1. Pre-adoption review: contract and then-current harness requirements

The pinned source references in this review concern the pre-adoption head above.
Sections 1–2 preserve review of that contract and broader harness policy; the
owner-approved prospective policy is in sections 3–6. The
approved documentation checkpoint was
`05907846c59d909990d0c6159edce1da88dea7c6`; later scoped implementation permission
did not close D6 or grant final acceptance.

### Approved durability acceptance

The [contract, section 8](https://github.com/estul26/Contextarium/blob/ae5e4db31274d84eb6088330f6ff85915bfbee48/docs/m2-contract.md#8-compatibility-approval-decisions-and-acceptance-limits)
defines D6 as identifying an approved isolated WAL/storage fault rehearsal
environment and retaining the open gate until exercised. D1–D5 remain the approved
application specification; an R3 method is not authority to alter them.

The [test plan, T30](https://github.com/estul26/Contextarium/blob/ae5e4db31274d84eb6088330f6ff85915bfbee48/docs/m2-test-plan.md#2-invariant-to-test-acceptance-matrix)
requires an approved isolated fault environment exercising the actual bundled
SQLite/WAL write and sync paths, including migration, commit and checkpoint.
Recovery must verify durable acknowledged effects, all-or-none ambiguous effects,
complete linkage and exact data. Its original environment-absent annotation is
historical planning text; the later evidence documents executed runs and the
current incomplete/blocked gate.

The [test plan, R3](https://github.com/estul26/Contextarium/blob/ae5e4db31274d84eb6088330f6ff85915bfbee48/docs/m2-test-plan.md#r3--simulated-storage-failure-and-power-loss)
specifically requires:

1. A test-only VFS/storage harness against the same bundled SQLite and WAL/FULL
   settings, or an approved isolated environment that actually models storage
   loss. SQL errors or SIGKILL alone do not establish this property.
2. Recorded write/sync/checkpoint fault points; interrupted or reordered unsynced
   writes within a declared model; reopening the resulting database image.
3. Record mutations and upgrade at precommit, WAL commit/sync, postcommit and
   checkpoint boundaries, with multiple deterministic seeds/positions. Include
   I/O errors and simulated disk full without filling the owner's disk.
4. Complete survival and replay of durably acknowledged commits. An ambiguous,
   unacknowledged commit may be wholly present or absent. Previously committed
   history cannot disappear or change. A sync error is unavailable/ambiguous,
   never proof of an acknowledged successful commit.
5. Integrity/FK checks and the full shared application oracle after every recovery.
   The model must specify successful-sync guarantees; dishonest sync completion
   is outside the claim.
6. Explicit environment/setup authorization and recorded execution evidence.
   A relaxation needs an explicitly approved milestone/design change; ordinary
   tests cannot waive the gate. No owner-machine reboot/power cycle is allowed.

The shared oracle in test-plan section 1 compares complete contents/linkage of
current records, snapshots, events, replay rows and migration ledger; checks
`integrity_check`, `foreign_key_check`, contiguous revision numbers, head=max,
current/snapshot equality, exactly one correct event per revision, exact data and
M2 response-to-revision agreement. Legacy M1 keys retain their original bytes and
are exempt from M2 revision linkage. Live multi-table observations require one
consistent read snapshot on a separate connection.

### T18 and migration requirements that this proposal preserves

T18 specifies F0–F8 for create, data/metadata PATCH, archive, unarchive and restore:
connection/lock, validation, revision, current/head, required event,
serialization/replay, precommit, commit and postcommit response loss. Required
event failures must roll back the other stores; serialization and each SQL store
must be failed independently. F7 includes constraint and storage/sync failure;
commit errors must not be assumed to mean no commit. F8 retries must return the
saved committed result without another effect.

T20–T24 and U0–U5 retain fresh/M0/empty/populated upgrade, interrupted schema and
adoption, original ledger and legacy response retention, repeated startup,
old-binary refusal and explicit rollback requirements. R1 and R2 remain separate
from R3. The recorded ordinary failure/process tests contribute to those gates;
this review neither reruns nor upgrades their status. Representative R3 only
addresses the remaining storage-fault dimension. It does not replace F0–F8 or
U0–U5 with a smaller SQL/process suite.

The [M2 milestone acceptance](https://github.com/estul26/Contextarium/blob/ae5e4db31274d84eb6088330f6ff85915bfbee48/docs/milestones/M2-revisions.md#acceptance)
requires linked T01–T30 evidence, four-part transactional rollback, adoption and
legacy keys, plus an executed isolated storage/power-loss rehearsal. Missing
environment, approval or evidence must be reported. No accepted mutation may
erase history or lack its required event.

### Later harness completeness policy, superseded for future acceptance

The [current evidence, bounded sampling v4](https://github.com/estul26/Contextarium/blob/ae5e4db31274d84eb6088330f6ff85915bfbee48/docs/m2-evidence.md#bounded-sampling-v4--source-review-only)
and read-only inspection of `scripts/r3-check.py` add these concrete rules:

| Current rule | Exact scope |
| --- | --- |
| Application families | `create`, `patch`, `metadata`, `archive`, `unarchive`, `noop`, `restore`, `checkpoint`, `autocheckpoint`, `migration`, `fresh`, `empty-m1` |
| Sector profiles | All 12 families under both 512 and 4096; the historical database page size is 4096 in both profiles |
| Targets | First/middle/last, coalescing duplicates, in every coverage bucket; Nth live bucket member; exact full live descriptor/private trace verification |
| Bucket identity | Phase/file/role/I/O operation/semantic meaning/flags/validated WAL page size plus write shape and certain lengths or truncate sizes; incidental physical offsets are already excluded from many v4 buckets |
| Modes per target | Every target: cut-before, cut-after, IOERR; write/truncate: also FULL; write: also partial |
| Recovery schedules per case | discard/17, retain/17, reorder-torn/17, reorder-torn/29, reorder-torn/101 |
| Preflight | All 24 family/sector baselines before application injections; full plan; at most 1,000 cases and 5,000 recoveries; oversized plan fails rather than skipping cases |
| Acknowledgement coverage | For each sector, explicit and automatic checkpoint database write and sync; explicit WAL truncate; automatic WAL-header reset: 12 category combinations total |
| Completion | Fresh invocation accounting, every planned target/mode, all five schedules, all required acknowledgement categories; no historical case credit |

These were binding harness requirements at review, not optional just because
they were implementation choices. They are not enumerated by D1–D5. The owner
has now explicitly approved replacement of the target/mode/schedule expansion
for future acceptance; the unchanged present harness does not implement this
new matrix. Earlier completeness statements/results remain historical.
V4 is source-reviewed and NOT RUN; this is a refinement beyond v4, not a claim
that the current v4 planner still keys every database write by physical offset.

## 2. Review findings: fundamental requirements versus harness choices

| Fundamental application/storage acceptance requirement | Harness-specific choice and proposed treatment |
| --- | --- |
| Correct SQLite WAL/FULL configuration and actual engine/storage paths | Preserve pinned bundled SQLite, connection policy and no-bypass checks. Do not substitute mock transactions or system SQLite. |
| All-or-none revision/current/event/replay transaction | Preserve the complete oracle and F0–F8. R3 assigns faults to transaction classes instead of every physical page/bucket. |
| Atomic schema/adoption/ledger and unchanged legacy results | Preserve U0–U5 and genuine M1 fixtures. Use targeted populated/fresh/empty R3 cases instead of every migration I/O bucket. |
| Exercise loss, early persistence and interrupted/reordered writes | Preserve discard, retain and reorder/torn classes. Three particular reorder seeds are a sampling choice; propose one declared seed with demonstrated model behavior. |
| Deterministic coverage of meaningful failure points | Preserve multiple selected positions across the overall matrix. Universal first/middle/last in every bucket and every compatible mode at every target are proposed for replacement. |
| Truthful successful-sync and acknowledgement guarantees | Preserve sync semantics, independent fsynced ledger, exact response linkage and all 12 post-ack category combinations. No reduction to a single global acknowledgement count. |
| Faithful semantic target and injection evidence | Preserve validated classifier, exact pending/live private-trace checks, reached proof and mismatch failure. No raw fragment may substitute for a complete semantic target. |
| Complete execution of the agreed plan | Preserve fresh completeness, explicit failure/incomplete outcomes and source/manifests. Replace the current generated plan with enumerated case IDs; never sample a subset after discovering an oversized plan. |
| Appropriate isolated, bounded execution | Preserve synthetic ownership, safe resource handling and separate execution permission. The 1,000/5,000 planner limits and historical time allowances are harness/job choices, not the durability theorem. |

The two sectors and all 12 families were also harness choices, but the approved
matrix retains them: they are inexpensive once fault multiplication is removed,
and retaining the status/no-op/fresh/empty variants avoids relying solely on an
asserted equivalence to the main transaction classes. No approval to drop them
was requested or granted by the owner approval.

## 3. Approved responsibility boundary and D6 environment

**D6 environment class — OWNER APPROVED:** standard GitHub-hosted `ubuntu-24.04`
runner, public repository/$0 paid usage, test-only VFS, the application's exact
pinned go-sqlite3/bundled SQLite, synthetic/disposable databases and isolated
runner-owned temporary storage. No personal database, user-machine reboot/power
cycle, real-disk filling or production fault surface. This resolves environment
selection; D6/R3 remains OPEN until representative-r3-v1 is implemented, executed
and accepted. No execution/setup permission or new slot is granted.

Contextarium is responsible for placing validation, base/head predicate,
revision, current state, required event and replay result inside one transaction;
atomic migration/adoption and ledger publication; connection settings; sending
success only after successful commit; same-key retry after an ambiguous result;
preserving SQLite-managed recovery files; and the correctness of its recovery
oracle and acknowledgement evidence. It must not weaken syncs, bypass the VFS,
write database pages itself, or assume errors mean rollback.

SQLite is responsible for its WAL algorithm, checksums, internal page layout,
commit-frame recovery and checkpoint ordering. Exhaustive failure at arbitrary
SQLite page offsets belongs to SQLite's upstream crash-test program. Storage is
responsible for honoring successful sync and the declared file/namespace ordering
assumptions. A device that reports successful flush without preserving its promised
contents, collateral corruption outside addressed writes, and arbitrary hardware
failure are outside this modeled guarantee.

This approved boundary follows the accepted single-process Go/SQLite ADRs and
existing model. It is an acceptance-scope judgment, not a conclusion that SQLite is
infallible or that any VPS filesystem has been verified. SQLite documents a WAL
sync with FULL transactions and WAL/database barriers around checkpoint/reset.
See [SQLite WAL](https://sqlite.org/wal.html#performance_considerations) and
[synchronous](https://sqlite.org/pragma.html#pragma_synchronous). SQLite describes
its own simulated VFS crash tests and repeated failure-point exploration in
[How SQLite Is Tested](https://sqlite.org/testing.html#crash_testing).

The intended self-hosted small Linux VPS architecture provides the rationale;
M2 remains synthetic, local/development-only. This policy grants no deployment
permission and no claim about physical VPS power-loss durability.

## 4. Approved representative acceptance policy

Policy identifier: `representative-r3-v1`. Its acceptance claim is:

> For the recorded application candidate and bundled SQLite, Contextarium's
> important transaction classes preserve their complete effects or complete
> absence under the enumerated storage faults and recovery classes; acknowledged
> effects and earlier durable history survive within the unchanged declared model.

Approved mandatory acceptance conditions are:

1. Pin application, harness, M1 fixture and policy identities independently.
   Record production-source equality, dependency/SQLite source and build hashes,
   Linux environment, actual PRAGMAs, page and sector sizes, settings and model.
   Keep WAL, FULL, foreign keys, single-connection/private-cache/IMMEDIATE policy
   and default automatic-checkpoint setting. No altered production PRAGMAs to
   manufacture an easier acceptance result.
2. Validate classifier/targeting/fixtures, wrapper, model/VFS interception,
   native/instrumented positive oracle and replay, and the five exact-reason
   negative controls at the future harness candidate before accepting results.
   Keep wrong-sync, lost acknowledgement, broken event/revision and broken replay
   controls. Historical validation does not automatically validate modified code.
3. Discover the 24 no-fault family/sector baselines and produce the complete
   enumerated manifest before application fault injection. Associate every target
   with its application transaction or checkpoint epoch. A matching byte shape
   alone cannot prove it belongs to the intended operation.
4. Execute every enumerated case in section 6 under both sector profiles, and
   every class in section 5. Validate exact live descriptors and reached injection;
   retain physical offset/length/flags/page context as evidence, not as an axis
   generating more cases. Missing, ambiguous, mismatched or wrongly classified
   targets fail the plan; no nearest-match substitution or skip-to-PASS.
5. Reconstruct only the modeled image and recover in fresh processes using the
   bundled SQLite. Do not gracefully close/checkpoint the faulted database to
   manufacture recovery. Check state before application migration retry so a
   partial upgrade cannot be hidden by startup completing it.
6. For every image, perform the full existing integrity/FK/content/linkage oracle,
   check earlier durable records/history/replay/ledger, then retry the original
   scoped key and body. If absent, it may succeed once; if present, it must replay
   the saved result without change. Retry again and reopen to prove no duplicate.
   Migration retry must preserve original 001/002 entries and response bytes,
   complete adoption once and leave adoption time/event/ledger unchanged later.
7. Acknowledged effects must be present. Ambiguous unacknowledged effects may be
   wholly present or absent only within the model; successful sync guarantees
   cannot be undone. In particular, a cut after the verified committing WAL sync
   must preserve the complete commit covered by that barrier even before response
   receipt. A generic WAL sync or a commit-marker header alone is not proof of a
   complete durable transaction. IOERR/FULL must not be relabelled as success.
8. For every explicit/automatic checkpoint boundary specified below, prove that
   at least one complete successful application mutation response was received,
   then written, flushed and fsynced to the ledger outside the fault domain
   **before** injection was authorized. Bind the retained result to its original
   key/body and revision. Preserve all 12 current post-ack category combinations.
9. Completion means exactly the approved IDs, assigned faults, both sectors,
   all three recovery classes, all prerequisite gates, all acknowledgement
   categories and all oracles succeeded. Timeout, resource exhaustion, missed
   target or incomplete recovery remains FAIL/INCOMPLETE; no proportion-based
   pass and no credit for old runs. A case count is not a runtime guarantee.

Successful-sync semantics stay `write-back-v1`: sync persists that file's visible
bytes, length and creation; it does not sync another file. SyncDir deletion
persists the namespace removal. Pending writes may survive early; reorder/torn
does not cross namespace/truncate epochs or discard earlier sync guarantees.
Tears affect only the addressed range. Shared-memory indexes remain volatile and
rebuildable; no atomic-write, safe-append or powersafe-overwrite capability is
assumed. This approved amendment changes sampling and completeness scope, not these rules.

## 5. Approved recovery classes

| Class | Required schedule | What it contributes |
| --- | --- | --- |
| D | `discard/17` | Lose all pending unsynced changes; successful sync barriers remain durable. |
| R | `retain/17` | Retain pending changes in issued order, including the actual applied prefix of an injected partial write; exercises early persistence and commit ambiguity. |
| T | `reorder-torn/17` | One reproducible reconstruction that can drop, retain, reorder or prefix-tear pending writes within the existing epochs. |

Run D/R/T on every case; do not deduplicate recoveries merely because two images
happen to match. Discard and retain do not use random seed behavior in the current
model, so their `/17` is retained as an evidence identifier. The approved reduction
is specifically removal of `reorder-torn/29` and `/101` from mandatory application
acceptance, not removal of any storage behavior class.

The three reorder seeds explore more samples of one model rather than three
different guarantees. Multiple physical fault positions and two sector profiles
remain. Existing multi-seed evidence remains supporting coverage. Removing two
seeds reduces recoveries per fault by 40%, from five to three, while deliberately
giving up breadth of random sampling; it is not proof of equivalence to five
seeds. The owner explicitly approved this reduction as a policy amendment.

Before future acceptance, deterministic model/VFS validation must demonstrate
actual reordering and a proper tear on a suitable multi-write probe, not merely
label an unchanged image T. Report applied/dropped/torn operations or equivalent
sanitized model evidence. A post-sync case with no pending writes legitimately
has no reorder opportunity; do not add artificial writes to change its outcome.
If seed 17 cannot demonstrate the required behavior on the validation probe,
the policy is blocked pending a reviewed deterministic schedule, not silently
reseeded or retried until it passes. Additional seeds may be proposed as later
exploratory work, but are neither required nor authorized here.

## 6. Approved fault matrix

Every row uses both **512 and 4096 device-sector profiles**, separately, with the
actual database page size recorded. A sector profile is not a different database
page size or a claim of sector-atomic persistence. Every listed fault mode creates
one case per sector; each case has D/R/T recovery. Only the listed assignments
count: the table is not a cross product of all targets and compatible modes.

Common invariants checked in every row:

- **Q:** Full SQLite and four-store oracle, exact JSON tokens/schema/identity,
  contiguous history, correct current head/snapshot, event attribution/action/
  linkage, unchanged unrelated durable state, original result and repeated replay.
- **U:** Atomic schema/ledger/all-record adoption, original M1 data/timestamps and
  legacy key/hash/response bytes, correct history boundary; retry/reopen once.
- **A:** All prior external-ledger acknowledgements survive with their saved
  results and without duplicate effects. Rows marked A require a newly captured
  acknowledgement before their fault; Q and applicable U still run.

Mode meanings retain current behavior: **CB** cut before the selected I/O;
**CA** cut after it; **E** injected IOERR; **F** injected FULL on a write;
**P** write `length // 2` bytes at the original offset then return IOERR. P must
have a nonzero proper prefix and its actual applied bytes must be verified.
No FULL/partial variant is assigned to sync, and no FULL/partial variant is
assigned to the shrinking WAL truncate, which does not allocate new space.

For a WAL commit-write target, require the complete validated commit-frame
header, not a fragment/raw label. For a sync target, identify the transaction's
committing WAL sync, not an unrelated startup or checkpoint sync. Select one
matching target for that transaction; header labels do not establish that its
page payload/checksum is complete or acknowledged.

For U1 select the first validated non-commit WAL frame page-data write in the
upgrade transaction before its commit frame. This covers pre-durable-commit
interruption, not an asserted SQL mid-adoption barrier. U0–U5's SQL/process
barriers remain separately required. Do not call a byte position "mid-adoption"
without application-phase evidence.

For checkpoint database writes, use first and last writes in one identified
explicit checkpoint pass, plus the middle write (`count // 2 + 1`, one-based) in
one automatic pass. First demonstrates incomplete page transfer, last explores
an almost-finished pass, and middle gives a separate automatic-path/space-failure
sample. At least two explicit-pass writes and three automatic-pass writes must
be observed in the bounded synthetic baseline. Do not multiply these positions
by all fault modes or all page lengths/offsets. WAL reset means reuse **after a
completed automatic checkpoint**, not first-ever WAL initialization.

| ID | Application operation | Storage boundary / selected position | Fault mode(s) | Recoveries | Invariant and distinct coverage | Cases per sector |
| --- | --- | --- | --- | --- | --- | ---: |
| C1 | Create one record | Its complete WAL commit-frame header write | CB, CA, E, F, P | D/R/T | Q: four new stores share one outcome. Principal commit-write fault-mode anchor; cuts, rejection and applied-prefix failure differ. | 5 |
| C2 | Same create fixture, independently faulted | Its committing WAL sync | CB, CA, E | D/R/T | Q: durability barrier, success-before-delivery and failed sync. Distinct from writing a marker. | 3 |
| P1 | Representative data PATCH including metadata and status change | Its complete WAL commit-frame header write | P | D/R/T | Q: successor/current replacement and one correctly selected event survive or roll back together. Exercises existing-record update rather than create. | 1 |
| P2 | Same combined PATCH fixture | Its committing WAL sync | E | D/R/T | Q: F7 sync ambiguity on the actual update branch; P1 is a write fault. | 1 |
| R1 | Restore an older state from at least three distinct revisions | Its complete WAL commit-frame header write | CB, CA, P | D/R/T | Q: append with correct base/source, historical metadata/status/data, new event/action, old history immutable. Separate historical-copy branch. | 3 |
| R2 | Same restore fixture | Its committing WAL sync | E | D/R/T | Q: F7 sync failure and same restore-key/target replay; distinct from restore write interruption. | 1 |
| U1 | Genuine populated M1 adoption, active and archived records plus legacy keys | First non-commit WAL frame page-data write before the upgrade commit frame | CB | D/R/T | U/Q: pre-durable-commit schema/adoption staging cannot publish a partial ledger/batch. Distinct from the final commit marker. | 1 |
| U2 | Same populated M1 upgrade fixture | Its complete WAL commit-frame header write | CB, CA, F, P | D/R/T | U/Q: all records, guards, schema and ledger form one upgrade; FULL tests growth and P tests a torn upgrade commit. No invented prior history. | 4 |
| U3 | Same populated M1 upgrade fixture | Its committing WAL sync | E | D/R/T | U/Q: unavailable/ambiguous migration commit, exact old legacy bytes and no repeated adoption after retry. | 1 |
| V1 | Metadata-only PATCH, including clearing nullable metadata | Its committing WAL sync | E | D/R/T | Q: omitted data remains exact and correct successor/event/replay exists. Retained single branch check avoids assuming combined PATCH proves this path. | 1 |
| V2 | Status-only archive | Its committing WAL sync | E | D/R/T | Q: archived head and `record.archived` event/replay agree; no data rewrite. Distinct status transition. | 1 |
| V3 | Status-only unarchive | Its committing WAL sync | E | D/R/T | Q: active head and `record.unarchived` event/replay agree; latest content retained. Reverse transition differs from restore. | 1 |
| V4 | Valid nonempty no-op PATCH with a fresh key/current base | Its complete WAL commit-frame header write | P | D/R/T | Q: a fresh accepted no-op still appends exactly once; replay does not. Distinct from a changed-data update. | 1 |
| I1 | Fresh initialization, migrations 001–003 | Initialization transaction's complete WAL commit-frame header write | P | D/R/T | U/Q: empty initial or complete schema/ledger; no partial installation or fabricated adoption. No pre-existing M1 schema. | 1 |
| I2 | Same fresh initialization fixture | Its committing WAL sync | E | D/R/T | U/Q: failed initialization sync cannot publish partial schema/readiness; restart resolves it. | 1 |
| I3 | Genuine empty-M1 upgrade, with subjects/schemas/keys but no records | Upgrade's complete WAL commit-frame header write | F | D/R/T | U/Q: original rows/keys unchanged and no adoption events; distinguishes additive upgrade from fresh DDL. | 1 |
| I4 | Same empty-M1 fixture | Its committing WAL sync | E | D/R/T | U/Q: ambiguous commit/retry must not rewrite old ledger or create history. Distinct from FULL write failure. | 1 |
| K1 | Acknowledged create followed by explicit TRUNCATE checkpoint | First database page write in that pass | P | D/R/T | A/Q: previously acknowledged WAL effects survive incomplete transfer into the main database. | 1 |
| K2 | Same explicit-checkpoint scenario, independently faulted | Last database page write in that pass | E | D/R/T | A/Q: error near completion cannot discard the durable WAL or acknowledge a partial transfer; differs from K1's first-write prefix. | 1 |
| K3 | Same explicit-checkpoint scenario | Database sync completing that pass | CB, CA, E | D/R/T | A/Q: database barrier before WAL disposal, including successful versus failed sync. Distinct file/barrier from C2. | 3 |
| K4 | Same explicit-checkpoint scenario | WAL truncate to zero after checkpoint | CB, CA, E | D/R/T | A/Q: length/epoch transition after durable database transfer; old effects/replay cannot be lost by WAL disposal. | 3 |
| A1 | Actual default automatic checkpoint during a bounded sequence of creates, with earlier responses ledger-fsynced | Middle database page write in an identified automatic pass | F | D/R/T | A/Q: space error in the application's default checkpoint path; distinguish prior acknowledgements from the triggering request, which may be ambiguous. | 1 |
| A2 | Same automatic-checkpoint scenario | Database sync for that identified pass | E | D/R/T | A/Q: default-path failed database barrier must preserve earlier acknowledged effects; explicit checkpoint alone cannot supply this evidence. | 1 |
| A3 | Subsequent create reusing WAL after completed automatic checkpoint, with earlier responses ledger-fsynced | First complete WAL reset header write, length 32/offset 0 | P | D/R/T | A/Q: reused WAL generation after prior durable transfer; differs from initial header creation and explicit truncate. New create may be ambiguous; prior acknowledgements may not be lost. | 1 |
| **Total per sector** | | | | | | **39** |

The following explicit family mapping is mandatory; no grouped representative
operation may replace another family. Each approved assigned fault runs under
both sectors and all three recovery classes:

| Required family | IDs | Faults per sector | Both sectors | Recovery executions |
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

The combined PATCH supplements rather than replaces V1–V3. Restore deliberately
uses different historical content/status, not an identical-state restore. Existing
T03/T10/T11 semantic tests continue to cover other no-op/restore variants. All
12 current family identities are represented; `patch` uses P1/P2.

### Count and completeness accounting

| Group | Cases per sector | Both sectors | D/R/T recovery checks |
| --- | ---: | ---: | ---: |
| Create C1–C2 | 8 | 16 | 48 |
| Combined PATCH P1–P2 | 2 | 4 | 12 |
| Restore R1–R2 | 4 | 8 | 24 |
| Populated adoption U1–U3 | 6 | 12 | 36 |
| Metadata/archive/unarchive/no-op V1–V4 | 4 | 8 | 24 |
| Fresh/empty-M1 I1–I4 | 4 | 8 | 24 |
| Explicit checkpoint K1–K4 | 8 | 16 | 48 |
| Automatic checkpoint/reset A1–A3 | 3 | 6 | 18 |
| **Total** | **39** | **78** | **234** |

The number follows the distinct transaction branches, barriers and chosen failure
mechanisms, then retaining both sectors; it was not selected as a target count
and filled with page offsets. C1/C2 anchor every applicable commit-write/sync
fault. Other operations retain their own F7 and special invariants without
repeating the full anchor matrix. K/A cover copying, database sync and WAL
length/generation transitions. Removing the residual variants or a sector could
shrink the count, but that is not the recommended policy.

K/A account for **22 newly acknowledged fault cases / 66 corresponding recovery
checks minimum**. They cover all six current acknowledgement categories per sector:
explicit database write, database sync and WAL truncate; automatic database write,
database sync and WAL reset. One earlier ledger-fsynced response suffices per
case, but **every** prior recorded acknowledgement must survive. Case and response
counts are separate; repeated fixture responses are not unique production effects.

The twelve required post-ack category combinations are individually enumerated:

| Sector profile | Family | File role / boundary | IDs |
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

The 234 figure counts recovered application images. It excludes 24 no-fault
discoveries, fixture preparation, builds, classifier/targeting/fixture tests,
wrapper checks, model/VFS controls and negative controls. Their work/resource
cost must still be included in any future execution plan. No actual discovery
or timing has established that these targets can all be reached within a job.

Each final ID is `(policy, row ID, sector, assigned mode)` with a target descriptor
bound to that row's transaction/epoch. For example C1/512/CB is one case with three
recoveries. One completed case cannot satisfy two different IDs. A missing target
or unexpectedly different layout must lead to a reviewed correction, not an
automatic extra target, substituted fragment, silent cap or historical-case credit.

## 7. Historical evidence retained and its permitted use

Keep the complete existing `docs/m2-evidence.md` history, run identities, source
hashes, manifests, failures, incomplete cases and semantic qualifications. This
proposal appends interpretation; it does not delete or rewrite historical results.

The [eighth run](https://github.com/estul26/Contextarium/actions/runs/36896062479)
tested harness `627e4c0fc392410910ec71ed7d522d1a7e27eefe`, the same application
candidate and genuine M1 source pinned above. It completed **1,560 fault cases /
7,800 recoveries**: explicit checkpoint **345/345** cases for sector 512 and
**345/345** for 4096, plus **870/7,843** announced automatic-checkpoint/512 cases.
Automatic-checkpoint/4096 and later ten families did not execute for that source.
The database page size was 4096 under both sector profiles.

Most importantly, **1,405 completed fault cases** had a complete successful
mutation response written, flushed and fsynced to the external acknowledgement
ledger before injection. **All 7,025 corresponding recoveries preserved those
acknowledged effects and replayed without duplication.** Explicit checkpoint's
required post-ack database write/sync and WAL truncate completed in both sectors.
The automatic path still lacked five required category combinations: database
sync/reset in sector 512 and database write/sync/reset in sector 4096.

This provides substantial supporting evidence for the current application's
checkpoint/replay integration and acknowledgement handling within that model.
It also supports the fidelity of classifier, targeting, fixtures, wrapper,
model/VFS, positive oracle/replay and five exact-reason negative controls, which
passed at that executed harness. It does not validate future targeting/policy
code, demonstrate missing families or establish real hardware behavior.

The eighth run still **FAILED by timeout** after the 55-minute command bound;
there was no final completeness record. The in-flight
`autocheckpoint-s512-b2262-ioerr` has no completed injection/recovery result. Limited
retained metadata does not prove its exact stalled stage or an application
durability failure. Private page traces/ledgers/images are no longer available
for independent reclassification; public hashes and runtime verification records
retain their stated evidential scope.

Other historical results remain useful and qualified:

| Historical result | Supporting value and limitation |
| --- | --- |
| First/second jobs | Build and instrumented-baseline failures remain failures; preliminary model/VFS checks are not application acceptance. |
| Third dedicated no-fault diagnostic | Instrumented open/settings/create/oracle diagnostic passed; no fault matrix or recovery-class evidence. |
| Fourth job | Prerequisite/wrapper/model/oracle/replay/negative controls passed, then assertion stopped the run before application cases; missing fixture diagnostics remain unresolved. |
| Fifth job | 386 cases / 1,930 recoveries across several mutations; no new acknowledgements in counted cases. Missing no-op target stopped the matrix. |
| Sixth job | 379 cases / 1,895 recoveries; no new acknowledgements. Pre-injection descriptor mismatch stopped the run without injecting the failed case. |
| Seventh job | 399 cases / 1,995 recoveries; no new acknowledgements. Stable-prefix mismatch stopped the next no-op case before injection. |
| V3/classifier/v4 source-only records | Keep original NOT RUN labels; eighth-run validation applies only to its exact executed source. V4's proposed planner/preflight has no runtime evidence. |

Earlier old-classifier labels remain qualified; a partial header must not be
retroactively promoted to a complete commit marker. The counts above belong to
their separate runs and are not pooled into a new acceptance total. This review
uses recorded evidence and source inspection, not a new reconciliation of raw
private traces or re-execution.

Approved policy: historical results provide supporting confidence and design
feedback only. A future `representative-r3-v1` invocation starts empty completion
accounting and must complete all 78 cases/234 recoveries. Any exception allowing
old evidence to satisfy a particular future ID would require separate explicit
owner approval and a case-by-case source/target/acknowledgement/model mapping;
no such exception was requested or granted by this owner approval.

## 8. Documentation change scope and contract implications

The original proposal changed only this document. The owner's subsequent explicit
approval now authorizes alignment/publication of exactly these five documents;
the canonical assignments in section 6 are unchanged from the approved proposal.
This is a documentation policy amendment, not a claim that the existing harness
implements the new IDs.

| File / location | Adopted documentation alignment |
| --- | --- |
| `docs/m2-test-plan.md`, T30 and section 5 R3 | Reference the approved representative policy and enumerated matrix/three recovery classes; explicitly replace universal bucket/mode expansion and mandatory multiple reorder seeds. Retain simulated storage recovery, shared oracle, F0–F8/U0–U5 and environment/execution gates. |
| `docs/m2-contract.md`, section 8 D6/acceptance limits | Record representative-r3-v1 as the owner-approved acceptance method and environment class while D6 remains OPEN until representative implementation, execution and acceptance. D1–D5 are unchanged. |
| `docs/milestones/M2-revisions.md`, acceptance | Link the approved representative R3 scope and clarify upstream SQLite/storage responsibility; retain all application invariants and pending executed-rehearsal gate. |
| `docs/m2-evidence.md`, prospective policy notice | Add a dated approval/source-only record and the new acceptance-plan reference. Preserve all historical sections/counts/labels and T18 PARTIAL/T30 BLOCKED until separately supported status changes. |
| `docs/m2-r3-representative-proposal.md` | Convert the historical proposal to this approved decision record; retain pre-adoption findings/policy history, exact 78-case assignments and pending acceptance statuses. |

The standalone policy/matrix document is retained, with owner approval
recorded explicitly rather than inferred from its existence. No development guide
or unrelated milestone needs editing to express this refinement. Historical
development instructions are not authority to execute a consumed slot.

This is a real acceptance-plan change: it gives up per-bucket mode multiplication
and two mandatory reorder samples. Owner approval is recorded as a D6/T30 policy
refinement, not as "no gate change." It preserves the architecture, database
configuration, data/replay/upgrade contracts, every application family, both
sector profiles, all acknowledgement categories and oracle guarantees. No new ADR
is needed on this scope; changing those guarantees, the storage model or production
architecture would require a different design review.

Policy adoption does not mean T18 PASS, T30 PASS, D6 CLOSED, M2 accepted,
merge-ready or production-ready. Subsequent harness changes would require their
own review and validation; execution still requires an independently authorized
slot/environment with recorded limits. Approval of this document alone grants
neither implementation nor execution permission. All workflows remain untouched.

## 9. Owner-approved decisions and remaining boundary

The owner's 2026-10-01 approval resolves the original proposal's approval questions:

1. Adopt representative-r3-v1 with exactly 78 assigned faults / 234 recovery
   executions, all 12 explicitly mapped families, both sector profiles and all
   12 post-ack categories. No physical-offset, universal-position, fragment-length,
   compatible-mode or arbitrary-seed acceptance expansion without separate approval.
2. Require discard/17, retain/17 and reorder-torn/17 only; retain validation of
   intended reorder/torn behavior. Seeds 29/101 are historical/supporting or
   optional future exploratory work, with no present permission to run them.
3. Approve the already demonstrated standard GitHub-hosted ubuntu-24.04/public/$0
   test-only VFS environment class under the candidate's exact pinned SQLite,
   with runner-owned synthetic/disposable storage and the exclusions in section 3.
   Environment selection is resolved; D6/R3 remains OPEN.
4. Approve Contextarium/SQLite/storage responsibility boundaries in section 3,
   preserving atomicity, correct durability use, complete-response acknowledgement,
   integrity/FK/linkage/exact-data oracle and duplicate-free replay guarantees.
5. Preserve T18 F0–F8, migration U0–U5 and ordinary/process-level tests. The
   representative matrix supplies relevant storage-level evidence, not a substitute
   for unrelated acceptance requirements.
6. Preserve historical counts, failures/timeouts and semantic qualifications.
   Historical cases never automatically fill the new completeness matrix; no
   evidence-credit exception or retroactive relabelling is approved.
7. Authorize documentation-only alignment, validation, commit and push to the
   existing m2-implementation/Draft PR #6. Harness implementation, execution/new
   slot, production/migration changes, merge, deployment and M3 remain unauthorized.

Stop after documentation publication. Current statuses remain **T18 PARTIAL,
T30 BLOCKED, D6/R3 OPEN, M2 acceptance PENDING, PR #6 DRAFT; execution slots ZERO**.
