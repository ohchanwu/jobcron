package credential

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ohchanwu/jobcron/internal/appdata"
)

const (
	// MasterKeyBytes is the only accepted decoded AES-256 master-key length.
	MasterKeyBytes = 32
	keyFileMode    = 0o600
	keyDirMode     = 0o700
)

// ParseMasterKey decodes a base64 master key and enforces the AES-256 key
// length without reflecting key material in errors.
func ParseMasterKey(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, errors.New("credential: master key must be valid base64")
	}
	if len(decoded) != MasterKeyBytes {
		return nil, fmt.Errorf("credential: master key must decode to exactly %d bytes", MasterKeyBytes)
	}
	return decoded, nil
}

// DefaultMasterKeyPath returns the local credential master-key path below the
// canonical OS application configuration directory.
func DefaultMasterKeyPath() (string, error) {
	return defaultMasterKeyPath(os.UserConfigDir)
}

func defaultMasterKeyPath(userConfigDir func() (string, error)) (string, error) {
	root, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("credential: locate user config dir: %w", err)
	}
	return filepath.Join(appdata.Dir(root), "credential-encryption.key"), nil
}

// LoadOrCreateLocalMasterKey loads the canonical local master key or creates
// it once with owner-only permissions.
func LoadOrCreateLocalMasterKey() ([]byte, error) {
	path, err := DefaultMasterKeyPath()
	if err != nil {
		return nil, err
	}
	return LoadOrCreateLocalMasterKeyAt(path)
}

// LoadOrCreateLocalMasterKeyAt loads or creates a protected local master key at
// path. It is exported for callers with an explicit application-data root.
func LoadOrCreateLocalMasterKeyAt(path string) ([]byte, error) {
	return loadOrCreateLocalMasterKeyAt(path, rand.Reader)
}

func loadOrCreateLocalMasterKeyAt(path string, random io.Reader) ([]byte, error) {
	key, err := readLocalMasterKey(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, keyDirMode); err != nil {
		return nil, fmt.Errorf("credential: create local key directory: %w", err)
	}

	generated := make([]byte, MasterKeyBytes)
	if _, err := io.ReadFull(random, generated); err != nil {
		return nil, errors.New("credential: generate local master key")
	}
	encoded := base64.StdEncoding.EncodeToString(generated) + "\n"

	tmp, err := os.CreateTemp(dir, ".credential-encryption.key-*")
	if err != nil {
		return nil, fmt.Errorf("credential: create local key temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(keyFileMode); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("credential: protect local key temp file: %w", err)
	}
	if _, err := io.WriteString(tmp, encoded); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("credential: write local key temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("credential: sync local key temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("credential: close local key temp file: %w", err)
	}

	if err := os.Link(tmpPath, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return readLocalMasterKey(path)
		}
		return nil, fmt.Errorf("credential: install local master key: %w", err)
	}
	return append([]byte(nil), generated...), nil
}

func readLocalMasterKey(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != keyFileMode {
		return nil, fmt.Errorf("credential: local master key permissions must be %04o", keyFileMode)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("credential: read local master key: %w", err)
	}
	key, err := ParseMasterKey(string(contents))
	if err != nil {
		return nil, fmt.Errorf("credential: parse local master key: %w", err)
	}
	return key, nil
}
