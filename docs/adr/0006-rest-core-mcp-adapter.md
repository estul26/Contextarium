# ADR 0006: REST contract with MCP adapter

Status: Accepted

## Context

Human applications, CLI tools, and AI clients need different protocol ergonomics. The domain must not become coupled to one agent protocol.

## Decision

REST under `/api/v1` is the initial public API contract.

MCP under `/mcp` is an adapter over the same application services.

Neither REST nor MCP owns business logic.

## Consequences

- one source of domain behavior
- multiple clients/protocols can coexist
- MCP can evolve without changing storage/domain semantics
- OpenAPI can describe the REST contract independently
