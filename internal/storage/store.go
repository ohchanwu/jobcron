package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	_ "github.com/jackc/pgx/v5/stdlib" // pure-Go PostgreSQL driver, registered as "pgx"
	"github.com/ohchanwu/job-scraper/internal/appdata"
	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed postgres_migrations/*.sql
var postgresMigrationsFS embed.FS

// Store is the job-scraper persistence layer: a single concrete handle over
// the configured SQL database, with every repository method hanging off it.
type Store struct {
	db      *sql.DB
	dialect Dialect
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

// OpenSQLiteAt opens a SQLite database at an explicit path.
func OpenSQLiteAt(path string) (*Store, error) {
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
	return &Store{db: db, dialect: DialectSQLite}, nil
}

// OpenPostgres opens a PostgreSQL database URL and applies pending PostgreSQL
// migrations.
func OpenPostgres(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("storage: open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: open postgres: %w", err)
	}
	if err := migratePostgres(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, dialect: DialectPostgres}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Dialect returns the SQL backend used by this store.
func (s *Store) Dialect() Dialect { return s.dialect }

// SQLDB returns the underlying database handle for command-line maintenance
// tools that need table-level operations outside the app's runtime methods.
func (s *Store) SQLDB() *sql.DB { return s.db }

// DefaultDBPath is the database path under the user's OS config directory,
// e.g. ~/Library/Application Support/jobcron/jobs.db on macOS.
func DefaultDBPath() (string, error) {
	return defaultDBPath(os.UserConfigDir)
}

func defaultDBPath(userConfigDir func() (string, error)) (string, error) {
	root, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("storage: locate user config dir: %w", err)
	}
	return filepath.Join(appdata.Dir(root), "jobs.db"), nil
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

func migratePostgres(db *sql.DB) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		return fmt.Errorf("storage: ensure postgres schema_migrations: %w", err)
	}
	entries, err := fs.ReadDir(postgresMigrationsFS, "postgres_migrations")
	if err != nil {
		return fmt.Errorf("storage: read postgres migrations: %w", err)
	}
	for _, e := range entries {
		version, err := strconv.Atoi(e.Name()[:4])
		if err != nil {
			return fmt.Errorf("storage: postgres migration %q: name must start with a 4-digit version", e.Name())
		}
		var applied int
		if err := db.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = $1`, version).Scan(&applied); err == nil {
			continue
		} else if err != sql.ErrNoRows {
			return fmt.Errorf("storage: check postgres migration %q: %w", e.Name(), err)
		}
		stmts, err := postgresMigrationsFS.ReadFile("postgres_migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("storage: read postgres migration %q: %w", e.Name(), err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("storage: begin postgres migration %q: %w", e.Name(), err)
		}
		if _, err := tx.Exec(string(stmts)); err != nil {
			tx.Rollback()
			return fmt.Errorf("storage: apply postgres migration %q: %w", e.Name(), err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES ($1, now())`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("storage: record postgres migration %q: %w", e.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("storage: commit postgres migration %q: %w", e.Name(), err)
		}
	}
	return nil
}
