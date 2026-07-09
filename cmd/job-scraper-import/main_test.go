package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ohchanwu/job-scraper/internal/ai"
	"github.com/ohchanwu/job-scraper/internal/scraper"
	"github.com/ohchanwu/job-scraper/internal/storage"
)

func TestImportDryRunReportsSQLiteCountsWithoutPostgres(t *testing.T) {
	sqlitePath := seedSQLiteImportFixture(t)

	var out bytes.Buffer
	err := runImport(context.Background(), importOptions{
		sqlitePath:  sqlitePath,
		postgresURL: "postgres://dry-run-should-not-connect.invalid/jobs",
		dryRun:      true,
		out:         &out,
	})
	if err != nil {
		t.Fatalf("runImport dry-run: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"dry run: true",
		"postings: 2",
		"profile: 1",
		"scores: 1",
		"bookmarks: 1",
		"not_interested: 1",
		"ai_extractions: 1",
		"ai_scores: 1",
		"ai_usage: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, got)
		}
	}
}

func TestImportSQLiteToPostgresCopiesRepresentativeDataAndIsIdempotent(t *testing.T) {
	postgresURL := os.Getenv("JOBSCRAPER_TEST_POSTGRES_URL")
	if postgresURL == "" {
		t.Skip("JOBSCRAPER_TEST_POSTGRES_URL not set")
	}
	sqlitePath := seedSQLiteImportFixture(t)
	schema := createPostgresImportSchema(t, postgresURL)
	targetURL := databaseURLWithSearchPath(postgresURL, schema)
	ownerEmail := "intended-owner@example.com"

	preexisting, err := storage.OpenPostgres(targetURL)
	if err != nil {
		t.Fatalf("OpenPostgres preexisting target: %v", err)
	}
	if _, err := preexisting.SQLDB().Exec(`
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ('existing-user@example.com', 'preexisting', now(), now())`); err != nil {
		t.Fatalf("seed preexisting user: %v", err)
	}
	if err := preexisting.Close(); err != nil {
		t.Fatalf("close preexisting target: %v", err)
	}

	for i := 0; i < 2; i++ {
		var out bytes.Buffer
		if err := runImport(context.Background(), importOptions{
			sqlitePath:  sqlitePath,
			postgresURL: targetURL,
			ownerEmail:  ownerEmail,
			out:         &out,
		}); err != nil {
			t.Fatalf("runImport pass %d: %v\n%s", i+1, err, out.String())
		}
	}

	db, err := sql.Open("pgx", targetURL)
	if err != nil {
		t.Fatalf("open postgres verification db: %v", err)
	}
	defer db.Close()

	assertPostgresScalar(t, db, `SELECT count(*) FROM users`, 2)
	assertPostgresScalar(t, db, `SELECT count(*) FROM postings`, 2)
	assertPostgresScalar(t, db, `SELECT count(*) FROM profiles`, 1)
	assertPostgresScalar(t, db, `SELECT count(*) FROM scores`, 1)
	assertPostgresScalar(t, db, `SELECT count(*) FROM bookmarks`, 1)
	assertPostgresScalar(t, db, `SELECT count(*) FROM not_interested`, 1)
	assertPostgresScalar(t, db, `SELECT count(*) FROM ai_extractions`, 1)
	assertPostgresScalar(t, db, `SELECT count(*) FROM ai_scores`, 1)
	assertPostgresScalar(t, db, `SELECT count(*) FROM ai_usage`, 1)
	assertAIUsage(t, db, "2026-07-09", 123, 45)

	if _, err := db.Exec(`
UPDATE ai_usage
SET input_tokens = 200, output_tokens = 60
WHERE day = '2026-07-09'`); err != nil {
		t.Fatalf("raise target ai_usage: %v", err)
	}
	var out bytes.Buffer
	if err := runImport(context.Background(), importOptions{
		sqlitePath:  sqlitePath,
		postgresURL: targetURL,
		ownerEmail:  ownerEmail,
		out:         &out,
	}); err != nil {
		t.Fatalf("runImport after higher live usage: %v\n%s", err, out.String())
	}
	assertAIUsage(t, db, "2026-07-09", 200, 60)

	var title, company, profileJSON, importedOwnerEmail string
	if err := db.QueryRow(`
SELECT p.title, p.company, pr.profile_json, u.email
FROM postings p
JOIN scores s ON s.posting_id = p.id
JOIN bookmarks b ON b.posting_id = p.id
JOIN not_interested n ON n.posting_id = p.id
JOIN profiles pr ON pr.profile_json LIKE '%"stacks":["Go"]%'
JOIN users u ON u.id = pr.user_id
WHERE p.source = 'jumpit' AND p.source_posting_id = 'import-1' AND s.total = 87`,
	).Scan(&title, &company, &profileJSON, &importedOwnerEmail); err != nil {
		t.Fatalf("query representative imported data: %v", err)
	}
	if title != "신입 백엔드 개발자" || company != "테스트컴퍼니" {
		t.Fatalf("posting title/company = %q/%q", title, company)
	}
	if !strings.Contains(profileJSON, `"stacks":["Go"]`) {
		t.Fatalf("profile_json = %s", profileJSON)
	}
	if importedOwnerEmail != ownerEmail {
		t.Fatalf("profile owner email = %q, want %q", importedOwnerEmail, ownerEmail)
	}

	var duplicateOf sql.NullInt64
	if err := db.QueryRow(`SELECT duplicate_of FROM postings WHERE source_posting_id = 'import-duplicate'`).Scan(&duplicateOf); err != nil {
		t.Fatalf("query duplicate_of: %v", err)
	}
	if !duplicateOf.Valid || duplicateOf.Int64 != 1 {
		t.Fatalf("duplicate_of = %+v, want posting 1", duplicateOf)
	}
}

func seedSQLiteImportFixture(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.db")
	st, err := storage.OpenSQLiteAt(path)
	if err != nil {
		t.Fatalf("OpenSQLiteAt: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	profileJSON := `{"stacks":["Go"],"locations":["서울"],"min_score":40}`
	profileHash, _, err := st.SaveProfile(ctx, profileJSON)
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	id, _, err := st.UpsertPosting(ctx, scraper.Posting{
		Source:          "jumpit",
		SourcePostingID: "import-1",
		URL:             "https://jumpit.example/jobs/import-1",
		Title:           "신입 백엔드 개발자",
		Company:         "테스트컴퍼니",
		Location:        "서울",
		Newcomer:        true,
		MinCareer:       0,
		MaxCareer:       0,
		CareerLevel:     "신입",
		StackTags:       []string{"Go", "PostgreSQL"},
		Tags:            []scraper.Tag{{ID: "tag-1", Name: "신입", Category: "career"}},
		Description:     "Go와 PostgreSQL을 사용하는 신입 백엔드 포지션",
		RawJSON:         `{"id":"import-1"}`,
		AlwaysOpen:      true,
		FirstSeenAt:     time.Date(2026, 7, 9, 1, 2, 3, 0, time.UTC),
		LastSeenAt:      time.Date(2026, 7, 9, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	duplicateID, _, err := st.UpsertPosting(ctx, scraper.Posting{
		Source:          "rallit",
		SourcePostingID: "import-duplicate",
		URL:             "https://rallit.example/jobs/import-duplicate",
		Title:           "신입 백엔드 개발자",
		Company:         "테스트컴퍼니",
		Location:        "서울",
		Newcomer:        true,
		MinCareer:       0,
		MaxCareer:       0,
		CareerLevel:     "신입",
		StackTags:       []string{"Go"},
		Description:     "중복 공고",
		RawJSON:         `{"id":"import-duplicate"}`,
		AlwaysOpen:      true,
		FirstSeenAt:     time.Date(2026, 7, 9, 1, 3, 3, 0, time.UTC),
		LastSeenAt:      time.Date(2026, 7, 9, 1, 3, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("UpsertPosting duplicate: %v", err)
	}
	if _, err := st.SQLDB().ExecContext(ctx, `UPDATE postings SET duplicate_of = ? WHERE id = ?`, id, duplicateID); err != nil {
		t.Fatalf("set duplicate_of: %v", err)
	}
	when := time.Date(2026, 7, 9, 2, 3, 4, 0, time.UTC)
	if err := st.UpsertScore(ctx, storage.Score{PostingID: id, ProfileHash: profileHash, Total: 87, BreakdownJSON: `[{"label":"stack"}]`, ComputedAt: when}); err != nil {
		t.Fatalf("UpsertScore: %v", err)
	}
	if err := st.SetBookmark(ctx, id, when); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}
	if err := st.SetNotInterested(ctx, id, when.Add(time.Minute)); err != nil {
		t.Fatalf("SetNotInterested: %v", err)
	}
	maxCareer := 2
	if err := st.UpsertAIExtraction(ctx, id, "content-hash", "ai-v1", ai.Extraction{
		MinCareer:     0,
		MaxCareer:     &maxCareer,
		Newcomer:      true,
		EducationEnum: "bachelor",
		Evidence:      "신입",
	}, when); err != nil {
		t.Fatalf("UpsertAIExtraction: %v", err)
	}
	if err := st.UpsertAIScore(ctx, id, "input-hash", "ai-v1", ai.Delta{
		Items:    []ai.DeltaItem{{Signal: "Go", Kind: "positive", Delta: 3, Evidence: "Go", MatchedGoal: "Go"}},
		NetDelta: 3,
	}, when); err != nil {
		t.Fatalf("UpsertAIScore: %v", err)
	}
	if err := st.AddAIUsage(ctx, "2026-07-09", 123, 45); err != nil {
		t.Fatalf("AddAIUsage: %v", err)
	}
	return path
}

func createPostgresImportSchema(t *testing.T, databaseURL string) string {
	t.Helper()
	schema := postgresImportSchemaName(t)
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

func assertPostgresScalar(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if got != want {
		t.Fatalf("%s = %d, want %d", query, got, want)
	}
}

func assertAIUsage(t *testing.T, db *sql.DB, day string, wantInput, wantOutput int) {
	t.Helper()
	var gotInput, gotOutput int
	if err := db.QueryRow(`SELECT input_tokens, output_tokens FROM ai_usage WHERE day = $1`, day).
		Scan(&gotInput, &gotOutput); err != nil {
		t.Fatalf("query ai_usage for %s: %v", day, err)
	}
	if gotInput != wantInput || gotOutput != wantOutput {
		t.Fatalf("ai_usage[%s] = input %d output %d, want input %d output %d", day, gotInput, gotOutput, wantInput, wantOutput)
	}
}

var nonSchemaChars = regexp.MustCompile(`[^a-z0-9_]`)

func postgresImportSchemaName(t *testing.T) string {
	t.Helper()
	name := strings.ToLower(t.Name())
	name = strings.ReplaceAll(name, "/", "_")
	name = nonSchemaChars.ReplaceAllString(name, "_")
	return fmt.Sprintf("test_import_%s_%d_%d", name, time.Now().UnixNano(), rand.Uint64())
}

func databaseURLWithSearchPath(databaseURL, schema string) string {
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}
	return databaseURL + separator + "search_path=" + schema
}
