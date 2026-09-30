package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func execSQL(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func TestFreshDirectoryAndLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "synthetic ?# database.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := CheckReady(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for p, mode := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("permissions = %o, want %o", info.Mode().Perm(), mode)
		}
	}
	var names string
	if err := db.QueryRow("SELECT group_concat(name, ',') FROM sqlite_schema WHERE type='table' AND substr(name, 1, 7) != 'sqlite_'").Scan(&names); err != nil {
		t.Fatal(err)
	}
	if names != "schema_migrations,subjects,schemas,records,idempotency_keys" {
		t.Fatalf("unexpected milestone tables: %s", names)
	}
	if db.Stats().MaxOpenConnections != 1 {
		t.Fatal("connection pool is not bounded to one")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := CheckReady(context.Background(), db); err == nil {
		t.Fatal("closed database reported ready")
	}
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
}

func TestConnectionPolicyAndForeignKeys(t *testing.T) {
	db := testDB(t)
	if err := verifyConnection(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	// Force replacement to prove connection-local settings are reapplied.
	db.SetMaxIdleConns(0)
	if err := verifyConnection(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	db.SetMaxIdleConns(1)
	execSQL(t, db, "CREATE TEMP TABLE test_parent (id INTEGER PRIMARY KEY)")
	execSQL(t, db, "CREATE TEMP TABLE test_child (parent_id INTEGER REFERENCES test_parent(id))")
	if _, err := db.Exec("INSERT INTO test_child(parent_id) VALUES (123)"); err == nil {
		t.Fatal("foreign-key violation accepted")
	}
	execSQL(t, db, "INSERT INTO test_parent(id) VALUES (123)")
	execSQL(t, db, "INSERT INTO test_child(parent_id) VALUES (123)")
}

func TestRepeatedMigrationDoesNotRewriteLedger(t *testing.T) {
	db := testDB(t)
	read := func() string {
		var result string
		if err := db.QueryRow("SELECT name || checksum || applied_at FROM schema_migrations WHERE version=1").Scan(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := read()
	for range 3 {
		if err := Migrate(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	if after := read(); before != after {
		t.Fatal("repeat startup rewrote the migration ledger")
	}
}

func TestFailedMigrationRollsBackWholeBatch(t *testing.T) {
	for _, fresh := range []bool{true, false} {
		t.Run(map[bool]string{true: "from_zero", false: "from_existing"}[fresh], func(t *testing.T) {
			db, err := openDatabase(context.Background(), filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if !fresh {
				if err := Migrate(context.Background(), db); err != nil {
					t.Fatal(err)
				}
			}
			steps := append([]migration{}, migrations...)
			steps = append(steps, migration{name: "broken_test", sql: "CREATE TABLE rollback_probe (id INTEGER); INSERT INTO missing_test_table VALUES (1);"})
			if err := migrate(context.Background(), db, steps); err == nil {
				t.Fatal("broken migration succeeded")
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='rollback_probe'").Scan(&count); err != nil || count != 0 {
				t.Fatalf("DDL was not rolled back: count=%d, error=%v", count, err)
			}
			if fresh {
				if err := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='schema_migrations'").Scan(&count); err != nil || count != 0 {
					t.Fatalf("fresh bookkeeping was not rolled back: count=%d, error=%v", count, err)
				}
			} else {
				if err := CheckReady(context.Background(), db); err != nil {
					t.Fatalf("previous version was damaged: %v", err)
				}
			}
			if err := Migrate(context.Background(), db); err != nil {
				t.Fatalf("corrected retry failed: %v", err)
			}
		})
	}
}

func TestIncompatibleSchemaIsRejected(t *testing.T) {
	cases := map[string]string{
		"newer":     "INSERT INTO schema_migrations(version,name,checksum) VALUES(2147483647,'future','future')",
		"gap":       "UPDATE schema_migrations SET version=2147483646 WHERE version=1",
		"checksum":  "UPDATE schema_migrations SET checksum='edited'",
		"name":      "UPDATE schema_migrations SET name='renamed'",
		"empty":     "DELETE FROM schema_migrations",
		"malformed": "DROP TABLE schema_migrations; CREATE TABLE schema_migrations (unexpected TEXT)",
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.db")
			db, err := Open(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			execSQL(t, db, query)
			db.Close()
			reopened, err := Open(context.Background(), path)
			if reopened != nil {
				reopened.Close()
				t.Fatal("incompatible database was opened")
			}
			if !errors.Is(err, ErrIncompatibleSchema) {
				t.Fatalf("want incompatible-schema error, got %v", err)
			}
		})
	}
}

func TestUnmanagedDatabaseIsNotAdopted(t *testing.T) {
	db, err := openDatabase(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	execSQL(t, db, "CREATE TABLE unrelated (value TEXT)")
	if err := Migrate(context.Background(), db); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("unmanaged database was not rejected: %v", err)
	}
}

func TestBusyWaitingIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	first, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	tx, err := first.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	start := time.Now()
	if err := Migrate(ctx, second); err == nil {
		t.Fatal("migration bypassed a competing writer")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("lock wait exceeded bounded busy timeout: %v", elapsed)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), second); err != nil {
		t.Fatalf("database did not recover after contention: %v", err)
	}
}

func TestInvalidDatabasePaths(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "not-a-database")
	if err := os.WriteFile(plain, []byte("synthetic invalid database"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "symlink.db")
	if err := os.Symlink(plain, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", ":memory:", "file:unwanted.db", "bad\x00path", dir, plain, link, filepath.Join(plain, "child.db")} {
		t.Run(strings.ReplaceAll(path, "\x00", "NUL"), func(t *testing.T) {
			if db, err := Open(context.Background(), path); err == nil {
				db.Close()
				t.Fatal("invalid database path accepted")
			}
		})
	}
}

func TestCancelledMigrationDoesNotChangeSchema(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Migrate(ctx, db); !errors.Is(err, context.Canceled) {
		t.Fatalf("want cancellation, got %v", err)
	}
	if err := CheckReady(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}

func TestM0UpgradeAndOldBinaryRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m0.db")
	db, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate(context.Background(), db, migrations[:1]); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := db.QueryRow("SELECT checksum || applied_at FROM schema_migrations WHERE version=1").Scan(&original); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := db.QueryRow("SELECT checksum || applied_at FROM schema_migrations WHERE version=1").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if original != after {
		t.Fatal("M0 ledger changed")
	}
	if err := migrate(context.Background(), db, migrations[:1]); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("M0 accepted M1 database: %v", err)
	}
	if err := CheckReady(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}
