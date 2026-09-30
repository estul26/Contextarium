# Contributing

Contextarium is implementing M0 against the merged architecture baseline. Use the [M0 developer commands and runtime contract](docs/m0-runtime.md); do not begin M1 in an M0 change.

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
