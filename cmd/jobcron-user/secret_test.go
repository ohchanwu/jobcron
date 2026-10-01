package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandPasswordFile(t *testing.T) {
	for _, name := range []string{"JOBCRON_DATABASE_PASSWORD", "JOBCRON_OWNER_PASSWORD", "JOBCRON_USER_PASSWORD"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "secret")
			if err := os.WriteFile(path, []byte("synthetic-password\n"), 0600); err != nil {
				t.Fatal(err)
			}
			env := envMap{name + "_FILE": path}
			got, err := commandPassword(env, name, "test", strings.NewReader(""), io.Discard)
			if err != nil || got != "synthetic-password" {
				t.Fatalf("file password not consumed: %v", err)
			}
			env[name] = ""
			if _, err := commandPassword(env, name, "test", strings.NewReader(""), io.Discard); err == nil {
				t.Fatal("ambiguity accepted")
			}
		})
	}
}
