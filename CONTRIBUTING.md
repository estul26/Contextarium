# Contributing

Contextarium is implementing the approved M2 D1–D5 contract from `05907846c59d909990d0c6159edce1da88dea7c6`. Use the [M2 contract](docs/m2-contract.md), [acceptance plan](docs/m2-test-plan.md), and [development guide](docs/m2-development.md). Final acceptance and D6/R3 remain open. Do not begin M3 or provision a fault-testing environment without separate authorization.

Before implementation, read the project charter, architecture, all accepted ADRs, and the active milestone document.

Rules:

- Do not add speculative features outside the active milestone.
- Do not introduce vendor lock-in into the domain model.
- Do not hard-code profile, memory, project, or task semantics into the core record engine.
- Do not bypass proposal/revision/audit rules for agent-originated changes.
- Do not introduce major infrastructure without an ADR.
- Never commit real personal data in examples or tests.
- Use synthetic examples such as `user_001`, `example.com`, and `Example Certification`.
- Architecture changes require a new ADR.

Each implementation PR should state scope, non-goals, tests run, security impact, migration/rollback impact, and any architecture deviation.
