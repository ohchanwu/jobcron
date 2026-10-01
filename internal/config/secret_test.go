package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("synthetic-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Secret(map[string]string{"TEST_FILE": path}, "TEST")
	if err != nil || got != "synthetic-value" {
		t.Fatalf("file input failed: %v", err)
	}
	for _, tc := range []struct {
		name string
		env  map[string]string
	}{
		{"ambiguous", map[string]string{"TEST": "", "TEST_FILE": path}},
		{"missing", map[string]string{"TEST_FILE": path + "-missing"}},
		{"empty path", map[string]string{"TEST_FILE": ""}},
		{"relative", map[string]string{"TEST_FILE": "secret"}},
		{"directory", map[string]string{"TEST_FILE": dir}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Secret(tc.env, "TEST")
			if err == nil {
				t.Fatal("accepted invalid input")
			}
			if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "synthetic-value") {
				t.Fatal("error leaks input")
			}
		})
	}
	for _, value := range []string{"", "\n", "a\nb", "a\r\n", "a\x00b", strings.Repeat("a", 65537)} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Secret(map[string]string{"TEST_FILE": path}, "TEST"); err == nil {
			t.Fatal("accepted invalid file content")
		}
	}
	if err := os.WriteFile(path, []byte("synthetic-value"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0644, 0640, 0700, 0000} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := Secret(map[string]string{"TEST_FILE": path}, "TEST"); err == nil {
			t.Fatal("accepted unsafe mode")
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Secret(map[string]string{"TEST_FILE": link}, "TEST"); err == nil {
		t.Fatal("accepted symlink")
	}
}
