# M1 temporary local record contract

This contract is specified before the M1 endpoints are implemented. It supplements
the draft OpenAPI inventory; it is not the frozen M7 REST contract. M1 runs on
loopback with synthetic/disposable data only. There is no authentication,
authorization, revision history, mutation audit, proposal workflow, FTS, or MCP.
No response claims an authenticated actor or includes a revision number.

## Decisions

- Subjects and records receive `sub_` / `rec_` followed by 32 lowercase hex
  characters from 128 cryptographically random bits. IDs are opaque, exact, and
  immutable. Client-supplied IDs and implicit upsert are rejected.
- Namespaces follow the exact baseline grammar (1–8 lowercase ASCII segments,
  each `[a-z][a-z0-9_-]{0,62}`, total at most 255 bytes). They are values on records,
  not a separate mutable registry. Schema IDs use the same grammar and are explicit
  registry names; versions are integers from 1 through 2147483647.
- A published `(schema_id, schema_version)` is immutable. Publishing an existing
  pair conflicts. No schema deletion endpoint exists. Records pin the exact pair;
  changing it is deferred until revision-producing schema migration exists.
- `key` is optional nullable text, at most 255 bytes, and non-unique. `sensitivity`
  is `public`, `private` (default), or `restricted`, and is descriptive only.
- Provenance is optional/null or `{ "source": "...", "note": "..." }`. `source`
  is required (1–1024 bytes); optional `note` is at most 2048 bytes. This is an
  untrusted claim, never fetched or treated as actor identity.
- Records start `active`. PATCH may set `active` or `archived` in either direction,
  including an unchanged status. Unarchiving retains current data; it is not a
  historical restore. Archived records remain readable/editable. No hard delete.
- PATCH replaces only supplied fields: `data`, `key`, `sensitivity`, `provenance`,
  `status`. `data` replaces the entire object; this is not JSON Merge Patch or
  JSON Patch. Missing fields are unchanged; null clears only key/provenance.
  Empty patches, unknown fields, IDs, subject, namespace, schema pair, timestamps,
  and revision fields are rejected. Updates read/validate/write in one IMMEDIATE
  transaction. Distinct updates are last-accepted-write-wins in M1; stale-client
  detection and revision/audit guarantees belong to M2.

## Deterministic schema profile

The language is a deliberately bounded subset of JSON Schema 2020-12, validated
by pinned `github.com/santhosh-tekuri/jsonschema/v6`. Root schemas must declare
`"$schema":"https://json-schema.org/draft/2020-12/schema"` and `"type":"object"`.
Supported keywords are `$schema` (root only), `title`, `description`, `type`,
`properties`, `required`, `additionalProperties` (boolean only), `items`, `enum`,
`const`, `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`,
`minLength`, `maxLength`, `minItems`, `maxItems`, `minProperties`, `maxProperties`.
These six length/count constraints must be integers between 0 and 2147483647.
Nested schemas may be objects or booleans. All other keywords are rejected,
including references (local and remote), IDs, defaults, format, regexes, and
combinators. External resource loading is also disabled in the compiler.
Property names and literal enum/const contents are data, not schema keywords.

Additional record data properties are allowed by default, rejected only when
the schema says `additionalProperties:false`, and are never stripped. No coercion,
default insertion, Unicode normalization, or floating-point conversion occurs.
Numbers retain their JSON representation and validate using exact arithmetic:
at most 128 bytes per numeric token, with exponent between -308 and 308.
JSON accepts at most 32 nesting levels and 4096 value nodes, valid UTF-8/Unicode
escapes, no duplicate object keys, and exactly one top-level value. Schema bodies
are at most 32 KiB; record data is an object at most 64 KiB. Schema keyword counts
are checked by the same structural bounds; finite trees and no references or
combinators bound validation work. Validation failures return a static error,
never raw validator diagnostics containing data.

## HTTP

All routes below are under `/api/v1`. They have no credentials in M1 and inherit
the enforced loopback listener. Browser cross-origin requests are rejected, and
no CORS access is granted. Health/readiness probes retain their M0 contract.

| Method/path | Request | Success |
| --- | --- | --- |
| POST `/subjects` | `{kind,display_name}` | 201 Subject |
| GET `/subjects` | list query | 200 Subject[] |
| GET `/subjects/{id}` | none | 200 Subject |
| POST `/schemas` | `{schema_id,schema_version,definition,migration_policy}` | 201 Schema |
| GET `/schemas/{schema_id}/{schema_version}` | none | 200 Schema |
| POST `/records` | `{subject_id,namespace,schema_id,schema_version,data,key?,sensitivity?,provenance?}` | 201 Record |
| GET `/records` | list query; optional exact `subject_id`, `namespace`, `status` | 200 Record[] |
| GET `/records/{id}` | none | 200 Record |
| PATCH `/records/{id}` | nonempty replacement-field object above | 200 Record |

Subject has `id,kind,display_name,created_at`. Kind is a single namespace segment;
display name is 1–255 UTF-8 bytes. Text metadata rejects NUL characters. Schema has the four submitted fields plus
`created_at`; `migration_policy` must be `explicit` (no automatic migration).
Record has `id,subject_id,namespace,schema_id,schema_version,data,key,sensitivity,
provenance,status,created_at,updated_at`. Timestamps are server UTC RFC3339Nano
strings and are not concurrency tokens.

Success is `{ "data": object-or-array, "meta": { "api_version":"v1",
"request_id":"req_<32 lowercase hex>", "next_cursor":"..." } }`; next_cursor is
present only for lists, empty at the end. Request IDs are server-generated; inbound
X-Request-ID is not reflected. Errors are `{ "error": { "code":"...",
"message":"static safe text" } }` with the server request ID in X-Request-ID.
Every response uses application/json, no-store, and nosniff. No request content,
bearer headers, database errors, or record bodies are logged.

POST/PATCH require `Content-Type: application/json` (optional UTF-8 charset),
no content encoding, and one `Idempotency-Key` header of 1–255 visible ASCII
characters. Request bodies are capped at 96 KiB, including unknown fields. Unknown
or differently cased envelope fields, trailing JSON, and duplicate keys fail.
Required fields cannot be null. All routes reject unsupported query parameters
and duplicates. Request work has a 3-second context deadline.

List parameters: `limit` integer 1–100 (default 20), `cursor` opaque continuation.
Rows use ascending immutable ID order (not creation time). A cursor binds the
resource/filter set and last ID; changing filters conflicts with cursor validation.
Lists default to all statuses, use keyset pagination, and are not snapshots:
concurrent inserts/updates can affect later pages. Filters never match descendants.
Responses are bounded by page size and the stored per-item limits.

## Idempotency and errors

Keys are scoped to method/collection, or method/record ID for PATCH, within this
single local database (no fabricated client identity). The service fingerprints
the validated JSON value with sorted object keys, preserving number spellings and
array order. Repeating a committed key/body returns the originally stored object
and original success status, even after restart or later record edits. A different
body under the same key returns 409. The mutation and replay snapshot commit in
the same transaction. Failed writes consume no key. Keys have no expiration in
M1; reset of the disposable database removes them. Replay returns the original
data snapshot, not a fresh read. Caller-generated keys are not logged.

| HTTP | Code | Meaning |
| --- | --- | --- |
| 400 | INVALID_ARGUMENT | invalid JSON, field, ID, namespace, query, cursor or key |
| 403 | ORIGIN_REJECTED | cross-origin browser access |
| 404 | NOT_FOUND | absent subject, record, schema or route |
| 405 | METHOD_NOT_ALLOWED | known route with unsupported method (Allow provided) |
| 409 | CONFLICT | existing schema pair or changed idempotency payload |
| 413 | PAYLOAD_TOO_LARGE | request exceeds byte cap |
| 415 | UNSUPPORTED_MEDIA_TYPE | unsupported content type/encoding |
| 422 | SCHEMA_VALIDATION_FAILED | invalid/unsupported schema or data mismatch |
| 503 | UNAVAILABLE | database contention/unavailability or operation deadline |
| 500 | INTERNAL_ERROR | unexpected internal failure |

## Storage and rollback

Migration 002 appends `subjects`, immutable `schemas`, `records`, and
`idempotency_keys`, with foreign keys, checks, and record filter indexes. Migration
001 is never edited. No revision/proposal/auth/audit/search tables are introduced.
Schema retention and immutable record identity are also guarded in SQLite.

M0 databases upgrade automatically on startup. An old M0 binary refuses the newer
ledger; there is no automatic downgrade. Before trying M1, stop M0 and preserve
its closed disposable database separately. To return to M0, stop M1 and restore
that copy or choose a fresh disposable DB path. Do not delete ledger rows to force
an older binary to open M1 data. Production downgrade/export guarantees remain
part of later milestones; M1 data must be disposable.
