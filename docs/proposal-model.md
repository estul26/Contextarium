# Proposal Model

## Purpose

A proposal separates an agent's suggested change from authoritative state.

## Initial operations

- create
- update
- archive
- restore

Hard deletion is not an ordinary v0.1 proposal operation.

## Required proposal information

A proposal must contain enough immutable submitted state to review it later, including:

- proposal ID
- operation
- target record ID when applicable
- target subject and namespace
- base revision when applicable
- exact `schema_id` and `schema_version` for the proposed resulting state
- proposed data or patch
- reason
- source client attribution appropriate to the active milestone
- optional claimed provenance
- status
- created timestamp
- reviewed timestamp when resolved
- accepted revision identifier when approved

From M3 onward, protected client-originated proposals use authenticated client identity. Any earlier synthetic development fixtures must remain explicitly labeled as development/test attribution.

For a create proposal, a target record ID may be reserved when the proposal is created. Approval must verify inside the transaction that the reserved record does not already exist.

## Lifecycle

```text
pending
  |-- approved
  |-- rejected
  |-- conflicted
```

Terminal states are immutable. A second approve/reject attempt against a terminal proposal returns a conflict such as `PROPOSAL_RESOLVED`; it does not silently change the decision.

## Approval transaction

Approval begins as a single atomic decision attempt.

Inside one transaction, Contextarium must:

1. authenticate the reviewer when authentication is available/required by the active milestone
2. authorize the review and underlying mutation when policy enforcement is active
3. verify that the proposal is still `pending`
4. verify the create precondition or compare `base_revision` with the current record revision
5. validate the proposed resulting state against the proposal's exact pinned schema ID/version
6. create the accepted immutable revision
7. update/create the current authoritative record
8. change the proposal to `approved` and link its accepted revision
9. append the required mutation/decision audit event

If steps 1–3, 5–9, or any storage operation fail as an **operational failure**, the authoritative record, proposal decision, revision, and required audit event remain unchanged.

A failed create precondition or stale base revision is handled by the explicit conflict transaction below rather than by the operational-failure rollback rule.

## Conflict transaction

If step 4 detects a failed create precondition or stale base revision, Contextarium performs a separate atomic terminal decision:

1. verify the proposal is still `pending`
2. change the proposal to `conflicted`
3. record conflict metadata sufficient for later review
4. append the required `proposal.conflicted` audit event
5. create or modify **no** authoritative record or revision

If persisting the conflict decision or audit event fails, the transaction rolls back and the proposal remains pending.

A conflicted proposal is terminal. Rebase is represented by a new proposal or another future explicitly defined workflow; v0.1 does not mutate the original submitted proposal into a new request.

## Reject transaction

Rejecting a proposal atomically verifies `pending`, records the reviewer/decision metadata, changes the proposal to `rejected`, and appends its required decision audit event.

## Conflicts

The original submitted proposal payload remains immutable so that a later replacement/rebase can be reviewed explicitly.

Automatic semantic merging is outside v0.1 scope.

M4 must test concurrent approve/approve, approve/reject, and simultaneous stale/conflict decisions.
