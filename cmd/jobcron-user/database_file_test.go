package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserCommandsRejectInvalidDatabaseFileBeforeConnecting(t *testing.T) {
	for _, command := range []string{"migrate", "create-owner", "reset-password", "delete-user"} {
		t.Run(command, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "secret")
			if err := os.WriteFile(path, []byte("synthetic-url"), 0644); err != nil {
				t.Fatal(err)
			}
			err := run(context.Background(), []string{command}, envMap{"DATABASE_URL_FILE": path}, strings.NewReader(""), io.Discard)
			if err == nil || !strings.Contains(err.Error(), "DATABASE_URL_FILE") {
				t.Fatalf("file validation not reached: %v", err)
			}
		})
	}
}

func TestUserDatabaseInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("synthetic-url"), 0600); err != nil {
		t.Fatal(err)
	}
	env := envMap{"DATABASE_URL_FILE": path}
	got, err := databaseInput(env, "")
	if err != nil || got != "synthetic-url" {
		t.Fatalf("file input: %v", err)
	}
	if _, err := databaseInput(env, "other"); err == nil {
		t.Fatal("file/flag ambiguity")
	}
	env["DATABASE_URL"] = ""
	if _, err := databaseInput(env, ""); err == nil {
		t.Fatal("env/file ambiguity")
	}
}
