package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
)

//go:embed migrations/001_foundation.sql
var foundationSQL string

type migration struct {
	name string
	sql  string
}

// Position defines the version. Never edit a released migration; append instead.
var migrations = []migration{{name: "foundation", sql: foundationSQL}}

var ErrIncompatibleSchema = errors.New("database schema is incompatible with this binary")

func Migrate(ctx context.Context, db *sql.DB) error {
	return migrate(ctx, db, migrations)
}

func migrate(ctx context.Context, db *sql.DB, steps []migration) error {
	if len(steps) == 0 {
		return errors.New("no migrations configured")
	}
	// Open configures BEGIN IMMEDIATE, serializing inspection and migration writes.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name = 'schema_migrations')").Scan(&exists); err != nil {
		return fmt.Errorf("inspect migration metadata: %w", err)
	}
	applied := 0
	if exists {
		rows, err := tx.QueryContext(ctx, "SELECT version, name, checksum FROM schema_migrations ORDER BY version")
		if err != nil {
			return fmt.Errorf("%w: unreadable migration ledger", ErrIncompatibleSchema)
		}
		defer rows.Close()
		for rows.Next() {
			var version int
			var name, checksum string
			if err := rows.Scan(&version, &name, &checksum); err != nil {
				return fmt.Errorf("%w: malformed migration ledger", ErrIncompatibleSchema)
			}
			if version != applied+1 || version > len(steps) {
				return fmt.Errorf("%w: unexpected migration version %d", ErrIncompatibleSchema, version)
			}
			if name != steps[applied].name || checksum != migrationChecksum(steps[applied]) {
				return fmt.Errorf("%w: migration %d differs from the binary", ErrIncompatibleSchema, version)
			}
			applied++
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read migration ledger: %w", err)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if applied == 0 {
			return fmt.Errorf("%w: empty migration ledger", ErrIncompatibleSchema)
		}
	} else {
		var objects int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE substr(name, 1, 7) != 'sqlite_'").Scan(&objects); err != nil {
			return err
		}
		if objects != 0 {
			return fmt.Errorf("%w: nonempty database without migration ledger", ErrIncompatibleSchema)
		}
	}
	for i := applied; i < len(steps); i++ {
		if _, err := tx.ExecContext(ctx, steps[i].sql); err != nil {
			return fmt.Errorf("apply migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, name, checksum) VALUES (?, ?, ?)", i+1, steps[i].name, migrationChecksum(steps[i])); err != nil {
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func migrationChecksum(m migration) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(m.sql)))
}

// CheckReady reads real infrastructure metadata, rather than just SELECT 1.
func CheckReady(ctx context.Context, db *sql.DB) error {
	var count, version int
	if err := db.QueryRowContext(ctx, "SELECT count(*), COALESCE(max(version), 0) FROM schema_migrations").Scan(&count, &version); err != nil {
		return err
	}
	if count != len(migrations) || version != len(migrations) {
		return ErrIncompatibleSchema
	}
	return nil
}
