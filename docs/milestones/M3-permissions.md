# M3 — Authentication & Permissions

## Scope
- client identities
- hashed credential verifiers
- bootstrap/recovery model
- credential rotation/revocation
- scope-to-operation matrix
- namespace restrictions using the frozen segment matcher
- deny-by-default policy evaluation
- application-service authorization enforcement

## Non-goals
No OAuth, SSO, enterprise IAM, or external identity provider.

## Data changes
Clients, credential metadata, grants/policies.

## API changes
Adapters establish authenticated context, but protected application services own authorization. Endpoint middleware may pre-screen requests but is not sufficient by itself.

## Security
- fail closed on malformed/failed policy evaluation
- define how client grants and credential restrictions combine
- define review authority and self-approval policy for later proposal use
- define revocation behavior for pending/in-flight work
- historical reads use the record's immutable namespace/subject plus the required revision scope
- sensitivity remains descriptive, not an authorization boundary

## Tests
- allowed/denied scope matrix
- exact and descendant namespace edge cases
- direct application-service denial tests
- revoked credentials
- historical revision access
- subject-list visibility
- malformed policy inputs

## Acceptance
No client can access an operation or namespace it was not explicitly granted, regardless of whether the call arrives through REST or a direct service test.

## Rollback
Administrative recovery procedure must exist before production enforcement.
