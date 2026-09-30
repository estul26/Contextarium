# Schema System

## Goal

Allow Contextarium to store many context domains without hard-coding each domain into core application logic.

## Registry

Each record and revision references:

- `schema_id`
- `schema_version`

Example:

```text
schema_id: core.fact
schema_version: 1
```

## Requirements

A schema definition must include:

- stable schema ID
- integer version
- validation rules for `data`
- migration policy when a newer version is introduced
- optional indexing/search hints

## Immutability and retention

Released schema versions are immutable. A material change to validation semantics creates a new schema version.

A schema definition must be retained for as long as any retained record revision, pending proposal, or supported portable backup may depend on it.

Publishing version 2 must not change whether data pinned to version 1 validates.

## Domain packs

Schema packs may group definitions under domains such as:

```text
core/
profile/
memory/
knowledge/
projects/
tasks/
events/
documents/
preferences/
```

No domain pack may be required for the generic record engine itself.

## Validation

Every write/proposal that produces structured record state identifies the exact `schema_id` and `schema_version` for that resulting state.

Approval validates against that exact pair, not "the latest" registry version.

Adopting a different schema version is an explicit state change and must not occur implicitly during read, approval, restore, or export.

Unknown or unavailable schema versions fail closed.

## Representation

JSON is the initial record payload representation because it is portable and maps cleanly to REST and SQLite JSON facilities.

Before M1 is frozen, Contextarium must specify the supported schema language/dialect, reference-resolution rules, unknown-field behavior, numeric behavior, validation limits, and whether any defaults/coercions are permitted. Accepted data must not be silently transformed by the validator.

The validator library remains an implementation detail as long as it implements the frozen deterministic semantics.
