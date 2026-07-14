package credential

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestParseMasterKey(t *testing.T) {
	if MasterKeyBytes != 32 {
		t.Fatalf("MasterKeyBytes = %d, want 32", MasterKeyBytes)
	}
	valid := bytes.Repeat([]byte{0x42}, 32)
	encodedValid := base64.StdEncoding.EncodeToString(valid)

	t.Run("valid", func(t *testing.T) {
		got, err := ParseMasterKey(encodedValid)
		if err != nil {
			t.Fatalf("ParseMasterKey: %v", err)
		}
		if !bytes.Equal(got, valid) {
			t.Fatalf("ParseMasterKey = %x, want %x", got, valid)
		}
	})

	t.Run("surrounding whitespace", func(t *testing.T) {
		got, err := ParseMasterKey(" \n\t" + encodedValid + "\r\n")
		if err != nil {
			t.Fatalf("ParseMasterKey: %v", err)
		}
		if !bytes.Equal(got, valid) {
			t.Fatalf("ParseMasterKey = %x, want %x", got, valid)
		}
	})

	tests := []struct {
		name    string
		encoded string
	}{
		{name: "malformed base64", encoded: "not-valid-base64!"},
		{name: "empty decoded key", encoded: base64.StdEncoding.EncodeToString(nil)},
		{name: "sixteen bytes", encoded: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 16))},
		{name: "thirty-one bytes", encoded: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 31))},
		{name: "thirty-three bytes", encoded: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 33))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMasterKey(tt.encoded)
			if err == nil {
				t.Fatal("ParseMasterKey succeeded, want error")
			}
			assertErrorOmits(t, err, tt.encoded)
			if decoded, decodeErr := base64.StdEncoding.DecodeString(tt.encoded); decodeErr == nil {
				assertErrorOmits(t, err, string(decoded), base64.StdEncoding.EncodeToString(decoded))
			}
		})
	}
}

func TestLoadOrCreateLocalMasterKeyAtUsesSecureRandomness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential-encryption.key")
	got, err := LoadOrCreateLocalMasterKeyAt(path)
	if err != nil {
		t.Fatalf("LoadOrCreateLocalMasterKeyAt: %v", err)
	}
	if len(got) != MasterKeyBytes {
		t.Fatalf("key length = %d, want %d", len(got), MasterKeyBytes)
	}
}

func TestDefaultMasterKeyPathUsesCanonicalApplicationDirectory(t *testing.T) {
	got, err := defaultMasterKeyPath(func() (string, error) { return "/config-root", nil })
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/config-root", "jobcron", "credential-encryption.key")
	if got != want {
		t.Fatalf("defaultMasterKeyPath = %q, want %q", got, want)
	}
}

func TestDefaultMasterKeyPathPropagatesConfigDirectoryError(t *testing.T) {
	wantErr := errors.New("config directory unavailable")
	_, err := defaultMasterKeyPath(func() (string, error) { return "", wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("defaultMasterKeyPath error = %v, want wrapped %v", err, wantErr)
	}
}

func TestLoadOrCreateLocalMasterKeyCreatesProtectedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "jobcron", "credential-encryption.key")
	want := bytes.Repeat([]byte{0x42}, 32)

	got, err := loadOrCreateLocalMasterKeyAt(path, bytes.NewReader(want))
	if err != nil {
		t.Fatalf("loadOrCreateLocalMasterKeyAt: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("created key = %x, want %x", got, want)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	if bytes.Contains(contents, want) {
		t.Fatal("key file contains raw master-key bytes")
	}
	parsed, err := ParseMasterKey(string(contents))
	if err != nil {
		t.Fatalf("parse key file: %v", err)
	}
	if !bytes.Equal(parsed, want) {
		t.Fatalf("parsed key = %x, want %x", parsed, want)
	}

	if runtime.GOOS != "windows" {
		assertFileMode(t, filepath.Dir(path), 0o700)
		assertFileMode(t, path, 0o600)
	}
}

func TestLoadOrCreateLocalMasterKeyReusesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential-encryption.key")
	want := bytes.Repeat([]byte{0x42}, 32)
	if _, err := loadOrCreateLocalMasterKeyAt(path, bytes.NewReader(want)); err != nil {
		t.Fatalf("first loadOrCreateLocalMasterKeyAt: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	got, err := loadOrCreateLocalMasterKeyAt(path, errorReader{})
	if err != nil {
		t.Fatalf("second loadOrCreateLocalMasterKeyAt: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("reused key = %x, want %x", got, want)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("existing key file was rewritten: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func TestLoadOrCreateLocalMasterKeyRejectsInvalidExistingFileWithoutReplacement(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{name: "malformed", contents: "sensitive-invalid-file-contents"},
		{name: "wrong length", contents: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 31))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credential-encryption.key")
			if err := os.WriteFile(path, []byte(tt.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := loadOrCreateLocalMasterKeyAt(path, bytes.NewReader(bytes.Repeat([]byte{0x42}, 32)))
			if err == nil {
				t.Fatal("loadOrCreateLocalMasterKeyAt succeeded, want error")
			}
			assertErrorOmits(t, err, tt.contents)
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != tt.contents {
				t.Fatalf("invalid file was replaced: got %q, want %q", got, tt.contents)
			}
		})
	}
}

func TestLoadOrCreateLocalMasterKeyRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	path := filepath.Join(t.TempDir(), "credential-encryption.key")
	contents := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	_, err := loadOrCreateLocalMasterKeyAt(path, errorReader{})
	if err == nil {
		t.Fatal("loadOrCreateLocalMasterKeyAt succeeded for group-readable file")
	}
	assertErrorOmits(t, err, contents)
}

func TestLoadOrCreateLocalMasterKeyConcurrentCreationConverges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential-encryption.key")
	start := make(chan struct{})
	keys := make(chan []byte, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	for _, fill := range []byte{0x42, 0x24} {
		wg.Add(1)
		go func(fill byte) {
			defer wg.Done()
			<-start
			key, err := loadOrCreateLocalMasterKeyAt(path, bytes.NewReader(bytes.Repeat([]byte{fill}, 32)))
			if err != nil {
				errs <- err
				return
			}
			keys <- key
		}(fill)
	}
	close(start)
	wg.Wait()
	close(keys)
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent loadOrCreateLocalMasterKeyAt: %v", err)
	}
	var gotKeys [][]byte
	for key := range keys {
		gotKeys = append(gotKeys, key)
	}
	if len(gotKeys) != 2 {
		t.Fatalf("returned key count = %d, want 2", len(gotKeys))
	}
	if !bytes.Equal(gotKeys[0], gotKeys[1]) {
		t.Fatalf("concurrent callers received different keys: %x and %x", gotKeys[0], gotKeys[1])
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persistedKey, err := ParseMasterKey(string(persisted))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotKeys[0], persistedKey) {
		t.Fatalf("returned key %x differs from persisted key %x", gotKeys[0], persistedKey)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
