package storage

import (
	"context"
	"testing"
)

func TestPostgresMigrationsCreateCoreTables(t *testing.T) {
	st, schema := newPostgresTestStoreWithSchema(t)

	for _, table := range []string{
		"users",
		"user_ai_credentials",
		"sessions",
		"postings",
		"profiles",
		"bookmarks",
		"not_interested",
		"scores",
		"scrape_runs",
	} {
		var exists bool
		err := st.db.QueryRowContext(context.Background(), `SELECT EXISTS (
			SELECT 1
			  FROM information_schema.tables
			 WHERE table_schema = $1
			   AND table_name = $2
		)`, schema, table).Scan(&exists)
		if err != nil || !exists {
			t.Fatalf("table %s exists=%v err=%v", table, exists, err)
		}
	}
}

func TestPostgresCredentialMigrationConstraintsAndCascade(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()

	var userID int64
	if err := st.db.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('credential-schema@example.invalid', 'synthetic-password-hash', now(), now())
RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert credential owner: %v", err)
	}

	validCiphertext := []byte("synthetic-ciphertext")
	validNonce := []byte("nonce-12byte")
	if _, err := st.db.ExecContext(ctx, `
INSERT INTO user_ai_credentials (user_id, provider, ciphertext, nonce, encryption_version)
VALUES ($1, $2, $3, $4, $5)`, userID, "anthropic", validCiphertext, validNonce, 1); err != nil {
		t.Fatalf("insert valid credential: %v", err)
	}

	if _, err := st.db.ExecContext(ctx, `
INSERT INTO user_ai_credentials (user_id, provider, ciphertext, nonce, encryption_version)
VALUES ($1, $2, $3, $4, $5)`, userID, "anthropic", validCiphertext, validNonce, 1); err == nil {
		t.Fatal("duplicate user/provider insert succeeded, want primary-key violation")
	}

	tests := []struct {
		name       string
		provider   string
		ciphertext []byte
		nonce      []byte
		version    int16
	}{
		{name: "empty provider", provider: "", ciphertext: validCiphertext, nonce: validNonce, version: 1},
		{name: "ciphertext is only the GCM tag", provider: "short-ciphertext", ciphertext: []byte("0123456789abcdef"), nonce: validNonce, version: 1},
		{name: "nonce is not twelve bytes", provider: "short-nonce", ciphertext: validCiphertext, nonce: []byte("eleven-byte"), version: 1},
		{name: "version is zero", provider: "zero-version", ciphertext: validCiphertext, nonce: validNonce, version: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := st.db.ExecContext(ctx, `
INSERT INTO user_ai_credentials (user_id, provider, ciphertext, nonce, encryption_version)
VALUES ($1, $2, $3, $4, $5)`, userID, tt.provider, tt.ciphertext, tt.nonce, tt.version); err == nil {
				t.Fatal("invalid credential insert succeeded")
			}
		})
	}

	if _, err := st.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("delete credential owner: %v", err)
	}
	var count int
	if err := st.db.QueryRowContext(ctx, `
SELECT count(*) FROM user_ai_credentials WHERE user_id = $1`, userID).Scan(&count); err != nil {
		t.Fatalf("count cascaded credentials: %v", err)
	}
	if count != 0 {
		t.Fatalf("credential rows after user delete = %d, want 0", count)
	}
}

func TestRenameImportOwnerMigrationRenamesLegacyFallbackAndPreservesID(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	if _, err := st.SQLDB().ExecContext(ctx, `DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	var legacyID int64
	if err := st.SQLDB().QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('sqlite-import-owner@job-scraper.local', 'imported-sqlite-no-login', now(), now())
RETURNING id`).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}

	executeRenameImportOwnerMigration(t, st)

	var gotID int64
	var gotEmail, gotPasswordHash string
	if err := st.SQLDB().QueryRowContext(ctx, `
SELECT id, email, password_hash
  FROM users
 WHERE email = 'sqlite-import-owner@jobcron.local'`).Scan(&gotID, &gotEmail, &gotPasswordHash); err != nil {
		t.Fatal(err)
	}
	if gotID != legacyID {
		t.Fatalf("renamed owner ID = %d, want %d", gotID, legacyID)
	}
	if gotEmail != "sqlite-import-owner@jobcron.local" || gotPasswordHash != "imported-sqlite-no-login" {
		t.Fatalf("renamed owner = email %q password_hash %q", gotEmail, gotPasswordHash)
	}
}

func TestRenameImportOwnerMigrationAlreadyRenamedIsIdempotent(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	if _, err := st.SQLDB().ExecContext(ctx, `DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	var canonicalID int64
	if err := st.SQLDB().QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('sqlite-import-owner@jobcron.local', 'imported-sqlite-no-login', now(), now())
RETURNING id`).Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}

	executeRenameImportOwnerMigration(t, st)
	executeRenameImportOwnerMigration(t, st)

	var gotID int64
	if err := st.SQLDB().QueryRowContext(ctx, `
SELECT id
  FROM users
 WHERE email = 'sqlite-import-owner@jobcron.local'
   AND password_hash = 'imported-sqlite-no-login'`).Scan(&gotID); err != nil {
		t.Fatal(err)
	}
	if gotID != canonicalID {
		t.Fatalf("already-renamed owner ID = %d, want %d", gotID, canonicalID)
	}
	var legacyCount int
	if err := st.SQLDB().QueryRowContext(ctx, `
SELECT count(*)
  FROM users
 WHERE email = 'sqlite-import-owner@job-scraper.local'`).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatalf("legacy owner count = %d, want 0", legacyCount)
	}
}

func TestRenameImportOwnerMigrationPreservesRealOwner(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	if _, err := st.SQLDB().ExecContext(ctx, `DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SQLDB().ExecContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('owner@example.com', 'real-hash', now(), now())`); err != nil {
		t.Fatal(err)
	}

	executeRenameImportOwnerMigration(t, st)

	var count int
	if err := st.SQLDB().QueryRowContext(ctx, `
SELECT count(*)
  FROM users
 WHERE email = 'owner@example.com'
   AND password_hash = 'real-hash'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("real owner count = %d, want 1", count)
	}
}

func TestRenameImportOwnerMigrationPreservesLegacyRowWhenCanonicalAddressExists(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	if _, err := st.SQLDB().ExecContext(ctx, `DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	var legacyID, canonicalID int64
	if err := st.SQLDB().QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('sqlite-import-owner@job-scraper.local', 'imported-sqlite-no-login', now(), now())
RETURNING id`).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if err := st.SQLDB().QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('sqlite-import-owner@jobcron.local', 'canonical-owner-hash', now(), now())
RETURNING id`).Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}

	executeRenameImportOwnerMigration(t, st)

	var gotLegacyID, gotCanonicalID int64
	if err := st.SQLDB().QueryRowContext(ctx, `
SELECT id
  FROM users
 WHERE email = 'sqlite-import-owner@job-scraper.local'
   AND password_hash = 'imported-sqlite-no-login'`).Scan(&gotLegacyID); err != nil {
		t.Fatal(err)
	}
	if err := st.SQLDB().QueryRowContext(ctx, `
SELECT id
  FROM users
 WHERE email = 'sqlite-import-owner@jobcron.local'
   AND password_hash = 'canonical-owner-hash'`).Scan(&gotCanonicalID); err != nil {
		t.Fatal(err)
	}
	if gotLegacyID != legacyID || gotCanonicalID != canonicalID {
		t.Fatalf("user IDs after collision = legacy %d canonical %d, want legacy %d canonical %d", gotLegacyID, gotCanonicalID, legacyID, canonicalID)
	}
}

func executeRenameImportOwnerMigration(t *testing.T, st *Store) {
	t.Helper()
	sqlBytes, err := postgresMigrationsFS.ReadFile("postgres_migrations/0013_rename_import_owner_email.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SQLDB().ExecContext(context.Background(), string(sqlBytes)); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresScrapeRunsStartFinishAndLatestRuntime(t *testing.T) {
	st, _ := newPostgresTestStoreWithSchema(t)
	ctx := context.Background()

	first, err := st.StartScrapeRun(ctx, ScrapeTriggerManual)
	if err != nil {
		t.Fatalf("StartScrapeRun first: %v", err)
	}
	if first.Trigger != ScrapeTriggerManual {
		t.Fatalf("first Trigger = %q, want %q", first.Trigger, ScrapeTriggerManual)
	}
	if first.Status != ScrapeRunStatusRunning {
		t.Fatalf("first Status = %q, want %q", first.Status, ScrapeRunStatusRunning)
	}
	if first.FinishedAt != nil {
		t.Fatalf("first FinishedAt = %v, want nil while running", first.FinishedAt)
	}

	firstResult := ScrapeResult{
		Listed:     5,
		New:        2,
		Refreshed:  1,
		Scored:     4,
		Removed:    1,
		Duplicates: 1,
		Failed:     0,
	}
	if err := st.FinishScrapeRun(ctx, first.ID, firstResult, ScrapeRunStatusSuccess, ""); err != nil {
		t.Fatalf("FinishScrapeRun first: %v", err)
	}
	latest, ok, err := st.LatestScrapeRun(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestScrapeRun after first ok=%v err=%v", ok, err)
	}
	if latest.ID != first.ID {
		t.Fatalf("latest ID = %d, want first ID %d", latest.ID, first.ID)
	}
	if latest.Status != ScrapeRunStatusSuccess {
		t.Fatalf("latest Status = %q, want %q", latest.Status, ScrapeRunStatusSuccess)
	}
	if latest.Result != firstResult {
		t.Fatalf("latest Result = %+v, want %+v", latest.Result, firstResult)
	}
	if latest.ErrorSummary != "" {
		t.Fatalf("latest ErrorSummary = %q, want empty", latest.ErrorSummary)
	}
	if latest.FinishedAt == nil {
		t.Fatal("latest FinishedAt = nil, want success finish time")
	}

	second, err := st.StartScrapeRun(ctx, ScrapeTriggerScheduled)
	if err != nil {
		t.Fatalf("StartScrapeRun second: %v", err)
	}
	if second.ID <= first.ID {
		t.Fatalf("second ID = %d, want greater than first ID %d", second.ID, first.ID)
	}
	secondResult := ScrapeResult{
		Listed:     7,
		New:        3,
		Refreshed:  2,
		Scored:     6,
		Removed:    2,
		Duplicates: 0,
		Failed:     1,
	}
	if err := st.FinishScrapeRun(ctx, second.ID, secondResult, ScrapeRunStatusFailure, "jumpit timeout"); err != nil {
		t.Fatalf("FinishScrapeRun second: %v", err)
	}
	latest, ok, err = st.LatestScrapeRun(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestScrapeRun after second ok=%v err=%v", ok, err)
	}
	if latest.ID != second.ID {
		t.Fatalf("latest ID = %d, want second ID %d", latest.ID, second.ID)
	}
	if latest.Trigger != ScrapeTriggerScheduled {
		t.Fatalf("latest Trigger = %q, want %q", latest.Trigger, ScrapeTriggerScheduled)
	}
	if latest.Status != ScrapeRunStatusFailure {
		t.Fatalf("latest Status = %q, want %q", latest.Status, ScrapeRunStatusFailure)
	}
	if latest.Result != secondResult {
		t.Fatalf("latest Result = %+v, want %+v", latest.Result, secondResult)
	}
	if latest.ErrorSummary != "jumpit timeout" {
		t.Fatalf("latest ErrorSummary = %q, want jumpit timeout", latest.ErrorSummary)
	}
	if latest.FinishedAt == nil {
		t.Fatal("latest FinishedAt = nil, want failure finish time")
	}
}
