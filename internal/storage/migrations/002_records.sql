CREATE TABLE subjects (
    id TEXT PRIMARY KEY NOT NULL,
    kind TEXT NOT NULL,
    display_name TEXT NOT NULL CHECK (length(CAST(display_name AS BLOB)) BETWEEN 1 AND 255),
    created_at TEXT NOT NULL
);
CREATE TABLE schemas (
    schema_id TEXT NOT NULL,
    schema_version INTEGER NOT NULL CHECK (schema_version BETWEEN 1 AND 2147483647),
    definition TEXT NOT NULL CHECK (json_valid(definition) AND length(CAST(definition AS BLOB)) <= 32768),
    migration_policy TEXT NOT NULL CHECK (migration_policy = 'explicit'),
    created_at TEXT NOT NULL,
    PRIMARY KEY (schema_id, schema_version)
);
CREATE TRIGGER schemas_no_update BEFORE UPDATE ON schemas BEGIN
    SELECT RAISE(ABORT, 'schema versions are immutable');
END;
CREATE TRIGGER schemas_no_delete BEFORE DELETE ON schemas BEGIN
    SELECT RAISE(ABORT, 'schema versions are retained');
END;
CREATE TABLE records (
    id TEXT PRIMARY KEY NOT NULL,
    subject_id TEXT NOT NULL REFERENCES subjects(id),
    namespace TEXT NOT NULL,
    schema_id TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    data TEXT NOT NULL CHECK (json_valid(data) AND json_type(data) = 'object' AND length(CAST(data AS BLOB)) <= 65536),
    key TEXT CHECK (key IS NULL OR length(CAST(key AS BLOB)) <= 255),
    sensitivity TEXT NOT NULL CHECK (sensitivity IN ('public', 'private', 'restricted')),
    provenance TEXT NOT NULL CHECK (json_valid(provenance) AND length(CAST(provenance AS BLOB)) <= 20000),
    status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (schema_id, schema_version) REFERENCES schemas(schema_id, schema_version)
);
CREATE TRIGGER records_identity_immutable
BEFORE UPDATE OF id, subject_id, namespace, schema_id, schema_version, created_at ON records
WHEN NEW.id != OLD.id OR NEW.subject_id != OLD.subject_id OR NEW.namespace != OLD.namespace
  OR NEW.schema_id != OLD.schema_id OR NEW.schema_version != OLD.schema_version OR NEW.created_at != OLD.created_at
BEGIN
    SELECT RAISE(ABORT, 'record identity and schema pin are immutable in M1');
END;
CREATE INDEX records_subject_id ON records(subject_id, id);
CREATE INDEX records_namespace_id ON records(namespace, id);
CREATE INDEX records_status_id ON records(status, id);
CREATE TABLE idempotency_keys (
    scope TEXT NOT NULL,
    key TEXT NOT NULL CHECK (length(key) BETWEEN 1 AND 255),
    request_hash TEXT NOT NULL,
    response TEXT NOT NULL CHECK (json_valid(response)),
    PRIMARY KEY (scope, key)
);
