# ADR 0003: Use SQLite as the initial authoritative database

Status: Accepted

## Context

v0.1 targets individuals and small teams, relatively low write concurrency, strong transactions, portability, and simple operations.

## Decision

Use SQLite as the authoritative store and SQLite FTS5 for initial full-text indexing.

## Consequences

Benefits:
- one-file database
- ACID transactions
- minimal operations burden
- easy backup/migration
- built-in full-text search

Trade-offs:
- not designed for distributed multi-writer clusters
- concurrency behavior must be tested

Future storage engines may sit behind the storage boundary.
