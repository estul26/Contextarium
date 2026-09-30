# M0 — Foundation

## Scope
- initialize Go module/package layout
- configuration loading
- logging baseline
- SQLite connection lifecycle
- migration runner
- health/readiness endpoints
- test harness

## Non-goals
No domain records, auth, proposals, revisions, FTS, MCP, or backup behavior.

## Data changes
Only migration/infrastructure metadata needed to bootstrap later milestones.

## API changes
Health/readiness only.

## Security
- safe runtime defaults
- no secrets in source
- restricted default listen address unless explicitly configured otherwise

## Tests
- configuration tests
- migration tests
- SQLite open/close test
- HTTP health endpoint test

## Acceptance
- clean checkout builds and tests
- empty database migrates from zero
- process starts and shuts down cleanly
- health endpoint works
- no M1+ behavior exists

## Rollback
Remove M0 runtime artifacts; no user-domain data exists yet.
