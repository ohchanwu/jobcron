// Package sqlitesnapshot creates immutable, verified snapshots of legacy
// SQLite databases for one-time import.
package sqlitesnapshot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	sqlite "modernc.org/sqlite"
)

// Snapshot is a private working copy and its complete-file SHA-256 digest.
type Snapshot struct {
	Path   string
	SHA256 string
}

type backuper interface {
	NewBackup(string) (*sqlite.Backup, error)
}

// Create refuses an active writer, takes an online backup, verifies it, and
// returns the snapshot for the caller to remove.
func Create(ctx context.Context, sourcePath, workDir string) (_ Snapshot, retErr error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("inspect sqlite source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Snapshot{}, errors.New("sqlite source is not a regular file")
	}

	placeholder, err := os.CreateTemp(workDir, "jobcron-sqlite-snapshot-*.db")
	if err != nil {
		return Snapshot{}, fmt.Errorf("reserve sqlite snapshot: %w", err)
	}
	snapshotPath := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		_ = os.Remove(snapshotPath)
		return Snapshot{}, fmt.Errorf("close sqlite snapshot placeholder: %w", err)
	}
	if err := os.Remove(snapshotPath); err != nil {
		return Snapshot{}, fmt.Errorf("prepare sqlite snapshot path: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = os.Remove(snapshotPath)
			_ = os.Remove(snapshotPath + "-journal")
			_ = os.Remove(snapshotPath + "-wal")
			_ = os.Remove(snapshotPath + "-shm")
		}
	}()

	db, err := sql.Open("sqlite", sqliteFileURI(sourcePath, url.Values{
		"mode":    {"rw"},
		"_pragma": {"busy_timeout(0)"},
	}))
	if err != nil {
		return Snapshot{}, fmt.Errorf("open sqlite source: %w", err)
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return Snapshot{}, fmt.Errorf("connect sqlite source: %w", err)
	}
	locked := false
	defer func() {
		if locked {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
		_ = conn.Close()
		_ = db.Close()
	}()

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return Snapshot{}, fmt.Errorf("refuse sqlite source with active writer: %w", err)
	}
	locked = true
	backupDB, err := sql.Open("sqlite", sqliteFileURI(sourcePath, url.Values{
		"mode":    {"ro"},
		"_pragma": {"busy_timeout(0)"},
	}))
	if err != nil {
		return Snapshot{}, fmt.Errorf("open read-only sqlite backup source: %w", err)
	}
	backupConn, err := backupDB.Conn(ctx)
	if err != nil {
		_ = backupDB.Close()
		return Snapshot{}, fmt.Errorf("connect sqlite source for backup: %w", err)
	}
	backupErr := backupConn.Raw(func(driverConn any) error {
		source, ok := driverConn.(backuper)
		if !ok {
			return errors.New("sqlite driver does not support online backup")
		}
		backup, err := source.NewBackup(snapshotPath)
		if err != nil {
			return err
		}
		_, stepErr := backup.Step(-1)
		finishErr := backup.Finish()
		return errors.Join(stepErr, finishErr)
	})
	backupConnErr := backupConn.Close()
	backupDBErr := backupDB.Close()
	if err := errors.Join(backupErr, backupConnErr, backupDBErr); err != nil {
		return Snapshot{}, fmt.Errorf("backup and close sqlite source: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `ROLLBACK`); err != nil {
		return Snapshot{}, fmt.Errorf("release sqlite source lock: %w", err)
	}
	locked = false
	if err := conn.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("close sqlite source connection: %w", err)
	}
	if err := db.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("close sqlite source: %w", err)
	}

	if err := quickCheck(ctx, snapshotPath); err != nil {
		return Snapshot{}, err
	}
	file, err := os.Open(snapshotPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open sqlite snapshot for hashing: %w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return Snapshot{}, fmt.Errorf("hash sqlite snapshot: %w", err)
	}
	if err := os.Chmod(snapshotPath, 0o400); err != nil {
		return Snapshot{}, fmt.Errorf("make sqlite snapshot read-only: %w", err)
	}
	return Snapshot{Path: snapshotPath, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func quickCheck(ctx context.Context, snapshotPath string) error {
	db, err := sql.Open("sqlite", sqliteFileURI(snapshotPath, url.Values{
		"mode":      {"ro"},
		"immutable": {"1"},
	}))
	if err != nil {
		return fmt.Errorf("open sqlite snapshot for verification: %w", err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&result); err != nil {
		return fmt.Errorf("quick-check sqlite snapshot: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("quick-check sqlite snapshot: %s", result)
	}
	return nil
}

func sqliteFileURI(path string, query url.Values) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: query.Encode()}
	return u.String()
}
