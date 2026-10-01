package engine

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/estul26/Contextarium/internal/storage"
)

func revisionFixture(t *testing.T) (*Service, *sql.DB, string, Record) {
	t.Helper()
	s, db, path := fixture(t)
	sub := subject(t, s)
	publish(t, s, "example.exact", 1, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
	r := create(t, s, "initial", recordBody(sub.ID, "example.records", "example.exact", `{"n":9007199254740993,"decimal":0.30,"exponent":1e2,"negative":-0,"unknown":{"text":"雪"}}`))
	return s, db, path, r
}
func update(t *testing.T, s *Service, r Record, key, fields string) Record {
	t.Helper()
	raw, err := s.UpdateRecord(ctx, r.ID, key, []byte(fmt.Sprintf(`{"base_revision":%d,%s}`, r.Revision, fields)))
	return decode[Record](t, raw, err)
}

// Observe all four stores in one explicit DEFERRED read transaction. A second
// physical connection sees a consistent committed snapshot without waiting for
// the paused writer's BEGIN IMMEDIATE transaction.
func observe(t *testing.T, db *sql.DB) string {
	t.Helper()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "BEGIN"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var all []any
	for _, table := range []string{"records", "record_revisions", "mutation_audit", "idempotency_keys"} {
		rows, err := conn.QueryContext(context.Background(), "SELECT * FROM "+table+" ORDER BY 1,2")
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		var values [][]any
		for rows.Next() {
			row := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range row {
				ptrs[i] = &row[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			values = append(values, row)
		}
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		rows.Close()
		all = append(all, values)
	}
	raw, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func healthy(t *testing.T, db *sql.DB) {
	t.Helper()
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal(integrity, err)
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() || rows.Err() != nil {
		t.Fatal("foreign key violation", rows.Err())
	}
	rows.Close()
	var bad int
	err = db.QueryRow(`SELECT count(*) FROM records r WHERE r.revision!=(SELECT max(revision_number) FROM record_revisions v WHERE v.record_id=r.id) OR r.revision!=(SELECT count(*) FROM record_revisions v WHERE v.record_id=r.id) OR NOT EXISTS(SELECT 1 FROM record_revisions v JOIN mutation_audit a ON a.event_id=v.audit_event_id AND a.resource_id=v.record_id AND a.revision_number=v.revision_number WHERE v.record_id=r.id AND v.revision_number=r.revision AND v.data IS r.data AND v.key IS r.key AND v.provenance IS r.provenance AND v.status IS r.status AND v.sensitivity IS r.sensitivity AND v.subject_id IS r.subject_id AND v.namespace IS r.namespace AND v.schema_id IS r.schema_id AND v.schema_version IS r.schema_version AND v.created_at IS r.created_at AND v.updated_at IS r.updated_at)`).Scan(&bad)
	if err != nil || bad != 0 {
		t.Fatal("head/snapshot/chain invariant", bad, err)
	}
}
func TestM2SnapshotsRestoreAndReplay(t *testing.T) {
	s, db, path, r := revisionFixture(t)
	initial := r
	v, err := s.GetRevision(ctx, r.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.Snapshot, r.Snapshot) || v.Base != nil || v.Source != nil || v.Operation != "create" || v.RecordedAt != r.CreatedAt || v.Actor.ID != "test-harness" {
		t.Fatal("initial snapshot", v)
	}
	r = update(t, s, r, "all", `"data":{"different":1},"key":null,"provenance":{"source":"synthetic","note":"replace"},"sensitivity":"restricted","status":"archived"`)
	archived := r
	r = update(t, s, r, "unarchive", `"status":"active","provenance":{"source":"replacement"}`)
	if r.Provenance.Note != "" || string(r.Data) != string(archived.Data) {
		t.Fatal("replacement/omitted field semantics")
	}
	restoreBody := []byte(`{"base_revision":3}`)
	a := WithAttribution(context.Background(), Actor{Kind: "development_test", ID: "restorer"}, "req_00000000000000000000000000000002")
	raw, err := s.RestoreRecord(a, r.ID, 1, "restore", restoreBody)
	r = decode[Record](t, raw, err)
	if r.Revision != 4 || r.CreatedAt != initial.CreatedAt || r.UpdatedAt == initial.UpdatedAt || r.Status != initial.Status || r.Key == nil || *r.Key != *initial.Key || r.Provenance != nil || string(r.Data) != string(initial.Data) {
		t.Fatal("restore content", r)
	}
	v, err = s.GetRevision(ctx, r.ID, 4)
	if err != nil || v.Source == nil || *v.Source != 1 || v.Base == nil || *v.Base != 3 || v.Actor.ID != "restorer" || v.RecordedAt != r.UpdatedAt {
		t.Fatal("restore attribution", v, err)
	}
	historical, err := s.GetRevision(ctx, r.ID, 2)
	if err != nil || !reflect.DeepEqual(historical.Snapshot, archived.Snapshot) {
		t.Fatal("history changed")
	}
	// Identical and current-target restores still append; no-op PATCH also appends.
	r = update(t, s, r, "noop", `"status":"active"`)
	again, err := s.RestoreRecord(ctx, r.ID, r.Revision, "current", []byte(`{"base_revision":5}`))
	r = decode[Record](t, again, err)
	if r.Revision != 6 {
		t.Fatal("no-op revisions")
	}
	healthy(t, db)
	before := observe(t, db)
	db.Close()
	db, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = New(db)
	replay, err := s.RestoreRecord(ctx, r.ID, 1, "restore", restoreBody)
	if err != nil || string(replay.Data) != string(raw.Data) || replay.Contract != "m2" {
		t.Fatal("historical replay", err)
	}
	if observe(t, db) != before {
		t.Fatal("replay wrote state")
	}
	// Create and PATCH originals remain replayable even when their old base is stale.
	_, err = s.CreateRecord(ctx, "initial", recordBody(initial.SubjectID, "example.records", "example.exact", string(initial.Data)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateRecord(ctx, r.ID, "all", []byte(`{"base_revision":1,"data":{"different":1},"key":null,"provenance":{"source":"synthetic","note":"replace"},"sensitivity":"restricted","status":"archived"}`))
	if err != nil {
		t.Fatal(err)
	}
	if observe(t, db) != before {
		t.Fatal("replay duplicate effects")
	}
	var actions string
	if err := db.QueryRow("SELECT group_concat(action,',') FROM (SELECT action FROM mutation_audit ORDER BY revision_number)").Scan(&actions); err != nil || actions != "record.created,record.archived,record.unarchived,record.revision.restored,record.updated,record.revision.restored" {
		t.Fatal(actions, err)
	}
}
func TestM2PreconditionsAttributionAndFingerprints(t *testing.T) {
	s, db, _, r := revisionFixture(t)
	before := observe(t, db)
	for _, b := range []string{`{"status":"active"}`, `{"base_revision":null,"status":"active"}`, `{"base_revision":true,"status":"active"}`, `{"base_revision":1,"base_revision":1,"status":"active"}`, `{"Base_revision":1,"status":"active"}`, `{"base_revision":"1","status":"active"}`, `{"base_revision":1.0,"status":"active"}`, `{"base_revision":1e0,"status":"active"}`, `{"base_revision":0,"status":"active"}`, `{"base_revision":-1,"status":"active"}`, `{"base_revision":9007199254740992,"status":"active"}`, `{"base_revision":1}`, `{}`, `{"base_revision":1,"patch":{"status":"active"}}`} {
		_, err := s.UpdateRecord(ctx, r.ID, "bad", []byte(b))
		if err == nil {
			t.Fatal("invalid precondition accepted", b)
		}
	}
	for _, actor := range []Actor{{Kind: "authenticated", ID: "person"}, {Kind: "development_test", ID: "Bad actor"}, {}} {
		_, err := s.UpdateRecord(WithAttribution(context.Background(), actor, "req_00000000000000000000000000000001"), r.ID, "actor", []byte(`{"base_revision":1,"status":"active"}`))
		wantError(t, err, ErrInvalid)
	}
	_, err := s.CreateRecord(context.Background(), "missing-context", recordBody(r.SubjectID, r.Namespace, r.SchemaID, `{}`))
	wantError(t, err, ErrInvalid)
	if observe(t, db) != before {
		t.Fatal("rejection changed stores")
	}
	r = update(t, s, r, "number", `"data":{"n":0.30}`)
	before = observe(t, db)
	replay, err := s.UpdateRecord(ctx, r.ID, "number", []byte(" { \"data\": {\"n\":0.30}, \"base_revision\":1 } "))
	if err != nil || replay.Contract != "m2" {
		t.Fatal(err)
	}
	for _, body := range []string{`{"base_revision":1,"data":{"n":0.3}}`, `{"base_revision":2,"data":{"n":0.30}}`, `{"base_revision":1,"key":null}`} {
		_, err := s.UpdateRecord(ctx, r.ID, "number", []byte(body))
		wantError(t, err, ErrConflict)
	}
	for _, field := range []string{`"data":{}`, `"key":null`, `"sensitivity":"public"`, `"provenance":null`, `"status":"archived"`, `"status":"active"`} {
		_, err := s.UpdateRecord(ctx, r.ID, "stale", []byte(`{"base_revision":1,`+field+`}`))
		var conflict *RevisionConflict
		if !errors.As(err, &conflict) || conflict.Expected != 1 || conflict.Current != 2 {
			t.Fatal(err)
		}
	}
	_, err = s.RestoreRecord(ctx, r.ID, 1, "stale", []byte(`{"base_revision":1}`))
	var conflict *RevisionConflict
	if !errors.As(err, &conflict) {
		t.Fatal(err)
	}
	if observe(t, db) != before {
		t.Fatal("rejections/replay wrote state")
	}
	raw, err := s.RestoreRecord(ctx, r.ID, 1, "target", []byte(`{"base_revision":2}`))
	r = decode[Record](t, raw, err)
	_, err = s.RestoreRecord(ctx, r.ID, 2, "target", []byte(`{"base_revision":2}`))
	wantError(t, err, ErrConflict)
	// A rejected key can be used by a corrected request; different scopes are independent.
	r = update(t, s, r, "stale", `"key":null`)
	r = update(t, s, r, "target", `"status":"archived"`)
	healthy(t, db)
}
func TestM2RevisionPagination(t *testing.T) {
	s, db, _, r := revisionFixture(t)
	for i := 2; i <= 205; i++ {
		r = update(t, s, r, fmt.Sprint(i), `"status":"active"`)
	}
	first, err := s.ListRevisions(ctx, r.ID, ListOptions{})
	if err != nil || len(first.Items) != 20 || first.NextCursor == "" {
		t.Fatal(first, err)
	}
	r = update(t, s, r, "after-page", `"status":"archived"`)
	restored, err := s.RestoreRecord(ctx, r.ID, 1, "restore-between-pages", []byte(`{"base_revision":206}`))
	r = decode[Record](t, restored, err)
	count := len(first.Items)
	cursor := first.NextCursor
	for cursor != "" {
		page, err := s.ListRevisions(ctx, r.ID, ListOptions{Limit: 37, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range page.Items {
			count++
			if v.Number != int64(count) {
				t.Fatal("page order")
			}
		}
		cursor = page.NextCursor
	}
	if count != 205 {
		t.Fatal("moving high water", count)
	}
	for _, limit := range []int{1, 100} {
		seen := int64(0)
		cursor := ""
		for {
			page, err := s.ListRevisions(ctx, r.ID, ListOptions{Limit: limit, Cursor: cursor})
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range page.Items {
				seen++
				if v.Number != seen {
					t.Fatal("pagination duplicate/gap")
				}
			}
			cursor = page.NextCursor
			if cursor == "" {
				break
			}
		}
		if seen != 207 {
			t.Fatal("new traversal missed append", seen)
		}
	}
	beforeRead := observe(t, db)
	if _, err := s.GetRevision(ctx, r.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if observe(t, db) != beforeRead {
		t.Fatal("read changed stores")
	}

	encode := func(c revisionCursor) string {
		raw, _ := json.Marshal(c)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	for _, o := range []ListOptions{{Limit: -1}, {Limit: 101}, {Cursor: "!"}, {Cursor: strings.Repeat("a", 513)}, {Namespace: "example"}, {Cursor: encode(revisionCursor{2, r.ID, 1, 205})}, {Cursor: encode(revisionCursor{1, r.ID, 0, 205})}, {Cursor: encode(revisionCursor{1, r.ID, 20, 999})}, {Cursor: encode(revisionCursor{1, "rec_00000000000000000000000000000000", 1, 205})}} {
		if _, err := s.ListRevisions(ctx, r.ID, o); err == nil {
			t.Fatal("bad cursor accepted", o)
		}
	}
	empty, err := s.ListRevisions(ctx, r.ID, ListOptions{Cursor: encode(revisionCursor{1, r.ID, 205, 205})})
	if err != nil || len(empty.Items) != 0 || empty.NextCursor != "" {
		t.Fatal(empty, err)
	}
	_, err = s.ListRevisions(ctx, "rec_00000000000000000000000000000000", ListOptions{})
	wantError(t, err, ErrNotFound)
	healthy(t, db)
}
func TestM2AtomicFailureBoundaries(t *testing.T) {
	for _, operation := range []string{"create", "data", "metadata", "archive", "unarchive", "restore"} {
		for _, point := range []string{"F0", "F1", "F2", "F3", "F4", "F5", "F6", "F8"} {
			t.Run(operation+"/"+point, func(t *testing.T) {
				s, db, path, r := revisionFixture(t)
				if operation == "unarchive" {
					r = update(t, s, r, "setup-archive", `"status":"archived"`)
				}
				observer, err := storage.Open(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				defer observer.Close()
				before := observe(t, observer)
				s.fault = func(p string) error {
					if p != point {
						return nil
					}
					if p != "F8" && observe(t, observer) != before {
						t.Fatal("partial state visible")
					}
					return ErrUnavailable
				}
				call := func() (MutationResult, error) {
					switch operation {
					case "create":
						return s.CreateRecord(ctx, "failure", recordBody(r.SubjectID, r.Namespace, r.SchemaID, `{}`))
					case "data", "metadata", "archive", "unarchive":
						fields := map[string]string{"data": `"data":{"changed":true}`, "metadata": `"key":null,"provenance":{"source":"synthetic"}`, "archive": `"status":"archived"`, "unarchive": `"status":"active"`}[operation]
						return s.UpdateRecord(ctx, r.ID, "failure", []byte(fmt.Sprintf(`{"base_revision":%d,%s}`, r.Revision, fields)))
					default:
						return s.RestoreRecord(ctx, r.ID, 1, "failure", []byte(`{"base_revision":1}`))
					}
				}
				_, err = call()
				wantError(t, err, ErrUnavailable)
				after := observe(t, observer)
				if (point == "F8") == (before == after) {
					t.Fatal("wrong commit outcome")
				}
				s.fault = nil
				_, err = call()
				if err != nil {
					t.Fatal("same-key retry", err)
				}
				if point == "F8" && observe(t, observer) != after {
					t.Fatal("postcommit retry repeated effects")
				}
				healthy(t, db)
			})
		}
	}
}
func TestM2SQLWriteAndDeferredCommitFailures(t *testing.T) {
	for _, table := range []string{"record_revisions", "records", "mutation_audit", "idempotency_keys", "commit"} {
		for _, operation := range []string{"create", "data", "metadata", "archive", "unarchive", "restore"} {
			t.Run(table+"/"+operation, func(t *testing.T) {
				s, db, _, r := revisionFixture(t)
				if operation == "unarchive" {
					r = update(t, s, r, "setup-archive", `"status":"archived"`)
				}
				before := observe(t, db)
				if table == "commit" {
					_, err := db.Exec(`CREATE TABLE test_deferred(value TEXT REFERENCES subjects(id) DEFERRABLE INITIALLY DEFERRED);CREATE TRIGGER fail_write AFTER INSERT ON idempotency_keys BEGIN INSERT INTO test_deferred VALUES('missing'); END`)
					if err != nil {
						t.Fatal(err)
					}
				} else {
					verb := "INSERT"
					if table == "records" && operation != "create" {
						verb = "UPDATE"
					}
					if _, err := db.Exec("CREATE TRIGGER fail_write BEFORE " + verb + " ON " + table + " BEGIN SELECT RAISE(ABORT,'test failure'); END"); err != nil {
						t.Fatal(err)
					}
				}
				var err error
				switch operation {
				case "create":
					_, err = s.CreateRecord(ctx, "fail", recordBody(r.SubjectID, r.Namespace, r.SchemaID, `{}`))
				case "data", "metadata", "archive", "unarchive":
					fields := map[string]string{"data": `"data":{"changed":true}`, "metadata": `"key":null`, "archive": `"status":"archived"`, "unarchive": `"status":"active"`}[operation]
					_, err = s.UpdateRecord(ctx, r.ID, "fail", []byte(fmt.Sprintf(`{"base_revision":%d,%s}`, r.Revision, fields)))
				case "restore":
					_, err = s.RestoreRecord(ctx, r.ID, 1, "fail", []byte(`{"base_revision":1}`))
				}
				wantError(t, err, ErrUnavailable)
				if observe(t, db) != before {
					t.Fatal("partial write")
				}
				healthy(t, db)
			})
		}
	}
}
func TestM2ImmutableSQLHistory(t *testing.T) {
	s, db, path, r := revisionFixture(t)
	r = update(t, s, r, "next", `"key":null`)
	before := observe(t, db)
	for _, q := range []string{
		"UPDATE record_revisions SET data='{}'", "DELETE FROM record_revisions", "INSERT OR REPLACE INTO record_revisions SELECT * FROM record_revisions", "INSERT INTO record_revisions SELECT * FROM record_revisions WHERE true ON CONFLICT(record_id,revision_number) DO UPDATE SET data='{}'",
		"UPDATE mutation_audit SET actor_id='other'", "DELETE FROM mutation_audit", "INSERT OR REPLACE INTO mutation_audit SELECT * FROM mutation_audit", "INSERT INTO mutation_audit SELECT * FROM mutation_audit WHERE true ON CONFLICT(event_id) DO UPDATE SET actor_id='other'",
		"DELETE FROM records", "INSERT OR REPLACE INTO records SELECT * FROM records", "UPDATE records SET revision=revision+1", "UPDATE records SET revision=1", "UPDATE records SET data='{}'", "UPDATE records SET schema_version=2",
	} {
		if _, err := db.Exec(q); err == nil {
			t.Fatal("guard bypass", q)
		}
	}
	if observe(t, db) != before {
		t.Fatal("guard failure changed state")
	}
	healthy(t, db)
	db.Close()
	db, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if observe(t, db) != before {
		t.Fatal("immutable history changed after reopen")
	}
}
func TestM2Child(t *testing.T) {
	task := os.Getenv("CONTEXTARIUM_TEST_CHILD")
	if task == "" {
		return
	}
	db, err := storage.Open(ctx, os.Getenv("CONTEXTARIUM_TEST_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	if point := os.Getenv("CONTEXTARIUM_TEST_BARRIER"); point != "" {
		s.fault = func(p string) error {
			if p == point {
				fmt.Println("BARRIER")
				var b [1]byte
				os.Stdin.Read(b[:])
			}
			return nil
		}
	}
	id := os.Getenv("CONTEXTARIUM_TEST_RECORD")
	key := os.Getenv("CONTEXTARIUM_TEST_KEY")
	if task == "create" {
		r, readErr := s.GetRecord(ctx, id)
		if readErr != nil {
			t.Fatal(readErr)
		}
		_, err = s.CreateRecord(ctx, key, recordBody(r.SubjectID, r.Namespace, r.SchemaID, string(r.Data)))
	} else if task == "restore" {
		_, err = s.RestoreRecord(ctx, id, 1, key, []byte(`{"base_revision":1}`))
	} else {
		_, err = s.UpdateRecord(ctx, id, key, []byte(`{"base_revision":1,"status":"archived"}`))
	}
	var conflict *RevisionConflict
	switch {
	case err == nil:
		fmt.Println("ACCEPTED")
	case errors.As(err, &conflict):
		fmt.Println("STALE")
	default:
		t.Fatal(err)
	}
}
func childCommand(t *testing.T, path, id, task, key, barrier string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestM2Child$", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), "CONTEXTARIUM_TEST_CHILD="+task, "CONTEXTARIUM_TEST_DB="+path, "CONTEXTARIUM_TEST_RECORD="+id, "CONTEXTARIUM_TEST_KEY="+key, "CONTEXTARIUM_TEST_BARRIER="+barrier)
	return cmd
}
func TestM2IndependentProcessRaces(t *testing.T) {
	for _, variant := range []string{"patch", "restore", "replay"} {
		t.Run(variant, func(t *testing.T) {
			_, db, path, r := revisionFixture(t)
			secondTask := "patch"
			secondKey := "second"
			if variant == "restore" {
				secondTask = "restore"
			}
			if variant == "replay" {
				secondKey = "first"
			}
			a := childCommand(t, path, r.ID, "patch", "first", "")
			b := childCommand(t, path, r.ID, secondTask, secondKey, "")
			results := make(chan string, 2)
			for _, cmd := range []*exec.Cmd{a, b} {
				go func(c *exec.Cmd) {
					out, err := c.CombinedOutput()
					if err != nil {
						results <- "ERROR " + string(out)
					} else {
						results <- string(out)
					}
				}(cmd)
			}
			accepted, stale := 0, 0
			for range 2 {
				out := <-results
				if strings.Contains(out, "ACCEPTED") {
					accepted++
				} else if strings.Contains(out, "STALE") {
					stale++
				} else {
					t.Fatal(out)
				}
			}
			if variant == "replay" {
				if accepted != 2 {
					t.Fatal(accepted, stale)
				}
			} else if accepted != 1 || stale != 1 {
				t.Fatal(accepted, stale)
			}
			healthy(t, db)
			var head int
			if err := db.QueryRow("SELECT revision FROM records").Scan(&head); err != nil || head != 2 {
				t.Fatal(head, err)
			}
		})
	}
}
func TestM2AbruptChildRecovery(t *testing.T) {
	for _, operation := range []string{"create", "patch", "restore"} {
		for _, point := range []string{"F0", "F1", "F2", "F3", "F4", "F5", "F6", "F8"} {
			t.Run(operation+"/"+point, func(t *testing.T) {
				s, db, path, r := revisionFixture(t)
				before := observe(t, db)
				cmd := childCommand(t, path, r.ID, operation, "crash", point)
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				stdin, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				defer stdin.Close()
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if cmd.ProcessState == nil {
						cmd.Process.Kill()
						cmd.Wait()
					}
				}()
				reached := make(chan bool, 1)
				go func() { scanner := bufio.NewScanner(stdout); reached <- scanner.Scan() && scanner.Text() == "BARRIER" }()
				select {
				case ok := <-reached:
					if !ok {
						t.Fatal("child exited before barrier")
					}
				case <-time.After(10 * time.Second):
					t.Fatal("barrier timeout")
				}
				live := observe(t, db)
				if (point == "F8") == (before == live) {
					t.Fatal("inconsistent live snapshot")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				if err := cmd.Wait(); err == nil {
					t.Fatal("child did not terminate abruptly")
				}
				db.Close()
				db, err = storage.Open(ctx, path)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				s = New(db)
				healthy(t, db)
				if observe(t, db) != live {
					t.Fatal("reopen differs from committed snapshot")
				}
				if operation == "create" {
					_, err = s.CreateRecord(ctx, "crash", recordBody(r.SubjectID, r.Namespace, r.SchemaID, string(r.Data)))
				} else if operation == "restore" {
					_, err = s.RestoreRecord(ctx, r.ID, 1, "crash", []byte(`{"base_revision":1}`))
				} else {
					_, err = s.UpdateRecord(ctx, r.ID, "crash", []byte(`{"base_revision":1,"status":"archived"}`))
				}
				if err != nil {
					t.Fatal(err)
				}
				current, err := s.GetRecord(ctx, r.ID)
				want := int64(2)
				if operation == "create" {
					want = 1
					var records int
					db.QueryRow("SELECT count(*) FROM records").Scan(&records)
					if records != 2 {
						t.Fatal("create retry duplicated/lost effect", records)
					}
				}
				if err != nil || current.Revision != want {
					t.Fatal("retry duplicated/lost mutation", current, err)
				}
				healthy(t, db)
			})
		}
	}
}

func TestM2RestoreCorruptionAndLimitFixtures(t *testing.T) {
	// These isolated fixtures deliberately remove guards only to manufacture
	// otherwise unreachable corrupt states. Production never removes a guard.
	for _, kind := range []string{"pair", "identity", "missing-schema", "invalid-data", "limit"} {
		t.Run(kind, func(t *testing.T) {
			s, db, _, r := revisionFixture(t)
			publish(t, s, "example.exact", 2, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["v2"]}`)
			if _, err := s.RestoreRecord(ctx, r.ID, 1, "pinned", []byte(`{"base_revision":1}`)); err != nil {
				t.Fatal("restore used newest schema", err)
			}
			if _, err := s.RestoreRecord(ctx, r.ID, 99, "missing-target", []byte(`{"base_revision":2}`)); !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
			var want error
			switch kind {
			case "pair":
				_, err := db.Exec("DROP TRIGGER record_revisions_no_update;UPDATE record_revisions SET schema_version=2 WHERE revision_number=1")
				if err != nil {
					t.Fatal(err)
				}
				want = ErrSchemaPair
			case "identity":
				_, err := db.Exec("DROP TRIGGER record_revisions_no_update;UPDATE record_revisions SET namespace='other' WHERE revision_number=1")
				if err != nil {
					t.Fatal(err)
				}
				want = ErrInvariant
			case "missing-schema":
				_, err := db.Exec("PRAGMA foreign_keys=OFF;DROP TRIGGER schemas_no_delete;DELETE FROM schemas WHERE schema_version=1;PRAGMA foreign_keys=ON")
				if err != nil {
					t.Fatal(err)
				}
				want = ErrUnavailable
			case "invalid-data":
				_, err := db.Exec(`DROP TRIGGER schemas_no_update;UPDATE schemas SET definition='{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["unavailable"]}' WHERE schema_version=1`)
				if err != nil {
					t.Fatal(err)
				}
				want = ErrValidation
			case "limit":
				_, err := db.Exec("DROP TRIGGER records_head_update;UPDATE records SET revision=9007199254740991")
				if err != nil {
					t.Fatal(err)
				}
				want = ErrRevisionLimit
			}
			before := observe(t, db)
			base := int64(2)
			if kind == "limit" {
				base = MaxRevision
			}
			_, err := s.RestoreRecord(ctx, r.ID, 1, "corrupt", []byte(fmt.Sprintf(`{"base_revision":%d}`, base)))
			wantError(t, err, want)
			if kind == "limit" {
				_, err = s.UpdateRecord(ctx, r.ID, "limit", []byte(fmt.Sprintf(`{"base_revision":%d,"status":"active"}`, base)))
				wantError(t, err, ErrRevisionLimit)
			}
			if observe(t, db) != before {
				t.Fatal("failed restore changed corrupt fixture")
			}
		})
	}
}
func TestM2LinkageGuards(t *testing.T) {
	// Insert a next revision directly to test statement and deferred-commit guards.
	for _, kind := range []string{"missing-record", "missing-subject", "missing-schema", "missing-event", "wrong-event", "wrong-pair", "duplicate-event", "base", "source", "actor", "zero", "fraction", "overflow", "head-content", "head-null"} {
		t.Run(kind, func(t *testing.T) {
			s, db, _, r := revisionFixture(t)
			before := observe(t, db)
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			rid, sub, pair := r.ID, r.SubjectID, r.SchemaID
			number, base, source := any(int64(2)), any(int64(1)), any(nil)
			kindActor := "development_test"
			event := "evt_00000000000000000000000000000001"
			op := "update"
			switch kind {
			case "missing-record":
				rid = "rec_000000000000000000000000000000ff"
				number = int64(1)
				base = nil
				op = "create"
			case "missing-subject":
				sub = "sub_000000000000000000000000000000ff"
			case "missing-schema":
				pair = "absent"
			case "base":
				base = 7
			case "source":
				source = 9
				op = "restore"
			case "actor":
				kindActor = "authenticated"
			case "zero":
				number = 0
			case "fraction":
				number = 2.5
			case "overflow":
				number = int64(9007199254740992)
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO record_revisions(`+revisionColumns+`) VALUES(?,?,?,?,?,'2001-01-01T00:00:00Z','test-harness',?,'req_00000000000000000000000000000001',?,?,?,?,?,?,?,?,?,?,?,?)`, rid, number, base, op, source, kindActor, event, sub, r.Namespace, pair, r.SchemaVersion, string(r.Data), r.Key, r.Sensitivity, "null", r.Status, r.CreatedAt, "2001-01-01T00:00:00Z")
			if kind == "missing-event" || kind == "wrong-event" || kind == "wrong-pair" || kind == "duplicate-event" || strings.HasPrefix(kind, "head-") {
				if err != nil {
					t.Fatal("valid snapshot insertion failed before tested guard", err)
				}
			}
			if err == nil && strings.HasPrefix(kind, "head-") {
				data := string(r.Data)
				var key any = r.Key
				if kind == "head-null" {
					key = nil
				} else {
					data = "{}"
				}
				_, err = tx.Exec("UPDATE records SET revision=2,updated_at='2001-01-01T00:00:00Z',key=?,data=? WHERE id=?", key, data, r.ID)
			}
			if err == nil && (kind == "wrong-event" || kind == "wrong-pair" || kind == "duplicate-event") {
				auditID := event
				rev := 2
				if kind == "wrong-event" {
					auditID = "evt_00000000000000000000000000000002"
				}
				if kind == "wrong-pair" {
					rev = 1
				}
				if kind == "duplicate-event" {
					tx.QueryRow("SELECT event_id FROM mutation_audit LIMIT 1").Scan(&auditID)
				}
				_, err = tx.Exec(`INSERT INTO mutation_audit(event_id,timestamp,actor_id,attribution_kind,action,resource_type,resource_id,subject_id,namespace,revision_number,base_revision,request_id,result) VALUES(?,'2001-01-01T00:00:00Z','test-harness','development_test','record.updated','record',?,?,?, ?,1,'req_00000000000000000000000000000001','accepted')`, auditID, r.ID, r.SubjectID, r.Namespace, rev)
			}
			if err == nil {
				err = tx.Commit()
			}
			if err == nil {
				t.Fatal("invalid linkage committed", kind)
			}
			tx.Rollback()
			if observe(t, db) != before {
				t.Fatal("rejected SQL changed state")
			}
			healthy(t, db)
			// Keep the compiler checking the supported service constructor in this fixture.
			_ = s
		})
	}
}
func TestM2SerializationFailureRollsBack(t *testing.T) {
	s, db, _, r := revisionFixture(t)
	before := observe(t, db)
	a, _ := caller(ctx)
	_, err := s.mutateResult(ctx, "PATCH records/"+r.ID, "serialize", []byte(`{"base_revision":1,"status":"archived"}`), func(tx *sql.Tx) (any, error) {
		next := r
		next.Status = "archived"
		if _, err := s.writeRevision(ctx, tx, next, r, "update", nil, a); err != nil {
			return nil, err
		}
		return json.RawMessage(`{`), nil
	})
	if err == nil {
		t.Fatal("invalid response serialized")
	}
	if observe(t, db) != before {
		t.Fatal("serialization left partial effect")
	}
}
func TestM2ContentionCancellationAndReplay(t *testing.T) {
	s, db, path, r := revisionFixture(t)
	r = update(t, s, r, "committed", `"status":"archived"`)
	other, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	lock, err := other.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	before := observe(t, db)
	for _, key := range []string{"committed", "new"} {
		started := time.Now()
		_, err := s.UpdateRecord(ctx, r.ID, key, []byte(`{"base_revision":1,"status":"archived"}`))
		wantError(t, err, ErrUnavailable)
		if time.Since(started) > 3*time.Second {
			t.Fatal("unbounded busy wait")
		}
	}
	lock.Rollback()
	if observe(t, db) != before {
		t.Fatal("busy request changed state")
	}
	if _, err := s.UpdateRecord(ctx, r.ID, "committed", []byte(`{"base_revision":1,"status":"archived"}`)); err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateRecord(ctx, r.ID, "new", []byte(`{"base_revision":1,"status":"archived"}`))
	var conflict *RevisionConflict
	if !errors.As(err, &conflict) {
		t.Fatal(err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = s.UpdateRecord(short, r.ID, "pool", []byte(`{"base_revision":2,"status":"active"}`))
	conn.Close()
	wantError(t, err, ErrUnavailable)
	cancelled, stop := context.WithCancel(ctx)
	stop()
	_, err = s.UpdateRecord(cancelled, r.ID, "cancelled", []byte(`{"base_revision":2,"status":"active"}`))
	wantError(t, err, ErrUnavailable)
	if observe(t, db) != before {
		t.Fatal("cancelled request changed state")
	}
	r = update(t, s, r, "pool", `"status":"active"`)
	if r.Revision != 3 {
		t.Fatal("pool recovery")
	}
}

func TestM2AllMetadataPathsAndCreateDomains(t *testing.T) {
	s, db, _, r := revisionFixture(t)
	for _, field := range []string{`"data":{"different":true}`, `"key":"new"`, `"sensitivity":"public"`, `"provenance":{"source":"synthetic"}`, `"status":"archived"`, `"status":"active"`, `"data":{},"key":null,"provenance":null,"status":"archived"`} {
		before := observe(t, db)
		_, err := s.UpdateRecord(ctx, r.ID, "missing", []byte(`{`+field+`}`))
		wantError(t, err, ErrBaseRequired)
		_, err = s.UpdateRecord(ctx, r.ID, "stale", []byte(fmt.Sprintf(`{"base_revision":%d,%s}`, r.Revision+1, field)))
		var conflict *RevisionConflict
		if !errors.As(err, &conflict) {
			t.Fatal(err)
		}
		if observe(t, db) != before {
			t.Fatal("missing/stale changed stores")
		}
		r = update(t, s, r, fmt.Sprint(r.Revision), field)
		healthy(t, db)
	}
	for i, domain := range []string{"example.skills", "example.tasks"} {
		publish(t, s, domain, 1, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
		metadata := `"key":null,"provenance":null`
		if i == 1 {
			metadata = `"key":"label","provenance":{"source":"synthetic","note":"full metadata"}`
		}
		body := []byte(fmt.Sprintf(`{"subject_id":%q,"namespace":%q,"schema_id":%q,"schema_version":1,"data":{},"sensitivity":"restricted",%s}`, r.SubjectID, domain, domain, metadata))
		got := create(t, s, domain, body)
		v, err := s.GetRevision(ctx, got.ID, 1)
		if err != nil || !reflect.DeepEqual(v.Snapshot, got.Snapshot) || v.Base != nil || v.Source != nil || got.Revision != 1 {
			t.Fatal("complete create snapshot", err)
		}
		var events, keys int
		db.QueryRow("SELECT count(*) FROM mutation_audit WHERE resource_id=? AND revision_number=1 AND timestamp=? AND request_id=? AND event_id=?", got.ID, got.CreatedAt, v.RequestID, v.AuditEventID).Scan(&events)
		db.QueryRow("SELECT count(*) FROM idempotency_keys WHERE scope='POST records' AND key=? AND response_contract='m2'", domain).Scan(&keys)
		if events != 1 || keys != 1 {
			t.Fatal("create linkage")
		}
	}
	healthy(t, db)
}
func TestM2ControlledPoolRaces(t *testing.T) {
	for _, variant := range []string{"patch", "restore", "replay"} {
		t.Run(variant, func(t *testing.T) {
			s, db, path, r := revisionFixture(t)
			otherDB, err := storage.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer otherDB.Close()
			other := New(otherDB)
			held, release := make(chan struct{}), make(chan struct{})
			s.fault = func(point string) error {
				if point == "F1" {
					close(held)
					<-release
				}
				return nil
			}
			results := make(chan error, 2)
			go func() {
				_, err := s.UpdateRecord(ctx, r.ID, "first", []byte(`{"base_revision":1,"status":"archived"}`))
				results <- err
			}()
			<-held
			go func() {
				var err error
				if variant == "restore" {
					_, err = other.RestoreRecord(ctx, r.ID, 1, "second", []byte(`{"base_revision":1}`))
				} else {
					key := "second"
					if variant == "replay" {
						key = "first"
					}
					_, err = other.UpdateRecord(ctx, r.ID, key, []byte(`{"base_revision":1,"status":"archived"}`))
				}
				results <- err
			}()
			close(release)
			successes, conflicts := 0, 0
			for range 2 {
				err := <-results
				var conflict *RevisionConflict
				if err == nil {
					successes++
				} else if errors.As(err, &conflict) {
					conflicts++
				} else {
					t.Fatal(err)
				}
			}
			if variant == "replay" {
				if successes != 2 {
					t.Fatal(successes, conflicts)
				}
			} else if successes != 1 || conflicts != 1 {
				t.Fatal(successes, conflicts)
			}
			healthy(t, db)
			var head int
			db.QueryRow("SELECT revision FROM records").Scan(&head)
			if head != 2 {
				t.Fatal("duplicate effect")
			}
		})
	}
}
func TestM2CancellationBeforeCommit(t *testing.T) {
	s, db, _, r := revisionFixture(t)
	before := observe(t, db)
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	s.fault = func(point string) error {
		if point == "F6" {
			cancel()
		}
		return nil
	}
	_, err := s.UpdateRecord(cancelled, r.ID, "cancel-at-commit", []byte(`{"base_revision":1,"status":"archived"}`))
	if err == nil {
		t.Fatal("cancelled mutation committed")
	}
	if observe(t, db) != before {
		t.Fatal("cancelled commit changed state")
	}
}

func TestM2ExactJSONAcrossHistory(t *testing.T) {
	s, db, path, r := revisionFixture(t)
	data := `{"n":9007199254740993,"decimal":0.30,"exponent":1e2,"negative":-0,"low":1e-308,"high":1e308,"large":` + strings.Repeat("9", 128) + `,"unknown":[{"unicode":"雪"},null,true]}`
	r = create(t, s, "boundaries", recordBody(r.SubjectID, r.Namespace, r.SchemaID, data))
	r = update(t, s, r, "metadata", `"sensitivity":"restricted"`)
	raw, err := s.RestoreRecord(ctx, r.ID, 1, "restore-numbers", []byte(`{"base_revision":2}`))
	r = decode[Record](t, raw, err)
	if string(r.Data) != data {
		t.Fatal("restore transformed exact data")
	}
	db.Close()
	db, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = New(db)
	got, err := s.GetRecord(ctx, r.ID)
	if err != nil || string(got.Data) != data {
		t.Fatal("restart transformed data", err)
	}
	page, err := s.ListRevisions(ctx, r.ID, ListOptions{})
	if err != nil || len(page.Items) != 3 {
		t.Fatal(err)
	}
	for _, v := range page.Items {
		single, err := s.GetRevision(ctx, r.ID, v.Number)
		if err != nil || string(single.Snapshot.Data) != data || string(v.Snapshot.Data) != data {
			t.Fatal("revision transformed data", err)
		}
	}
	before := observe(t, db)
	replay, err := s.RestoreRecord(ctx, r.ID, 1, "restore-numbers", []byte(`{"base_revision":2}`))
	if err != nil || string(replay.Data) != string(raw.Data) || observe(t, db) != before {
		t.Fatal("numeric replay", err)
	}
}

func TestM2NoopKindsAndFullRestore(t *testing.T) {
	s, db, _, r := revisionFixture(t)
	for i, fields := range []string{`"status":"active"`, `"data":` + string(r.Data), `"key":"same-key","sensitivity":"private","provenance":null`} {
		old := r
		r = update(t, s, r, fmt.Sprintf("noop-%d", i), fields)
		if r.Revision != old.Revision+1 || r.UpdatedAt == old.UpdatedAt {
			t.Fatal("no-op did not create action")
		}
		before := observe(t, db)
		_, err := s.UpdateRecord(ctx, r.ID, fmt.Sprintf("noop-%d", i), []byte(fmt.Sprintf(`{"base_revision":%d,%s}`, old.Revision, fields)))
		if err != nil || observe(t, db) != before {
			t.Fatal("no-op replay", err)
		}
	}
	r = update(t, s, r, "full", `"data":{"archive":true},"key":"historical","sensitivity":"restricted","provenance":{"source":"synthetic-private-marker","note":"payload-must-not-enter-audit"},"status":"archived"`)
	target := r
	r = update(t, s, r, "later", `"data":{},"key":null,"sensitivity":"private","provenance":null,"status":"active"`)
	raw, err := s.RestoreRecord(ctx, r.ID, target.Revision, "full-restore", []byte(fmt.Sprintf(`{"base_revision":%d}`, r.Revision)))
	r = decode[Record](t, raw, err)
	restored := r.Snapshot
	restored.UpdatedAt = target.UpdatedAt
	if !reflect.DeepEqual(restored, target.Snapshot) {
		t.Fatal("full historical metadata not restored")
	}
	rows, err := db.Query("SELECT * FROM mutation_audit")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	for _, name := range cols {
		for _, forbidden := range []string{"data", "provenance", "schema_definition", "idempotency_key"} {
			if name == forbidden {
				t.Fatal("non-minimal audit")
			}
		}
	}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(values)
		if strings.Contains(string(raw), "synthetic-private-marker") || strings.Contains(string(raw), "payload-must-not-enter-audit") {
			t.Fatal("audit leaked payload")
		}
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
}
