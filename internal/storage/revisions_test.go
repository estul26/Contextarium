package storage

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/estul26/Contextarium/internal/engine"
)

const adoptedID = "rec_00000000000000000000000000000001"
const adoptedSubject = "sub_00000000000000000000000000000001"
const legacyPatch = `{"status":"archived"}`
const legacyData = `{"n":9007199254740993,"d":0.30,"e":1e2,"z":-0,"extra":{"text":"雪","array":[null,true]},"low":1e-308,"high":1e308,"large":99999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999}`

func seedM1(t *testing.T, db *sql.DB) {
	t.Helper()
	execSQL(t, db, `INSERT INTO subjects VALUES('`+adoptedSubject+`','project','Synthetic migration','2000-01-01T00:00:00Z');INSERT INTO schemas VALUES('example.exact',1,'{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}','explicit','2000-01-01T00:00:00Z')`)
	for i, status := range []string{"archived", "active"} {
		_, err := db.Exec(`INSERT INTO records(id,subject_id,namespace,schema_id,schema_version,data,key,sensitivity,provenance,status,created_at,updated_at) VALUES(?,?,'example.records','example.exact',1,?,NULL,'private','null',?,'2000-01-01T00:00:00Z','2000-01-02T00:00:00Z')`, fmt.Sprintf("rec_%032d", i+1), adoptedSubject, legacyData, status)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Preserve the exact M1 response and hash; the response is deliberately older
	// than current content and has no revision field.
	response := `{"id":"` + adoptedID + `","subject_id":"` + adoptedSubject + `","namespace":"example.records","schema_id":"example.exact","schema_version":1,"data":{},"key":null,"sensitivity":"private","provenance":null,"status":"archived","created_at":"2000-01-01T00:00:00Z","updated_at":"2000-01-01T01:00:00Z"}`
	sum := sha256.Sum256([]byte(legacyPatch))
	if _, err := db.Exec("INSERT INTO idempotency_keys VALUES(?,?,?,?)", "PATCH records/"+adoptedID, "legacy", fmt.Sprintf("%x", sum), response); err != nil {
		t.Fatal(err)
	}
}
func m1DB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	db, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrate(context.Background(), db, migrations[:2]); err != nil {
		t.Fatal(err)
	}
	return db, path
}
func readLegacy(t *testing.T, db *sql.DB) string {
	t.Helper()
	var result string
	if err := db.QueryRow("SELECT scope||key||request_hash||response FROM idempotency_keys WHERE key='legacy'").Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestM2AdoptionAndLegacyReplay(t *testing.T) {
	db, path := m1DB(t)
	seedM1(t, db)
	before := readLegacy(t, db)
	var oldLedger string
	if err := db.QueryRow("SELECT group_concat(name||checksum||applied_at,'|') FROM schema_migrations").Scan(&oldLedger); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var data, op, actor, recorded, created, reason string
	var count, head int
	if err := db.QueryRow(`SELECT v.data,v.operation,v.actor_id,v.recorded_at,v.created_at,a.reason,r.revision FROM record_revisions v JOIN records r ON r.id=v.record_id JOIN mutation_audit a ON a.event_id=v.audit_event_id WHERE v.record_id=?`, adoptedID).Scan(&data, &op, &actor, &recorded, &created, &reason, &head); err != nil {
		t.Fatal(err)
	}
	if data != legacyData || op != "m1_adoption" || actor != "local-migration-003" || recorded == created || reason != "m1_history_boundary" || head != 1 {
		t.Fatal("adoption falsified history")
	}
	if err := db.QueryRow("SELECT count(*) FROM record_revisions").Scan(&count); err != nil || count != 2 {
		t.Fatal("invented/missing history", count, err)
	}
	var ledger string
	db.QueryRow("SELECT group_concat(name||checksum||applied_at,'|') FROM schema_migrations WHERE version<3").Scan(&ledger)
	if ledger != oldLedger {
		t.Fatal("old ledger changed")
	}
	a := engine.WithAttribution(context.Background(), engine.Actor{Kind: "development_test", ID: "migration-test"}, "req_00000000000000000000000000000001")
	s := engine.New(db)
	raw, err := s.UpdateRecord(a, adoptedID, "legacy", []byte(legacyPatch))
	if err != nil || raw.Contract != "m1" || strings.Contains(string(raw.Data), `"revision"`) {
		t.Fatal("legacy contract", err)
	}
	if readLegacy(t, db) != before {
		t.Fatal("legacy row rewritten")
	}
	if _, err := s.UpdateRecord(a, adoptedID, "edit", []byte(`{"base_revision":1,"status":"active"}`)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = engine.New(db)
	replay, err := s.UpdateRecord(a, adoptedID, "legacy", []byte(legacyPatch))
	if err != nil || string(replay.Data) != string(raw.Data) || replay.Contract != "m1" {
		t.Fatal("restart legacy replay", err)
	}
	if readLegacy(t, db) != before {
		t.Fatal("legacy bytes changed")
	}
	_, err = s.UpdateRecord(a, adoptedID, "legacy", []byte(`{"base_revision":2,"status":"archived"}`))
	if !errors.Is(err, engine.ErrConflict) {
		t.Fatal(err)
	}
	_, err = s.UpdateRecord(a, adoptedID, "unknown", []byte(legacyPatch))
	if !errors.Is(err, engine.ErrBaseRequired) {
		t.Fatal(err)
	}
	r, err := s.GetRecord(a, adoptedID)
	if err != nil || r.Revision != 2 || r.Status != "active" || string(r.Data) != legacyData {
		t.Fatal("legacy replay changed head", err)
	}
	for _, prefix := range []int{1, 2} {
		if err := migrate(context.Background(), db, migrations[:prefix]); !errors.Is(err, ErrIncompatibleSchema) {
			t.Fatal("old binary accepted M2 ledger", err)
		}
	}
}
func TestM2MigrationInterruptionAndRetry(t *testing.T) {
	points := []string{"U0", "U1", "U2", "U3", "U4_before_ledger", "U4", "U5"}
	for _, kind := range []string{"fresh", "m0", "empty-m1", "metadata-m1", "populated-m1"} {
		for _, point := range points {
			t.Run(kind+"/"+point, func(t *testing.T) {
				db, err := openDatabase(context.Background(), filepath.Join(t.TempDir(), "synthetic.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				previous := 0
				if kind == "m0" {
					previous = 1
				} else if kind != "fresh" {
					previous = 2
				}
				if previous > 0 {
					if err := migrate(context.Background(), db, migrations[:previous]); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "populated-m1" || kind == "metadata-m1" {
					seedM1(t, db)
					if kind == "metadata-m1" {
						execSQL(t, db, "DELETE FROM records")
					}
				}
				injected := errors.New("injected migration interruption")
				err = migrateWithHook(context.Background(), db, migrations, func(p string) error {
					if p == point {
						return injected
					}
					return nil
				})
				if !errors.Is(err, injected) {
					t.Fatal(err)
				}
				var tables int
				if err := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='record_revisions'").Scan(&tables); err != nil {
					t.Fatal(err)
				}
				if point != "U5" && tables != 0 {
					t.Fatal("partial migration leaked")
				}
				if previous > 0 {
					var n int
					db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n)
					want := previous
					if point == "U5" {
						want = 3
					}
					if n != want {
						t.Fatal("partial ledger", n)
					}
				}
				if err := Migrate(context.Background(), db); err != nil {
					t.Fatal(err)
				}
				if err := Migrate(context.Background(), db); err != nil {
					t.Fatal(err)
				}
				var count int
				db.QueryRow("SELECT count(*) FROM record_revisions").Scan(&count)
				want := 0
				if kind == "populated-m1" {
					want = 2
				}
				if count != want {
					t.Fatal("duplicate/missing adoption", count)
				}
			})
		}
	}
}
func TestM2MigrationCancellationAndCommitFailure(t *testing.T) {
	for _, point := range []string{"U0", "U1", "U2", "U3", "U4"} {
		t.Run(point, func(t *testing.T) {
			db, _ := m1DB(t)
			seedM1(t, db)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := migrateWithHook(ctx, db, migrations, func(p string) error {
				if p == point {
					cancel()
				}
				return nil
			})
			if err == nil {
				t.Fatal("cancelled upgrade succeeded")
			}
			if err := Migrate(context.Background(), db); err != nil {
				t.Fatal(err)
			}
		})
	}
	db, _ := m1DB(t)
	seedM1(t, db)
	steps := append([]migration(nil), migrations...)
	steps[2].sql += `CREATE TABLE test_deferred(id TEXT REFERENCES subjects(id) DEFERRABLE INITIALLY DEFERRED);INSERT INTO test_deferred VALUES('missing');`
	// validateAdoption catches FK defects before the ledger is recorded.
	if err := migrate(context.Background(), db, steps); err == nil {
		t.Fatal("inconsistent upgrade accepted")
	}
	var n int
	db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n)
	if n != 2 {
		t.Fatal("inconsistent upgrade changed ledger")
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}
func TestM2MigrationChild(t *testing.T) {
	point := os.Getenv("CONTEXTARIUM_TEST_MIGRATION")
	if point == "" {
		return
	}
	db, err := openDatabase(context.Background(), os.Getenv("CONTEXTARIUM_TEST_DB"))
	if err != nil {
		t.Fatal(err)
	}
	err = migrateWithHook(context.Background(), db, migrations, func(p string) error {
		if p == point {
			fmt.Println("BARRIER")
			var b [1]byte
			os.Stdin.Read(b[:])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}
func TestM2MigrationAbruptRecovery(t *testing.T) {
	for _, point := range []string{"U0", "U1", "U2", "U3", "U4", "U5"} {
		t.Run(point, func(t *testing.T) {
			db, path := m1DB(t)
			seedM1(t, db)
			db.Close()
			cmd := exec.Command(os.Args[0], "-test.run=^TestM2MigrationChild$", "-test.timeout=20s")
			cmd.Env = append(os.Environ(), "CONTEXTARIUM_TEST_MIGRATION="+point, "CONTEXTARIUM_TEST_DB="+path)
			out, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			in, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer in.Close()
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
			go func() { scanner := bufio.NewScanner(out); reached <- scanner.Scan() && scanner.Text() == "BARRIER" }()
			select {
			case ok := <-reached:
				if !ok {
					t.Fatal("migration barrier missing")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("migration child timed out")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			cmd.Wait()
			db, err = openDatabase(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var n int
			db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n)
			want := 2
			if point == "U5" {
				want = 3
			}
			if n != want {
				t.Fatal("crash ledger", n)
			}
			if err := Migrate(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			var integrity string
			db.QueryRow("PRAGMA integrity_check").Scan(&integrity)
			if integrity != "ok" {
				t.Fatal(integrity)
			}
			var raw string
			db.QueryRow("SELECT data FROM record_revisions WHERE record_id=?", adoptedID).Scan(&raw)
			if raw != legacyData {
				t.Fatal("adoption recovery changed data")
			}
			var total int
			db.QueryRow("SELECT count(*) FROM record_revisions").Scan(&total)
			if total != 2 {
				t.Fatal("adoption duplicated")
			}
		})
	}
}
func TestM2MigrationMetadataAndConnectionEvidence(t *testing.T) {
	db := testDB(t)
	var version string
	db.QueryRow("SELECT sqlite_version()").Scan(&version)
	evidence := map[string]any{"sqlite_version": version}
	for _, p := range []string{"foreign_keys", "synchronous", "busy_timeout", "journal_mode"} {
		var value any
		if err := db.QueryRow("PRAGMA " + p).Scan(&value); err != nil {
			t.Fatal(err)
		}
		evidence[p] = value
	}
	raw, _ := json.Marshal(evidence)
	t.Log(string(raw))
}

func TestM2MigrationMidAdoptionAndDeadline(t *testing.T) {
	for _, mode := range []string{"mid-adoption", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			db, _ := m1DB(t)
			seedM1(t, db)
			before := readLegacy(t, db)
			var err error
			if mode == "mid-adoption" {
				steps := append([]migration(nil), migrations...)
				steps[2].sql = strings.Replace(steps[2].sql, "-- M2 U1", `CREATE TRIGGER test_abort_adoption BEFORE INSERT ON record_revisions WHEN NEW.record_id='rec_00000000000000000000000000000002' BEGIN SELECT RAISE(ABORT,'synthetic adoption failure');END;
-- M2 U1`, 1)
				err = migrate(context.Background(), db, steps)
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()
				err = migrateWithHook(ctx, db, migrations, func(point string) error {
					if point == "U2" {
						<-ctx.Done()
					}
					return nil
				})
			}
			if err == nil {
				t.Fatal("interrupted adoption committed")
			}
			var version int
			db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version)
			if version != 2 || readLegacy(t, db) != before {
				t.Fatal("partial adoption")
			}
			if err := Migrate(context.Background(), db); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestM2UpgradeCommitConstraintRollback(t *testing.T) {
	db, _ := m1DB(t)
	seedM1(t, db)
	before := readLegacy(t, db)
	// Empty until the migration ledger is inserted, after adoption validation.
	execSQL(t, db, `CREATE TABLE test_commit_fault(id TEXT REFERENCES subjects(id) DEFERRABLE INITIALLY DEFERRED);CREATE TRIGGER test_commit_fault_trigger AFTER INSERT ON schema_migrations WHEN NEW.version=3 BEGIN INSERT INTO test_commit_fault VALUES('absent');END;`)
	if err := Migrate(context.Background(), db); err == nil {
		t.Fatal("deferred commit failure accepted")
	}
	var n int
	db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='record_revisions'").Scan(&n)
	if n != 0 || readLegacy(t, db) != before {
		t.Fatal("commit failure leaked adoption")
	}
	execSQL(t, db, "DROP TRIGGER test_commit_fault_trigger;DROP TABLE test_commit_fault")
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}

func TestM2ReviewedCandidateChecksumRefusal(t *testing.T) {
	db, path := m1DB(t)
	seedM1(t, db)
	// Prepare a NEW disposable database using the reviewed candidate's exact 003
	// bytes. No existing database or migration ledger is edited to manufacture it.
	previous := append([]migration(nil), migrations...)
	previous[2].sql = strings.ReplaceAll(previous[2].sql, "'revision.restored'", "'record.revision.restored'")
	if migrationChecksum(previous[2]) != "31865ddf5b43c6d64b741d8be40c46da191fd01d77681b88c4aecf9ebbae01cf" {
		t.Fatal("fixture does not match reviewed candidate")
	}
	if err := migrate(context.Background(), db, previous); err != nil {
		t.Fatal(err)
	}
	var ledger, history string
	if err := db.QueryRow("SELECT group_concat(version||name||checksum||applied_at) FROM schema_migrations").Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT group_concat(record_id||data||recorded_at||audit_event_id) FROM record_revisions").Scan(&history); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	rejected, err := Open(context.Background(), path)
	if rejected != nil {
		rejected.Close()
		t.Fatal("incompatible database returned")
	}
	if !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatal("old candidate 003 checksum silently accepted", err)
	}
	// Read the test-owned file only to prove refusal retained the old ledger/history.
	db, err = openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var gotLedger, gotHistory string
	if err := db.QueryRow("SELECT group_concat(version||name||checksum||applied_at) FROM schema_migrations").Scan(&gotLedger); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT group_concat(record_id||data||recorded_at||audit_event_id) FROM record_revisions").Scan(&gotHistory); err != nil {
		t.Fatal(err)
	}
	if gotLedger != ledger || gotHistory != history {
		t.Fatal("refusal rewrote existing candidate state")
	}
}
