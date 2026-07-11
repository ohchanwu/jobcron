package appdata

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareDirectoryStates(t *testing.T) {
	tests := []struct {
		name          string
		makeLegacy    bool
		makeCanonical bool
		wantErr       string
		wantLegacy    bool
		wantCanonical bool
	}{
		{
			name:          "neither directory exists",
			wantLegacy:    false,
			wantCanonical: false,
		},
		{
			name:          "canonical directory exists",
			makeCanonical: true,
			wantLegacy:    false,
			wantCanonical: true,
		},
		{
			name:          "legacy directory exists",
			makeLegacy:    true,
			wantLegacy:    false,
			wantCanonical: true,
		},
		{
			name:          "both directories exist",
			makeLegacy:    true,
			makeCanonical: true,
			wantErr:       "both",
			wantLegacy:    true,
			wantCanonical: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.makeLegacy {
				if err := os.MkdirAll(LegacyDir(root), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if tt.makeCanonical {
				if err := os.MkdirAll(Dir(root), 0o700); err != nil {
					t.Fatal(err)
				}
			}

			err := Prepare(root)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Prepare error = %v, want substring %q", err, tt.wantErr)
			}
			assertDirectoryExists(t, LegacyDir(root), tt.wantLegacy)
			assertDirectoryExists(t, Dir(root), tt.wantCanonical)
		})
	}
}

func TestPrepareMigratesLegacyDirectoryAtomically(t *testing.T) {
	root := t.TempDir()
	legacy := LegacyDir(root)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"jobs.db", "jobs.db-wal", "jobs.db-shm", "jobs.db.bak", "ai_keys.json"} {
		if err := os.WriteFile(filepath.Join(legacy, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := Prepare(root); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy still exists: %v", err)
	}
	for _, name := range []string{"jobs.db", "jobs.db-wal", "jobs.db-shm", "jobs.db.bak", "ai_keys.json"} {
		if got, err := os.ReadFile(filepath.Join(Dir(root), name)); err != nil || string(got) != name {
			t.Fatalf("%s: got=%q err=%v", name, got, err)
		}
	}
}

func TestPrepareRejectsDirectoryCollision(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{LegacyDir(root), Dir(root)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	if err := Prepare(root); err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("Prepare error = %v, want collision", err)
	}
}

func TestPrepareReturnsRenameFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(LegacyDir(root), 0o700); err != nil {
		t.Fatal(err)
	}
	want := errors.New("rename denied")

	err := prepare(root, func(string, string) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("prepare error = %v, want wrapped %v", err, want)
	}
	assertDirectoryExists(t, LegacyDir(root), true)
	assertDirectoryExists(t, Dir(root), false)
}

func assertDirectoryExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	got := err == nil
	if got != want {
		t.Fatalf("directory %q exists = %t, want %t (err = %v)", path, got, want, err)
	}
}
