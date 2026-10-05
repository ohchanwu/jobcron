//go:build !windows

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionVerifiedCommandsRedactConnectionFailure(t *testing.T) {
	ca, _ := tunnelCertificate(t, testRDSHost)
	raw := strings.Replace(tunnelURI(t, ca), ":15432/", ":1/", 1)
	dir := t.TempDir()
	inputs := map[string]string{
		"DATABASE_URL":              raw,
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
		{"create-owner", "--email", "owner@example.com"},
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
