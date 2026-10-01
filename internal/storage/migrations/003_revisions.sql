-- M2 adoption is one transaction with the migration ledger. Existing JSON text
-- and timestamps are copied verbatim; no pre-M2 history is inferred.
ALTER TABLE records ADD COLUMN revision INTEGER NOT NULL DEFAULT 1
    CHECK (typeof(revision) = 'integer' AND revision BETWEEN 1 AND 9007199254740991);
ALTER TABLE idempotency_keys ADD COLUMN response_contract TEXT NOT NULL DEFAULT 'm1'
    CHECK (response_contract IN ('m1', 'm2'));
CREATE TABLE record_revisions (
    record_id TEXT NOT NULL,
    revision_number INTEGER NOT NULL CHECK (typeof(revision_number) = 'integer' AND revision_number BETWEEN 1 AND 9007199254740991),
    base_revision INTEGER,
    operation TEXT NOT NULL CHECK (operation IN ('create','update','restore','m1_adoption')),
    source_revision INTEGER,
    recorded_at TEXT NOT NULL,
    actor_id TEXT NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 64 AND substr(actor_id,1,1) GLOB '[a-z]' AND actor_id NOT GLOB '*[^a-z0-9_-]*'),
    attribution_kind TEXT NOT NULL CHECK (attribution_kind = 'development_test'),
    request_id TEXT NOT NULL CHECK (length(request_id)=36 AND substr(request_id,1,4)='req_' AND substr(request_id,5) NOT GLOB '*[^0-9a-f]*'),
    audit_event_id TEXT NOT NULL,
    subject_id TEXT NOT NULL REFERENCES subjects(id),
    namespace TEXT NOT NULL,
    schema_id TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    data TEXT NOT NULL CHECK (json_valid(data) AND json_type(data)='object' AND length(CAST(data AS BLOB)) <= 65536),
    key TEXT CHECK (key IS NULL OR length(CAST(key AS BLOB)) <= 255),
    sensitivity TEXT NOT NULL CHECK (sensitivity IN ('public','private','restricted')),
    provenance TEXT NOT NULL CHECK (json_valid(provenance) AND length(CAST(provenance AS BLOB)) <= 20000),
    status TEXT NOT NULL CHECK (status IN ('active','archived')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(record_id, revision_number),
    UNIQUE(audit_event_id, record_id, revision_number),
    CHECK ((revision_number=1 AND base_revision IS NULL AND operation IN ('create','m1_adoption')) OR
           (revision_number>1 AND typeof(base_revision)='integer' AND base_revision=revision_number-1 AND operation IN ('update','restore'))),
    CHECK ((operation='restore' AND typeof(source_revision)='integer' AND source_revision BETWEEN 1 AND base_revision) OR
           (operation!='restore' AND source_revision IS NULL)),
    FOREIGN KEY(record_id) REFERENCES records(id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(schema_id,schema_version) REFERENCES schemas(schema_id,schema_version),
    FOREIGN KEY(record_id,base_revision) REFERENCES record_revisions(record_id,revision_number),
    FOREIGN KEY(record_id,source_revision) REFERENCES record_revisions(record_id,revision_number),
    FOREIGN KEY(audit_event_id,record_id,revision_number) REFERENCES mutation_audit(event_id,resource_id,revision_number) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE mutation_audit (
    event_id TEXT PRIMARY KEY NOT NULL CHECK (length(event_id)=36 AND substr(event_id,1,4)='evt_' AND substr(event_id,5) NOT GLOB '*[^0-9a-f]*'),
    timestamp TEXT NOT NULL,
    actor_id TEXT NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 64 AND substr(actor_id,1,1) GLOB '[a-z]' AND actor_id NOT GLOB '*[^a-z0-9_-]*'),
    attribution_kind TEXT NOT NULL CHECK (attribution_kind='development_test'),
    action TEXT NOT NULL CHECK (action IN ('record.created','record.updated','record.archived','record.unarchived','revision.restored','record.history_adopted')),
    resource_type TEXT NOT NULL CHECK (resource_type='record'),
    resource_id TEXT NOT NULL,
    subject_id TEXT NOT NULL REFERENCES subjects(id),
    namespace TEXT NOT NULL,
    revision_number INTEGER NOT NULL,
    base_revision INTEGER,
    source_revision INTEGER,
    proposal_id TEXT CHECK (proposal_id IS NULL),
    request_id TEXT NOT NULL CHECK (length(request_id)=36 AND substr(request_id,1,4)='req_' AND substr(request_id,5) NOT GLOB '*[^0-9a-f]*'),
    result TEXT NOT NULL CHECK (result='accepted'),
    reason TEXT CHECK ((action='record.history_adopted' AND reason IS 'm1_history_boundary') OR (action!='record.history_adopted' AND reason IS NULL)),
    UNIQUE(resource_id,revision_number),
    UNIQUE(event_id,resource_id,revision_number),
    FOREIGN KEY(event_id,resource_id,revision_number) REFERENCES record_revisions(audit_event_id,record_id,revision_number) DEFERRABLE INITIALLY DEFERRED
);
-- M2 U1
INSERT INTO record_revisions(record_id,revision_number,base_revision,operation,source_revision,recorded_at,actor_id,attribution_kind,request_id,audit_event_id,subject_id,namespace,schema_id,schema_version,data,key,sensitivity,provenance,status,created_at,updated_at)
SELECT id,1,NULL,'m1_adoption',NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now'),'local-migration-003','development_test','req_'||lower(hex(randomblob(16))),'evt_'||lower(hex(randomblob(16))),subject_id,namespace,schema_id,schema_version,data,key,sensitivity,provenance,status,created_at,updated_at FROM records;
-- M2 U2
INSERT INTO mutation_audit(event_id,timestamp,actor_id,attribution_kind,action,resource_type,resource_id,subject_id,namespace,revision_number,base_revision,source_revision,proposal_id,request_id,result,reason)
SELECT audit_event_id,recorded_at,actor_id,attribution_kind,'record.history_adopted','record',record_id,subject_id,namespace,revision_number,NULL,NULL,NULL,request_id,'accepted','m1_history_boundary' FROM record_revisions;
CREATE TRIGGER record_revisions_no_update BEFORE UPDATE ON record_revisions BEGIN SELECT RAISE(ABORT,'immutable M2 history'); END;
CREATE TRIGGER record_revisions_no_delete BEFORE DELETE ON record_revisions BEGIN SELECT RAISE(ABORT,'immutable M2 history'); END;
CREATE TRIGGER record_revisions_no_replace BEFORE INSERT ON record_revisions WHEN EXISTS(SELECT 1 FROM record_revisions WHERE record_id=NEW.record_id AND revision_number=NEW.revision_number) BEGIN SELECT RAISE(ABORT,'immutable M2 history'); END;
CREATE TRIGGER mutation_audit_no_update BEFORE UPDATE ON mutation_audit BEGIN SELECT RAISE(ABORT,'immutable M2 history'); END;
CREATE TRIGGER mutation_audit_no_delete BEFORE DELETE ON mutation_audit BEGIN SELECT RAISE(ABORT,'immutable M2 history'); END;
CREATE TRIGGER mutation_audit_no_replace BEFORE INSERT ON mutation_audit WHEN EXISTS(SELECT 1 FROM mutation_audit WHERE event_id=NEW.event_id OR (resource_id=NEW.resource_id AND revision_number=NEW.revision_number)) BEGIN SELECT RAISE(ABORT,'immutable M2 history'); END;
CREATE TRIGGER records_no_delete BEFORE DELETE ON records BEGIN SELECT RAISE(ABORT,'records are retained'); END;
CREATE TRIGGER records_no_replace BEFORE INSERT ON records WHEN EXISTS(SELECT 1 FROM records WHERE id=NEW.id) BEGIN SELECT RAISE(ABORT,'records cannot be replaced'); END;
CREATE TRIGGER revisions_next BEFORE INSERT ON record_revisions
WHEN (NOT EXISTS(SELECT 1 FROM records WHERE id=NEW.record_id) AND (NEW.revision_number!=1 OR NEW.operation!='create'))
 OR EXISTS(SELECT 1 FROM records r WHERE r.id=NEW.record_id AND (NEW.revision_number!=r.revision+1 OR NEW.subject_id!=r.subject_id OR NEW.namespace!=r.namespace OR NEW.schema_id!=r.schema_id OR NEW.schema_version!=r.schema_version OR NEW.created_at!=r.created_at))
BEGIN SELECT RAISE(ABORT,'revision identity or sequence mismatch'); END;
CREATE TRIGGER records_head_insert BEFORE INSERT ON records
WHEN NEW.revision!=1 OR NOT EXISTS(SELECT 1 FROM record_revisions v WHERE v.record_id=NEW.id AND v.revision_number=NEW.revision AND v.subject_id IS NEW.subject_id AND v.namespace IS NEW.namespace AND v.schema_id IS NEW.schema_id AND v.schema_version IS NEW.schema_version AND v.data IS NEW.data AND v.key IS NEW.key AND v.sensitivity IS NEW.sensitivity AND v.provenance IS NEW.provenance AND v.status IS NEW.status AND v.created_at IS NEW.created_at AND v.updated_at IS NEW.updated_at)
BEGIN SELECT RAISE(ABORT,'current revision mismatch'); END;
CREATE TRIGGER records_head_update BEFORE UPDATE ON records
WHEN NEW.revision!=OLD.revision+1 OR NOT EXISTS(SELECT 1 FROM record_revisions v WHERE v.record_id=NEW.id AND v.revision_number=NEW.revision AND v.subject_id IS NEW.subject_id AND v.namespace IS NEW.namespace AND v.schema_id IS NEW.schema_id AND v.schema_version IS NEW.schema_version AND v.data IS NEW.data AND v.key IS NEW.key AND v.sensitivity IS NEW.sensitivity AND v.provenance IS NEW.provenance AND v.status IS NEW.status AND v.created_at IS NEW.created_at AND v.updated_at IS NEW.updated_at)
BEGIN SELECT RAISE(ABORT,'current revision mismatch'); END;
CREATE TRIGGER audit_matches_revision BEFORE INSERT ON mutation_audit
WHEN NOT EXISTS(SELECT 1 FROM record_revisions v WHERE v.audit_event_id=NEW.event_id AND v.record_id=NEW.resource_id AND v.revision_number=NEW.revision_number
 AND v.recorded_at IS NEW.timestamp AND v.actor_id IS NEW.actor_id AND v.attribution_kind IS NEW.attribution_kind AND v.request_id IS NEW.request_id
 AND v.subject_id IS NEW.subject_id AND v.namespace IS NEW.namespace AND v.base_revision IS NEW.base_revision AND v.source_revision IS NEW.source_revision
 AND CASE v.operation WHEN 'create' THEN NEW.action='record.created' WHEN 'm1_adoption' THEN NEW.action='record.history_adopted' WHEN 'restore' THEN NEW.action='revision.restored'
 ELSE NEW.action=CASE WHEN v.status='archived' AND (SELECT status FROM record_revisions WHERE record_id=v.record_id AND revision_number=v.base_revision)='active' THEN 'record.archived'
 WHEN v.status='active' AND (SELECT status FROM record_revisions WHERE record_id=v.record_id AND revision_number=v.base_revision)='archived' THEN 'record.unarchived' ELSE 'record.updated' END END)
BEGIN SELECT RAISE(ABORT,'audit revision mismatch'); END;
