package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ohchanwu/jobcron/internal/appdata"
	"github.com/ohchanwu/jobcron/internal/credential"
)

// keysFileMode is the permission the BYOK key file is held at. 0600 keeps the
// user's paid API credential readable only by the owner.
const keysFileMode = 0o600

// DefaultKeysPath is the BYOK key store path under the user's OS config
// directory — the same directory as jobs.db (see storage.DefaultDBPath).
func DefaultKeysPath() (string, error) {
	return defaultKeysPath(os.UserConfigDir)
}

func defaultKeysPath(userConfigDir func() (string, error)) (string, error) {
	root, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("ai: locate user config dir: %w", err)
	}
	return filepath.Join(appdata.Dir(root), "ai_keys.json"), nil
}

// LoadKeys reads the provider->key map from path. A MISSING file is not an
// error: it returns an empty map and a nil error, because a missing key is a
// deliberate silent fallback to the offline regex path (D4), not a failure.
func LoadKeys(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ai: read keys %s: %w", path, err)
	}
	keys := map[string]string{}
	if len(b) == 0 {
		return keys, nil
	}
	if err := json.Unmarshal(b, &keys); err != nil {
		return nil, fmt.Errorf("ai: parse keys %s: %w", path, err)
	}
	return keys, nil
}

// NormalizeKeys canonicalizes provider identifiers and rejects aliases that
// collapse to the same storage key. Empty provider names and values are ignored
// to preserve the legacy file's disabled-key behavior.
func NormalizeKeys(raw map[string]string) (map[string]string, error) {
	normalized := make(map[string]string, len(raw))
	for provider, key := range raw {
		if strings.TrimSpace(provider) == "" || strings.TrimSpace(key) == "" {
			continue
		}
		canonical, err := credential.NormalizeProvider(provider)
		if err != nil {
			return nil, errors.New("invalid legacy AI provider")
		}
		if _, exists := normalized[canonical]; exists {
			return nil, fmt.Errorf("duplicate provider %q after normalization", canonical)
		}
		normalized[canonical] = strings.TrimSpace(key)
	}
	return normalized, nil
}

// SaveKeys writes the provider->key map to path with 0600 permissions,
// creating the parent directory if needed. It writes to a temp file and
// renames over the target so an interrupted write never leaves a truncated
// key file, and so an overwrite still lands at 0600 (os.WriteFile does not
// chmod an already-existing file).
func SaveKeys(path string, keys map[string]string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ai: create keys directory: %w", err)
	}
	b, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("ai: marshal keys: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".ai_keys-*.tmp")
	if err != nil {
		return fmt.Errorf("ai: create temp keys file: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename succeeds.
	defer os.Remove(tmpName)
	if err := tmp.Chmod(keysFileMode); err != nil {
		tmp.Close()
		return fmt.Errorf("ai: chmod temp keys file: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("ai: write temp keys file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("ai: close temp keys file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("ai: replace keys file: %w", err)
	}
	return nil
}
