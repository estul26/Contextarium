//go:build r3 && cgo

// This executable is a test process, never a Contextarium production command.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/estul26/Contextarium/internal/engine"
	"github.com/estul26/Contextarium/internal/r3/vfs"
	"github.com/estul26/Contextarium/internal/storage"
	"github.com/mattn/go-sqlite3"
)

type Input struct {
	Batch              []Input
	Operation, ID, Key string
	Body               json.RawMessage
	Target             int64
}

var ctx = engine.WithAttribution(context.Background(), engine.Actor{Kind: "development_test", ID: "r3-worker"}, "req_00000000000000000000000000000300")
var phase, query = "startup", ""
var instrumented bool
var protocolProgress bool

// Only fixed phase/query labels and numeric SQLite codes cross the diagnostic
// boundary. Error strings may contain SQL, data, or paths and are never printed.
func diagnostic(kind, stage string, err error) {
	x := map[string]any{"kind": kind, "stage": stage, "phase": phase, "query": query}
	if err != nil {
		x["reason"] = "operation-failed"
		if errors.Is(err, sql.ErrNoRows) {
			x["reason"] = "sql-no-rows"
		}
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) {
			x["reason"] = "sqlite-error"
			x["code"], x["extended_code"] = int(sqliteErr.Code), int(sqliteErr.ExtendedCode)
		}
	}
	_ = json.NewEncoder(os.Stderr).Encode(x)
	if protocolProgress {
		_ = json.NewEncoder(os.Stdout).Encode(x)
	}
}
func startPhase(name string) {
	phase, query = name, ""
	diagnostic("worker-progress", "start", nil)
	vfs.Phase(name)
}
func completed() { diagnostic("worker-progress", "complete", nil) }

func must(err error) {
	if err != nil {
		diagnostic("worker-fatal", "failed", err)
		os.Exit(2)
	}
}
func emit(x any) { must(json.NewEncoder(os.Stdout).Encode(x)) }
func block() {
	vfs.Report()
	emit(map[string]any{"kind": "done"})
	var b [1]byte
	os.Stdin.Read(b[:])
	os.Exit(3)
}
func main() {
	root := flag.String("root", "", "owned test directory")
	mode := flag.String("mode", "", "test role")
	cfg := flag.String("input", "", "input JSON")
	trace := flag.String("trace", "", "trace path")
	aim := flag.Int("target", 0, "I/O occurrence")
	fault := flag.String("fault", "none", "test fault")
	sector := flag.Int("sector", 4096, "modeled sector bytes")
	diagnosticOnly := flag.Bool("diagnostic-only", false, "one no-fault create, inspect, clean close; no probes or schedules")
	flag.Parse()
	protocolProgress = *mode == "run" || *mode == "native" || *mode == "probe"
	if *diagnosticOnly && (*aim != 0 || *fault != "none" || (*mode != "native" && *mode != "run")) {
		must(errors.New("invalid diagnostic role or fault configuration"))
	}
	path := filepath.Join(*root, "store.db")
	if *mode == "inspect" {
		startPhase("inspection")
		db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path)+"?mode=rw&cache=private&_foreign_keys=on&_synchronous=FULL&_busy_timeout=1000")
		must(err)
		defer db.Close()
		db.SetMaxOpenConns(1)
		emit(inspect(db))
		completed()
		return
	}
	if *mode == "probe" {
		must(vfs.Init(*root, *trace, *aim, *fault, *sector))
		startPhase("model-validation")
		rc := vfs.Probe(filepath.Join(*root, "probe.db"))
		emit(map[string]any{"kind": "response", "ok": rc == 0, "code": rc})
		block()
	}
	if *mode == "fixture" {
		startPhase("fixture")
		db, err := storage.Open(ctx, path)
		must(err)
		fixture(db)
		must(db.Close())
		return
	}
	raw, err := os.ReadFile(*cfg)
	must(err)
	var input Input
	must(json.Unmarshal(raw, &input))
	switch input.Operation {
	case "create", "patch", "metadata", "archive", "unarchive", "noop", "restore", "migration", "fresh", "empty-m1", "checkpoint", "autocheckpoint", "batch":
	default:
		must(errors.New("unknown operation"))
	}
	if *diagnosticOnly && (input.Operation != "create" || len(input.Batch) != 0) {
		must(errors.New("diagnostic permits one create only"))
	}
	if *mode == "run" {
		must(vfs.Init(*root, *trace, *aim, *fault, *sector))
		instrumented = true
	}
	startPhase("open")
	if input.Operation == "migration" || input.Operation == "fresh" || input.Operation == "empty-m1" {
		startPhase(input.Operation)
	}
	db, err := storage.Open(ctx, path)
	if err != nil {
		diagnostic("worker-error", "failed", err)
		if *diagnosticOnly {
			must(err)
		}
		emit(map[string]any{"kind": "response", "ok": false, "error": "open-unavailable"})
		block()
	}
	completed()
	s := engine.New(db)
	if *mode == "retry" {
		if len(input.Batch) > 0 {
			for _, item := range input.Batch {
				result, e := mutate(s, item)
				must(e)
				emit(map[string]any{"kind": "response", "ok": true, "result_text": string(result.Data), "contract": result.Contract})
			}
			must(db.Close())
			return
		}
		result, err := mutate(s, input)
		if err != nil {
			emit(map[string]any{"kind": "response", "ok": false, "error": "retry-unavailable"})
		} else {
			emit(map[string]any{"kind": "response", "ok": true, "result_text": string(result.Data), "contract": result.Contract})
		}
		must(db.Close())
		return
	}
	emit(map[string]any{"kind": "settings", "value": settings(db)})
	if input.Operation == "migration" || input.Operation == "fresh" || input.Operation == "empty-m1" {
		emit(map[string]any{"kind": "response", "ok": true, "result": map[string]int{"migration": 3}})
		block()
	}
	if input.Operation == "checkpoint" || input.Operation == "autocheckpoint" {
		startPhase("checkpoint-prepare")
		count := 1
		if input.Operation == "autocheckpoint" {
			count = 40
		}
		for i := 0; i < count; i++ {
			startPhase(input.Operation)
			var sub string
			must(db.QueryRow("SELECT id FROM subjects ORDER BY id LIMIT 1").Scan(&sub))
			body := []byte(fmt.Sprintf(`{"subject_id":%q,"namespace":"example.r3","schema_id":"example.r3","schema_version":1,"data":{"n":9007199254740993,"d":0.30,"e":1e2,"z":-0,"payload":%q}}`, sub, strings.Repeat("c", 48000)))
			key := fmt.Sprintf("checkpoint-%03d", i)
			result, err := s.CreateRecord(ctx, key, body)
			emit(map[string]any{"kind": "response", "ok": err == nil, "result_text": string(result.Data), "key": key, "body_text": string(body), "operation": "create"})
			if err != nil {
				block()
			}
		}
		if input.Operation == "checkpoint" {
			startPhase("checkpoint")
			var a, b, c int
			err = db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&a, &b, &c)
			emit(map[string]any{"kind": "response", "ok": err == nil && a == 0, "checkpoint": []int{a, b, c}})
		}
		block()
	}
	startPhase("mutation-" + input.Operation)
	result, err := mutate(s, input)
	if err != nil {
		diagnostic("worker-error", "failed", err)
		if *diagnosticOnly {
			must(err)
		}
	} else {
		completed()
	}
	emit(map[string]any{"kind": "response", "ok": err == nil, "result_text": string(result.Data), "contract": result.Contract})
	if *diagnosticOnly {
		startPhase("inspection")
		emit(map[string]any{"kind": "state", "value": inspect(db)})
		completed()
		startPhase("close")
		must(db.Close())
		completed()
		vfs.Report()
		emit(map[string]any{"kind": "done"})
		return
	}
	block()
}
func mutate(s *engine.Service, in Input) (engine.MutationResult, error) {
	switch in.Operation {
	case "create":
		return s.CreateRecord(ctx, in.Key, in.Body)
	case "restore":
		return s.RestoreRecord(ctx, in.ID, in.Target, in.Key, in.Body)
	case "migration", "fresh", "empty-m1":
		return engine.MutationResult{Data: json.RawMessage(`{"migration":3}`), Contract: "m2"}, nil
	default:
		return s.UpdateRecord(ctx, in.ID, in.Key, in.Body)
	}
}
func fixture(db *sql.DB) {
	s := engine.New(db)
	raw, err := s.CreateSubject(ctx, "subject", []byte(`{"kind":"project","display_name":"Synthetic R3"}`))
	must(err)
	var sub engine.Subject
	must(json.Unmarshal(raw, &sub))
	_, err = s.PublishSchema(ctx, "schema", []byte(`{"schema_id":"example.r3","schema_version":1,"definition":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"},"migration_policy":"explicit"}`))
	must(err)
	for i := 0; i < 2; i++ {
		body := []byte(fmt.Sprintf(`{"subject_id":%q,"namespace":"example.r3","schema_id":"example.r3","schema_version":1,"data":{"n":9007199254740993,"d":0.30,"e":1e2,"z":-0,"unknown":["雪",true],"payload":%q}}`, sub.ID, strings.Repeat("a", 12000)))
		result, err := s.CreateRecord(ctx, fmt.Sprintf("fixture-%d", i), body)
		must(err)
		var rec engine.Record
		must(json.Unmarshal(result.Data, &rec))
		_, err = s.UpdateRecord(ctx, rec.ID, fmt.Sprintf("fixture-patch-%d", i), []byte(fmt.Sprintf(`{"base_revision":1,"key":"synthetic","status":%q,"provenance":{"source":"synthetic"}}`, []string{"archived", "active"}[i])))
		must(err)
	}
	emit(map[string]any{"kind": "fixture", "settings": settings(db)})
}
func settings(db *sql.DB) map[string]any {
	startPhase("settings")
	result := map[string]any{}
	begin := func(q string) {
		query = q
		diagnostic("worker-progress", "start", nil)
		emit(map[string]any{"kind": "setting", "query": q, "status": "started"})
	}
	finish := func(q string, value any) {
		result[q] = value
		completed()
		emit(map[string]any{"kind": "setting", "query": q, "status": "complete", "value": value})
	}
	for _, q := range []string{"sqlite_version()", "sqlite_source_id()"} {
		begin(q)
		var x string
		must(db.QueryRow("SELECT " + q).Scan(&x))
		finish(q, x)
	}
	for _, q := range []string{"foreign_keys", "journal_mode", "synchronous", "busy_timeout", "wal_autocheckpoint", "page_size", "mmap_size"} {
		begin(q)
		var x any
		err := db.QueryRow("PRAGMA " + q).Scan(&x)
		// SQLite 3.53.4 PragTyp_MMAP_SIZE emits no row on FCNTL NOTFOUND
		// when SQLITE_MAX_MMAP_SIZE>0. Only our known shim diagnostic may
		// be unsupported. No numeric zero is inferred and every other error
		// (including a native-VFS no-row result) remains fatal.
		if q == "mmap_size" && instrumented && errors.Is(err, sql.ErrNoRows) {
			finish(q, map[string]any{"supported": false, "reason": "vfs-control-notfound-no-row"})
			continue
		}
		must(err)
		finish(q, x)
	}
	begin("compile_options")
	rows, err := db.Query("PRAGMA compile_options")
	must(err)
	var options []string
	for rows.Next() {
		var s string
		must(rows.Scan(&s))
		options = append(options, s)
	}
	must(rows.Err())
	must(rows.Close())
	finish("compile_options", options)
	begin("max_open_connections")
	finish("max_open_connections", db.Stats().MaxOpenConnections)
	query = ""
	completed()
	return result
}
func inspect(db *sql.DB) map[string]any {
	// This connection recovers the image but does NOT call storage.Open/Migrate.
	var integrity string
	must(db.QueryRow("PRAGMA integrity_check").Scan(&integrity))
	if integrity != "ok" {
		must(fmt.Errorf("integrity rejected"))
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	must(err)
	if rows.Next() {
		must(fmt.Errorf("foreign key rejected"))
	}
	must(rows.Err())
	must(rows.Close())
	conn, err := db.Conn(context.Background())
	must(err)
	defer conn.Close()
	_, err = conn.ExecContext(context.Background(), "BEGIN")
	must(err)
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	state := map[string]any{}
	for _, table := range []string{"subjects", "schemas", "records", "record_revisions", "mutation_audit", "idempotency_keys", "schema_migrations", "sqlite_schema"} {
		var present bool
		must(conn.QueryRowContext(context.Background(), "SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE type='table' AND name=?)", table).Scan(&present))
		if table == "sqlite_schema" {
			present = true
		}
		if !present {
			state[table] = []any{}
			continue
		}
		query := "SELECT * FROM " + table + " ORDER BY 1,2"
		if table == "sqlite_schema" {
			query = "SELECT type,name,tbl_name,sql FROM sqlite_schema WHERE substr(name,1,7) != 'sqlite_' ORDER BY name"
		}
		rows, err := conn.QueryContext(context.Background(), query)
		must(err)
		cols, err := rows.Columns()
		must(err)
		values := []map[string]any{}
		for rows.Next() {
			row := make([]any, len(cols))
			ptr := make([]any, len(cols))
			for i := range row {
				ptr[i] = &row[i]
			}
			must(rows.Scan(ptr...))
			v := map[string]any{}
			for i, col := range cols {
				if b, ok := row[i].([]byte); ok {
					v[col] = string(b)
				} else {
					v[col] = row[i]
				}
			}
			values = append(values, v)
		}
		must(rows.Err())
		must(rows.Close())
		state[table] = values
	}
	return state
}
