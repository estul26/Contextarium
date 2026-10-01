package engine

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
)

const MaxRevision int64 = 9007199254740991

var (
	ErrBaseRequired  = errors.New("base revision required")
	ErrRevisionLimit = errors.New("revision limit reached")
	ErrSchemaPair    = errors.New("schema pair conflict")
	ErrInvariant     = errors.New("revision invariant violated")
	actorPattern     = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	requestPattern   = regexp.MustCompile(`^req_[0-9a-f]{32}$`)
	revisionPattern  = regexp.MustCompile(`^[1-9][0-9]*$`)
)

type RevisionConflict struct{ Expected, Current int64 }

func (e *RevisionConflict) Error() string { return "revision conflict" }

type Actor struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type attribution struct {
	Actor     Actor
	RequestID string
}
type attributionKey struct{}

// WithAttribution is for explicit, trusted local adapter/test call context. It
// does not authenticate a person and must never be populated from HTTP headers.
func WithAttribution(ctx context.Context, actor Actor, requestID string) context.Context {
	return context.WithValue(ctx, attributionKey{}, attribution{actor, requestID})
}
func caller(ctx context.Context) (attribution, error) {
	a, ok := ctx.Value(attributionKey{}).(attribution)
	if !ok || a.Actor.Kind != "development_test" || !actorPattern.MatchString(a.Actor.ID) || !requestPattern.MatchString(a.RequestID) {
		return a, ErrInvalid
	}
	return a, nil
}
func ParseRevision(raw string) (int64, error) {
	if !revisionPattern.MatchString(raw) {
		return 0, ErrInvalid
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n > MaxRevision {
		return 0, ErrInvalid
	}
	return n, nil
}
func baseRevision(f map[string]json.RawMessage) (int64, error) {
	raw, ok := f["base_revision"]
	if !ok {
		return 0, ErrBaseRequired
	}
	return ParseRevision(string(raw))
}

type Revision struct {
	RecordID     string   `json:"record_id"`
	Number       int64    `json:"revision_number"`
	Base         *int64   `json:"base_revision"`
	Operation    string   `json:"operation"`
	Source       *int64   `json:"source_revision"`
	RecordedAt   string   `json:"recorded_at"`
	Actor        Actor    `json:"actor"`
	RequestID    string   `json:"request_id"`
	AuditEventID string   `json:"audit_event_id"`
	Snapshot     Snapshot `json:"snapshot"`
}

func (s *Service) CreateRecord(ctx context.Context, key string, body []byte) (MutationResult, error) {
	a, err := caller(ctx)
	if err != nil {
		return MutationResult{}, err
	}
	return s.mutateResult(ctx, "POST records", key, body, func(tx *sql.Tx) (any, error) {
		f, err := object(body, "subject_id", "namespace", "schema_id", "schema_version", "data", "key", "sensitivity", "provenance")
		if err != nil || !required(f, "subject_id", "namespace", "schema_id", "schema_version", "data") {
			return nil, ErrInvalid
		}
		r := Record{Snapshot: Snapshot{Sensitivity: "private", Status: "active"}, Revision: 1}
		if _, ok := f["sensitivity"]; ok && !required(f, "sensitivity") {
			return nil, ErrInvalid
		}
		if err := checkProvenance(f["provenance"]); err != nil {
			return nil, err
		}
		if json.Unmarshal(body, &r) != nil || !validID(r.SubjectID, "sub") || !ValidNamespace(r.Namespace) || !ValidNamespace(r.SchemaID) || !validVersion(r.SchemaVersion) || !validMetadata(r) {
			return nil, ErrInvalid
		}
		if _, err := getSubject(ctx, tx, r.SubjectID); err != nil {
			return nil, err
		}
		schema, err := getSchema(ctx, tx, r.SchemaID, r.SchemaVersion)
		if err != nil {
			return nil, err
		}
		if err := validateData(schema.Definition, r.Data); err != nil {
			return nil, err
		}
		r.ID, err = NewID("rec")
		if err != nil {
			return nil, err
		}
		r.CreatedAt = now()
		r.UpdatedAt = r.CreatedAt
		return s.writeRevision(ctx, tx, r, Record{}, "create", nil, a)
	})
}
func (s *Service) UpdateRecord(ctx context.Context, id, key string, body []byte) (MutationResult, error) {
	a, err := caller(ctx)
	if err != nil {
		return MutationResult{}, err
	}
	if !validID(id, "rec") {
		return MutationResult{}, ErrInvalid
	}
	return s.mutateResult(ctx, "PATCH records/"+id, key, body, func(tx *sql.Tx) (any, error) {
		f, err := object(body, "base_revision", "data", "key", "sensitivity", "provenance", "status")
		if err != nil {
			return nil, err
		}
		base, err := baseRevision(f)
		if err != nil {
			return nil, err
		}
		if len(f) <= 1 {
			return nil, ErrInvalid
		}
		for _, name := range []string{"data", "sensitivity", "status"} {
			if _, ok := f[name]; ok && !required(f, name) {
				return nil, ErrInvalid
			}
		}
		old, err := getRecord(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if old.Revision != base {
			return nil, &RevisionConflict{base, old.Revision}
		}
		if err := checkProvenance(f["provenance"]); err != nil {
			return nil, err
		}
		r := old
		if _, ok := f["provenance"]; ok {
			r.Provenance = nil
		}
		if json.Unmarshal(body, &r) != nil || !validMetadata(r) {
			return nil, ErrInvalid
		}
		schema, err := getSchema(ctx, tx, r.SchemaID, r.SchemaVersion)
		if err != nil {
			return nil, err
		}
		if err := validateData(schema.Definition, r.Data); err != nil {
			return nil, err
		}
		return s.writeRevision(ctx, tx, r, old, "update", nil, a)
	})
}
func (s *Service) RestoreRecord(ctx context.Context, id string, target int64, key string, body []byte) (MutationResult, error) {
	a, err := caller(ctx)
	if err != nil {
		return MutationResult{}, err
	}
	if !validID(id, "rec") || target < 1 || target > MaxRevision {
		return MutationResult{}, ErrInvalid
	}
	// Include the target in the fingerprint, but not the scope. The original body
	// (including a missing base on a rejected request) is not rewritten.
	if _, err := ParseJSON(body); err != nil {
		return MutationResult{}, err
	}
	fingerprint, err := json.Marshal(struct {
		Revision int64           `json:"revision"`
		Body     json.RawMessage `json:"body"`
	}{target, body})
	if err != nil {
		return MutationResult{}, ErrInvalid
	}
	return s.mutateResult(ctx, "POST records/"+id+"/restore", key, fingerprint, func(tx *sql.Tx) (any, error) {
		f, err := object(body, "base_revision")
		if err != nil {
			return nil, err
		}
		base, err := baseRevision(f)
		if err != nil {
			return nil, err
		}
		old, err := getRecord(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if old.Revision != base {
			return nil, &RevisionConflict{base, old.Revision}
		}
		v, err := getRevision(ctx, tx, id, target)
		if err != nil {
			return nil, err
		}
		r := Record{Snapshot: v.Snapshot}
		if r.SchemaID != old.SchemaID || r.SchemaVersion != old.SchemaVersion {
			return nil, ErrSchemaPair
		}
		if r.ID != old.ID || r.SubjectID != old.SubjectID || r.Namespace != old.Namespace || r.CreatedAt != old.CreatedAt {
			return nil, ErrInvariant
		}
		schema, err := getSchema(ctx, tx, r.SchemaID, r.SchemaVersion)
		if err != nil {
			return nil, ErrUnavailable
		}
		if err := validateData(schema.Definition, r.Data); err != nil {
			return nil, err
		}
		return s.writeRevision(ctx, tx, r, old, "restore", &target, a)
	})
}
func (s *Service) writeRevision(ctx context.Context, tx *sql.Tx, r, old Record, operation string, source *int64, a attribution) (Record, error) {
	if old.Revision >= MaxRevision {
		return Record{}, ErrRevisionLimit
	}
	r.Revision = old.Revision + 1
	var base *int64
	if old.Revision != 0 {
		base = &old.Revision
		r.UpdatedAt = now()
	}
	event, err := NewID("evt")
	if err != nil {
		return Record{}, err
	}
	provenance, err := json.Marshal(r.Provenance)
	if err != nil {
		return Record{}, err
	}
	if err := s.checkpoint("F1"); err != nil {
		return Record{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO record_revisions(record_id,revision_number,base_revision,operation,source_revision,recorded_at,actor_id,attribution_kind,request_id,audit_event_id,subject_id,namespace,schema_id,schema_version,data,key,sensitivity,provenance,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.Revision, base, operation, source, r.UpdatedAt, a.Actor.ID, a.Actor.Kind, a.RequestID, event, r.SubjectID, r.Namespace, r.SchemaID, r.SchemaVersion, string(r.Data), r.Key, r.Sensitivity, string(provenance), r.Status, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	if err := s.checkpoint("F2"); err != nil {
		return Record{}, err
	}
	if old.Revision == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO records(`+recordColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.SubjectID, r.Namespace, r.SchemaID, r.SchemaVersion, string(r.Data), r.Key, r.Sensitivity, string(provenance), r.Status, r.CreatedAt, r.UpdatedAt, r.Revision)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE records SET data=?,key=?,sensitivity=?,provenance=?,status=?,updated_at=?,revision=? WHERE id=? AND revision=?`, string(r.Data), r.Key, r.Sensitivity, string(provenance), r.Status, r.UpdatedAt, r.Revision, r.ID, old.Revision)
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if err == nil && n != 1 {
				return Record{}, ErrInvariant
			}
		}
	}
	if err != nil {
		return Record{}, ErrUnavailable
	}
	if err := s.checkpoint("F3"); err != nil {
		return Record{}, err
	}
	action := "record.updated"
	switch {
	case operation == "create":
		action = "record.created"
	case operation == "restore":
		action = "record.revision.restored"
	case old.Status != r.Status && r.Status == "archived":
		action = "record.archived"
	case old.Status != r.Status && r.Status == "active":
		action = "record.unarchived"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mutation_audit(event_id,timestamp,actor_id,attribution_kind,action,resource_type,resource_id,subject_id,namespace,revision_number,base_revision,source_revision,proposal_id,request_id,result,reason) VALUES(?,?,?,?,?,'record',?,?,?,?,?,?,NULL,?,'accepted',NULL)`, event, r.UpdatedAt, a.Actor.ID, a.Actor.Kind, action, r.ID, r.SubjectID, r.Namespace, r.Revision, base, source, a.RequestID)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	if err := s.checkpoint("F4"); err != nil {
		return Record{}, err
	}
	// Indexed, bounded induction check. The triggers check snapshot equality and
	// audit metadata; reciprocal deferred foreign keys check the commit linkage.
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM records r JOIN record_revisions v ON v.record_id=r.id AND v.revision_number=r.revision JOIN mutation_audit a ON a.event_id=v.audit_event_id AND a.resource_id=v.record_id AND a.revision_number=v.revision_number WHERE r.id=? AND r.revision=?)`, r.ID, r.Revision).Scan(&valid)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	if !valid {
		return Record{}, ErrInvariant
	}
	return r, nil
}

const revisionColumns = `record_id,revision_number,base_revision,operation,source_revision,recorded_at,actor_id,attribution_kind,request_id,audit_event_id,subject_id,namespace,schema_id,schema_version,data,key,sensitivity,provenance,status,created_at,updated_at`

func scanRevision(row scanner) (Revision, error) {
	var v Revision
	var data, provenance string
	r := &v.Snapshot
	err := row.Scan(&v.RecordID, &v.Number, &v.Base, &v.Operation, &v.Source, &v.RecordedAt, &v.Actor.ID, &v.Actor.Kind, &v.RequestID, &v.AuditEventID, &r.SubjectID, &r.Namespace, &r.SchemaID, &r.SchemaVersion, &data, &r.Key, &r.Sensitivity, &provenance, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return v, dbError(err)
	}
	r.ID = v.RecordID
	r.Data = json.RawMessage(data)
	if json.Unmarshal([]byte(provenance), &r.Provenance) != nil {
		return v, ErrInvariant
	}
	return v, nil
}
func getRevision(ctx context.Context, q queryer, id string, n int64) (Revision, error) {
	return scanRevision(q.QueryRowContext(ctx, "SELECT "+revisionColumns+" FROM record_revisions WHERE record_id=? AND revision_number=?", id, n))
}
func (s *Service) GetRevision(ctx context.Context, id string, n int64) (Revision, error) {
	if !validID(id, "rec") || n < 1 || n > MaxRevision {
		return Revision{}, ErrInvalid
	}
	return getRevision(ctx, s.db, id, n)
}

type revisionCursor struct {
	Version  int    `json:"version"`
	RecordID string `json:"record_id"`
	After    int64  `json:"after"`
	Through  int64  `json:"through"`
}

func (s *Service) ListRevisions(ctx context.Context, id string, o ListOptions) (Page[Revision], error) {
	page := Page[Revision]{Items: []Revision{}}
	if !validID(id, "rec") || o.SubjectID != "" || o.Namespace != "" || o.Status != "" {
		return page, ErrInvalid
	}
	limit := o.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 || len(o.Cursor) > 512 {
		return page, ErrInvalid
	}
	c := revisionCursor{Version: 1, RecordID: id}
	if o.Cursor != "" {
		raw, err := base64.RawURLEncoding.Strict().DecodeString(o.Cursor)
		if err != nil {
			return page, ErrInvalid
		}
		f, err := object(raw, "version", "record_id", "after", "through")
		if err != nil || !required(f, "version", "record_id", "after", "through") {
			return page, ErrInvalid
		}
		if json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.RecordID != id || c.After < 1 || c.After > c.Through || c.Through > MaxRevision {
			return page, ErrInvalid
		}
	}
	// Explicit deferred read transaction on a dedicated connection: BeginTx would
	// inherit _txlock=immediate and unnecessarily acquire the writer lock.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return page, ErrUnavailable
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		return page, ErrUnavailable
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var head int64
	if err := conn.QueryRowContext(ctx, "SELECT revision FROM records WHERE id=?", id).Scan(&head); err != nil {
		return page, dbError(err)
	}
	if o.Cursor == "" {
		c.Through = head
	} else if c.Through > head {
		return page, ErrInvalid
	}
	rows, err := conn.QueryContext(ctx, "SELECT "+revisionColumns+" FROM record_revisions WHERE record_id=? AND revision_number>? AND revision_number<=? ORDER BY revision_number LIMIT ?", id, c.After, c.Through, limit+1)
	if err != nil {
		return page, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanRevision(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, v)
	}
	if rows.Err() != nil {
		return page, ErrUnavailable
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		c.After = page.Items[limit-1].Number
		raw, _ := json.Marshal(c)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}
