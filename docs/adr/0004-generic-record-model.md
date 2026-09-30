# ADR 0004: Generic record model

Status: Accepted

## Context

Contextarium must not be limited to memory. Profile, memory, knowledge, projects, tasks, documents, events, preferences, and future domains should share one infrastructure layer.

## Decision

The core models generic subjects, namespaces, schemas, records, revisions, proposals, sources, clients, permissions, and audit events.

Domain concepts are expressed through schema packs and namespaces rather than dedicated core services.

## Consequences

- new domains can be added without changing the core engine
- authorization can reuse namespace boundaries
- schema governance becomes important
- domain-specific UX belongs in clients/schema packs
