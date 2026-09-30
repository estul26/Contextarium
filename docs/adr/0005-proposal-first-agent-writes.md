# ADR 0005: Proposal-first agent writes

Status: Accepted

## Context

AI assistants and agents may infer data incorrectly or act beyond user intent. Silent authoritative writes reduce trust in the source of truth.

## Decision

Agent clients default to read/search/propose privileges.

A proposal must be validated, authorized, revision-checked, and approved before it changes authoritative state unless an administrator explicitly grants direct write permission for a bounded scope.

## Consequences

- safer agent integration
- clear review trail
- additional workflow latency
- trusted automation can still receive explicitly scoped direct-write permission
