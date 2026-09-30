# M7 — REST API Stabilization

## Scope
- align implementation with OpenAPI
- consistent error envelopes/codes
- idempotency behavior where required
- pagination/filter conventions
- compatibility tests

## Non-goals
No MCP implementation.

## Data changes
None unless idempotency metadata is required.

## API changes
Freeze REST v1 behavior.

## Security
Uniform auth/error behavior and no sensitive information in errors.

## Tests
OpenAPI contract tests and negative/error cases.

## Acceptance
Documented REST v1 behavior matches implementation.

## Rollback
Breaking changes after freeze require explicit versioning/compatibility policy.
