# Contextarium

**Self-hosted context infrastructure for humans and AI agents.**

Contextarium is an API-first, user-controlled context and data platform. It provides a structured, versioned, permission-controlled source of truth that humans, AI assistants, coding agents, CLI tools, web apps, and future integrations can share without making any AI vendor authoritative.

> Contextarium is not an AI memory database. Memory is one domain built on top of the platform.

## Status

**M1 — Generic Record Engine. Local development only; synthetic/disposable data; not a production release.**

M0 and the Linux CI baseline are merged. M1 adds subjects, immutable schema versions, and generic schema-validated records with bounded create/read/list/update services and temporary REST endpoints. See the [M1 contract](docs/m1-contract.md) and [synthetic quick start](docs/m1-development.md). The [M0 runtime document](docs/m0-runtime.md) remains the historical foundation contract. Authentication, revision history, proposals, audit, FTS, and MCP are not implemented.

## Core principles

- Self-hosted and API-first
- AI-client and vendor agnostic
- Generic records rather than hard-coded domain objects
- Structured data is authoritative
- Proposal-first agent writes by default
- Immutable revision history
- Optimistic conflict detection
- Append-only audit events
- Least-privilege scopes and namespace restrictions
- Open, portable export formats
- Single-binary deployment on a small Linux server
- Minimal infrastructure

## Core concepts

Contextarium Core understands Subjects, Records, Namespaces, Schemas, Clients, Permissions, Proposals, Revisions, Audit Events, and Sources.

Higher-level domains such as profile, memory, knowledge, projects, tasks, events, documents, and preferences are schema packs built on the same generic record engine.

## Initial technology baseline

- Backend: Go
- Primary database: SQLite
- Full-text search: SQLite FTS5
- Public API: REST under `/api/v1`
- AI protocol adapter: MCP under `/mcp`
- Deployment: single Linux process; reverse proxy optional
- Remote access: authenticated traffic over untrusted networks **must use encrypted transport**
- Backup: portable SQLite/JSON/JSONL exports; sensitive remote backups **must be encrypted**

## Architecture

```text
AI / agents / CLI / web / mobile / integrations
                         |
                    REST / MCP
                         |
                 Contextarium API
                         |
   +---------------------+----------------------+
   | Auth | Policy | Schema | Records | Search |
   | Proposal | Revision | Audit | Import/Export|
   +---------------------+----------------------+
                         |
              SQLite + SQLite FTS5
```

See [docs/architecture.md](docs/architecture.md).

## v0.1 non-goals

No vector database, embeddings, built-in LLM, chat UI, RAG framework, Redis, PostgreSQL, MongoDB, Kubernetes, microservices, multi-node clustering, enterprise SSO, mobile app, or complex web dashboard.

## Documentation

- [Project charter](docs/project-charter.md)
- [Architecture](docs/architecture.md)
- [Baseline invariants](docs/baseline-invariants.md)
- [Domain model](docs/domain-model.md)
- [API contract](docs/api-contract.md)
- [Schema system](docs/schema-system.md)
- [Permission model](docs/permission-model.md)
- [Proposal model](docs/proposal-model.md)
- [Revision model](docs/revision-model.md)
- [Audit model](docs/audit-model.md)
- [Threat model](docs/threat-model.md)
- [Non-goals](docs/non-goals.md)
- [ADRs](docs/adr/)
- [Milestones](docs/milestones/)

## Development rule

**The repository is the specification. Chat history is not authoritative.**

Implement one milestone at a time. Architecture changes require a new ADR.

Milestone builds before the v0.1 release gate are development artifacts. See [docs/baseline-invariants.md](docs/baseline-invariants.md) for the safety and exposure rules.

## License

Contextarium is licensed under the [Apache License 2.0](LICENSE).

SPDX-License-Identifier: `Apache-2.0`
