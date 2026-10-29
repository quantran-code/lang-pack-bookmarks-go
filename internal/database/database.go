// Package database opens the SQLite database, applies the goose migrations, and holds the
// sqlc-generated query layer (dbgen).
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MemoryDSN is an in-memory SQLite database. Every call to Open gets its own, because the pool
// below holds a single connection and an in-memory database lives and dies with its connection.
const MemoryDSN = ":memory:"

// Open connects to dsn with the pure-Go "sqlite" driver and applies every pending goose
// migration. The pool is limited to one connection: SQLite allows one writer at a time anyway, and
// it keeps an in-memory database from being split across connections.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	db.SetMaxOpenConns(1)

	// SQLite only enforces ON DELETE CASCADE when foreign keys are switched on, and the pragma is
	// per connection, so it is set on the one pooled connection before anything else uses it.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	if err := migrateUp(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrateUp(db *sql.DB) error {
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("reading migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations)
	if err != nil {
		return fmt.Errorf("creating migration provider: %w", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	return nil
}
