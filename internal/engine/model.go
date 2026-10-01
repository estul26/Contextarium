package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var segment = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
var objectID = regexp.MustCompile(`^(sub|rec)_[0-9a-f]{32}$`)

func ValidNamespace(value string) bool {
	if len(value) > 255 {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) > 8 {
		return false
	}
	for _, part := range parts {
		if !segment.MatchString(part) {
			return false
		}
	}
	return true
}
func validID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && objectID.MatchString(value)
}
func NewID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b[:]), nil
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func textOK(s string, min, max int) bool {
	return len(s) >= min && len(s) <= max && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

type Subject struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}
type Schema struct {
	ID              string          `json:"schema_id"`
	Version         int             `json:"schema_version"`
	Definition      json.RawMessage `json:"definition"`
	MigrationPolicy string          `json:"migration_policy"`
	CreatedAt       string          `json:"created_at"`
}
type Provenance struct {
	Source string `json:"source"`
	Note   string `json:"note,omitempty"`
}
type Record struct {
	Snapshot
	Revision int64 `json:"revision"`
}

type Snapshot struct {
	ID            string          `json:"id"`
	SubjectID     string          `json:"subject_id"`
	Namespace     string          `json:"namespace"`
	SchemaID      string          `json:"schema_id"`
	SchemaVersion int             `json:"schema_version"`
	Data          json.RawMessage `json:"data"`
	Key           *string         `json:"key"`
	Sensitivity   string          `json:"sensitivity"`
	Provenance    *Provenance     `json:"provenance"`
	Status        string          `json:"status"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

func validVersion(v int) bool   { return v >= 1 && v <= 2147483647 }
func validStatus(v string) bool { return v == "active" || v == "archived" }
func validMetadata(r Record) bool {
	return (r.Key == nil || textOK(*r.Key, 0, 255)) &&
		(r.Sensitivity == "public" || r.Sensitivity == "private" || r.Sensitivity == "restricted") && validStatus(r.Status) &&
		(r.Provenance == nil || (textOK(r.Provenance.Source, 1, 1024) && textOK(r.Provenance.Note, 0, 2048)))
}
func checkProvenance(raw json.RawMessage) error {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	f, err := object(raw, "source", "note")
	if err != nil || !required(f, "source") {
		return ErrInvalid
	}
	if _, ok := f["note"]; ok && !required(f, "note") {
		return ErrInvalid
	}
	return nil
}
