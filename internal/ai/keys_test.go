package ai

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ohchanwu/job-scraper/internal/appdata"
)

func TestKeyStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai_keys.json")
	want := map[string]string{"anthropic": "sk-ant-x", "openai": "sk-oa-y"}
	if err := SaveKeys(path, want); err != nil {
		t.Fatalf("SaveKeys: %v", err)
	}
	got, err := LoadKeys(path)
	if err != nil {
		t.Fatalf("LoadKeys: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: got %v, want %v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != keysFileMode {
		t.Fatalf("key file perm = %o, want %o (a paid API key must not be world-readable)", perm, keysFileMode)
	}
}

func TestLoadKeysMissingFileIsEmptyNotError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	got, err := LoadKeys(path)
	if err != nil {
		t.Fatalf("LoadKeys on a missing file must be a silent fallback, got error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("missing file should yield an empty map, got %v", got)
	}
}

// TestSaveKeysOverwritePreserves0600 catches the os.WriteFile trap where
// overwriting an existing file does not re-apply the permission bits.
func TestSaveKeysOverwritePreserves0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai_keys.json")
	if err := SaveKeys(path, map[string]string{"anthropic": "sk-1"}); err != nil {
		t.Fatalf("first SaveKeys: %v", err)
	}
	// Loosen the perms, then overwrite — the rewrite must restore 0600.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := SaveKeys(path, map[string]string{"anthropic": "sk-2", "openai": "sk-3"}); err != nil {
		t.Fatalf("overwrite SaveKeys: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != keysFileMode {
		t.Fatalf("after overwrite, perm = %o, want %o", perm, keysFileMode)
	}
	got, err := LoadKeys(path)
	if err != nil {
		t.Fatalf("LoadKeys after overwrite: %v", err)
	}
	if got["anthropic"] != "sk-2" || got["openai"] != "sk-3" {
		t.Fatalf("overwrite content = %v, want updated keys", got)
	}
}

func TestLoadKeysEmptyFileIsEmptyMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai_keys.json")
	if err := os.WriteFile(path, nil, keysFileMode); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	got, err := LoadKeys(path)
	if err != nil {
		t.Fatalf("LoadKeys empty file: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty file should yield empty map, got %v", got)
	}
}

func TestDefaultKeysPathUsesCanonicalApplicationDirectory(t *testing.T) {
	root := t.TempDir()
	got, err := defaultKeysPath(func() (string, error) { return root, nil })
	if err != nil {
		t.Fatalf("defaultKeysPath: %v", err)
	}
	want := filepath.Join(appdata.Dir(root), "ai_keys.json")
	if got != want {
		t.Fatalf("defaultKeysPath = %q, want %q", got, want)
	}
}
