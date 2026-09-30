# M8 — MCP Adapter

## Scope
Expose safe MCP capabilities over existing application services.

Initial capabilities:
- search
- read
- propose

## Non-goals
No separate MCP business logic and no unrestricted destructive admin tools.

## Data changes
None required by the protocol itself.

## API changes
Add `/mcp` transport/adapter.

## Security
MCP uses the same client identity, scope, namespace, revision, and audit rules.

## Tests
REST/MCP semantic equivalence for shared operations and permission enforcement.

## Acceptance
MCP cannot bypass application-service authorization or revision rules.

## Rollback
MCP can be disabled without affecting REST or stored data.
