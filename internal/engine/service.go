package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }

// Every mutation, including replay bookkeeping, uses the storage pool's
// BEGIN IMMEDIATE transaction. No domain mutation is owned by the HTTP adapter.
func (s *Service) mutate(ctx context.Context, scope, key string, body []byte, apply func(*sql.Tx) (any, error)) (json.RawMessage, error) {
	if len(key) < 1 || len(key) > 255 {
		return nil, ErrInvalid
	}
	for _, c := range key {
		if c < 0x21 || c > 0x7e {
			return nil, ErrInvalid
		}
	}
	value, err := ParseJSON(body)
	if err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, ErrInvalid
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalid
	}
	sum := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(sum[:])
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer tx.Rollback()
	var previous, response string
	err = tx.QueryRowContext(ctx, "SELECT request_hash, response FROM idempotency_keys WHERE scope=? AND key=?", scope, key).Scan(&previous, &response)
	if err == nil {
		if previous != fingerprint {
			return nil, ErrConflict
		}
		return json.RawMessage(response), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnavailable
	}
	result, err := apply(tx)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO idempotency_keys(scope,key,request_hash,response) VALUES(?,?,?,?)", scope, key, fingerprint, string(encoded)); err != nil {
		return nil, ErrUnavailable
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrUnavailable
	}
	return encoded, nil
}

func (s *Service) CreateSubject(ctx context.Context, key string, body []byte) (json.RawMessage, error) {
	return s.mutate(ctx, "POST subjects", key, body, func(tx *sql.Tx) (any, error) {
		f, err := object(body, "kind", "display_name")
		if err != nil || !required(f, "kind", "display_name") {
			return nil, ErrInvalid
		}
		var subject Subject
		if err := json.Unmarshal(body, &subject); err != nil || !segment.MatchString(subject.Kind) || !textOK(subject.DisplayName, 1, 255) {
			return nil, ErrInvalid
		}
		subject.ID, err = NewID("sub")
		if err != nil {
			return nil, err
		}
		subject.CreatedAt = now()
		if _, err := tx.ExecContext(ctx, "INSERT INTO subjects(id,kind,display_name,created_at) VALUES(?,?,?,?)", subject.ID, subject.Kind, subject.DisplayName, subject.CreatedAt); err != nil {
			return nil, ErrUnavailable
		}
		return subject, nil
	})
}
func (s *Service) PublishSchema(ctx context.Context, key string, body []byte) (json.RawMessage, error) {
	return s.mutate(ctx, "POST schemas", key, body, func(tx *sql.Tx) (any, error) {
		f, err := object(body, "schema_id", "schema_version", "definition", "migration_policy")
		if err != nil || !required(f, "schema_id", "schema_version", "definition", "migration_policy") {
			return nil, ErrInvalid
		}
		var schema Schema
		if err := json.Unmarshal(body, &schema); err != nil || !ValidNamespace(schema.ID) || !validVersion(schema.Version) || schema.MigrationPolicy != "explicit" {
			return nil, ErrInvalid
		}
		if _, err := compileSchema(schema.Definition); err != nil {
			return nil, err
		}
		schema.CreatedAt = now()
		result, err := tx.ExecContext(ctx, "INSERT INTO schemas(schema_id,schema_version,definition,migration_policy,created_at) VALUES(?,?,?,?,?) ON CONFLICT(schema_id,schema_version) DO NOTHING", schema.ID, schema.Version, string(schema.Definition), schema.MigrationPolicy, schema.CreatedAt)
		if err != nil {
			return nil, ErrUnavailable
		}
		count, err := result.RowsAffected()
		if err != nil {
			return nil, ErrUnavailable
		}
		if count != 1 {
			return nil, ErrConflict
		}
		return schema, nil
	})
}
func (s *Service) CreateRecord(ctx context.Context, key string, body []byte) (json.RawMessage, error) {
	return s.mutate(ctx, "POST records", key, body, func(tx *sql.Tx) (any, error) {
		f, err := object(body, "subject_id", "namespace", "schema_id", "schema_version", "data", "key", "sensitivity", "provenance")
		if err != nil || !required(f, "subject_id", "namespace", "schema_id", "schema_version", "data") {
			return nil, ErrInvalid
		}
		record := Record{Sensitivity: "private", Status: "active"}
		if _, ok := f["sensitivity"]; ok && !required(f, "sensitivity") {
			return nil, ErrInvalid
		}
		if err := checkProvenance(f["provenance"]); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &record); err != nil || !validID(record.SubjectID, "sub") || !ValidNamespace(record.Namespace) || !ValidNamespace(record.SchemaID) || !validVersion(record.SchemaVersion) || !validMetadata(record) {
			return nil, ErrInvalid
		}
		if _, err := getSubject(ctx, tx, record.SubjectID); err != nil {
			return nil, err
		}
		schema, err := getSchema(ctx, tx, record.SchemaID, record.SchemaVersion)
		if err != nil {
			return nil, err
		}
		if err := validateData(schema.Definition, record.Data); err != nil {
			return nil, err
		}
		record.ID, err = NewID("rec")
		if err != nil {
			return nil, err
		}
		record.CreatedAt = now()
		record.UpdatedAt = record.CreatedAt
		provenance, err := json.Marshal(record.Provenance)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO records(id,subject_id,namespace,schema_id,schema_version,data,key,sensitivity,provenance,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, record.ID, record.SubjectID, record.Namespace, record.SchemaID, record.SchemaVersion, string(record.Data), record.Key, record.Sensitivity, string(provenance), record.Status, record.CreatedAt, record.UpdatedAt)
		if err != nil {
			return nil, ErrUnavailable
		}
		return record, nil
	})
}
func (s *Service) UpdateRecord(ctx context.Context, id, key string, body []byte) (json.RawMessage, error) {
	if !validID(id, "rec") {
		return nil, ErrInvalid
	}
	return s.mutate(ctx, "PATCH records/"+id, key, body, func(tx *sql.Tx) (any, error) {
		f, err := object(body, "data", "key", "sensitivity", "provenance", "status")
		if err != nil || len(f) == 0 {
			return nil, ErrInvalid
		}
		for _, name := range []string{"data", "sensitivity", "status"} {
			if _, ok := f[name]; ok && !required(f, name) {
				return nil, ErrInvalid
			}
		}
		if err := checkProvenance(f["provenance"]); err != nil {
			return nil, err
		}
		record, err := getRecord(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if _, ok := f["provenance"]; ok {
			record.Provenance = nil
		}
		if err := json.Unmarshal(body, &record); err != nil || !validMetadata(record) {
			return nil, ErrInvalid
		}
		schema, err := getSchema(ctx, tx, record.SchemaID, record.SchemaVersion)
		if err != nil {
			return nil, err
		}
		if err := validateData(schema.Definition, record.Data); err != nil {
			return nil, err
		}
		record.UpdatedAt = now()
		provenance, err := json.Marshal(record.Provenance)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, "UPDATE records SET data=?,key=?,sensitivity=?,provenance=?,status=?,updated_at=? WHERE id=?", string(record.Data), record.Key, record.Sensitivity, string(provenance), record.Status, record.UpdatedAt, id)
		if err != nil {
			return nil, ErrUnavailable
		}
		return record, nil
	})
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type scanner interface{ Scan(...any) error }

func dbError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return ErrUnavailable
}
func getSubject(ctx context.Context, q queryer, id string) (Subject, error) {
	var v Subject
	err := q.QueryRowContext(ctx, "SELECT id,kind,display_name,created_at FROM subjects WHERE id=?", id).Scan(&v.ID, &v.Kind, &v.DisplayName, &v.CreatedAt)
	return v, dbError(err)
}
func getSchema(ctx context.Context, q queryer, id string, version int) (Schema, error) {
	var v Schema
	var definition string
	err := q.QueryRowContext(ctx, "SELECT schema_id,schema_version,definition,migration_policy,created_at FROM schemas WHERE schema_id=? AND schema_version=?", id, version).Scan(&v.ID, &v.Version, &definition, &v.MigrationPolicy, &v.CreatedAt)
	v.Definition = json.RawMessage(definition)
	return v, dbError(err)
}

const recordColumns = "id,subject_id,namespace,schema_id,schema_version,data,key,sensitivity,provenance,status,created_at,updated_at"

func scanRecord(row scanner) (Record, error) {
	var v Record
	var data, provenance string
	err := row.Scan(&v.ID, &v.SubjectID, &v.Namespace, &v.SchemaID, &v.SchemaVersion, &data, &v.Key, &v.Sensitivity, &provenance, &v.Status, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, dbError(err)
	}
	v.Data = json.RawMessage(data)
	if err := json.Unmarshal([]byte(provenance), &v.Provenance); err != nil {
		return v, ErrUnavailable
	}
	return v, nil
}
func getRecord(ctx context.Context, q queryer, id string) (Record, error) {
	return scanRecord(q.QueryRowContext(ctx, "SELECT "+recordColumns+" FROM records WHERE id=?", id))
}
func (s *Service) GetSubject(ctx context.Context, id string) (Subject, error) {
	if !validID(id, "sub") {
		return Subject{}, ErrInvalid
	}
	return getSubject(ctx, s.db, id)
}
func (s *Service) GetSchema(ctx context.Context, id string, version int) (Schema, error) {
	if !ValidNamespace(id) || !validVersion(version) {
		return Schema{}, ErrInvalid
	}
	return getSchema(ctx, s.db, id, version)
}
func (s *Service) GetRecord(ctx context.Context, id string) (Record, error) {
	if !validID(id, "rec") {
		return Record{}, ErrInvalid
	}
	return getRecord(ctx, s.db, id)
}
