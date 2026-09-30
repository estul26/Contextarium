# M9 — Import, Export & Backup

## Scope
- JSON export
- JSONL export
- SQLite-safe backup workflow
- import into a clean instance
- documented encrypted-backup workflow

## Non-goals
No mandatory GitHub/cloud backup integration. Markdown is not authoritative.

## Data changes
Only import/export metadata if necessary.

## API changes
Administrative import/export endpoints or commands.

## Security
Explicit scopes, bounded imports, validation, encrypted-backup guidance.

## Tests
Round-trip export/import, corrupted input, version mismatch, clean-environment restore rehearsal.

## Acceptance
A fresh Contextarium instance can restore authoritative records and revision history from a supported portable backup/export.

## Rollback
Failed imports must not leave partially authoritative state.
