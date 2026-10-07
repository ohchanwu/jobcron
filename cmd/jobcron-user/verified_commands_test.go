//go:build !windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionPasswordCommandsPreserveSequentialStdin(t *testing.T) {
	const input = "synthetic-owner-password\r\nsynthetic-database-password"
	// Stop before CA access or dial, but only after both passwords were read.
	t.Setenv("PGHOST", "unapproved.invalid")
	path := filepath.Join(t.TempDir(), "database-url")
	raw := "postgres://master@" + testRDSHost + ":15432/jobcron?sslmode=verify-full&hostaddr=127.0.0.1&sslrootcert=/unused/rds-ca.pem"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	env := envMap{
		"JOBCRON_ENV": "production", "DATABASE_URL_FILE": path,
		"JOBCRON_OWNER_EMAIL_FILE": ownerEmailFile(t, "owner@example.com", 0600),
	}
	for _, command := range []string{"create-owner", "reset-password"} {
		for _, source := range []string{"reader", "already-buffered", "prefilled-pipe"} {
			t.Run(command+"/"+source, func(t *testing.T) {
				var in io.Reader = strings.NewReader(input)
				switch source {
				case "already-buffered":
					buffered := bufio.NewReader(in)
					if _, err := buffered.Peek(len(input)); err != nil {
						t.Fatal(err)
					}
					in = buffered
				case "prefilled-pipe":
					reader, writer, err := os.Pipe()
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { reader.Close() })
					t.Cleanup(func() { writer.Close() })
					if _, err := io.WriteString(writer, input); err != nil {
						t.Fatal(err)
					}
					if err := writer.Close(); err != nil {
						t.Fatal(err)
					}
					in = reader
				}
				var out, prompts bytes.Buffer
				args := []string{command}
				if command == "reset-password" {
					args = append(args, "--email", "owner@example.com")
				}
				err := runWithPrompt(context.Background(), args, env, in, &out, &prompts)
				if err == nil || err.Error() != "user: verified operator connection forbids ambient PG settings" {
					t.Fatalf("sequential stdin did not reach the pre-dial guard: %v", err)
				}
				label := "Owner"
				if command == "reset-password" {
					label = "User"
				}
				if out.Len() != 0 || prompts.String() != label+" password: Database password: " {
					t.Fatal("unexpected output or password disclosure")
				}
			})
		}
	}
}

func TestProductionVerifiedCommandsRedactConnectionFailure(t *testing.T) {
	ca, _ := tunnelCertificate(t, testRDSHost)
	raw := strings.Replace(tunnelURI(t, ca), ":15432/", ":1/", 1)
	dir := t.TempDir()
	inputs := map[string]string{
		"DATABASE_URL":              raw,
		"JOBCRON_OWNER_EMAIL":       "private-owner@example.com",
		"JOBCRON_DATABASE_PASSWORD": "private-database-password",
		"JOBCRON_OWNER_PASSWORD":    "private-owner-password",
		"JOBCRON_USER_PASSWORD":     "private-user-password",
	}
	env := envMap{"JOBCRON_ENV": "production"}
	for key, value := range inputs {
		path := filepath.Join(dir, key)
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		env[key+"_FILE"] = path
	}
	for _, args := range [][]string{
		{"migrate"},
		{"create-owner"},
		{"reset-password", "--email", "owner@example.com"},
		{"delete-user", "--email", "owner@example.com", "--confirm-email", "owner@example.com"},
	} {
		t.Run(args[0], func(t *testing.T) {
			var out bytes.Buffer
			err := run(context.Background(), args, env, nil, &out)
			if err == nil || err.Error() != "user: open PostgreSQL database" || out.Len() != 0 {
				t.Fatalf("connection failure not sanitized: %v %s", err, out.String())
			}
		})
	}
}
