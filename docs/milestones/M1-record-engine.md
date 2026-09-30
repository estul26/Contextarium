# M1 — Generic Record Engine

The concrete M1 decisions and temporary endpoint contract are defined in
[../m1-contract.md](../m1-contract.md). Reproduction commands are in
[../m1-development.md](../m1-development.md).

## Scope
- subjects
- canonical namespace grammar
- schema registry baseline
- generic records
- create/read/list/update application services
- deterministic schema validation
- bounded list/read behavior

## Non-goals
No proposal workflow, authorization policy, revision guarantees, FTS, or MCP.

## Safety gate
M1 is a local development milestone only. Use synthetic/disposable data. Do not remotely expose M1 or treat its database as production data.

Record mutation behavior implemented here is provisional until M2 makes revision/audit guarantees mandatory.

## Data changes
Introduce subject, schema, and record storage.

## API changes
Draft subject/record REST endpoints. Before an endpoint is implemented, its milestone-local request/response/error contract must be explicit.

## Security
- bound request payloads and list responses
- deterministic ordering/pagination
- parameterized SQL only
- reject non-canonical namespaces
- safe structured logs must not record bearer credentials or full record bodies by default

## Design decisions required before completion
- schema language/dialect and deterministic validation semantics
- concrete opaque ID generator/constraints
- patch representation and immutable-field allowlist
- minimal provenance representation
- archive/status transition semantics

## Tests
Schema validation, exact namespace grammar, ID/key semantics, bounded CRUD/list behavior, patch behavior, transaction/error handling, and at least two independent synthetic domains.

## Acceptance
At least two synthetic domains can be stored without new domain-specific Go services, and no implementation contradicts the v0.1 immutable subject/namespace rules.

## Rollback
Migrations must document downgrade or export strategy before production use.
