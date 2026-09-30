// Package storage owns the SQLite lifecycle and embedded migrations.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const busyTimeoutMS = 1000

// Open creates/opens the database, verifies connection policy and migrates it.
// The caller owns the returned pool and must close it after HTTP shutdown.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := openDatabase(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := Migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func openDatabase(ctx context.Context, path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" || path == ":memory:" || strings.HasPrefix(path, "file:") || strings.ContainsRune(path, 0) {
		return nil, errors.New("database requires a filesystem path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		if err := f.Close(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("create database file: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("database path must be a regular file, not a symlink or directory")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	q := url.Values{
		"mode": {"rw"}, "cache": {"private"},
		"_foreign_keys": {"on"}, "_journal_mode": {"WAL"},
		"_synchronous": {"FULL"}, "_busy_timeout": {fmt.Sprint(busyTimeoutMS)},
		"_txlock": {"immediate"},
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := verifyConnection(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func verifyConnection(ctx context.Context, db *sql.DB) error {
	// These statements are compile-time constants, never client-supplied SQL.
	for _, check := range []struct {
		query string
		want  int
	}{
		{"PRAGMA foreign_keys", 1},
		{"PRAGMA synchronous", 2},
		{"PRAGMA busy_timeout", busyTimeoutMS},
	} {
		var got int
		if err := db.QueryRowContext(ctx, check.query).Scan(&got); err != nil {
			return fmt.Errorf("initialize SQLite: %w", err)
		}
		if got != check.want {
			return errors.New("SQLite connection policy could not be established")
		}
	}
	var journal string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
		return err
	}
	if journal != "wal" {
		return errors.New("SQLite WAL mode is unavailable")
	}
	return nil
}
