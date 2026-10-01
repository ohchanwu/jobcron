package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Secret reads either NAME or NAME_FILE, never both (including empty values).
// Files are absolute, owner-only regular files. One terminal LF is allowed;
// CR, NUL, embedded newlines and values larger than 64 KiB are rejected.
// Errors disclose only the configuration key, never the path or contents.
func Secret(env map[string]string, name string) (string, error) {
	value, direct := env[name]
	path, file := env[name+"_FILE"]
	if !file {
		if direct && env["JOBCRON_ENV"] == "production" {
			return "", fmt.Errorf("%s: production requires file input", name)
		}
		return value, nil
	}
	invalid := func() (string, error) { return "", fmt.Errorf("%s_FILE: invalid secret input", name) }
	if direct || !filepath.IsAbs(path) {
		return invalid()
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return invalid()
	}
	f, err := openSecret(path)
	if err != nil {
		return invalid()
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !os.SameFile(before, info) || !info.Mode().IsRegular() || (info.Mode().Perm() != 0600 && info.Mode().Perm() != 0400) || !secretOwned(info) {
		return invalid()
	}
	b, err := io.ReadAll(io.LimitReader(f, 65538))
	if err != nil || len(b) > 65537 {
		return invalid()
	}
	value = strings.TrimSuffix(string(b), "\n")
	if value == "" || len(value) > 65536 || strings.ContainsAny(value, "\r\n\x00") {
		return invalid()
	}
	return value, nil
}
