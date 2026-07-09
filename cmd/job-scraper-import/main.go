package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ohchanwu/job-scraper/internal/storage"
)

const defaultOwnerEmail = "sqlite-import-owner@job-scraper.local"

type importOptions struct {
	sqlitePath  string
	postgresURL string
	ownerEmail  string
	dryRun      bool
	out         io.Writer
}

type tableCounts struct {
	Profile       int
	Postings      int
	Scores        int
	Bookmarks     int
	NotInterested int
	AIExtractions int
	AIScores      int
	AIUsage       int
}

func main() {
	var opts importOptions
	flag.StringVar(&opts.sqlitePath, "sqlite", "", "path to the source SQLite jobs.db")
	flag.StringVar(&opts.postgresURL, "postgres", "", "target PostgreSQL database URL")
	flag.StringVar(&opts.ownerEmail, "owner-email", defaultOwnerEmail, "owner account email for imported single-user profile and state")
	flag.BoolVar(&opts.dryRun, "dry-run", false, "report source counts without writing PostgreSQL")
	flag.Parse()
	opts.out = os.Stdout

	if err := runImport(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runImport(ctx context.Context, opts importOptions) error {
	if opts.out == nil {
		opts.out = io.Discard
	}
	if opts.sqlitePath == "" {
		return fmt.Errorf("import: --sqlite is required")
	}
	if opts.postgresURL == "" {
		return fmt.Errorf("import: --postgres is required")
	}
	if opts.ownerEmail == "" {
		opts.ownerEmail = defaultOwnerEmail
	}

	source, err := storage.OpenSQLiteAt(opts.sqlitePath)
	if err != nil {
		return err
	}
	defer source.Close()
	sourceDB := source.SQLDB()

	counts, err := readCounts(ctx, sourceDB)
	if err != nil {
		return err
	}
	printCounts(opts.out, opts.dryRun, counts)
	if opts.dryRun {
		return nil
	}

	target, err := storage.OpenPostgres(opts.postgresURL)
	if err != nil {
		return err
	}
	defer target.Close()
	targetDB := target.SQLDB()

	tx, err := targetDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("import: begin postgres transaction: %w", err)
	}
	defer tx.Rollback()

	ownerID, err := ensureOwnerUser(ctx, tx, opts.ownerEmail)
	if err != nil {
		return err
	}
	if err := copyProfile(ctx, sourceDB, tx, ownerID); err != nil {
		return err
	}
	if err := copyPostings(ctx, sourceDB, tx); err != nil {
		return err
	}
	if err := copyScores(ctx, sourceDB, tx); err != nil {
		return err
	}
	if err := copyBookmarks(ctx, sourceDB, tx); err != nil {
		return err
	}
	if err := copyNotInterested(ctx, sourceDB, tx); err != nil {
		return err
	}
	if err := copyAIExtractions(ctx, sourceDB, tx); err != nil {
		return err
	}
	if err := copyAIScores(ctx, sourceDB, tx); err != nil {
		return err
	}
	if err := copyAIUsage(ctx, sourceDB, tx); err != nil {
		return err
	}
	if err := resetPostgresSequences(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("import: commit postgres transaction: %w", err)
	}
	fmt.Fprintln(opts.out, "import complete")
	return nil
}

func readCounts(ctx context.Context, db *sql.DB) (tableCounts, error) {
	var c tableCounts
	counts := []struct {
		table string
		dest  *int
	}{
		{"profile", &c.Profile},
		{"postings", &c.Postings},
		{"scores", &c.Scores},
		{"bookmarks", &c.Bookmarks},
		{"not_interested", &c.NotInterested},
		{"ai_extractions", &c.AIExtractions},
		{"ai_scores", &c.AIScores},
		{"ai_usage", &c.AIUsage},
	}
	for _, item := range counts {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+item.table).Scan(item.dest); err != nil {
			return c, fmt.Errorf("import: count %s: %w", item.table, err)
		}
	}
	return c, nil
}

func printCounts(w io.Writer, dryRun bool, c tableCounts) {
	fmt.Fprintf(w, "dry run: %t\n", dryRun)
	fmt.Fprintf(w, "profile: %d\n", c.Profile)
	fmt.Fprintf(w, "postings: %d\n", c.Postings)
	fmt.Fprintf(w, "scores: %d\n", c.Scores)
	fmt.Fprintf(w, "bookmarks: %d\n", c.Bookmarks)
	fmt.Fprintf(w, "not_interested: %d\n", c.NotInterested)
	fmt.Fprintf(w, "ai_extractions: %d\n", c.AIExtractions)
	fmt.Fprintf(w, "ai_scores: %d\n", c.AIScores)
	fmt.Fprintf(w, "ai_usage: %d\n", c.AIUsage)
}

func ensureOwnerUser(ctx context.Context, tx *sql.Tx, ownerEmail string) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ($1, $2, now(), now())
ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
RETURNING id`,
		ownerEmail, "imported-sqlite-no-login").Scan(&id); err != nil {
		return 0, fmt.Errorf("import: create owner user: %w", err)
	}
	return id, nil
}

func copyProfile(ctx context.Context, source *sql.DB, tx *sql.Tx, ownerID int64) error {
	row := source.QueryRowContext(ctx, `SELECT profile_json, profile_hash, updated_at FROM profile WHERE id = 1`)
	var (
		profileJSON string
		profileHash string
		updatedAt   time.Time
	)
	err := row.Scan(&profileJSON, &profileHash, &updatedAt)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("import: read profile: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO profile (id, profile_json, profile_hash, updated_at)
VALUES (1, $1, $2, $3)
ON CONFLICT (id) DO UPDATE SET
    profile_json = EXCLUDED.profile_json,
    profile_hash = EXCLUDED.profile_hash,
    updated_at = EXCLUDED.updated_at`,
		profileJSON, profileHash, updatedAt.UTC()); err != nil {
		return fmt.Errorf("import: write legacy profile: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO profiles (user_id, profile_json, profile_hash, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE SET
    profile_json = EXCLUDED.profile_json,
    profile_hash = EXCLUDED.profile_hash,
    updated_at = EXCLUDED.updated_at`,
		ownerID, profileJSON, profileHash, updatedAt.UTC()); err != nil {
		return fmt.Errorf("import: write owner profile: %w", err)
	}
	return nil
}

func copyPostings(ctx context.Context, source *sql.DB, tx *sql.Tx) error {
	rows, err := source.QueryContext(ctx, `
SELECT id, source, source_posting_id, url, title, company, location,
       newcomer, min_career, max_career, career_level, education, education_name,
       stack_tags_json, tags_json, description, raw_json, published_at, closed_at,
       always_open, first_seen_at, last_seen_at, duplicate_of, detail_fetched_at
FROM postings
ORDER BY id`)
	if err != nil {
		return fmt.Errorf("import: read postings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r postingRow
		if err := rows.Scan(
			&r.ID, &r.Source, &r.SourcePostingID, &r.URL, &r.Title, &r.Company, &r.Location,
			&r.Newcomer, &r.MinCareer, &r.MaxCareer, &r.CareerLevel, &r.Education, &r.EducationName,
			&r.StackTagsJSON, &r.TagsJSON, &r.Description, &r.RawJSON, &r.PublishedAt, &r.ClosedAt,
			&r.AlwaysOpen, &r.FirstSeenAt, &r.LastSeenAt, &r.DuplicateOf, &r.DetailFetchedAt,
		); err != nil {
			return fmt.Errorf("import: scan posting: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO postings (
    id, source, source_posting_id, url, title, company, location,
    newcomer, min_career, max_career, career_level, education, education_name,
    stack_tags_json, tags_json, description, raw_json, published_at, closed_at,
    always_open, first_seen_at, last_seen_at, duplicate_of, detail_fetched_at
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24
)
ON CONFLICT (id) DO UPDATE SET
    source = EXCLUDED.source,
    source_posting_id = EXCLUDED.source_posting_id,
    url = EXCLUDED.url,
    title = EXCLUDED.title,
    company = EXCLUDED.company,
    location = EXCLUDED.location,
    newcomer = EXCLUDED.newcomer,
    min_career = EXCLUDED.min_career,
    max_career = EXCLUDED.max_career,
    career_level = EXCLUDED.career_level,
    education = EXCLUDED.education,
    education_name = EXCLUDED.education_name,
    stack_tags_json = EXCLUDED.stack_tags_json,
    tags_json = EXCLUDED.tags_json,
    description = EXCLUDED.description,
    raw_json = EXCLUDED.raw_json,
    published_at = EXCLUDED.published_at,
    closed_at = EXCLUDED.closed_at,
    always_open = EXCLUDED.always_open,
    first_seen_at = EXCLUDED.first_seen_at,
    last_seen_at = EXCLUDED.last_seen_at,
    duplicate_of = EXCLUDED.duplicate_of,
    detail_fetched_at = EXCLUDED.detail_fetched_at`,
			r.ID, r.Source, r.SourcePostingID, r.URL, r.Title, r.Company, r.Location,
			r.Newcomer, r.MinCareer, r.MaxCareer, r.CareerLevel, r.Education, r.EducationName,
			r.StackTagsJSON, r.TagsJSON, r.Description, r.RawJSON, r.PublishedAt, r.ClosedAt,
			r.AlwaysOpen, r.FirstSeenAt.UTC(), r.LastSeenAt.UTC(), r.DuplicateOf, r.DetailFetchedAt); err != nil {
			return fmt.Errorf("import: write posting %d: %w", r.ID, err)
		}
	}
	return rows.Err()
}

func copyScores(ctx context.Context, source *sql.DB, tx *sql.Tx) error {
	rows, err := source.QueryContext(ctx, `SELECT posting_id, profile_hash, total, breakdown_json, computed_at FROM scores`)
	if err != nil {
		return fmt.Errorf("import: read scores: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var postingID int64
		var profileHash, breakdownJSON string
		var total int
		var computedAt time.Time
		if err := rows.Scan(&postingID, &profileHash, &total, &breakdownJSON, &computedAt); err != nil {
			return fmt.Errorf("import: scan score: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO scores (posting_id, profile_hash, total, breakdown_json, computed_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (posting_id) DO UPDATE SET
    profile_hash = EXCLUDED.profile_hash,
    total = EXCLUDED.total,
    breakdown_json = EXCLUDED.breakdown_json,
    computed_at = EXCLUDED.computed_at`,
			postingID, profileHash, total, breakdownJSON, computedAt.UTC()); err != nil {
			return fmt.Errorf("import: write score for posting %d: %w", postingID, err)
		}
	}
	return rows.Err()
}

func copyBookmarks(ctx context.Context, source *sql.DB, tx *sql.Tx) error {
	rows, err := source.QueryContext(ctx, `SELECT posting_id, bookmarked_at FROM bookmarks`)
	if err != nil {
		return fmt.Errorf("import: read bookmarks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var postingID int64
		var bookmarkedAt time.Time
		if err := rows.Scan(&postingID, &bookmarkedAt); err != nil {
			return fmt.Errorf("import: scan bookmark: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO bookmarks (posting_id, bookmarked_at)
VALUES ($1, $2)
ON CONFLICT (posting_id) DO UPDATE SET bookmarked_at = EXCLUDED.bookmarked_at`,
			postingID, bookmarkedAt.UTC()); err != nil {
			return fmt.Errorf("import: write bookmark for posting %d: %w", postingID, err)
		}
	}
	return rows.Err()
}

func copyNotInterested(ctx context.Context, source *sql.DB, tx *sql.Tx) error {
	rows, err := source.QueryContext(ctx, `SELECT posting_id, muted_at FROM not_interested`)
	if err != nil {
		return fmt.Errorf("import: read not_interested: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var postingID int64
		var mutedAt time.Time
		if err := rows.Scan(&postingID, &mutedAt); err != nil {
			return fmt.Errorf("import: scan not_interested: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO not_interested (posting_id, muted_at)
VALUES ($1, $2)
ON CONFLICT (posting_id) DO UPDATE SET muted_at = EXCLUDED.muted_at`,
			postingID, mutedAt.UTC()); err != nil {
			return fmt.Errorf("import: write not_interested for posting %d: %w", postingID, err)
		}
	}
	return rows.Err()
}

func copyAIExtractions(ctx context.Context, source *sql.DB, tx *sql.Tx) error {
	rows, err := source.QueryContext(ctx, `
SELECT posting_id, content_hash, ai_version, min_career, max_career, newcomer, education_enum, evidence, computed_at
FROM ai_extractions`)
	if err != nil {
		return fmt.Errorf("import: read ai_extractions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var postingID int64
		var contentHash, aiVersion, educationEnum, evidence string
		var minCareer int
		var maxCareer sql.NullInt64
		var newcomer bool
		var computedAt time.Time
		if err := rows.Scan(&postingID, &contentHash, &aiVersion, &minCareer, &maxCareer, &newcomer, &educationEnum, &evidence, &computedAt); err != nil {
			return fmt.Errorf("import: scan ai_extraction: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ai_extractions
    (posting_id, content_hash, ai_version, min_career, max_career, newcomer, education_enum, evidence, computed_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (posting_id, content_hash, ai_version) DO UPDATE SET
    min_career = EXCLUDED.min_career,
    max_career = EXCLUDED.max_career,
    newcomer = EXCLUDED.newcomer,
    education_enum = EXCLUDED.education_enum,
    evidence = EXCLUDED.evidence,
    computed_at = EXCLUDED.computed_at`,
			postingID, contentHash, aiVersion, minCareer, maxCareer, newcomer, educationEnum, evidence, computedAt.UTC()); err != nil {
			return fmt.Errorf("import: write ai_extraction for posting %d: %w", postingID, err)
		}
	}
	return rows.Err()
}

func copyAIScores(ctx context.Context, source *sql.DB, tx *sql.Tx) error {
	rows, err := source.QueryContext(ctx, `SELECT posting_id, ai_input_hash, ai_version, items_json, net_delta, computed_at FROM ai_scores`)
	if err != nil {
		return fmt.Errorf("import: read ai_scores: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var postingID int64
		var inputHash, aiVersion, itemsJSON string
		var netDelta int
		var computedAt time.Time
		if err := rows.Scan(&postingID, &inputHash, &aiVersion, &itemsJSON, &netDelta, &computedAt); err != nil {
			return fmt.Errorf("import: scan ai_score: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ai_scores (posting_id, ai_input_hash, ai_version, items_json, net_delta, computed_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (posting_id, ai_input_hash, ai_version) DO UPDATE SET
    items_json = EXCLUDED.items_json,
    net_delta = EXCLUDED.net_delta,
    computed_at = EXCLUDED.computed_at`,
			postingID, inputHash, aiVersion, itemsJSON, netDelta, computedAt.UTC()); err != nil {
			return fmt.Errorf("import: write ai_score for posting %d: %w", postingID, err)
		}
	}
	return rows.Err()
}

func copyAIUsage(ctx context.Context, source *sql.DB, tx *sql.Tx) error {
	rows, err := source.QueryContext(ctx, `SELECT day, input_tokens, output_tokens FROM ai_usage`)
	if err != nil {
		return fmt.Errorf("import: read ai_usage: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var day string
		var inputTokens, outputTokens int
		if err := rows.Scan(&day, &inputTokens, &outputTokens); err != nil {
			return fmt.Errorf("import: scan ai_usage: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ai_usage (day, input_tokens, output_tokens)
VALUES ($1, $2, $3)
ON CONFLICT (day) DO UPDATE SET
    input_tokens = EXCLUDED.input_tokens,
    output_tokens = EXCLUDED.output_tokens`,
			day, inputTokens, outputTokens); err != nil {
			return fmt.Errorf("import: write ai_usage for day %s: %w", day, err)
		}
	}
	return rows.Err()
}

func resetPostgresSequences(ctx context.Context, tx *sql.Tx) error {
	for _, item := range []struct {
		sequence string
		table    string
		column   string
	}{
		{"postings_id_seq", "postings", "id"},
		{"users_id_seq", "users", "id"},
	} {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			`SELECT setval(pg_get_serial_sequence('%s', '%s'), COALESCE((SELECT MAX(%s) FROM %s), 1), true)`,
			item.table, item.column, item.column, item.table,
		)); err != nil {
			return fmt.Errorf("import: reset %s: %w", item.sequence, err)
		}
	}
	return nil
}

type postingRow struct {
	ID              int64
	Source          string
	SourcePostingID string
	URL             string
	Title           string
	Company         string
	Location        sql.NullString
	Newcomer        bool
	MinCareer       int
	MaxCareer       int
	CareerLevel     sql.NullString
	Education       sql.NullInt64
	EducationName   sql.NullString
	StackTagsJSON   string
	TagsJSON        string
	Description     string
	RawJSON         string
	PublishedAt     sql.NullTime
	ClosedAt        sql.NullTime
	AlwaysOpen      bool
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	DuplicateOf     sql.NullInt64
	DetailFetchedAt sql.NullTime
}
