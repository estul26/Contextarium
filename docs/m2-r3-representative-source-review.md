# representative-r3-v1 harness source review

**SOURCE-ONLY CANDIDATE — RUNTIME VALIDATION NOT RUN**

Reviewed policy baseline: `197d6db1c3cbc299860bb1c4f3ae2cc6c57481aa`.
Application candidate: `b6f63a7564977555faffe6f9ca6b1a9c22910d43`.
Genuine M1 source: `14dc96ad18810202f63d5ac590822f117a787e82`.
The containing commit identifies this harness source without a self-reference.

On 2026-10-01 the owner instructed implementation of the previously scoped next
step: test-only harness/test-source adaptation, static review, commit and push
to existing `m2-implementation` / Draft PR #6. Repository writing is English.
This is separate from the earlier documentation-only policy adoption. It does
not authorize tests, classifier bridge, compilation/build, no-fault discovery,
fixtures/probes, R3, fault/recovery schedules, or a new execution slot. Production
code, migrations, dependencies, workflows and VFS/classifier source are unchanged.

**T18 PARTIAL; T30 BLOCKED; D6/R3 OPEN; M2 acceptance PENDING; PR #6 DRAFT;
additional execution slots ZERO.** No merge, deployment or M3 is authorized.

## Fixed acceptance identities and arithmetic

The [owner-approved decision](m2-r3-representative-proposal.md#6-approved-fault-matrix)
remains authoritative. Controller `REPRESENTATIVE_ROWS` explicitly encodes its
24 rows, assigned modes and separate operation identities. No physical-offset,
fragment-shape, compatible-mode or arbitrary seed expansion occurs. Each row
selects one baseline target under each sector, yielding 48 targets with 78
assigned fault IDs. Baseline sequence numbers are evidence, not case identities.

| Family | Rows | Faults per sector | Both sectors | Recoveries |
| --- | --- | ---: | ---: | ---: |
| create | C1, C2 | 8 | 16 | 48 |
| patch | P1, P2 | 2 | 4 | 12 |
| metadata | V1 | 1 | 2 | 6 |
| archive | V2 | 1 | 2 | 6 |
| unarchive | V3 | 1 | 2 | 6 |
| noop | V4 | 1 | 2 | 6 |
| restore | R1, R2 | 4 | 8 | 24 |
| checkpoint | K1, K2, K3, K4 | 8 | 16 | 48 |
| autocheckpoint | A1, A2, A3 | 3 | 6 | 18 |
| migration/adoption | U1, U2, U3 | 6 | 12 | 36 |
| fresh initialization | I1, I2 | 2 | 4 | 12 |
| empty-M1 initialization | I3, I4 | 2 | 4 | 12 |
| **Total** | **24 rows** | **39** | **78** | **234** |

Required recoveries are exactly **discard/17, retain/17, reorder-torn/17** for
every case: **39 faults × 2 sectors = 78 faults; 78 × 3 = 234 recoveries**.
Preflight completes all 24 no-fault discoveries before it yields any application
fault and verifies the exact `(policy, row ID, sector, assigned mode)` set. Missing,
extra, duplicated or mode-expanded rows fail. Final completeness also uses that
fresh exact set. Legacy bucket helpers support geometry/probe regressions; their
sequence-based targets cannot fill the representative gate. Seeds 29/101 remain
historical/optional evidence and are not required schedules in this source.

The 234 count excludes builds, discovery, fixture preparation, validation probes
and negative controls. None of those activities ran here. No elapsed-time or
real-layout claim follows from the proposed bounded source plan.

## Transaction and storage-boundary selection

The test worker assigns fixed numbered phases to each checkpoint-preparation
create. It retains the actual application engine and SQLite default automatic
checkpoint setting; controller settings checks now explicitly require 1000.
This separates explicit checkpoint preparation from its checkpoint call and
separates each automatic-path create transaction. No production switch is added.

`BoundaryTracker` tracks complete validated frame headers, page-data coverage,
WAL generations and one checkpoint pass within an application phase. A sync
before a complete committing frame payload cannot qualify as committing sync.
After a qualifying sync, another header/checkpoint-only sync cannot borrow the
already durable commit. The first complete commit header and the final qualifying
committing sync of that transaction are selected; intervening padding/fragment
shapes do not produce additional cases. U1 selects the first page-data write
following a validated non-commit frame header before the upgrade commit header.
It is not labelled as an SQL mid-adoption barrier.

K1/K2 use the first/last writes of the same completed explicit baseline pass.
A1 uses `count // 2 + 1` in the first completed automatic baseline pass. Explicit
passes require at least two complete database-page writes; automatic passes
require at least three. K3/A2 select the sync completing that identified pass.
K4 requires the subsequent truncate to zero. A3 selects the first complete reset
header in a subsequent create after that automatic pass completed. Initialization
headers do not qualify as reuse. Multiple passes/syncs within one identified
phase are rejected as ambiguous rather than producing a larger matrix.

Live selection requires the planned phase, semantic boundary, WAL generation,
position and relevant checkpoint predecessor/write count. Requested page shape
and sync/truncate flags retain exact identity. Physical append/database offsets
remain observed geometry. Before an injection command, the controller verifies
the entire successful private I/O prefix and reconstructs the selected binding
independently, as well as retaining the exact pending private-trace check. Missing
targets, semantic/epoch drift, incomplete prefix or incompatible shape fail.
Discovery determines baseline first/middle/last positions; a terminated fault
trace does not establish the unexecuted suffix. The real candidate's target
reachability/layout stability still requires later no-fault and targeting review.

Every fault checks its actual return code and applied bytes. P requires exactly
`length // 2`, a nonzero proper prefix; CB must not have a completed post-row.
CA requires successful complete application of the selected I/O. A cut after a
committing WAL sync must recover the transaction as present in all three classes,
even if the application has not delivered a successful response yet.

## Application fixture and acknowledgement coverage

The test-only fixture now has three distinct snapshots per seeded record, with
different data/metadata/status history. Restore targets revision 1 from head 3;
the controller rejects fixtures without three distinct content snapshots. PATCH
changes data, metadata and status together. Metadata V1 separately clears key
and provenance to null without supplying data. Archive, unarchive and a valid
nonempty no-op remain separate requests at the current base with fresh keys.
Genuine M1 populated/empty fixture construction remains unchanged.

Every K/A fault requires a complete successful mutation response already written,
flushed and fsynced to the external acknowledgement ledger before the injection
command. Every K/A completion requires that pre-fault acknowledgement again.
The existing ledger routine, exact oracle and repeated replay are unchanged.
All prior acknowledgements must survive; a WAL marker is not an acknowledgement.
These rows account for **22 acknowledged fault cases / 66 recoveries minimum**.

| Sector | Family | Required post-ack boundary | Rows |
| --- | --- | --- | --- |
| 512 | checkpoint | database write | K1, K2 |
| 512 | checkpoint | database sync | K3 |
| 512 | checkpoint | WAL truncate | K4 |
| 512 | autocheckpoint | database write | A1 |
| 512 | autocheckpoint | database sync | A2 |
| 512 | autocheckpoint | WAL reset/header | A3 |
| 4096 | checkpoint | database write | K1, K2 |
| 4096 | checkpoint | database sync | K3 |
| 4096 | checkpoint | WAL truncate | K4 |
| 4096 | autocheckpoint | database write | A1 |
| 4096 | autocheckpoint | database sync | A2 |
| 4096 | autocheckpoint | WAL reset/header | A3 |

The model algorithm is unchanged. A validation subclass records seed-17 applied
write order/lengths on a synthetic multi-write probe, requiring an actual order
inversion and a proper-prefix tear, repeatability and an image distinct from
discard/retain. Deliberately degenerate discard/retain implementations must be
rejected. This validation source is **NOT RUN**; it does not claim observed model
behavior for this candidate. Post-sync cases need no artificial pending writes.

## Evidence, static validation and remaining work

The storage model, WAL classifier, exact current/revision/audit/idempotency
consistency/oracle, replay, acknowledgement ledger and genuine M1 fixture builder
remain unchanged. All F0–F8, U0–U5 and unrelated ordinary/process-level acceptance
requirements remain intact. A future successful R3 controller result keeps M2
acceptance and unrelated T18 requirements separate from its storage-policy result.

The eighth run remains **1,560 completed fault cases / 7,800 recoveries**, including
**1,405 pre-fault acknowledged cases / 7,025 corresponding recoveries preserving
effects and replaying without duplication**. Its timeout, incomplete families,
unfinished case and semantic qualifications remain historical facts. No old case
fills a representative ID and no evidence is relabelled.

Static review covers Python AST syntax, Go formatting, approved literal-table
comparison with the decision record, exact family/mode/sector arithmetic, all
twelve acknowledgement categories, unchanged protected source/document sections,
documentation links, English additions, whitespace and the source-path allowlist.
Targeting regression source is updated for the new fixed-ID plan and boundary
guards; obsolete universal-sampling acceptance assertions are replaced. Legacy
geometry checks and private-trace/ledger safeguards remain. All targeting,
classifier and fixture tests for this source are **NOT RUN**.

The D6 environment remains **OWNER APPROVED**: standard GitHub-hosted
`ubuntu-24.04`, public repository / $0 paid usage, candidate-pinned
go-sqlite3/bundled SQLite, test-only VFS, synthetic/disposable databases and
isolated runner-owned temporary storage. No personal database, user-machine
reboot/power cycle, real-disk filling or production fault surface.

Next eligible work is a separate review of this source candidate and, only with
explicit execution authorization, its prerequisite validation and no-fault plan.
This publication requests and consumes no execution allowance. No runtime pass,
R3 acceptance, D6 closure, merge readiness or M3 authorization is asserted.
