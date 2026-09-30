# Project Charter

## Product definition

Contextarium is a self-hosted, API-first context and data platform.

It provides a user-controlled source of truth that humans, AI assistants, coding agents, CLI tools, web apps, and integrations can read and, subject to policy, change.

Memory is one possible domain. It is not the platform boundary.

## Problem

Useful context is fragmented across chat systems, coding agents, files, project trackers, notes, and vendor-specific memory features. Each client may keep an incomplete copy, and agent writes may silently overwrite authoritative information.

## Core guarantees

At the v0.1 release gate, Contextarium guarantees:

1. Authoritative state is structured and explicit.
2. Accepted authoritative changes are versioned.
3. Historical revisions are immutable.
4. Conflicting writes are detected.
5. Agent writes can default to proposal-first behavior.
6. Access is scoped by action and namespace.
7. Security- and mutation-relevant changes are auditable.
8. Data is portable through open export formats.
9. The core remains useful without an external AI API.
10. Protected operations enforce authorization in the application-service layer rather than relying on one protocol adapter.

Earlier milestone builds are development checkpoints and may intentionally lack later guarantees. Their permitted use is defined in [baseline-invariants.md](baseline-invariants.md).

## Core concepts

- Subject
- Record
- Namespace
- Schema
- Client
- Credential
- Permission
- Proposal
- Revision
- Audit Event
- Source

Profile, memory, knowledge, projects, tasks, preferences, events, and documents are schema-level domains built on the generic core.

## Deployment objective

The initial system should run comfortably as a single Go process backed by SQLite on a small Linux server.

## v0.1 success criteria

v0.1 is complete only when the milestone acceptance criteria prove that Contextarium can:

- create and read generic schema-validated records
- preserve immutable revision history
- enforce scoped client permissions
- support proposal-first changes and conflict detection
- search structured data and full text without authorization leakage
- produce audit history
- expose a stable REST API
- expose MCP as an adapter over the same application services
- export portable data and restore it in a clean environment
- operate remotely only with authenticated encrypted transport

## Governance

The repository is authoritative for architecture. Material architecture changes require a new ADR.
