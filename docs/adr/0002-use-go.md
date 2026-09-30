# ADR 0002: Use Go for the core server

Status: Accepted

## Context

The server should run efficiently on a small VPS, distribute as a simple binary, and expose HTTP/MCP adapters with minimal runtime dependencies.

## Decision

Use Go for the initial core server.

## Consequences

- simple deployment
- strong standard-library HTTP/concurrency support
- predictable resource footprint
- mature SQLite driver options

Client SDKs may use other languages; Go is not part of the wire-format contract.
