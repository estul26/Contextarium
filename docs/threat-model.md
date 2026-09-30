# Threat Model

## Assets

- authoritative records
- revision history
- proposals and review decisions
- client credentials
- audit history
- schema definitions
- attachment references
- exported backups

## Trust boundaries

1. external clients -> API
2. protocol adapters -> application services
3. application -> SQLite
4. application -> optional file storage
5. export process -> backup destination

## Primary threats

### Stolen client credential
Controls: hashed credential verifiers, least-privilege scopes, namespace restrictions, rotation/revocation, audit.

### Agent overreach
Controls: proposal-first defaults, narrow namespace scopes, approval workflow, audit trail.

### Lost update / stale mutation
Controls: base revisions, optimistic concurrency, atomic transactions.

### SQL injection
Controls: parameterized SQL, validated filter/sort fields, no user-controlled SQL fragments.

### Schema abuse
Controls: request-size limits, bounded nesting/arrays where appropriate, schema complexity limits, validation budgets if needed.

### Backup leakage
Controls: explicit export permission and encryption guidance. Plaintext private data must not be pushed to public repositories.

### Audit leakage
Controls: restricted audit scope, payload minimization, redaction rules.

### Denial of service
Controls: request/body limits, pagination, query limits, bounded imports, optional edge/app rate limiting.

## Deferred threats

Multi-tenant isolation, enterprise federation, distributed consensus, and multi-node failover are not v0.1 concerns.

This document must evolve as implementation details become concrete.
