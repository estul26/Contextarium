# M2 temporary revision and mutation-audit contract

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

**representative-r3-v1 — OWNER APPROVED**, 2026-10-01, following review at
`ae5e4db31274d84eb6088330f6ff85915bfbee48`. The owner adopted the exact
[78-fault/234-recovery matrix](m2-r3-representative-proposal.md#6-approved-fault-matrix)
and demonstrated standard GitHub-hosted `ubuntu-24.04`/test-only VFS environment
class. Environment selection is resolved; D6/R3 remains OPEN until the
representative policy is implemented, executed and accepted. See the
[authoritative R3 test plan](m2-test-plan.md#r3--simulated-storage-failure-and-power-loss).

The policy approval covered documentation alignment/validation/commit/push only;
no harness implementation, R3 execution/new slot, production/migration changes,
merge, deployment or M3. Application remains
`b6f63a7564977555faffe6f9ca6b1a9c22910d43`.
**T18 PARTIAL; T30 BLOCKED; D6/R3 OPEN; M2 acceptance PENDING; PR #6 DRAFT;
execution slots ZERO.** D1–D5, F0–F8 and U0–U5 are unchanged. Earlier planning
statements about environment absence below remain historical.

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


**D1–D5 APPROVED — DOCUMENTATION BASELINE ONLY**

**D6 / R3 OPEN — M2 IMPLEMENTATION NOT AUTHORIZED**

Owner approval of D1–D5 was recorded on 2026-09-30, together with authorization
for the consistent-snapshot test-plan clarification and documentation-only merge
of PR #5. This records design approval, not completed M2 acceptance. D6/R3 remains
open; no acceptance gate is waived.

This is a documentation-only contract for **M2 — Revision Engine + Minimal
Mutation Audit**. It does not authorize implementation or assert that M2 tests
have passed. The companion [test plan](m2-test-plan.md) defines acceptance.

## 1. Baseline, authority, and scope

Design baseline: merged M1 commit
`14dc96ad18810202f63d5ac590822f117a787e82`, from
[PR #4](https://github.com/estul26/Contextarium/pull/4). The reviewed candidate is
`d15a63a453dd998fa639934b30ab51ce78321747`; its tree matches that merge. Origin main
was verified at the merged checkpoint on 2026-09-30; there were no intervening
changes. Recheck this baseline before a later implementation begins.

Repository requirements remain authoritative. D1–D5 are approved as the temporary
milestone-local contract permitted by the [OpenAPI completion rule](baseline-invariants.md#11-openapi-completion-rule).
Sections labeled **proposed** describe that approved design, not implemented
behavior. D6/R3 remains open. This does not freeze REST v1 or rewrite any accepted ADR.

| Existing requirement | Source and implication for M2 |
| --- | --- |
| Stable identities, unique per-record revision numbers, one current revision, immutable history, accepted mutations create revisions | [Revision model](revision-model.md), [domain model](domain-model.md). Restore must append history; no silent stale overwrite. |
| Revision predicate, snapshot, current state, and required audit commit together | [Architecture](architecture.md), [baseline invariants](baseline-invariants.md), [audit model](audit-model.md). Extend the M1 replay transaction. |
| Exact pinned schemas, retained definitions, no silent data transformation | [Schema system](schema-system.md), [M1 contract](m1-contract.md). No implicit switch to a newer schema. |
| Subject/namespace/ID immutability; non-unique key; descriptive sensitivity | [Baseline invariants](baseline-invariants.md). History has the same identity boundary. |
| Development/test attribution before M3; full audit belongs to M6 | [Audit model](audit-model.md), [permission model](permission-model.md). Do not fabricate authenticated clients. |
| One process, Go, SQLite, domain-neutral services, proposal-first future agents, REST adapters | Accepted [ADRs 0001](adr/0001-single-binary.md), [0002](adr/0002-use-go.md), [0003](adr/0003-use-sqlite.md), [0004](adr/0004-generic-record-model.md), [0005](adr/0005-proposal-first-agent-writes.md), [0006](adr/0006-rest-core-mcp-adapter.md). No architectural replacement is proposed. |
| Local development data, bounded work, privacy, safe lifecycle | [README](../README.md), [CONTRIBUTING](../CONTRIBUTING.md), [SECURITY](../SECURITY.md), [M0 runtime](m0-runtime.md), [M1 development](m1-development.md). Keep loopback-only and synthetic/disposable data. |

M2 scope is record snapshots, head checks, bounded revision reads, historical
restore, minimal transactional mutation events, attribution, and their migration,
concurrency, rollback, and recovery tests. Subjects and schema registration retain
their M1 services/idempotency; they do not gain revision engines or expanded audit
coverage. Authentication, API keys, permissions, proposals, public audit queries,
full audit reporting, denied-attempt reporting, FTS, MCP, embeddings, AI, remote
deployment, managed attachments, new operational infrastructure, and a general
schema-migration framework remain outside this change. See [non-goals](non-goals.md).

### Observed M1 implementation

- [Service.mutate](../internal/engine/service.go) strictly parses JSON, preserves
  number tokens when hashing the sorted-key JSON value, begins an IMMEDIATE
  transaction, and looks up `(scope,key)` **before** the operation's validation.
  A matching key returns its saved JSON object; a changed hash conflicts. Apply,
  response serialization, and replay insertion precede commit. The stored object
  has no revision; HTTP success status is determined by the fixed route.
- PATCH replaces supplied `data`, `key`, `sensitivity`, `provenance`, and `status`.
  Data and provenance are whole-field replacements. Missing fields stay unchanged;
  null clears only key/provenance. M1 serializes writers but accepts different
  requests without a stale-head predicate. The dotted `data.status` example in
  [revision-model.md](revision-model.md) is illustrative, not a new patch protocol.
- [JSON parsing](../internal/engine/json.go), [validation](../internal/engine/schema.go),
  and raw JSON storage preserve exact numbers and permitted unknown data fields.
  No float conversion, defaults, coercion, or external schema loading occurs.
- [Migration 001](../internal/storage/migrations/001_foundation.sql) creates the
  migration ledger. [Migration 002](../internal/storage/migrations/002_records.sql)
  creates M1 tables, schema retention/immutability triggers, and
  `records_identity_immutable`, which also forbids changing a record's schema pair
  and `created_at`. Neither migration may be edited.
- [Storage](../internal/storage/sqlite.go) uses one open/idle connection per pool,
  private cache, foreign keys ON, WAL, synchronous FULL, a 1000 ms busy timeout,
  and IMMEDIATE transactions. Replacement connections reapply the DSN settings.
  [Migrations](../internal/storage/migrate.go) validate names/checksums and run all
  pending SQL plus ledger entries in one transaction before a listener opens.
- [HTTP](../internal/server/api.go) supplies fresh correlation IDs and 3-second
  operation deadlines, with Host/Origin checks, strict bounded input, safe errors,
  and no access logger. [Lifecycle](../internal/server/server.go) withdraws readiness
  and drains for up to five seconds before the application closes SQLite.
- [OpenAPI](../api/openapi.yaml) inventories revision list/read and restore paths;
  its bearer security is a later target, not M2 authentication. The existing
  [Linux CI](../.github/workflows/ci.yml) runs formatting, vet, tests, race tests,
  and build with Go 1.26.x/CGO. This design pass changes none of that behavior.

## 2. Proposed snapshot and numbering contract

An M2 current `Record` is the complete M1 Record plus an integer `revision`.
Each immutable `Revision` has the following response/storage meaning:

| Field | Meaning |
| --- | --- |
| `record_id`, `revision_number` | Unique identity of this revision; record ID is unchanged. |
| `base_revision` | Previous head; null for creation or M1 adoption, otherwise `revision_number - 1`. |
| `operation` | `create`, `update`, `restore`, or `m1_adoption`. |
| `source_revision` | Historical target number for restore; null otherwise. Must refer to the same record. |
| `recorded_at` | Server UTC RFC3339Nano action/adoption time, newly generated for this revision. |
| `actor` | `{ "kind":"development_test", "id":"..." }` under section 6. |
| `request_id`, `audit_event_id` | Trusted action correlation and the one required event for this revision. |
| `snapshot` | Complete M1 Record fields: `id`, `subject_id`, `namespace`, `schema_id`, `schema_version`, `data`, `key`, `sensitivity`, `provenance`, `status`, `created_at`, `updated_at`. |

Snapshot `id` equals `record_id`. The schema pair references a retained immutable
schema definition, and `subject_id` references the same retained subject identity;
this is not a revision of the subject or schema itself. Null key/provenance, active
or archived status, and all accepted metadata are preserved. No record field is
inferred from an audit log. There is no content-hash compatibility promise in M2.

Store/copy record `data` as raw JSON text, including its numeric spellings; never
round-trip through `float64`, SQLite numeric extraction, or a schema coercion.
Preserve accepted unknown data members, arrays, Unicode values, and nulls. The
existing JSON/size/schema limits remain in force. Transport serialization may
compact whitespace or escape strings as M1 already does; it may not change values
or number tokens. Snapshot and current storage use the same resulting raw data.

First revision is **1**, both for a new record and an adopted M1 record. Each newly
accepted mutation appends exactly `head + 1`; failures/replays allocate no number.
Numbers are integers from 1 through **9007199254740991** (safe integer range for
common JSON clients; independent of M1's schema-version limit). At the maximum,
reject a new mutation with `409 REVISION_LIMIT_REACHED`; never wrap or reuse.

At every successful application commit, `records.revision` points to that record's
largest stored revision, and the complete current M1 fields equal its snapshot.
There is exactly one head, no committed gaps from 1 through head, and no future
unattached revisions. Other connections see either the old or new complete state.

An otherwise valid, nonempty PATCH with a fresh key and correct base **creates a
new revision even if every supplied field already has that value**. It also updates
`updated_at` and appends one mutation event. This follows the existing accepted-
mutation invariant and M1's acceptance of unchanged status. An empty field set is
invalid. Restore of the current head or identical historical content also appends
a revision. Replaying the same key is the only successful retry with no new effect.

For create, `created_at = updated_at = recorded_at`. For update/restore, preserve
original `created_at`, and generate `updated_at = recorded_at` for the new action.
Clock equality/regression does not affect numbering: timestamps are not tokens.
M1 adoption is explicitly different: retain both original record timestamps in
the snapshot/current state and use a new adoption `recorded_at` (section 7).

## 3. Proposed temporary HTTP contract

All paths are under `/api/v1`, with the M1 loopback/Host/Origin boundary and **no
authentication**. Application services enforce the same revision and attribution
rules for internal calls. Body cap 96 KiB, strict JSON parsing, metadata/data limits,
single visible-ASCII Idempotency-Key of 1–255 bytes, media rules, 3-second deadline,
safe response headers, and unsupported/duplicate query rejection remain unchanged.

| Operation | Request | Success `data` |
| --- | --- | --- |
| POST `/records` | Existing M1 create body; no supplied ID/revision/base | 201 M2 Record, revision 1 |
| GET `/records`, GET `/records/{id}` | Existing M1 list/item inputs | 200 M2 Record[] / Record |
| PATCH `/records/{id}` | Top-level `base_revision` plus at least one M1 replacement field | 200 M2 Record at new head |
| GET `/records/{id}/revisions` | `limit?`, `cursor?` only | 200 Revision[] |
| GET `/records/{id}/revisions/{revision}` | No query/body input | 200 Revision |
| POST `/records/{id}/restore/{revision}` | Exactly `{ "base_revision": n }` | 200 M2 Record at new head |

The success envelope remains `{data,meta:{api_version:"v1",request_id,...}}`.
Lists include `next_cursor`, empty at the end. Record mutation responses additionally
include `meta.result_contract` equal to `m2`, or `m1` for a legacy replay (section 7).
The request ID in the envelope/header is fresh for each HTTP attempt, including
replay; a revision/event retains the ID from its original accepted action. No audit
query endpoint is added. Subjects/schemas retain their M1 response shapes.

### One precondition convention

PATCH example (illustrative revision number; record already exists):

```json
{
  "base_revision": 4,
  "data": { "title": "Synthetic task", "done": true },
  "status": "archived"
}
```

`base_revision` is a required positive integer JSON token written in ordinary
decimal notation within the revision range. Null, string, boolean, fraction,
exponent notation, zero, negative, and overflow are invalid. Path revision numbers
use canonical decimal digits without leading zeros. IDs keep M1's exact grammar.

Only the five M1 replacement fields are writable. No `patch` wrapper, dotted-field
keys, JSON Patch operations, Merge Patch semantics, or schema-changing fields are
introduced. Nested data keys remain data. Headers such as `If-Match` are not an
alternative; reject `If-Match` on record mutations with `400 INVALID_ARGUMENT` so
it cannot appear to supply an enforced precondition. Never fill in a missing base
from a current read, timestamp, or idempotency response.

The rule covers data, key, sensitivity, provenance, status-only archive/unarchive,
combined patches, and restore, including on archived records. A valid base is
compared with the head **inside** the IMMEDIATE mutation transaction, after replay
lookup. Updates use a conditional head predicate and require exactly one affected
record. A stale request cannot become a last-write-wins update.

### Errors and precedence

Transport/syntax/key/path checks precede the transaction. Matching committed replay
and key mismatch take precedence over new-operation checks. For a new operation,
validate body shape/base, read current record, compare base, then validate resulting
content or load/validate the restore target. This fixes the outcome when a stale
request also contains data that would fail its schema.

| HTTP/code | Meaning; no partial new mutation |
| --- | --- |
| 400 `BASE_REVISION_REQUIRED` | New PATCH/restore has no base member. The legacy replay exception does not accept a new write. |
| 400 `INVALID_ARGUMENT` | Invalid base/path, immutable or unknown field, empty replacement set, invalid query/cursor/key/JSON, or conflicting precondition header. |
| 404 `NOT_FOUND` | Missing record or historical revision; also existing M1 missing subject/schema behavior on create. |
| 409 `REVISION_CONFLICT` | Valid base differs from current head. Static message; `error.details` contains only `expected_revision` and `current_revision`. |
| 409 `CONFLICT` | Same scoped key with different fingerprint (also existing schema-pair registration conflict). |
| 409 `SCHEMA_PAIR_CONFLICT` | Restore target has a different pinned schema pair from the current record; no migration is attempted. |
| 409 `REVISION_LIMIT_REACHED` | No representable next revision. |
| 422 `SCHEMA_VALIDATION_FAILED` | Resulting data fails its exact pinned schema; safe static text. |
| 503 `UNAVAILABLE` | Lock contention, deadline, unavailable retained schema/storage, or uncertain commit outcome. This is not a stale-revision conflict. |
| 500 `INTERNAL_ERROR` | Unexpected internal/invariant failure; static text, no SQL/record diagnostics. |

Existing 403 Origin, 405 method/Allow, 413 size, and 415 media errors are retained.
Missing retained schema during restore is a storage inconsistency: fail closed,
never substitute a newer version. Requests rejected before acceptance consume no
key. A commit error or lost response may be ambiguous; retry the **same** key/body
after availability returns to discover the durable outcome, not a fresh key.

### Bounded deterministic revision pagination

Default limit 20, allowed 1–100. Return full snapshots ordered by ascending revision
number using `(record_id, revision_number)` keyset lookup with at most `limit + 1`
rows examined for page construction. No offset, total count scan, or status filter.
Archived history remains visible under the same local development boundary.

The first page captures the current head as `through` in the same consistent read
as its rows. Issued continuation cursors bind format version 1, record ID, last
returned number `after`, and this fixed `through`. Later pages query
`after < revision_number <= through`; new revisions cannot extend that traversal.
This needs no transaction spanning requests. A new traversal sees the newer head.

Cursor encoding is base64url without padding of a strict bounded JSON object;
maximum encoded length 512 bytes. Validate fields, version, record binding,
`1 <= after <= through <= current head`, and integer limits. Reject other-resource,
malformed, oversize, duplicate-field, or unsupported-version cursors with 400.
Changing page size within bounds is allowed. Cursors are opaque continuation data,
not credentials or a security boundary. A missing record returns 404; an exhausted
page returns `[]` and an empty next cursor. Read/list operations write nothing.

## 4. Proposed atomic mutation and replay sequence

Extend the existing `Service.mutate` boundary; do not build an adapter-owned write
path or a second idempotency store. All SQL stays parameterized. Keep existing
SQLite connection/transaction settings and finite deadlines.

1. Establish explicit internal development/test call context (section 6). Perform
   bounded transport/key/strict-JSON checks and compute the request fingerprint.
2. Begin the existing IMMEDIATE transaction; look up the unchanged scoped key.
   Matching hash: return the original saved object/route success status and its
   stored contract tag, ending the read-only transaction. Do not revalidate its old
   base against today's head, update timestamps, or append an event/revision.
   Different hash: 409 `CONFLICT`, no write.
3. For a new request only, validate its complete M2 shape/base and metadata. Read
   record/subject/exact schema as appropriate. Check create identity absence or
   update/restore base inside this transaction. Validate the complete resulting
   data against the exact pair; check restore identity/pair and revision limit.
4. Allocate revision/event IDs and a fresh action time. Insert the immutable
   snapshot for revision 1 or `head + 1`. Insert/create or conditionally update the
   current record/head to the identical resulting state. Check affected-row count.
5. Insert the one required mutation event linking that exact revision. Validate
   the resulting head/snapshot/linkage postconditions. A required event failure
   aborts the whole operation, including the current state and revision.
6. Serialize the returned M2 Record without numeric conversion. Insert the key,
   fingerprint, saved object, and response-contract tag in the same transaction.
   The fixed operation determines 201 for create, 200 for PATCH/restore.
7. Commit once. Only after successful commit may the adapter send success. Response
   delivery failure cannot undo a commit; a retry returns the stored result. Any
   precommit error exits through rollback. Resolve uncertain commit errors through
   a later same-key replay lookup, without claiming that nothing committed.

Scopes `POST records` and `PATCH records/<id>` remain exactly as in M1. Proposed
restore scope is `POST records/<id>/restore`, deliberately **excluding** the target
revision. Its fingerprint covers both the path target and the body, as the canonical
JSON object `{ "revision": target, "body": original_body }`. Reusing a restore key
for a different target therefore conflicts rather than silently doing another write.

For create/PATCH retain M1's SHA-256 of strict parsed JSON serialized with sorted
object keys, array order and exact number spellings. PATCH includes `base_revision`
in that body; changing it changes the hash. Whitespace/object key order do not;
`0.30` versus `0.3` does. A missing base remains missing for legacy hash comparison.
Do not add a default before hashing. Actor/correlation/action time are server call
context, excluded from the fingerprint and never updated on replay. Keep database-
local key scope and indefinite retention; M3 actor-scoped keys are a later design.

## 5. Proposed historical restore

Restore addresses one retained revision of the same record. Inside the transaction,
check the supplied base against the **current** head, then load the target. Restore
`data`, `key`, `sensitivity`, `provenance`, and `status`, governed by the target's
exact `schema_id/schema_version`. Validate that data with that retained definition.

Preserve current `id`, `subject_id`, `namespace`, and original `created_at`; verify
the target agrees with those immutable values. M2 never creates legitimate history
with different identity or schema pairs for one record. A mismatched identity is
an invariant failure; a mismatched pair returns `SCHEMA_PAIR_CONFLICT`. Migration
002's identity/schema-pin trigger remains installed and unchanged. Do not drop it,
choose the latest schema, or expose a schema-changing write to make restore work.
A future schema-transition feature needs its own approved contract and migration.

Generate a new head number, action `recorded_at`/record `updated_at`, actor,
correlation ID, and audit event. Do not copy the old revision's actor, event ID, or
action time into the new action. Link `source_revision` to the historical target;
link `base_revision` to the head that was replaced. Earlier snapshots remain intact.

Restoring an archived snapshot can archive the record; restoring an active snapshot
can reactivate it. PATCH `{base_revision,status:"active"}` merely unarchives the
current content. It does not select historical data. Both paths check the base,
create a new revision, and participate in the same replay transaction.

## 6. Proposed minimal audit and explicit attribution

Exactly one required event accompanies each new record revision, including adoption.
The event is internal persistence, not a public reporting/query API. Required fields:

| Field | M2 value |
| --- | --- |
| `event_id` | Server-issued `evt_` plus 32 lowercase random hex digits; immutable. |
| `timestamp` | Same newly generated action/adoption time as revision `recorded_at`. |
| `actor_id`, `attribution_kind` | Explicit internal actor ID and `development_test`; never `authenticated_client` in M2. |
| `action` | `record.created`, `record.updated`, `record.archived`, `record.unarchived`, `revision.restored`, or `record.history_adopted`. |
| `resource_type`, `resource_id` | `record`, stable record ID. |
| `subject_id`, `namespace` | Immutable record identity context. |
| `revision_number`, `base_revision`, `source_revision` | Exact new revision and nullable predecessor/restore target links. |
| `proposal_id` | Null; proposals are not implemented. |
| `request_id` | Server/harness-generated `req_` plus 32 lowercase random hex digits. |
| `result` | `accepted` (adoption means accepted history boundary, not historical creation). |
| `reason` | Null, except fixed internal `m1_history_boundary` for adoption. No free-form submitted reason in M2. |

PATCH uses `record.archived` for active-to-archived, `record.unarchived` for the
reverse, otherwise `record.updated`, including no-op/metadata updates. A combined
status/data PATCH still has one event. Restore always uses `revision.restored`.
No full data, schema body, provenance text, key header, token, machine path, or
request body is duplicated into an event. Record provenance remains an untrusted
content claim, never actor identity or an instruction to fetch a URL.

The local application bootstrap explicitly constructs an adapter context with
`{kind:"development_test",id:"local-http-adapter"}`. This identifies the initiating
component, not the human/OS user behind an HTTP caller. The adapter generates each
request ID internally, reusing it for that action's event/revision. No new actor
header, credential, or environment-based identity protocol is introduced.
`Authorization`, `X-Actor-ID`, `X-Client-ID`, inbound `X-Request-ID`, and provenance
cannot override attribution. Unknown actor fields in bodies are rejected.

Direct revision-bearing application-service calls must explicitly receive an internal
development/test actor and generated request correlation from their harness;
there is no silent anonymous default. Synthetic actor IDs use lowercase ASCII
`[a-z][a-z0-9_-]{0,63}`. Missing/invalid context or a claimed authenticated kind is
rejected before mutation/replay; the HTTP bootstrap must make this impossible for
normal local requests. Internal callers are trusted test/development components,
not an authentication mechanism. Migration attribution is fixed separately below.

Required event writes fail the transaction; there is no best-effort fallback.
Replays, reads, validation failures, stale requests, and contention produce no M2
mutation event. M6 owns separate denied/failed-attempt policy and audit access;
M3 will establish authenticated actors without relabeling M2 history.

## 7. Proposed M1-to-M2 policy: explicit initial adoption

Recommend one atomic, additive migration **003**, appended when implementation is
authorized. Keep the contents/checksums and protections of 001/002 unchanged. Stop
the previous server before upgrade; concurrent old binaries are unsupported.

Each existing M1 record is copied exactly into revision **1** with
`operation:"m1_adoption"`, `base_revision:null`, `source_revision:null`, and a
`record.history_adopted` event. Its head becomes 1. The actor is explicitly
`{kind:"development_test",id:"local-migration-003"}`; this identifies the actual
upgrade component, not the unknown earlier writer. Generate action time and
event/correlation IDs during the successful migration, and persist them once.
Embedded SQL can generate these bounded fixed-kind values during adoption; it
must not require a general migration callback framework or a fabricated HTTP call.

Keep every original record field, including old `created_at`/`updated_at`, schema
pair, raw data, and provenance. The newly recorded adoption time is separate.
Revision 1 is the first **known retained state**, not evidence that no earlier
changes occurred. This boundary is visible in revision responses. Do not infer
earlier revisions, authors, or times from record timestamps or idempotency results.
Adoption copies M1-accepted state; it does not apply a newer validator/schema or
repair inconsistent input. Structural/foreign-key/backfill inconsistency fails the
upgrade atomically; affected data is not skipped or silently rewritten.

### Legacy idempotency policy

Add an explicit stored `response_contract` discriminator, default `m1` for all
existing idempotency rows; new M2 writes explicitly store `m2`. Keep legacy scope,
key, request hash, and response text **unchanged**. Do not infer a format from a
zero revision or rewrite a saved response through the new Record type. Subject and
schema replay object shapes stay the same regardless of the discriminator.

A matching old record-create/PATCH key returns the exact saved M1 result object,
without a `revision` member, with its original route status and
`meta.result_contract:"m1"`. It does not create an audit event or revision, advance
the head, or assign that old result to adopted revision 1. A subsequent GET returns
the current M2 Record and its head; the client must read that head for a new edit.

For old PATCH keys, replay lookup occurs before enforcing the new base requirement.
Adding a base to that old request under the same key changes its fingerprint and
returns 409. Reusing a legacy key with changed content also conflicts. An unseen
legacy-shaped PATCH with no base returns `BASE_REVISION_REQUIRED` and cannot write.
Keys are neither deleted, expired, consumed again, nor imported into revision
history. Malformed JSON/transport is still rejected before any replay lookup.

### Proposed storage shape and guard ownership

- Add `records.revision` as a checked non-null integer (initial value 1 for
  backfill). Keep the existing records table/identity trigger and all M1 indexes.
- Add `record_revisions`, with primary key `(record_id,revision_number)`, bounded
  metadata, complete typed snapshot fields/raw data, retained-schema and subject
  foreign keys, predecessor/source references, and non-null audit linkage. The
  record foreign key is deferred so create can insert a snapshot before its record.
  Numeric guards check integer storage class and range, base = number - 1 except
  null at revision 1, and source present only for restore and no greater than base.
  First-operation/adoption and later update/restore combinations are checked.
- Add `mutation_audit_events` with unique event ID and unique record/revision pair.
  Use reciprocal deferred composite foreign keys tying the event ID and exact
  record/revision identity on both sides. Thus neither a revision without its
  required event nor an event pointing to the wrong revision can commit.
  Both new tables constrain attribution kind to `development_test`, with bounded
  actor/correlation identifiers; event action/result and null proposal/reason rules
  follow section 6. Services check the duplicated event/snapshot metadata agrees.
- Add unconditional UPDATE/DELETE rejection triggers for revisions and events.
  Do not use replace/upsert writes on these tables. Add insert checks rejecting
  duplicate/replacement identities before SQLite conflict handling can replace an
  immutable row; test `INSERT OR REPLACE` explicitly. Prevent record deletion and
  insertion replacing an existing record ID, retaining M1's no-upsert identity rule.
- After adoption, record INSERT/UPDATE guards require an existing snapshot at the
  named head matching every current field (null-safe metadata, exact raw data),
  initial head 1, and update head exactly old head + 1. Revision insert guards check
  the next number and immutable identity/pair against an existing record. Adoption
  backfill precedes activation of these new guards within the same transaction.
- Application services additionally enforce full schema validation, complete
  sequential history, latest-head/snapshot equality, required event metadata, and
  replay linkage before commit. Establish the chain inductively from the valid old
  head, immutable predecessor, and exactly one successor; use bounded indexed
  lookups, not a full history scan on each write. Startup adoption validates every backfilled link
  and row count before publishing the migration ledger. These service postconditions
  are mandatory; triggers alone are not advertised as a general arbitrary-SQL write
  API or protection from a privileged operator modifying the database schema.

This is a proposed constraint design, not executed SQL. Deferred links allow the
planned insertion order but are checked at commit; see SQLite's
[deferred foreign keys](https://www.sqlite.org/foreignkeys.html#fk_deferred).
Static rejection triggers must abort the statement and cause the service to roll
back its transaction; see SQLite's [RAISE behavior](https://www.sqlite.org/lang_createtrigger.html#the_raise_function).
The implementation must prove these guards with migration and direct-SQL tests
before acceptance, including cycles, null comparisons, and replacement attempts.

### Database states and rollback

| Starting state | Proposed outcome |
| --- | --- |
| Fresh database | Apply 001–003 and ledger in one transaction; no adoption events. First new record begins at 1. |
| M0 database | Same existing upgrade path through 002/003; no record adoption needed. |
| Empty M1 database (possibly with subjects/schemas/keys) | Append 003, preserve all existing rows/keys, no fabricated record history. |
| Populated disposable M1 database | Adopt all records, including archived ones, in one transaction. No partial/batched visible upgrade. Preserve old replay results. |
| Interrupted before upgrade commit | Reopen at the complete old schema/data state (or empty initial state for a fresh DB); retry the migration. No partial adoption or ledger row may survive. |
| Upgrade committed, process dies before readiness | Reopen as M2; no duplicate adoption, changed timestamps, or new events on repeated startup. |
| Corrupt/incompatible ledger or unsupported data | Fail startup before listener/readiness; no guessed repair or forced adoption. |

Keep the existing 10-second startup bound. If a large disposable backfill exceeds
it, roll back and report a failed upgrade; do not silently introduce batching or
increase the timeout. A large-database strategy requires a separately reviewed
scope change. No real or irreplaceable data is in scope.

Before later upgrade rehearsal, preserve a verified closed disposable M1 database
copy using the existing SQLite-safe procedure; never copy only a live main file.
M1/M0 binaries must refuse a ledger containing 003. There is no down migration.
Rollback means stopping M2, retaining the complete M2 database/history separately,
and explicitly selecting the pre-upgrade closed copy or a fresh disposable path
for the old binary. Post-upgrade changes will not appear in that older copy; this
must be disclosed, never presented as a lossless downgrade. Never delete ledger
rows or history to force old-binary compatibility. Initial PRAGMA initialization
before incompatibility detection retains M0's documented caveat.

Fresh-database-only M2 is an alternative that avoids adoption but refuses populated
M1 compatibility. It is not the recommendation. Synthesizing past revisions from
old replay objects is not an acceptable alternative: replay objects are incomplete
history and lack reliable action attribution/order.

## 8. Compatibility, approval decisions, and acceptance limits

This is an explicit pre-M7 compatibility transition in `/api/v1`: new PATCH calls
require a base, current Record responses gain `revision`, and record mutation
metadata identifies its result contract. Existing successful requests retain their
replay semantics, including old shapes. M1 clients must GET the new head and supply
a base for new writes. The M1 quick start remains historical during this documentation-only phase;
updating executable examples belongs to a later authorized M2 implementation.

| Owner decision | Approved design / remaining question |
| --- | --- |
| D1: M1 upgrade/history boundary | Adopt current state as revision 1, label adoption, preserve legacy replay objects verbatim. |
| D2: Precondition and compatibility | Top-level mandatory `base_revision`; original replacement semantics; explicit legacy-replay-only exception. |
| D3: No-op, restore, numbering | Every accepted fresh-key mutation appends; safe-integer revision range; restore only within the unchanged schema pair. |
| D4: Storage/attribution | Additive 003 with planned guards; component-level development actors, no claimed human/authenticated identity. |
| D5: Read contract | Full snapshot DTO and bounded ascending pagination with a fixed traversal upper bound. |
| D6: Recovery acceptance environment and policy | **representative-r3-v1 — OWNER APPROVED:** exact 78 fault cases/234 recovery executions; standard GitHub-hosted ubuntu-24.04, public repository/$0 paid usage, test-only VFS, exact pinned candidate go-sqlite3/bundled SQLite, synthetic/disposable databases and isolated runner-owned temporary storage. Environment selection resolved; **D6/R3 OPEN** until representative implementation, execution and acceptance. No new execution slot. |

D1–D5 are owner-approved design resolutions for the original documentation
baseline; later scoped implementation permission is recorded above. In that
original design pass no runnable power-loss harness/environment had been found
or exercised. Subsequent harness runs remain historical supporting evidence.
On 2026-10-01 the owner resolved D6 environment selection and adopted
representative-r3-v1 as the D6/T30 acceptance-policy amendment. This supersedes
broader per-bucket/all-compatible-mode/five-schedule acceptance expansion without
changing application guarantees or retroactively accepting any run.

The [approved decision record](m2-r3-representative-proposal.md) and
[test plan](m2-test-plan.md#r3--simulated-storage-failure-and-power-loss) enumerate
all 12 families, both sector profiles, assigned CB/CA/IOERR/FULL/partial faults,
the three required seed-17 recovery classes and all 12 post-ack categories.
Reorder-torn seeds 29/101, incidental WAL fragment lengths, universal positions
for every I/O shape and every physical page offset are not mandatory acceptance
expansions. Additional acceptance requirements need separate owner approval.

Contextarium must preserve revision/current/audit/idempotency atomicity,
migration/adoption atomicity, WAL/FULL configuration, correct successful-sync
semantics, complete-response external-ledger write/flush/fsync before fault,
acknowledged-effect survival, exact integrity/FK/linkage recovery oracle and
duplicate-free replay, without bypassing SQLite durability guarantees. It need
not reproduce SQLite's upstream exhaustive internal page-layout crash program.
Hardware/storage that lies about successful sync completion is outside the
declared model. No personal database, owner-machine reboot/power cycle, real-disk
filling or production fault surface is approved. Policy approval does not grant
harness implementation, execution/setup permission or a new slot. T18/F0–F8,
U0–U5 and ordinary/process recovery remain required; historical faults/timeouts
and semantic qualifications are retained without automatic completeness credit.

No architecture deviation or new ADR is presently required: the proposal preserves
the accepted invariants and stages security as already specified. Cross-schema
restore, history pruning, authenticated claims without M3, or weakening atomicity
would require a new approved design (and an ADR where baseline architecture changes).
Neither this document nor the milestone references waive an acceptance gate.
