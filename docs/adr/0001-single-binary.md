# ADR 0001: Single-binary modular monolith

Status: Accepted

## Context

The initial project targets self-hosting on a small Linux server and needs strong local consistency across records, revisions, proposals, and audit events.

## Decision

Contextarium v0.1 is a modular monolith deployed as one application process.

Logical modules remain separated in code but communicate in-process.

## Consequences

Benefits:
- simple deployment and operations
- straightforward transactions
- low resource requirements
- easier local development

Trade-offs:
- modules cannot scale independently
- process failure affects the whole service

Microservices are deferred.
