package sqlitesnapshot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCreateRejectsMissingOrNonRegularSource(t *testing.T) {
	workDir := t.TempDir()
	for _, sourcePath := range []string{
		filepath.Join(t.TempDir(), "missing.db"),
		t.TempDir(),
	} {
		if _, err := Create(context.Background(), sourcePath, workDir); err == nil {
			t.Fatalf("Create(%q) succeeded for missing or non-regular source", sourcePath)
		}
	}
}

func TestCreateRejectsActiveWriter(t *testing.T) {
	sourcePath, db := createWALSource(t)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), `BEGIN IMMEDIATE`); err != nil {
		t.Fatalf("begin writer transaction: %v", err)
	}
	defer conn.ExecContext(context.Background(), `ROLLBACK`)

	started := time.Now()
	if _, err := Create(context.Background(), sourcePath, t.TempDir()); err == nil {
		t.Fatal("Create succeeded while another connection held a write transaction")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Create waited %s for active writer, want no-wait refusal", elapsed)
	}
}

func TestCreateIncludesCommittedWALRows(t *testing.T) {
	sourcePath, db := createWALSource(t)
	if _, err := db.Exec(`INSERT INTO snapshot_rows (value) VALUES ('committed-in-wal')`); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sourcePath + "-wal"); err != nil {
		t.Fatalf("expected WAL file: %v", err)
	}

	snapshot, err := Create(context.Background(), sourcePath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(snapshot.Path)

	snapshotDB := openSQLite(t, snapshot.Path)
	defer snapshotDB.Close()
	var count int
	if err := snapshotDB.QueryRow(`SELECT count(*) FROM snapshot_rows WHERE value = 'committed-in-wal'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("committed WAL rows = %d, want 1", count)
	}
}

func TestCreateProducesStableFingerprintForSameLogicalSource(t *testing.T) {
	sourcePath, _ := createWALSource(t)
	workDir := t.TempDir()

	first, err := Create(context.Background(), sourcePath, workDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(first.Path)
	second, err := Create(context.Background(), sourcePath, workDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(second.Path)

	if first.SHA256 != second.SHA256 {
		t.Fatalf("fingerprints differ for unchanged source: %s != %s", first.SHA256, second.SHA256)
	}
	if len(first.SHA256) != sha256.Size*2 || strings.ToLower(first.SHA256) != first.SHA256 {
		t.Fatalf("fingerprint = %q, want 64 lowercase hex characters", first.SHA256)
	}
}

func TestCreateSnapshotPassesQuickCheck(t *testing.T) {
	sourcePath, _ := createWALSource(t)
	snapshot, err := Create(context.Background(), sourcePath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(snapshot.Path)

	db := openSQLite(t, snapshot.Path)
	defer db.Close()
	var result string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&result); err != nil {
		t.Fatal(err)
	}
	if result != "ok" {
		t.Fatalf("quick_check = %q, want ok", result)
	}
}

func TestCreateHandlesURICharactersInSourcePath(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source?#.db")
	db := openSQLite(t, sourcePath)
	if _, err := db.Exec(`CREATE TABLE uri_path_rows (value TEXT NOT NULL); INSERT INTO uri_path_rows VALUES ('ok')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Create(context.Background(), sourcePath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(snapshot.Path)
	snapshotDB := openSQLite(t, snapshot.Path)
	defer snapshotDB.Close()
	var value string
	if err := snapshotDB.QueryRow(`SELECT value FROM uri_path_rows`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "ok" {
		t.Fatalf("snapshot URI-path row = %q, want ok", value)
	}
}

func TestCreatePreservesDurableSourceFilesSchemaAndRows(t *testing.T) {
	sourcePath, db := createWALSource(t)
	if _, err := db.Exec(`INSERT INTO snapshot_rows (value) VALUES ('keep-wal-open')`); err != nil {
		t.Fatal(err)
	}
	beforeFiles := durableSourceFileState(t, sourcePath)
	beforeSchema, beforeRows := sourceSchemaAndRows(t, db)

	snapshot, err := Create(context.Background(), sourcePath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(snapshot.Path)
	afterFiles := durableSourceFileState(t, sourcePath)
	afterSchema, afterRows := sourceSchemaAndRows(t, db)
	if !reflect.DeepEqual(afterFiles, beforeFiles) {
		t.Fatalf("durable source file state changed:\nbefore: %#v\nafter:  %#v", beforeFiles, afterFiles)
	}
	if afterSchema != beforeSchema || !reflect.DeepEqual(afterRows, beforeRows) {
		t.Fatalf("source logical state changed: schema %q -> %q, rows %v -> %v", beforeSchema, afterSchema, beforeRows, afterRows)
	}
}

func TestCreateRemovesPartialSnapshotOnFailure(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "corrupt.db")
	if err := os.WriteFile(sourcePath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	if _, err := Create(context.Background(), sourcePath, workDir); err == nil {
		t.Fatal("Create succeeded for corrupt SQLite source")
	}
	matches, err := filepath.Glob(filepath.Join(workDir, "jobcron-sqlite-snapshot-*.db"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("partial snapshots remain after failure: %v", matches)
	}
}

func createWALSource(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.db")
	db := openSQLite(t, path)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE snapshot_rows (id INTEGER PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO snapshot_rows (value) VALUES ('base')`); err != nil {
		t.Fatal(err)
	}
	return path, db
}

func openSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	db, err := sql.Open("sqlite", uri+"?_pragma=busy_timeout(0)")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func durableSourceFileState(t *testing.T, sourcePath string) map[string]string {
	t.Helper()
	state := make(map[string]string, 2)
	// SQLite documents -shm as a non-persistent, rebuildable WAL index. Readers
	// maintain coordination bytes there, so the durable byte contract covers the
	// database and WAL while schema/row assertions cover logical source safety.
	for _, path := range []string{sourcePath, sourcePath + "-wal"} {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			state[filepath.Base(path)] = fmt.Sprintf("%x", sha256.Sum256(data))
		case os.IsNotExist(err):
			state[filepath.Base(path)] = "missing"
		default:
			t.Fatalf("read source state %s: %v", path, err)
		}
	}
	return state
}

func sourceSchemaAndRows(t *testing.T, db *sql.DB) (string, []string) {
	t.Helper()
	var schema string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'snapshot_rows'`).Scan(&schema); err != nil {
		t.Fatalf("read source schema: %v", err)
	}
	rows, err := db.Query(`SELECT value FROM snapshot_rows ORDER BY id`)
	if err != nil {
		t.Fatalf("read source rows: %v", err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan source row: %v", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate source rows: %v", err)
	}
	return schema, values
}
