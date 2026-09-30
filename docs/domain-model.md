# Domain Model

## Design rule

Contextarium Core is domain-neutral. It does not contain dedicated services for profile, memory, knowledge, projects, tasks, events, documents, or preferences.

Those concepts are expressed through namespaces and schemas.

## Identifiers

Core object IDs are opaque, immutable identifiers issued by Contextarium when the object is created, unless a clean-instance restore explicitly preserves a previously issued ID.

Architectural guarantees:

- callers must not infer time, ordering, shard, or ownership semantics from an ID
- IDs compare exactly as stored
- IDs are unique within their object type
- normal create operations do not accept an existing ID as an implicit upsert
- clean-instance restore may preserve IDs after collision/precondition checks

The concrete generator and alphabet are implementation details finalized in the owning milestone.

A revision is uniquely identified by `(record_id, revision_number)`.

## Subject

A subject is the entity a record describes, such as a person, team, project, organization, agent, or device.

Synthetic example:

```json
{
  "id": "sub_01JEXAMPLE",
  "kind": "person",
  "display_name": "Example User"
}
```

## Namespace

A namespace groups records and is also an authorization boundary.

Examples:

```text
profile.identity
profile.skills
memory.events
knowledge.security
projects.example
tasks.personal
preferences.editor
```

### Canonical namespace grammar

Stored namespaces:

- are lowercase ASCII only
- consist of dot-separated segments
- each segment matches `[a-z][a-z0-9_-]{0,62}`
- contain no wildcard characters
- contain no empty segments
- have at most 8 segments
- have a maximum total length of 255 characters
- are rejected rather than silently normalized if input is non-canonical

Permission patterns are distinct from stored namespaces:

- `projects.example` matches that namespace exactly
- `projects.*` matches one or more descendants such as `projects.example` and `projects.example.notes`
- `projects.*` does **not** match the parent namespace `projects`
- `*` is not a namespace pattern; unrestricted access is represented by an explicit administrative grant
- wildcards anywhere except the final `.*` suffix are invalid

## Record

A record is the current authoritative state of one structured item.

```json
{
  "id": "rec_01JEXAMPLE",
  "subject_id": "sub_01JEXAMPLE",
  "namespace": "profile.skills",
  "schema_id": "core.fact",
  "schema_version": 1,
  "key": "example_skill",
  "data": {
    "name": "Example Skill",
    "level": "intermediate"
  },
  "status": "active",
  "sensitivity": "private",
  "revision": 4,
  "created_at": "2026-01-01T00:00:00Z",
  "updated_at": "2026-01-15T10:30:00Z"
}
```

### v0.1 record invariants

- `subject_id` is immutable after record creation.
- `namespace` is immutable after record creation.
- changing either requires creation of a new record; a future ADR may define a move operation.
- `schema_id` and `schema_version` are pinned by each accepted revision.
- `key` is optional application metadata and is **not unique by default**.
- `key` is not an identity, upsert selector, or authorization boundary unless a schema explicitly defines an additional uniqueness constraint in the future.
- `sensitivity` is descriptive metadata in v0.1, not an authorization mechanism.

## Revision

A revision is an immutable historical representation associated with one record. A record ID remains stable while revision numbers increase monotonically.

## Proposal

A proposal is a requested create, update, archive, or restore operation that has not yet become authoritative.

## Client

A client is an application or agent identity authenticated to Contextarium.

## Source / provenance

Records and proposals may carry provenance describing where information originated.

Server-authenticated actor identity and client-asserted provenance are separate concepts. A client may claim that information came from an external source, but that claim does not make the source trusted, does not grant authority, and does not authorize Contextarium to fetch or execute external content.

## Audit event

An audit event records actor, action, target, time, result, and optional reason/request metadata.

## Domain schema packs

Example schema packs may include profile, memory, knowledge, projects, tasks, events, documents, and preferences.

These packs must remain replaceable without changing the generic record engine.
