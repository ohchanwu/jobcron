package appdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirName       = "jobcron"
	legacyDirName = "job-scraper"
)

// Dir returns the canonical application-data directory under root.
func Dir(root string) string { return filepath.Join(root, dirName) }

// LegacyDir returns the pre-rename application-data directory under root.
func LegacyDir(root string) string { return filepath.Join(root, legacyDirName) }

// Prepare moves legacy application data into the canonical directory when
// needed. A collision is reported without modifying either directory.
func Prepare(root string) error { return prepare(root, os.Rename) }

func prepare(root string, rename func(string, string) error) error {
	legacy, canonical := LegacyDir(root), Dir(root)
	_, legacyErr := os.Stat(legacy)
	_, canonicalErr := os.Stat(canonical)
	legacyExists := legacyErr == nil
	canonicalExists := canonicalErr == nil
	if legacyErr != nil && !errors.Is(legacyErr, os.ErrNotExist) {
		return fmt.Errorf("appdata: inspect legacy directory: %w", legacyErr)
	}
	if canonicalErr != nil && !errors.Is(canonicalErr, os.ErrNotExist) {
		return fmt.Errorf("appdata: inspect canonical directory: %w", canonicalErr)
	}
	if legacyExists && canonicalExists {
		return fmt.Errorf("appdata: both legacy %q and canonical %q directories exist", legacy, canonical)
	}
	if !legacyExists {
		return nil
	}
	if err := rename(legacy, canonical); err != nil {
		return fmt.Errorf("appdata: rename %q to %q: %w", legacy, canonical, err)
	}
	return nil
}
