package storage

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/ohchanwu/jobcron/internal/ai"
)

type queryCounter struct{ count atomic.Int64 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.count.Add(1)
	return ctx
}

func (*queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestAIDealbreakerValidationRoundTrip(t *testing.T) {
	st, _ := newPostgresTestStoreWithSchema(t)
	ctx := context.Background()
	userID := insertMigrationTestUser(t, st, "dealbreaker-roundtrip@example.invalid")
	postingID := insertMigrationTestPosting(t, st, "dealbreaker-roundtrip")
	computedAt := time.Date(2026, 7, 18, 1, 2, 3, 0, time.UTC)
	validation := ai.DealbreakerValidation{
		CandidateID: "keyword-hash",
		Verdict:     ai.DealbreakerNotApplicable,
		Evidence:    "리서치 직무가 아닙니다",
	}

	if err := st.UpsertAIDealbreakerValidation(ctx, userID, postingID, "content-hash", "ai-v1", "keyword-hash", validation, computedAt); err != nil {
		t.Fatalf("UpsertAIDealbreakerValidation: %v", err)
	}
	got := readDealbreakerValidations(t, st, userID, "ai-v1")
	row := got[postingID]["content-hash\x00keyword-hash"]
	if row.PostingID != postingID || row.ContentHash != "content-hash" || row.AIVersion != "ai-v1" ||
		row.KeywordHash != "keyword-hash" || row.Validation != validation || !row.ComputedAt.Equal(computedAt) {
		t.Fatalf("round trip = %+v", row)
	}

	updatedAt := computedAt.Add(time.Hour)
	updated := ai.DealbreakerValidation{CandidateID: "keyword-hash", Verdict: ai.DealbreakerApplies, Evidence: "리서치 업무"}
	if err := st.UpsertAIDealbreakerValidation(ctx, userID, postingID, "content-hash", "ai-v1", "keyword-hash", updated, updatedAt); err != nil {
		t.Fatalf("update validation: %v", err)
	}
	row = readDealbreakerValidations(t, st, userID, "ai-v1")[postingID]["content-hash\x00keyword-hash"]
	if row.Validation != updated || !row.ComputedAt.Equal(updatedAt) {
		t.Fatalf("updated row = %+v", row)
	}
	var count int
	if err := st.db.QueryRowContext(ctx, `SELECT count(*) FROM ai_dealbreaker_validations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("upsert row count = %d, want 1", count)
	}
}

func TestAIDealbreakerValidationIsUserScoped(t *testing.T) {
	st, _ := newPostgresTestStoreWithSchema(t)
	ctx := context.Background()
	firstUserID := insertMigrationTestUser(t, st, "dealbreaker-first@example.invalid")
	secondUserID := insertMigrationTestUser(t, st, "dealbreaker-second@example.invalid")
	postingID := insertMigrationTestPosting(t, st, "dealbreaker-user-scope")
	when := time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)

	first := ai.DealbreakerValidation{CandidateID: "keyword", Verdict: ai.DealbreakerApplies, Evidence: "first"}
	second := ai.DealbreakerValidation{CandidateID: "keyword", Verdict: ai.DealbreakerNotApplicable, Evidence: "second"}
	if err := st.UpsertAIDealbreakerValidation(ctx, firstUserID, postingID, "content", "ai-v1", "keyword", first, when); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAIDealbreakerValidation(ctx, secondUserID, postingID, "content", "ai-v1", "keyword", second, when); err != nil {
		t.Fatal(err)
	}

	key := "content\x00keyword"
	if got := readDealbreakerValidations(t, st, firstUserID, "ai-v1")[postingID][key].Validation; got != first {
		t.Fatalf("first user read = %+v", got)
	}
	if got := readDealbreakerValidations(t, st, secondUserID, "ai-v1")[postingID][key].Validation; got != second {
		t.Fatalf("second user read = %+v", got)
	}
}

func TestAIDealbreakerValidationKeyChangesMiss(t *testing.T) {
	st, _ := newPostgresTestStoreWithSchema(t)
	ctx := context.Background()
	userID := insertMigrationTestUser(t, st, "dealbreaker-key@example.invalid")
	postingID := insertMigrationTestPosting(t, st, "dealbreaker-key")
	validation := ai.DealbreakerValidation{CandidateID: "keyword", Verdict: ai.DealbreakerUncertain}
	if err := st.UpsertAIDealbreakerValidation(ctx, userID, postingID, "content", "ai-v1", "keyword", validation, time.Now()); err != nil {
		t.Fatal(err)
	}

	rows := readDealbreakerValidations(t, st, userID, "ai-v1")[postingID]
	if _, ok := rows["changed-content\x00keyword"]; ok {
		t.Fatal("changed content hash unexpectedly hit cache")
	}
	if _, ok := rows["content\x00changed-keyword"]; ok {
		t.Fatal("changed keyword hash unexpectedly hit cache")
	}
	if got := readDealbreakerValidations(t, st, userID, "ai-v2"); len(got) != 0 {
		t.Fatalf("changed AI version returned %d postings", len(got))
	}
	validation.CandidateID = "different"
	if err := st.UpsertAIDealbreakerValidation(ctx, userID, postingID, "content", "ai-v1", "keyword", validation, time.Now()); err == nil {
		t.Fatal("candidate/hash mismatch succeeded")
	}
	validation.CandidateID = "keyword"
	validation.Verdict = ai.DealbreakerVerdict("invalid")
	if err := st.UpsertAIDealbreakerValidation(ctx, userID, postingID, "content", "ai-v1", "keyword", validation, time.Now()); err == nil {
		t.Fatal("invalid verdict succeeded")
	}
}

func TestAIDealbreakerValidationCascades(t *testing.T) {
	st, _ := newPostgresTestStoreWithSchema(t)
	ctx := context.Background()
	firstUserID := insertMigrationTestUser(t, st, "dealbreaker-cascade-first@example.invalid")
	secondUserID := insertMigrationTestUser(t, st, "dealbreaker-cascade-second@example.invalid")
	firstPostingID := insertMigrationTestPosting(t, st, "dealbreaker-cascade-first")
	secondPostingID := insertMigrationTestPosting(t, st, "dealbreaker-cascade-second")
	validation := ai.DealbreakerValidation{CandidateID: "keyword", Verdict: ai.DealbreakerApplies}
	for _, pair := range [][2]int64{{firstUserID, firstPostingID}, {secondUserID, secondPostingID}} {
		if err := st.UpsertAIDealbreakerValidation(ctx, pair[0], pair[1], "content", "ai-v1", "keyword", validation, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := st.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, firstUserID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.db.QueryRowContext(ctx, `SELECT count(*) FROM ai_dealbreaker_validations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows after user cascade = %d, want second user's row retained", count)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM postings WHERE id = $1`, secondPostingID); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRowContext(ctx, `SELECT count(*) FROM ai_dealbreaker_validations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("validation rows after cascades = %d, want 0", count)
	}
}

func TestAIDealbreakerValidationsUseOneBatchQuery(t *testing.T) {
	st, schema := newPostgresTestStoreWithSchema(t)
	ctx := context.Background()
	userID := insertMigrationTestUser(t, st, "dealbreaker-batch@example.invalid")
	validation := ai.DealbreakerValidation{CandidateID: "keyword", Verdict: ai.DealbreakerApplies}
	for _, suffix := range []string{"batch-first", "batch-second"} {
		postingID := insertMigrationTestPosting(t, st, suffix)
		if err := st.UpsertAIDealbreakerValidation(ctx, userID, postingID, "content", "ai-v1", "keyword", validation, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	config, err := pgx.ParseConfig(databaseURLWithSearchPath(os.Getenv("JOBCRON_TEST_POSTGRES_URL"), schema))
	if err != nil {
		t.Fatalf("parse traced PostgreSQL config: %v", err)
	}
	counter := &queryCounter{}
	config.Tracer = counter
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping traced PostgreSQL connection: %v", err)
	}
	tracedStore := &Store{db: db, dialect: DialectPostgres}
	counter.count.Store(0)

	got, err := tracedStore.AIDealbreakerValidationsByPostingID(ctx, userID, "ai-v1")
	if err != nil {
		t.Fatalf("AIDealbreakerValidationsByPostingID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("batched read returned %d postings, want 2", len(got))
	}
	if queries := counter.count.Load(); queries != 1 {
		t.Fatalf("batched read executed %d SQL queries, want 1", queries)
	}
}

func TestAIDealbreakerValidationRejectsSQLite(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	validation := ai.DealbreakerValidation{CandidateID: "keyword", Verdict: ai.DealbreakerApplies}
	if err := st.UpsertAIDealbreakerValidation(ctx, 1, 1, "content", "ai-v1", "keyword", validation, time.Now()); err == nil {
		t.Fatal("SQLite upsert succeeded")
	}
	if _, err := st.AIDealbreakerValidationsByPostingID(ctx, 1, "ai-v1"); err == nil {
		t.Fatal("SQLite read succeeded")
	}
}

func TestAIDealbreakerValidationRejectsNonPositiveUserID(t *testing.T) {
	st := &Store{dialect: DialectPostgres}
	validation := ai.DealbreakerValidation{CandidateID: "keyword", Verdict: ai.DealbreakerApplies}
	for _, userID := range []int64{0, -1} {
		if err := st.UpsertAIDealbreakerValidation(context.Background(), userID, 1, "content", "ai-v1", "keyword", validation, time.Now()); err == nil {
			t.Fatalf("upsert user ID %d succeeded", userID)
		}
		if _, err := st.AIDealbreakerValidationsByPostingID(context.Background(), userID, "ai-v1"); err == nil {
			t.Fatalf("read user ID %d succeeded", userID)
		}
	}
}

func readDealbreakerValidations(t *testing.T, st *Store, userID int64, aiVersion string) map[int64]map[string]AIDealbreakerValidation {
	t.Helper()
	got, err := st.AIDealbreakerValidationsByPostingID(context.Background(), userID, aiVersion)
	if err != nil {
		t.Fatalf("AIDealbreakerValidationsByPostingID: %v", err)
	}
	return got
}
