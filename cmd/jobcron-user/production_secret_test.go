package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionPasswordsRejectDirectEnvironment(t *testing.T) {
	for _, name := range []string{"JOBCRON_DATABASE_PASSWORD", "JOBCRON_OWNER_PASSWORD", "JOBCRON_USER_PASSWORD"} {
		_, err := commandPassword(envMap{"JOBCRON_ENV": "production", name: "synthetic-password"}, name, "test", strings.NewReader(""), io.Discard)
		if err == nil || err.Error() != name+": production requires file input" {
			t.Fatalf("direct production password accepted: %v", err)
		}
	}
}

func TestProductionUserCommandsRejectDatabaseURLArgumentBeforeDatabaseOpen(t *testing.T) {
	const argvSecret = "postgres://argv-secret@example.invalid/jobcron"
	commands := map[string][]string{
		"migrate":        {"migrate", "--database-url", argvSecret},
		"create-owner":   {"create-owner", "--database-url", argvSecret, "--email", "owner@example.com"},
		"reset-password": {"reset-password", "--database-url", argvSecret, "--email", "owner@example.com"},
		"delete-user":    {"delete-user", "--database-url", argvSecret, "--email", "owner@example.com", "--confirm-email", "owner@example.com"},
	}
	for name, args := range commands {
		for _, withFile := range []bool{false, true} {
			suffix := "/argument-only"
			if withFile {
				suffix = "/alongside-file"
			}
			t.Run(name+suffix, func(t *testing.T) {
				env := envMap{"JOBCRON_ENV": "production"}
				if withFile {
					path := filepath.Join(t.TempDir(), "database-url")
					if err := os.WriteFile(path, []byte("postgres://file-secret@example.invalid/jobcron"), 0600); err != nil {
						t.Fatal(err)
					}
					env["DATABASE_URL_FILE"] = path
				}
				err := run(context.Background(), args, env, strings.NewReader("unused-password\n"), io.Discard)
				if err == nil || err.Error() != "user: production requires DATABASE_URL_FILE" {
					t.Fatalf("database argument was not rejected before database opening: %v", err)
				}
				if strings.Contains(err.Error(), argvSecret) {
					t.Fatal("database argument leaked in rejection")
				}
			})
		}
	}
}
