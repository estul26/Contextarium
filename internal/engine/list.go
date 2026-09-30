package engine

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

type ListOptions struct {
	Limit     int
	Cursor    string
	SubjectID string
	Namespace string
	Status    string
}
type Page[T any] struct {
	Items      []T
	NextCursor string
}
type cursor struct {
	Scope string `json:"scope"`
	After string `json:"after"`
}

func listScope(resource string, o ListOptions) string {
	b, _ := json.Marshal([]string{resource, o.SubjectID, o.Namespace, o.Status})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func listBounds(resource string, o ListOptions) (int, string, error) {
	limit := o.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return 0, "", ErrInvalid
	}
	if resource == "subjects" && (o.SubjectID != "" || o.Namespace != "" || o.Status != "") {
		return 0, "", ErrInvalid
	}
	if (o.SubjectID != "" && !validID(o.SubjectID, "sub")) || (o.Namespace != "" && !ValidNamespace(o.Namespace)) || (o.Status != "" && !validStatus(o.Status)) {
		return 0, "", ErrInvalid
	}
	if o.Cursor == "" {
		return limit, "", nil
	}
	if len(o.Cursor) > 512 {
		return 0, "", ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(o.Cursor)
	if err != nil {
		return 0, "", ErrInvalid
	}
	f, err := object(raw, "scope", "after")
	if err != nil || !required(f, "scope", "after") {
		return 0, "", ErrInvalid
	}
	var c cursor
	if json.Unmarshal(raw, &c) != nil || c.Scope != listScope(resource, o) {
		return 0, "", ErrInvalid
	}
	prefix := "rec"
	if resource == "subjects" {
		prefix = "sub"
	}
	if !validID(c.After, prefix) {
		return 0, "", ErrInvalid
	}
	return limit, c.After, nil
}
func nextCursor(resource string, o ListOptions, id string) string {
	b, _ := json.Marshal(cursor{Scope: listScope(resource, o), After: id})
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Service) ListSubjects(ctx context.Context, o ListOptions) (Page[Subject], error) {
	page := Page[Subject]{Items: []Subject{}}
	limit, after, err := listBounds("subjects", o)
	if err != nil {
		return page, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,kind,display_name,created_at FROM subjects WHERE id>? ORDER BY id LIMIT ?", after, limit+1)
	if err != nil {
		return page, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var v Subject
		if err := rows.Scan(&v.ID, &v.Kind, &v.DisplayName, &v.CreatedAt); err != nil {
			return page, ErrUnavailable
		}
		page.Items = append(page.Items, v)
	}
	if rows.Err() != nil {
		return page, ErrUnavailable
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = nextCursor("subjects", o, page.Items[limit-1].ID)
	}
	return page, nil
}
func (s *Service) ListRecords(ctx context.Context, o ListOptions) (Page[Record], error) {
	page := Page[Record]{Items: []Record{}}
	limit, after, err := listBounds("records", o)
	if err != nil {
		return page, err
	}
	query := "SELECT " + recordColumns + " FROM records WHERE id>?"
	args := []any{after}
	if o.SubjectID != "" {
		query += " AND subject_id=?"
		args = append(args, o.SubjectID)
	}
	if o.Namespace != "" {
		query += " AND namespace=?"
		args = append(args, o.Namespace)
	}
	if o.Status != "" {
		query += " AND status=?"
		args = append(args, o.Status)
	}
	query += " ORDER BY id LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return page, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanRecord(rows)
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
		page.NextCursor = nextCursor("records", o, page.Items[limit-1].ID)
	}
	return page, nil
}
