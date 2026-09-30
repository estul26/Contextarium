# M4 — Proposal Engine

## Scope
- create/list/read proposals
- immutable submitted proposal payload
- approve/reject/conflicted terminal decisions
- create preconditions and base-revision checks
- exact schema-version validation
- atomic approved transaction into proposal decision + revision/current record + required audit event
- atomic `pending -> conflicted` transaction with required audit event and no record/revision mutation

## Non-goals
No automatic semantic merge.

## Data changes
Proposal storage, status constraints, accepted-revision linkage, conflict metadata, and required indexes/constraints for safe terminal transitions.

## API changes
Proposal REST endpoints with explicit request/response/error/idempotency semantics before implementation.

## Security
Review scope is required. M3 must define whether proposal review also requires operation-specific mutation authority; M4 implements that frozen rule.

Proposal creator identity and authenticated reviewer identity remain distinct.

## Tests
- approve
- reject
- stale-base transition to terminal `conflicted`
- duplicate create precondition transition to terminal `conflicted`
- no record/revision mutation on conflict
- rollback of conflict decision if its audit write fails
- unauthorized review
- immutable submitted payload
- concurrent approve/approve
- concurrent approve/reject
- concurrent conflict/approve decision attempts
- schema-version changes after proposal creation
- credential/authorization changes before review
- atomic rollback if revision/current-state/proposal/audit persistence fails

## Acceptance
An agent with propose-only scope cannot directly mutate authoritative records, a proposal can reach only one terminal decision, and a stale/create-conflicting approval attempt cannot partially mutate authoritative state.

## Rollback
Pending proposals may be safely exported/discarded; accepted revisions and terminal decisions remain authoritative.
