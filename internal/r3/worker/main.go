//go:build r3 && cgo

// This executable is a test process, never a Contextarium production command.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/estul26/Contextarium/internal/engine"
	"github.com/estul26/Contextarium/internal/r3/vfs"
	"github.com/estul26/Contextarium/internal/storage"
)

type Input struct {
	Batch              []Input
	Operation, ID, Key string
	Body               json.RawMessage
	Target             int64
}

var ctx = engine.WithAttribution(context.Background(), engine.Actor{Kind: "development_test", ID: "r3-worker"}, "req_00000000000000000000000000000300")

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "R3_WORKER_ERROR", err)
		os.Exit(2)
	}
}
func emit(x any) { must(json.NewEncoder(os.Stdout).Encode(x)) }
func block()     { emit(map[string]any{"kind": "done"}); var b [1]byte; os.Stdin.Read(b[:]); os.Exit(3) }
func main() {
	root := flag.String("root", "", "owned test directory")
	mode := flag.String("mode", "", "test role")
	cfg := flag.String("input", "", "input JSON")
	trace := flag.String("trace", "", "trace path")
	aim := flag.Int("target", 0, "I/O occurrence")
	fault := flag.String("fault", "none", "test fault")
	sector := flag.Int("sector", 4096, "modeled sector bytes")
	flag.Parse()
	path := filepath.Join(*root, "store.db")
	if *mode == "inspect" {
		db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path)+"?mode=rw&cache=private&_foreign_keys=on&_synchronous=FULL&_busy_timeout=1000")
		must(err)
		defer db.Close()
		db.SetMaxOpenConns(1)
		emit(inspect(db))
		return
	}
	if *mode == "probe" {
		must(vfs.Init(*root, *trace, *aim, *fault, *sector))
		vfs.Phase("model-validation")
		rc := vfs.Probe(filepath.Join(*root, "probe.db"))
		emit(map[string]any{"kind": "response", "ok": rc == 0, "code": rc})
		block()
	}
	if *mode == "fixture" {
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
	if *mode == "run" {
		must(vfs.Init(*root, *trace, *aim, *fault, *sector))
		vfs.Phase("open")
	}
	if input.Operation == "migration" || input.Operation == "fresh" || input.Operation == "empty-m1" {
		vfs.Phase(input.Operation)
	}
	db, err := storage.Open(ctx, path)
	if err != nil {
		emit(map[string]any{"kind": "response", "ok": false, "error": "open-unavailable"})
		block()
	}
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
		vfs.Phase("checkpoint-prepare")
		count := 1
		if input.Operation == "autocheckpoint" {
			count = 40
		}
		for i := 0; i < count; i++ {
			vfs.Phase(input.Operation)
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
			vfs.Phase("checkpoint")
			var a, b, c int
			err = db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&a, &b, &c)
			emit(map[string]any{"kind": "response", "ok": err == nil && a == 0, "checkpoint": []int{a, b, c}})
		}
		block()
	}
	vfs.Phase("mutation-" + input.Operation)
	result, err := mutate(s, input)
	emit(map[string]any{"kind": "response", "ok": err == nil, "result_text": string(result.Data), "contract": result.Contract})
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
	result := map[string]any{}
	for _, q := range []string{"sqlite_version()", "sqlite_source_id()"} {
		var x string
		must(db.QueryRow("SELECT " + q).Scan(&x))
		result[q] = x
	}
	for _, q := range []string{"foreign_keys", "journal_mode", "synchronous", "busy_timeout", "wal_autocheckpoint", "page_size", "mmap_size"} {
		var x any
		must(db.QueryRow("PRAGMA " + q).Scan(&x))
		result[q] = x
	}
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
	result["compile_options"] = options
	result["max_open_connections"] = db.Stats().MaxOpenConnections
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
