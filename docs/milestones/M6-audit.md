# M6 — Full Audit Service

## Context

M2 already introduced the minimum transactional audit event required for accepted revision-bearing mutations. M6 completes the audit subsystem.

## Scope
- complete audit event vocabulary and mandatory fields
- audit access scope
- query/read API
- actor / proposer / reviewer / proposal / revision linkage
- redaction and metadata-minimization rules
- policy for rejected, denied, and failed attempts
- coverage for state/security operations beyond the M2 minimum

## Non-goals
No external SIEM integration and no cryptographically witnessed external ledger.

## Data changes
Complete audit indexes/metadata needed for the read model and event coverage.

## API changes
Read-only audit endpoint.

## Security
Payload minimization, restricted access, server-derived identity attribution, bounded/redacted untrusted metadata.

## Tests
Required event coverage, append-only application behavior, unauthorized audit access, actor/proposal/revision linkage, rejected operations, and audit-write failure handling.

## Acceptance
Every accepted authoritative mutation and proposal decision has a traceable audit event, and the documented policy for denied/rejected/failed activity is enforced.

## Rollback
Audit data must not be silently discarded during downgrade.
