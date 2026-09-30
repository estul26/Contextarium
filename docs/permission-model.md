# Permission Model

## Goals

- least privilege
- explicit client identity
- action scopes
- namespace restrictions
- safe defaults for AI/agent clients
- deny by default

## Enforcement ownership

Authorization is an **application-service invariant**.

REST middleware, MCP transport code, CLI adapters, or reverse proxies may authenticate and pre-screen requests, but every protected application-service operation must enforce policy before accessing or changing authoritative data.

Direct storage access is internal implementation detail and is not an authorized bypass path.

M3 must include denial tests that call application services directly rather than relying only on endpoint tests.

## Authentication

v0.1 uses client credentials suitable for self-hosted deployments.

Credential material must not be stored in plaintext. The server stores a secure verifier/hash and associated metadata.

## Authorization dimensions

v0.1 authorization is evaluated across:

1. authenticated client
2. requested action/scope
3. target namespace
4. target subject where an operation explicitly defines a subject filter

`sensitivity` is descriptive metadata in v0.1. It is not an authorization dimension and must not be presented as a security boundary. Unsupported sensitivity-based policy configuration must fail rather than be silently ignored if such configuration is ever exposed.

## Provisional scopes

```text
subjects.read
records.read
records.search
records.propose
records.write
records.archive
proposals.read
proposals.review
revisions.read
audit.read
export.run
import.run
admin
```

The exact scope-to-operation matrix, bootstrap model, rotation/revocation rules, and review authority are frozen during M3.

## Namespace rules

Namespace grammar and permission-pattern semantics are defined in [domain-model.md](domain-model.md).

Credentials may be restricted to exact namespaces or terminal descendant patterns such as:

```text
projects.example
projects.*
knowledge.*
```

Matching must use parsed namespace segments, never substring comparison.

## Protected-resource rules

Because `record.subject_id` and `record.namespace` are immutable in v0.1:

- reading a current record requires access to that record's namespace
- reading a historical revision requires `revisions.read`, ordinary record-read authority, and access to the record's immutable namespace
- reading a proposal requires proposal-read authority plus access to its target namespace
- reviewing a proposal requires proposal-review authority plus whatever mutation authority M3 explicitly defines for the resulting operation
- logical exports must filter records, revisions, and proposal/history dependencies using the same authorization boundary
- direct record creation is authorized against the requested namespace before the record exists

A future feature that moves a record between subjects or namespaces requires a new policy design covering both source and destination. v0.1 does not support such moves.

## Agent default

AI and agent credentials should default to:

```text
read + search + propose
```

rather than direct authoritative write permission.

## Administrative clients

Administrative write/review/export/import privileges must be explicitly granted.

## Failure behavior

Missing permission, malformed policy input, or policy-evaluation failure must deny access.
