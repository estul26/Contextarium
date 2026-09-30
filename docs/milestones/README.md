# Milestones

Contextarium is implemented one milestone at a time.

Each milestone must define scope, non-goals, data/API changes, security considerations, tests, acceptance criteria, and rollback considerations.

Order:

1. M0 — Foundation
2. M1 — Generic Record Engine
3. M2 — Revision Engine + minimal transactional mutation audit
4. M3 — Authentication & Permissions
5. M4 — Proposal Engine
6. M5 — Search
7. M6 — Full Audit Service
8. M7 — REST API Stabilization
9. M8 — MCP Adapter
10. M9 — Portability
11. M10 — Example Schema Packs

Do not begin a later milestone until the current milestone is validated and checkpointed.

## Milestone safety gates

Milestones are implementation checkpoints, not releases.

- **M0–M1:** local development only; synthetic/disposable data; no remote exposure.
- **M2:** revision invariants and minimal mutation-audit persistence become mandatory for revision-bearing writes. Data remains development-only.
- **M3–M6:** integration builds may exercise protected behavior. Remote access, if deliberately enabled, requires encrypted transport. These builds are still pre-release.
- **M7–M10:** API/protocol/portability behavior is stabilized and rehearsed.
- **v0.1 release gate:** all milestone acceptance criteria pass, clean-instance restore is rehearsed, security documentation matches behavior, and no known blocking architecture contradiction remains.

No milestone before the v0.1 release gate should be represented as production-ready.

See [../baseline-invariants.md](../baseline-invariants.md).
