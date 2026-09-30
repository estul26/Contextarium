package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/estul26/Contextarium/internal/storage"
)

var ctx = context.Background()

const skillSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"name":{"type":"string","minLength":1},"level":{"type":"integer","minimum":0,"maximum":5}},"required":["name","level"],"additionalProperties":false}`
const taskSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"title":{"type":"string"},"done":{"type":"boolean"}},"required":["title","done"],"additionalProperties":false}`

func fixture(t *testing.T) (*Service, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	db, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db), db, path
}
func decode[T any](t *testing.T, raw []byte, err error) T {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func subject(t *testing.T, s *Service) Subject {
	t.Helper()
	raw, err := s.CreateSubject(ctx, "subject", []byte(`{"kind":"person","display_name":"Example User"}`))
	return decode[Subject](t, raw, err)
}
func publish(t *testing.T, s *Service, id string, version int, definition string) Schema {
	t.Helper()
	raw, err := s.PublishSchema(ctx, fmt.Sprintf("%s-%d", id, version), []byte(fmt.Sprintf(`{"schema_id":%q,"schema_version":%d,"definition":%s,"migration_policy":"explicit"}`, id, version, definition)))
	return decode[Schema](t, raw, err)
}
func recordBody(sub, ns, schema, data string) []byte {
	return []byte(fmt.Sprintf(`{"subject_id":%q,"namespace":%q,"schema_id":%q,"schema_version":1,"data":%s,"key":"same-key"}`, sub, ns, schema, data))
}
func create(t *testing.T, s *Service, key string, body []byte) Record {
	t.Helper()
	raw, err := s.CreateRecord(ctx, key, body)
	return decode[Record](t, raw, err)
}
func wantError(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error = %v, want %v", got, want)
	}
}

func TestTwoIndependentDomainsAndPersistence(t *testing.T) {
	s, db, path := fixture(t)
	sub := subject(t, s)
	publish(t, s, "example.skill", 1, skillSchema)
	publish(t, s, "example.task", 1, taskSchema)
	skill := create(t, s, "skill", recordBody(sub.ID, "profile.skills", "example.skill", `{"name":"Example Skill","level":3}`))
	task := create(t, s, "task", recordBody(sub.ID, "tasks.personal", "example.task", `{"title":"Synthetic task","done":false}`))
	duplicateKey := create(t, s, "second-skill", recordBody(sub.ID, "profile.skills", "example.skill", `{"name":"Another Skill","level":4}`))
	if skill.ID == duplicateKey.ID || *skill.Key != *duplicateKey.Key || !validID(skill.ID, "rec") || !validID(sub.ID, "sub") {
		t.Fatal("identity/key semantics failed")
	}
	if task.Sensitivity != "private" || task.Status != "active" {
		t.Fatal("defaults failed")
	}
	raw, err := s.UpdateRecord(ctx, skill.ID, "update", []byte(`{"data":{"name":"Example Skill","level":5},"status":"archived","provenance":{"source":"https://example.com","note":"Synthetic source"}}`))
	updated := decode[Record](t, raw, err)
	if updated.ID != skill.ID || updated.SubjectID != sub.ID || updated.Namespace != skill.Namespace || updated.CreatedAt != skill.CreatedAt || updated.Status != "archived" || updated.Provenance.Source != "https://example.com" {
		t.Fatal("patch invariant failed")
	}
	raw, err = s.UpdateRecord(ctx, skill.ID, "replace-provenance", []byte(`{"provenance":{"source":"synthetic note"}}`))
	replaced := decode[Record](t, raw, err)
	if replaced.Provenance.Note != "" {
		t.Fatal("provenance was merged instead of replaced")
	}
	raw, err = s.UpdateRecord(ctx, skill.ID, "unarchive", []byte(`{"status":"active","key":null,"provenance":null}`))
	restored := decode[Record](t, raw, err)
	if restored.Key != nil || restored.Provenance != nil || restored.Status != "active" || string(restored.Data) != string(updated.Data) {
		t.Fatal("unarchive/null semantics failed")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	s = New(reopened)
	got, err := s.GetRecord(ctx, skill.ID)
	if err != nil || got.Status != "active" || string(got.Data) != string(restored.Data) {
		t.Fatalf("persistence: %v %v", got, err)
	}
	replay, err := s.UpdateRecord(ctx, skill.ID, "update", []byte(`{"data":{"name":"Example Skill","level":5},"status":"archived","provenance":{"source":"https://example.com","note":"Synthetic source"}}`))
	historical := decode[Record](t, replay, err)
	if historical.Status != "archived" {
		t.Fatal("retry did not return original response")
	}
	got, _ = s.GetRecord(ctx, skill.ID)
	if got.Status != "active" {
		t.Fatal("retry repeated mutation")
	}
}

func TestSchemaPinningAndImmutableRegistry(t *testing.T) {
	s, db, _ := fixture(t)
	sub := subject(t, s)
	publish(t, s, "example.skill", 1, skillSchema)
	r := create(t, s, "create", recordBody(sub.ID, "profile.skills", "example.skill", `{"name":"Example","level":2}`))
	publish(t, s, "example.skill", 2, strings.Replace(skillSchema, `"maximum":5`, `"maximum":1`, 1))
	if _, err := s.UpdateRecord(ctx, r.ID, "v1-still-valid", []byte(`{"data":{"name":"Example","level":5}}`)); err != nil {
		t.Fatal("new schema changed v1 validation", err)
	}
	b := strings.Replace(string(recordBody(sub.ID, "profile.skills", "example.skill", `{"name":"Example","level":5}`)), `"schema_version":1`, `"schema_version":2`, 1)
	_, err := s.CreateRecord(ctx, "v2-invalid", []byte(b))
	wantError(t, err, ErrValidation)
	b = strings.Replace(b, `"schema_version":2`, `"schema_version":3`, 1)
	_, err = s.CreateRecord(ctx, "unknown-version", []byte(b))
	wantError(t, err, ErrNotFound)
	_, err = s.PublishSchema(ctx, "duplicate-pair", []byte(fmt.Sprintf(`{"schema_id":"example.skill","schema_version":1,"definition":%s,"migration_policy":"explicit"}`, taskSchema)))
	wantError(t, err, ErrConflict)
	for _, q := range []string{`UPDATE schemas SET definition='{}'`, `DELETE FROM schemas`, `UPDATE records SET namespace='other'`, `UPDATE records SET schema_version=2`, `DELETE FROM subjects`} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("storage invariant bypassed: %s", q)
		}
	}
}

func TestInvalidMutationsLeaveStateAndRetryKeyUntouched(t *testing.T) {
	s, db, _ := fixture(t)
	sub := subject(t, s)
	publish(t, s, "example.skill", 1, skillSchema)
	r := create(t, s, "create", recordBody(sub.ID, "profile.skills", "example.skill", `{"name":"Example","level":2}`))
	before, _ := s.GetRecord(ctx, r.ID)
	for i, b := range []string{`{}`, `{"id":"rec_00000000000000000000000000000000"}`, `{"subject_id":"x"}`, `{"namespace":"elsewhere"}`, `{"schema_version":2}`, `{"revision":1}`, `{"created_at":"x"}`, `{"status":"deleted"}`, `{"data":null}`, `{"sensitivity":null}`, `{"key":5}`, `{"Status":"archived"}`, `{"provenance":{"source":"x","trusted":true}}`, `{"data":{"name":"Example","level":8}}`, `{"data":{"name":"Example","level":"3"}}`, `{"data":{"name":"Example"}}`} {
		_, err := s.UpdateRecord(ctx, r.ID, fmt.Sprintf("bad-%d", i), []byte(b))
		if err == nil {
			t.Fatalf("accepted %s", b)
		}
	}
	after, _ := s.GetRecord(ctx, r.ID)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("rejected update changed record")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM idempotency_keys WHERE scope=?", "PATCH records/"+r.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed writes consumed keys", count, err)
	}
	_, err := s.UpdateRecord(ctx, r.ID, "recoverable", []byte(`{"data":{}}`))
	wantError(t, err, ErrValidation)
	if _, err := s.UpdateRecord(ctx, r.ID, "recoverable", []byte(`{"status":"archived"}`)); err != nil {
		t.Fatal(err)
	}
	for i, b := range []string{`{"id":"sub_custom","kind":"person","display_name":"Example"}`, `{"kind":null,"display_name":"Example"}`, `{"kind":"Person","display_name":"Example"}`} {
		_, err := s.CreateSubject(ctx, fmt.Sprint(i), []byte(b))
		wantError(t, err, ErrInvalid)
	}
}

func TestRetryAtomicityAndConcurrentRequests(t *testing.T) {
	s, db, path := fixture(t)
	otherDB, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	other := New(otherDB)
	body := []byte(`{"kind":"project","display_name":"Example Project"}`)
	var wg sync.WaitGroup
	results := make(chan string, 12)
	failures := make(chan error, 12)
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			selected := s
			if i%2 == 1 {
				selected = other
			}
			raw, err := selected.CreateSubject(ctx, "concurrent", body)
			if err != nil {
				failures <- err
			}
			results <- string(raw)
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	first := ""
	for got := range results {
		if first == "" {
			first = got
		}
		if got != first {
			t.Fatal("retry produced distinct responses")
		}
	}
	_, err = s.CreateSubject(ctx, "concurrent", []byte(`{"display_name":"Different","kind":"project"}`))
	wantError(t, err, ErrConflict)
	raw, err := s.CreateSubject(ctx, "concurrent", []byte(`{ "display_name":"Example Project", "kind":"project" }`))
	if err != nil || string(raw) != first {
		t.Fatal("semantic replay failed", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_replay BEFORE INSERT ON idempotency_keys BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateSubject(ctx, "rollback", body)
	wantError(t, err, ErrUnavailable)
	var count int
	_ = db.QueryRow("SELECT count(*) FROM subjects").Scan(&count)
	if count != 1 {
		t.Fatal("mutation survived replay-write failure")
	}
	if _, err := db.Exec(`DROP TRIGGER fail_replay`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSubject(ctx, "rollback", body); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = s.CreateSubject(cancelled, "cancel", body)
	wantError(t, err, ErrUnavailable)
	db.Close()
	_, err = s.GetSubject(ctx, "sub_00000000000000000000000000000000")
	wantError(t, err, ErrUnavailable)
}

func TestBoundedOrderedPaginationAndExactFilters(t *testing.T) {
	s, _, _ := fixture(t)
	sub := subject(t, s)
	publish(t, s, "example.task", 1, taskSchema)
	for i := range 5 {
		ns := "tasks.personal"
		if i == 4 {
			ns += ".child"
		}
		create(t, s, fmt.Sprint(i), recordBody(sub.ID, ns, "example.task", `{"title":"Example","done":false}`))
	}
	opts := ListOptions{Limit: 2, Namespace: "tasks.personal", SubjectID: sub.ID}
	ids := []string{}
	for {
		page, err := s.ListRecords(ctx, opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) > 2 {
			t.Fatal("unbounded page")
		}
		for _, r := range page.Items {
			ids = append(ids, r.ID)
		}
		if page.NextCursor == "" {
			break
		}
		opts.Cursor = page.NextCursor
	}
	if len(ids) != 4 {
		t.Fatalf("exact namespace/paging yielded %d", len(ids))
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Fatal("duplicate or unordered rows")
		}
	}
	page, err := s.ListRecords(ctx, ListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ListRecords(ctx, ListOptions{Limit: 1, Cursor: page.NextCursor, Status: "active"})
	wantError(t, err, ErrInvalid)
	_, err = s.ListSubjects(ctx, ListOptions{Cursor: page.NextCursor})
	wantError(t, err, ErrInvalid)
	for _, o := range []ListOptions{{Limit: 101}, {Limit: -1}, {Cursor: "garbage"}, {Namespace: "tasks.*"}, {SubjectID: "wrong"}, {Status: "deleted"}} {
		_, err := s.ListRecords(ctx, o)
		wantError(t, err, ErrInvalid)
	}
	missing, err := s.ListRecords(ctx, ListOptions{Namespace: "unknown"})
	if err != nil || len(missing.Items) != 0 || missing.NextCursor != "" {
		t.Fatal("empty list invalid")
	}
}

func TestConcurrentPatchesPreserveOmittedFields(t *testing.T) {
	s, _, path := fixture(t)
	sub := subject(t, s)
	publish(t, s, "example.task", 1, taskSchema)
	r := create(t, s, "create", recordBody(sub.ID, "tasks.personal", "example.task", `{"title":"Example","done":false}`))
	db, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other := New(db)
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		_, err := s.UpdateRecord(ctx, r.ID, "status", []byte(`{"status":"archived"}`))
		errs <- err
	}()
	go func() {
		<-start
		_, err := other.UpdateRecord(ctx, r.ID, "sensitivity", []byte(`{"sensitivity":"restricted"}`))
		errs <- err
	}()
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetRecord(ctx, r.ID)
	if err != nil || got.Status != "archived" || got.Sensitivity != "restricted" {
		t.Fatal("lost omitted field", got, err)
	}
}

func TestDataNumbersAndUnknownFieldsSurviveStorage(t *testing.T) {
	s, _, _ := fixture(t)
	sub := subject(t, s)
	publish(t, s, "example.exact", 1, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"n":{"type":"integer","const":9007199254740993}}}`)
	data := `{"n":9007199254740993,"decimal":0.30,"exponent":1e2,"extra":{"note":"synthetic"}}`
	r := create(t, s, "exact", recordBody(sub.ID, "example.exact", "example.exact", data))
	got, err := s.GetRecord(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != data {
		t.Fatalf("data transformed: %s", got.Data)
	}
	raw, err := s.UpdateRecord(ctx, r.ID, "metadata-only", []byte(`{"key":null}`))
	updated := decode[Record](t, raw, err)
	if string(updated.Data) != data {
		t.Fatalf("metadata update transformed data: %s", updated.Data)
	}
}

func TestMutationContentionReturnsUnavailableAndRecovers(t *testing.T) {
	s, _, path := fixture(t)
	db, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = s.CreateSubject(ctx, "busy", []byte(`{"kind":"project","display_name":"Synthetic"}`))
	wantError(t, err, ErrUnavailable)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSubject(ctx, "busy", []byte(`{"kind":"project","display_name":"Synthetic"}`)); err != nil {
		t.Fatal(err)
	}
}
