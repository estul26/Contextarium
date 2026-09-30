# M5 — Search

## Scope
- structured filters
- pagination
- SQLite FTS5
- index rebuild

## Non-goals
No vectors, embeddings, semantic ranking, or external search service.

## Data changes
FTS virtual tables and supporting metadata.

## API changes
`GET /api/v1/search`.

## Security
Search results must apply the same authorization and namespace filters as direct reads.

## Tests
Index sync, rebuild, text search, pagination, and permission filtering.

## Acceptance
FTS can be rebuilt from authoritative data and does not leak unauthorized records.

## Rollback
Drop/rebuild the derived index without losing authoritative records.
