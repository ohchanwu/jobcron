package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store is the job-scraper persistence layer: a single concrete handle over
// the SQLite database, with every repository method hanging off it.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the job-scraper database under the user's
// OS config directory and applies any pending migrations.
func Open() (*Store, error) {
	path, err := DefaultDBPath()
	if err != nil {
		return nil, err
	}
	return OpenAt(path)
}

// OpenAt opens the database at an explicit path — creating the parent
// directory and the file if needed — and applies any pending migrations.
func OpenAt(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("storage: create db directory: %w", err)
	}
	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", path, err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// DefaultDBPath is the database path under the user's OS config directory,
// e.g. ~/Library/Application Support/job-scraper/jobs.db on macOS.
func DefaultDBPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("storage: locate user config dir: %w", err)
	}
	return filepath.Join(dir, "job-scraper", "jobs.db"), nil
}

// migrate applies every embedded migration whose version is newer than the
// database's current PRAGMA user_version, in ascending order. Each migration
// file is named NNNN_description.sql, where NNNN is its version.
func migrate(db *sql.DB) error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("storage: read migrations: %w", err)
	}
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("storage: read schema version: %w", err)
	}
	for _, e := range entries {
		version, err := strconv.Atoi(e.Name()[:4])
		if err != nil {
			return fmt.Errorf("storage: migration %q: name must start with a 4-digit version", e.Name())
		}
		if version <= current {
			continue
		}
		stmts, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("storage: read migration %q: %w", e.Name(), err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("storage: begin migration %q: %w", e.Name(), err)
		}
		if _, err := tx.Exec(string(stmts)); err != nil {
			tx.Rollback()
			return fmt.Errorf("storage: apply migration %q: %w", e.Name(), err)
		}
		// PRAGMA user_version cannot be parameterized.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			tx.Rollback()
			return fmt.Errorf("storage: bump schema version for %q: %w", e.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("storage: commit migration %q: %w", e.Name(), err)
		}
	}
	return nil
}
