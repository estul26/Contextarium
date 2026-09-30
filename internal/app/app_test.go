package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estul26/Contextarium/internal/config"
	"github.com/estul26/Contextarium/internal/storage"
)

func TestRejectsUnsafeConfigurationBeforeCreatingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "must-not-exist.db")
	err := Run(context.Background(), config.Config{ListenAddr: "0.0.0.0:0", DBPath: path}, slog.Default())
	if err == nil {
		t.Fatal("unsafe programmatic configuration accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid startup created a database")
	}
}

func TestCancelledStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "must-not-exist.db")
	err := Run(ctx, config.Config{ListenAddr: "127.0.0.1:0", DBPath: path}, slog.Default())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled startup created a database")
	}
}

func TestNewerSchemaFailsStartupWithoutReadyLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SYNTHETIC_PRIVATE_PATH.db")
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO schema_migrations(version,name,checksum) VALUES(2147483647,'future','test')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var logs bytes.Buffer
	err = Run(context.Background(), config.Config{ListenAddr: "127.0.0.1:0", DBPath: path}, slog.New(slog.NewJSONHandler(&logs, nil)))
	if !errors.Is(err, storage.ErrIncompatibleSchema) {
		t.Fatalf("expected incompatible schema: %v", err)
	}
	if strings.Contains(logs.String(), "application ready") || strings.Contains(logs.String(), "SYNTHETIC_PRIVATE_PATH") {
		t.Fatalf("unsafe startup logging: %s", logs.String())
	}
}

func TestListenerFailureClosesStartupResources(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	path := filepath.Join(t.TempDir(), "test.db")
	var logs bytes.Buffer
	err = Run(context.Background(), config.Config{ListenAddr: ln.Addr().String(), DBPath: path}, slog.New(slog.NewJSONHandler(&logs, nil)))
	var listenErr *net.OpError
	if !errors.As(err, &listenErr) || listenErr.Op != "listen" {
		t.Fatalf("expected listener initialization failure, got %v", err)
	}
	if strings.Contains(logs.String(), "application ready") {
		t.Fatal("failed startup reported ready")
	}
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("database unusable after failed startup: %v", err)
	}
	defer db.Close()
	// Reopening alone permits a leaked connection. Leaving WAL mode requires an
	// exclusive database lock, which an unclosed startup connection would block.
	// See https://sqlite.org/wal.html#backwards_compatibility.
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=DELETE").Scan(&mode); err != nil {
		t.Fatalf("startup did not release its SQLite connection: %v", err)
	}
	if mode != "delete" {
		t.Fatalf("could not acquire exclusive database access: journal mode = %q", mode)
	}
}
