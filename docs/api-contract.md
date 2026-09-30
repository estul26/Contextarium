# API Contract

## Versioning

The REST API is namespaced under:

```text
/api/v1
```

Breaking changes require a new major namespace or an explicitly documented compatibility strategy.

## Security

Protected operations require authenticated client context and application-service authorization.

Bearer credentials must be carried only over encrypted transport when traffic crosses an untrusted network.

The adapter is not the authorization boundary: REST and MCP call the same protected application services.

## Request conventions

Clients should send:

```http
Content-Type: application/json
Authorization: Bearer <credential>
X-Request-ID: <request-id>
Idempotency-Key: <unique-key>
```

`Idempotency-Key` is required for mutation operations whose retry could duplicate an effect once that operation is implemented. Exact key scope, request-mismatch behavior, retention, and replay semantics are frozen in the milestone that introduces the mutation.

`X-Request-ID` is correlation metadata, not trusted actor identity.

## Success envelope

```json
{
  "data": {},
  "meta": {
    "api_version": "v1",
    "request_id": "req_example"
  }
}
```

## Error envelope

```json
{
  "error": {
    "code": "REVISION_CONFLICT",
    "message": "Record changed since the client last read it.",
    "details": {
      "expected_revision": 4,
      "current_revision": 5
    }
  }
}
```

## Endpoint families

```text
GET    /api/v1/subjects
POST   /api/v1/subjects

GET    /api/v1/records
POST   /api/v1/records
GET    /api/v1/records/{id}
PATCH  /api/v1/records/{id}

GET    /api/v1/records/{id}/revisions
GET    /api/v1/records/{id}/revisions/{revision}
POST   /api/v1/records/{id}/restore/{revision}

GET    /api/v1/search

GET    /api/v1/proposals
POST   /api/v1/proposals
GET    /api/v1/proposals/{id}
POST   /api/v1/proposals/{id}/approve
POST   /api/v1/proposals/{id}/reject

GET    /api/v1/audit

POST   /api/v1/export
POST   /api/v1/import
```

## Concurrency

Mutations must carry an expected/base revision where applicable.

A mismatch returns a conflict and must never silently overwrite current authoritative state.

Proposal decisions are additionally guarded by the proposal's own pending/terminal state.

## Resource identity and metadata

Record IDs are opaque and immutable.

`subject_id` and `namespace` are immutable for a v0.1 record.

`key` is optional and non-unique by default; it does not imply upsert.

Namespace grammar and permission matching are defined in the domain and baseline-invariant documents.

## MCP

The intended MCP endpoint is:

```text
/mcp
```

MCP is an adapter over the same application service layer as REST.

Initial MCP capabilities should focus on:

- search
- read
- propose

Direct destructive administration is not an initial MCP goal.

## OpenAPI authority and completion gate

During M0–M6, `api/openapi.yaml` is a **draft endpoint inventory plus already-frozen shared constraints**, not a complete production contract.

Before a milestone implements an endpoint, that endpoint's request body, response content, security requirement, concurrency/idempotency behavior where applicable, and error cases must be added to OpenAPI or recorded as an explicit milestone-local temporary contract.

M7 reconciles all implemented behavior and freezes REST v1.

No implementation may use an omission in the draft OpenAPI file to bypass an invariant defined elsewhere in the architecture baseline.
