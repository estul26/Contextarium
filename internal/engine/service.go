package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

type Service struct {
	db *sql.DB
	// Internal transaction barriers are configured only by package tests; never by HTTP.
	fault func(string) error
}

func (s *Service) checkpoint(point string) error {
	if s.fault != nil {
		return s.fault(point)
	}
	return nil
}

type MutationResult struct {
	Data     json.RawMessage
	Contract string
}

func (r MutationResult) MarshalJSON() ([]byte, error) { return r.Data, nil }

func New(db *sql.DB) *Service { return &Service{db: db} }

// Every mutation, including replay bookkeeping, uses the storage pool's
// BEGIN IMMEDIATE transaction. No domain mutation is owned by the HTTP adapter.
func (s *Service) mutate(ctx context.Context, scope, key string, body []byte, apply func(*sql.Tx) (any, error)) (json.RawMessage, error) {
	result, err := s.mutateResult(ctx, scope, key, body, apply)
	return result.Data, err
}
func (s *Service) mutateResult(ctx context.Context, scope, key string, body []byte, apply func(*sql.Tx) (any, error)) (MutationResult, error) {
	if len(key) < 1 || len(key) > 255 {
		return MutationResult{}, ErrInvalid
	}
	for _, c := range key {
		if c < 0x21 || c > 0x7e {
			return MutationResult{}, ErrInvalid
		}
	}
	value, err := ParseJSON(body)
	if err != nil {
		return MutationResult{}, err
	}
	if _, ok := value.(map[string]any); !ok {
		return MutationResult{}, ErrInvalid
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return MutationResult{}, ErrInvalid
	}
	sum := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(sum[:])
	if err := s.checkpoint("F0"); err != nil {
		return MutationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MutationResult{}, ErrUnavailable
	}
	defer tx.Rollback()
	var previous, response, contract string
	err = tx.QueryRowContext(ctx, "SELECT request_hash, response, response_contract FROM idempotency_keys WHERE scope=? AND key=?", scope, key).Scan(&previous, &response, &contract)
	if err == nil {
		if previous != fingerprint {
			return MutationResult{}, ErrConflict
		}
		return MutationResult{Data: json.RawMessage(response), Contract: contract}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MutationResult{}, ErrUnavailable
	}
	result, err := apply(tx)
	if err != nil {
		return MutationResult{}, err
	}
	if err := s.checkpoint("F5"); err != nil {
		return MutationResult{}, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return MutationResult{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO idempotency_keys(scope,key,request_hash,response,response_contract) VALUES(?,?,?,?,'m2')", scope, key, fingerprint, string(encoded)); err != nil {
		return MutationResult{}, ErrUnavailable
	}
	if err := s.checkpoint("F6"); err != nil {
		return MutationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MutationResult{}, ErrUnavailable
	}
	if err := s.checkpoint("F8"); err != nil {
		return MutationResult{}, err
	}
	return MutationResult{Data: encoded, Contract: "m2"}, nil
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

const recordColumns = "id,subject_id,namespace,schema_id,schema_version,data,key,sensitivity,provenance,status,created_at,updated_at,revision"

func scanRecord(row scanner) (Record, error) {
	var v Record
	var data, provenance string
	err := row.Scan(&v.ID, &v.SubjectID, &v.Namespace, &v.SchemaID, &v.SchemaVersion, &data, &v.Key, &v.Sensitivity, &provenance, &v.Status, &v.CreatedAt, &v.UpdatedAt, &v.Revision)
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
