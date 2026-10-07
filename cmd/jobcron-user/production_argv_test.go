package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionUserCommandsRejectRawDatabaseArguments(t *testing.T) {
	// Invalid ports make regressions fail locally, without dialing a database.
	const argvSecret = "postgres://argv-secret@example.invalid:notaport/jobcron"
	const fileSecret = "postgres://file-secret@example.invalid:notaport/jobcron"
	for _, command := range []string{"create-owner", "reset-password", "delete-user"} {
		for _, spelling := range []string{"--database-url", "-database-url"} {
			for _, form := range []string{"separate", "assignment", "empty", "missing"} {
				for _, placement := range []string{"first", "after-flags", "positional", "double-dash", "parse-error"} {
					for _, source := range []string{"argument-only", "alongside-file"} {
						t.Run(strings.Join([]string{command, spelling, form, placement, source}, "/"), func(t *testing.T) {
							option := []string{spelling, argvSecret}
							switch form {
							case "assignment":
								option = []string{spelling + "=" + argvSecret}
							case "empty":
								option = []string{spelling + "="}
							case "missing":
								option = []string{spelling}
							}
							args := []string{command}
							if placement != "first" {
								args = append(args, "--email", "owner@example.com")
								if command == "delete-user" {
									args = append(args, "--confirm-email", "owner@example.com")
								}
							}
							switch placement {
							case "positional":
								args = append(args, "positional")
							case "double-dash":
								args = append(args, "--")
							case "parse-error":
								args = append(args, "--unknown="+argvSecret)
							}
							args = append(args, option...)
							env := envMap{"JOBCRON_ENV": "production"}
							if source == "alongside-file" {
								path := filepath.Join(t.TempDir(), "database-url")
								if err := os.WriteFile(path, []byte(fileSecret), 0600); err != nil {
									t.Fatal(err)
								}
								env["DATABASE_URL_FILE"] = path
							}
							in := strings.NewReader("unused-password\n")
							var out, prompt bytes.Buffer
							err := runWithPrompt(context.Background(), args, env, in, &out, &prompt)
							if err == nil {
								t.Fatal("raw database argument accepted")
							}
							for _, secret := range []string{argvSecret, fileSecret, "argv-secret", "file-secret"} {
								if strings.Contains(err.Error()+out.String()+prompt.String(), secret) {
									t.Fatal("secret disclosed in error or output")
								}
							}
							if err.Error() != "user: production requires DATABASE_URL_FILE" {
								t.Fatalf("raw argument not rejected before parsing/database opening: %v", err)
							}
							if in.Len() != len("unused-password\n") || out.Len() != 0 || prompt.Len() != 0 {
								t.Fatal("rejection read password input or wrote output")
							}
						})
					}
				}
			}
		}
	}
}

func TestProductionUserCommandsPreserveFileOnlyInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(path, []byte("postgres://file-secret@example.invalid:notaport/jobcron"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"create-owner", "reset-password", "delete-user"} {
		for _, suffix := range [][]string{
			nil,
			{"positional", "--database-url-file=unused"},
			{"--", "prefix--database-url=unused", "-database-url-extra=unused"},
		} {
			t.Run(command+"/"+strings.Join(suffix, "/"), func(t *testing.T) {
				args := append([]string{command}, suffix...)
				var out bytes.Buffer
				err := run(context.Background(), args, envMap{
					"JOBCRON_ENV": "production", "DATABASE_URL_FILE": path,
				}, nil, &out)
				want := "user: --email is required"
				if command == "create-owner" {
					want = "user: production requires JOBCRON_OWNER_EMAIL_FILE"
					if len(suffix) > 0 {
						want = "user: unexpected positional arguments"
					}
				}
				if err == nil || err.Error() != want || out.Len() != 0 {
					t.Fatalf("file-only database input changed: %v", err)
				}
			})
		}
	}
}

func TestNonproductionUserCommandsPreserveDatabaseArgumentParsing(t *testing.T) {
	for _, command := range []string{"create-owner", "reset-password", "delete-user"} {
		for _, option := range [][]string{
			{"--database-url", "unused"}, {"--database-url=unused"},
			{"-database-url", "unused"}, {"-database-url=unused"},
		} {
			t.Run(command+"/"+option[0], func(t *testing.T) {
				args := append([]string{command}, option...)
				var out bytes.Buffer
				err := run(context.Background(), args, envMap{}, nil, &out)
				if err == nil || err.Error() != "user: --email is required" || out.Len() != 0 {
					t.Fatalf("nonproduction database argument parsing changed: %v", err)
				}
			})
		}
	}
}
