# Baseline Invariants

This document freezes cross-cutting rules that all milestones must preserve unless a later ADR explicitly changes them.

## 1. Release and data-safety gates

Milestones are development checkpoints, not releases.

- **M0–M1:** local development only; synthetic/disposable data; no remote exposure.
- **M2:** revision invariants and minimal transactional mutation-audit persistence become mandatory for revision-bearing writes. Data remains development-only.
- **M3–M6:** protected integration behavior may be exercised. Remote access, if deliberately enabled, requires encrypted transport. These builds remain pre-release.
- **M7–M10:** API, protocol, search, portability, and example-domain behavior are stabilized and rehearsed.
- **v0.1 release gate:** all milestone acceptance criteria pass, clean-instance restore is rehearsed, security documentation matches behavior, and no known blocking architecture contradiction remains.

No pre-release milestone should be represented as production-ready.

## 2. Authority boundary

Authoritative state is structured data managed by Contextarium application services.

Adapters such as REST, MCP, CLI commands, or future protocols do not own business rules.

Application services enforce authorization for every protected operation.

SQLite is the authoritative v0.1 store. FTS5, Markdown views, caches, and exports are derived or portable representations, not competing authorities.

## 3. Namespace grammar and matching

Stored namespace names:

- are lowercase ASCII
- consist of dot-separated segments
- each segment matches `[a-z][a-z0-9_-]{0,62}`
- contain 1–8 segments
- are at most 255 characters
- contain no wildcards or empty segments
- are rejected if non-canonical rather than silently normalized

Permission patterns are separate values:

- `a.b` = exact match only
- `a.*` = one or more descendants of `a`
- `a.*` does not match `a`
- wildcards anywhere except a final `.*` are invalid
- unrestricted administration is an explicit grant, not `*`

## 4. Record metadata

For v0.1:

- record ID is immutable
- subject ID is immutable after record creation
- namespace is immutable after record creation
- schema ID/version is pinned per accepted revision
- key is optional and non-unique by default
- sensitivity is descriptive metadata, not an authorization boundary

Moving a record between subjects or namespaces is out of scope.

## 5. Identifier semantics

IDs are opaque and issued by Contextarium for ordinary creates.

Clients must not infer ordering or other semantics from IDs.

Normal create requests do not implicitly upsert by ID or key.

Clean-instance restore may preserve IDs after validation.

A revision is uniquely identified by `(record_id, revision_number)`.

## 6. Schema interpretation

A proposal or revision identifies the exact schema ID/version governing its resulting state.

Validation never silently switches to the newest schema version.

Referenced schema versions are retained while live records, retained history, pending proposals, or supported backups depend on them.

Schema migration is explicit and revision-producing.

## 7. Proposal decision atomicity

A proposal decision is guarded by the proposal's own state as well as any record revision predicate.

Approval of a pending proposal atomically verifies authorization, verifies the proposal is still pending, verifies create/base-revision preconditions, validates the exact pinned schema, persists the new revision/current state, marks the proposal terminal, links the accepted revision, and persists the required audit event.

If the create precondition or base-revision predicate is stale, the attempt is an **expected conflict outcome**, not an operational failure. Contextarium atomically transitions `pending -> conflicted` and persists the required proposal-decision audit event **without** creating or changing any authoritative record or revision.

If persistence of that conflict decision or its required audit event fails, the transaction rolls back and the proposal remains pending.

Approve/approve and approve/reject races may produce only one terminal decision.

A repeated decision against a terminal proposal returns a conflict rather than changing the result.

## 8. Historical authorization

Because subject and namespace are immutable in v0.1, historical revision access uses the same record namespace/subject boundary plus the scope required to read revisions.

Proposals require their own proposal scope plus access to the target namespace.

Logical exports must not become a way to bypass ordinary visibility rules.

## 9. Transport security

Loopback-only/local access may use local HTTP according to deployment needs.

Authenticated traffic crossing an untrusted network must use encrypted transport supplied by a reverse proxy, private tunnel, or equivalent server deployment.

Bearer credentials must never be intentionally sent in plaintext across an untrusted network.

## 10. Audit staging

M2 introduces the minimum append-only mutation event needed to satisfy the atomic revision guarantee.

Before M3 exists, M2 uses an explicitly identified **development/test actor** supplied by the local development harness or internal call context. This attribution records what actually initiated the development mutation but **must not be described as authenticated client identity**.

Starting with M3, protected client-originated operations must use authenticated client attribution. Existing M2-era events keep their original development/test attribution and are never retroactively presented as authenticated.

M6 expands this into the complete audit model, coverage, query surface, privacy/redaction rules, denied/failed-attempt behavior, and access controls.

## 11. OpenAPI completion rule

Until M7 freezes REST v1, `api/openapi.yaml` is a draft inventory plus any already-frozen shared constraints.

Before an endpoint is implemented in a milestone, that endpoint's request body, response shape, security requirement, concurrency/idempotency behavior where applicable, and error cases must be specified in OpenAPI or explicitly documented as a temporary milestone-local contract.

M7 reconciles all implemented operations and freezes the public REST v1 contract.
