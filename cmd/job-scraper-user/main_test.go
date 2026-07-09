package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ohchanwu/job-scraper/internal/auth"
	"github.com/ohchanwu/job-scraper/internal/storage"
)

func TestRunCreateOwnerUsesPasswordFromEnv(t *testing.T) {
	postgresURL := os.Getenv("JOBSCRAPER_TEST_POSTGRES_URL")
	if postgresURL == "" {
		t.Skip("JOBSCRAPER_TEST_POSTGRES_URL not set")
	}
	schema := createUserCLITestSchema(t, postgresURL)
	databaseURL := databaseURLWithSearchPath(postgresURL, schema)

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"create-owner",
		"--database-url", databaseURL,
		"--email", "owner@example.com",
	}, envMap{"JOBSCRAPER_OWNER_PASSWORD": "top secret"}, nil, &out)
	if err != nil {
		t.Fatalf("run create-owner: %v", err)
	}
	if !strings.Contains(out.String(), "created owner user owner@example.com") {
		t.Fatalf("output = %q", out.String())
	}

	st, err := storage.OpenPostgres(databaseURL)
	if err != nil {
		t.Fatalf("OpenPostgres verify: %v", err)
	}
	defer st.Close()
	var encodedHash string
	if err := st.SQLDB().QueryRow(`SELECT password_hash FROM users WHERE email = $1`, "owner@example.com").Scan(&encodedHash); err != nil {
		t.Fatalf("query owner password hash: %v", err)
	}
	ok, err := auth.VerifyPassword(encodedHash, "top secret")
	if err != nil || !ok {
		t.Fatalf("VerifyPassword ok=%v err=%v", ok, err)
	}
}

func TestRunCreateOwnerFailsWhenOwnerExistsUntilResetPassword(t *testing.T) {
	postgresURL := os.Getenv("JOBSCRAPER_TEST_POSTGRES_URL")
	if postgresURL == "" {
		t.Skip("JOBSCRAPER_TEST_POSTGRES_URL not set")
	}
	schema := createUserCLITestSchema(t, postgresURL)
	databaseURL := databaseURLWithSearchPath(postgresURL, schema)

	env := envMap{"JOBSCRAPER_OWNER_PASSWORD": "first password"}
	if err := run(context.Background(), []string{
		"create-owner",
		"--database-url", databaseURL,
		"--email", "owner@example.com",
	}, env, nil, &bytes.Buffer{}); err != nil {
		t.Fatalf("first create-owner: %v", err)
	}

	err := run(context.Background(), []string{
		"create-owner",
		"--database-url", databaseURL,
		"--email", "owner@example.com",
	}, envMap{"JOBSCRAPER_OWNER_PASSWORD": "second password"}, nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("second create-owner error = nil, want owner-exists error")
	}
	if !strings.Contains(err.Error(), "owner user already exists") {
		t.Fatalf("second create-owner error = %v", err)
	}

	if err := run(context.Background(), []string{
		"reset-password",
		"--database-url", databaseURL,
		"--email", "owner@example.com",
	}, envMap{"JOBSCRAPER_OWNER_PASSWORD": "second password"}, nil, &bytes.Buffer{}); err != nil {
		t.Fatalf("reset-password: %v", err)
	}

	st, err := storage.OpenPostgres(databaseURL)
	if err != nil {
		t.Fatalf("OpenPostgres verify: %v", err)
	}
	defer st.Close()
	var encodedHash string
	if err := st.SQLDB().QueryRow(`SELECT password_hash FROM users WHERE email = $1`, "owner@example.com").Scan(&encodedHash); err != nil {
		t.Fatalf("query owner password hash: %v", err)
	}
	ok, err := auth.VerifyPassword(encodedHash, "second password")
	if err != nil || !ok {
		t.Fatalf("VerifyPassword after reset ok=%v err=%v", ok, err)
	}
}

func TestRunResetPasswordWithWrongEmailFailsWithoutRenamingOwner(t *testing.T) {
	postgresURL := os.Getenv("JOBSCRAPER_TEST_POSTGRES_URL")
	if postgresURL == "" {
		t.Skip("JOBSCRAPER_TEST_POSTGRES_URL not set")
	}
	schema := createUserCLITestSchema(t, postgresURL)
	databaseURL := databaseURLWithSearchPath(postgresURL, schema)

	if err := run(context.Background(), []string{
		"create-owner",
		"--database-url", databaseURL,
		"--email", "owner@example.com",
	}, envMap{"JOBSCRAPER_OWNER_PASSWORD": "first password"}, nil, &bytes.Buffer{}); err != nil {
		t.Fatalf("create-owner: %v", err)
	}

	err := run(context.Background(), []string{
		"reset-password",
		"--database-url", databaseURL,
		"--email", "wrong@example.com",
	}, envMap{"JOBSCRAPER_OWNER_PASSWORD": "second password"}, nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("reset-password error = nil, want email mismatch error")
	}
	if !strings.Contains(err.Error(), "owner user does not match email") {
		t.Fatalf("reset-password error = %v", err)
	}

	st, err := storage.OpenPostgres(databaseURL)
	if err != nil {
		t.Fatalf("OpenPostgres verify: %v", err)
	}
	defer st.Close()
	var email, encodedHash string
	if err := st.SQLDB().QueryRow(`SELECT email, password_hash FROM users`).Scan(&email, &encodedHash); err != nil {
		t.Fatalf("query owner after failed reset: %v", err)
	}
	if email != "owner@example.com" {
		t.Fatalf("owner email = %q, want owner@example.com", email)
	}
	ok, err := auth.VerifyPassword(encodedHash, "first password")
	if err != nil || !ok {
		t.Fatalf("VerifyPassword original after failed reset ok=%v err=%v", ok, err)
	}
}

func TestRunCreateOwnerRequiresPassword(t *testing.T) {
	err := run(context.Background(), []string{
		"create-owner",
		"--database-url", "postgres://example.invalid/jobs",
		"--email", "owner@example.com",
	}, envMap{}, strings.NewReader("\n"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("run error = nil, want password required error")
	}
	if !strings.Contains(err.Error(), "owner password is required") {
		t.Fatalf("run error = %v", err)
	}
}

func TestOwnerPasswordReadsFromNonTerminalReader(t *testing.T) {
	var out bytes.Buffer
	password, err := ownerPassword(envMap{}, strings.NewReader("typed password\n"), &out)
	if err != nil {
		t.Fatalf("ownerPassword: %v", err)
	}
	if password != "typed password" {
		t.Fatalf("password = %q", password)
	}
	if out.String() != "Owner password: " {
		t.Fatalf("prompt output = %q", out.String())
	}
}

func createUserCLITestSchema(t *testing.T, databaseURL string) string {
	t.Helper()
	schema := postgresTestSchemaName(t)
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open postgres admin: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
		_ = db.Close()
	})
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	return schema
}

func postgresTestSchemaName(t *testing.T) string {
	t.Helper()
	name := strings.ToLower(t.Name())
	name = strings.ReplaceAll(name, "/", "_")
	return "test_user_cli_" + strings.NewReplacer(" ", "_", "-", "_").Replace(name)
}

func databaseURLWithSearchPath(databaseURL, schema string) string {
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}
	return databaseURL + separator + "search_path=" + schema
}
