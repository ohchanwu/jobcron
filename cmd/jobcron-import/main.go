package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/sqlitesnapshot"
	_ "modernc.org/sqlite"
)

type importOptions struct {
	sqlitePath  string
	postgresURL string
	ownerEmail  string
	aiKeysPath  string
	apply       bool
	out         io.Writer
}

type categoryCounts struct {
	Profile       int `json:"profile"`
	Postings      int `json:"postings"`
	Scores        int `json:"scores"`
	Bookmarks     int `json:"bookmarks"`
	NotInterested int `json:"not_interested"`
	AIExtractions int `json:"ai_extractions"`
	AIScores      int `json:"ai_scores"`
	AIUsage       int `json:"ai_usage"`
	AIProviders   int `json:"ai_providers"`
}

type importPlan struct {
	SourceSHA256 string
	Source       categoryCounts
	Target       categoryCounts
	Collisions   categoryCounts
}

func main() {
	var opts importOptions
	flag.StringVar(&opts.sqlitePath, "sqlite", "", "path to the source SQLite jobs.db")
	flag.StringVar(&opts.postgresURL, "postgres", "", "target PostgreSQL database URL")
	flag.StringVar(&opts.ownerEmail, "owner-email", "", "existing owner account email for imported state")
	flag.StringVar(&opts.aiKeysPath, "ai-keys", "", "optional legacy ai_keys.json path")
	flag.BoolVar(&opts.apply, "apply", false, "apply the verified plan to PostgreSQL")
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
		return fmt.Errorf("import: --owner-email is required")
	}

	workDir, err := os.MkdirTemp("", "jobcron-import-snapshot-*")
	if err != nil {
		return fmt.Errorf("import: create private snapshot directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	snapshot, err := sqlitesnapshot.Create(ctx, opts.sqlitePath, workDir)
	if err != nil {
		return errors.New("import: SQLite source is unavailable, invalid, or busy")
	}
	defer os.Remove(snapshot.Path)
	sourceDB, err := openSQLiteSnapshot(ctx, snapshot.Path)
	if err != nil {
		return err
	}
	defer sourceDB.Close()

	_, providers, err := loadLegacyKeys(opts.aiKeysPath)
	if err != nil {
		return err
	}
	sourceCounts, err := readCounts(ctx, sourceDB)
	if err != nil {
		return err
	}
	sourceCounts.AIProviders = len(providers)

	targetDB, err := sql.Open("pgx", opts.postgresURL)
	if err != nil {
		return errors.New("import: PostgreSQL target is unavailable")
	}
	defer targetDB.Close()
	if err := targetDB.PingContext(ctx); err != nil {
		return errors.New("import: PostgreSQL target is unavailable")
	}
	ownerID, err := lookupSoleOwner(ctx, targetDB, opts.ownerEmail)
	if err != nil {
		return err
	}
	targetCounts, err := readTargetCounts(ctx, targetDB, ownerID)
	if err != nil {
		return err
	}
	collisions, err := readCollisionCounts(ctx, sourceDB, targetDB, ownerID, providers)
	if err != nil {
		return err
	}
	plan := importPlan{
		SourceSHA256: snapshot.SHA256,
		Source:       sourceCounts,
		Target:       targetCounts,
		Collisions:   collisions,
	}
	printPlan(opts.out, !opts.apply, plan, providers)
	if !opts.apply {
		return nil
	}

	tx, err := targetDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("import: begin postgres transaction: %w", err)
	}
	defer tx.Rollback()

	if err := copyProfile(ctx, sourceDB, tx, ownerID); err != nil {
		return err
	}
	if err := copyPostings(ctx, sourceDB, tx); err != nil {
		return err
	}
	if err := copyScores(ctx, sourceDB, tx, ownerID); err != nil {
		return err
	}
	if err := copyBookmarks(ctx, sourceDB, tx, ownerID); err != nil {
		return err
	}
	if err := copyNotInterested(ctx, sourceDB, tx, ownerID); err != nil {
		return err
	}
	if err := copyAIExtractions(ctx, sourceDB, tx); err != nil {
		return err
	}
	if err := copyAIScores(ctx, sourceDB, tx, ownerID); err != nil {
		return err
	}
	if err := copyAIUsage(ctx, sourceDB, tx, ownerID); err != nil {
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

func readCounts(ctx context.Context, db *sql.DB) (categoryCounts, error) {
	var c categoryCounts
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

func printPlan(w io.Writer, dryRun bool, plan importPlan, providers []string) {
	fmt.Fprintf(w, "dry run: %t\n", dryRun)
	fmt.Fprintf(w, "source_sha256: %s\n", plan.SourceSHA256)
	if len(providers) == 0 {
		fmt.Fprintln(w, "providers: none")
	} else {
		fmt.Fprintf(w, "providers: %s\n", strings.Join(providers, ","))
	}
	for _, section := range []struct {
		name   string
		counts categoryCounts
	}{
		{"source", plan.Source},
		{"target", plan.Target},
		{"collisions", plan.Collisions},
	} {
		fmt.Fprintf(w, "%s:\n", section.name)
		printCategoryCounts(w, section.counts)
	}
}

func printCategoryCounts(w io.Writer, c categoryCounts) {
	fmt.Fprintf(w, "profile: %d\n", c.Profile)
	fmt.Fprintf(w, "postings: %d\n", c.Postings)
	fmt.Fprintf(w, "scores: %d\n", c.Scores)
	fmt.Fprintf(w, "bookmarks: %d\n", c.Bookmarks)
	fmt.Fprintf(w, "not_interested: %d\n", c.NotInterested)
	fmt.Fprintf(w, "ai_extractions: %d\n", c.AIExtractions)
	fmt.Fprintf(w, "ai_scores: %d\n", c.AIScores)
	fmt.Fprintf(w, "ai_usage: %d\n", c.AIUsage)
	fmt.Fprintf(w, "ai_providers: %d\n", c.AIProviders)
}

func openSQLiteSnapshot(ctx context.Context, path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := u.Query()
	query.Set("mode", "ro")
	query.Set("immutable", "1")
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, errors.New("import: open verified SQLite snapshot")
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, errors.New("import: verify SQLite snapshot connection")
	}
	return db, nil
}

func loadLegacyKeys(path string) (map[string]string, []string, error) {
	if path == "" {
		return map[string]string{}, nil, nil
	}
	raw, err := ai.LoadKeys(path)
	if err != nil {
		return nil, nil, errors.New("import: legacy AI keys are unavailable or invalid")
	}
	normalized := make(map[string]string, len(raw))
	for provider, key := range raw {
		provider = strings.ToLower(strings.TrimSpace(provider))
		key = strings.TrimSpace(key)
		if provider == "" || key == "" {
			continue
		}
		if provider != "anthropic" {
			return nil, nil, fmt.Errorf("import: unsupported legacy AI provider %q", provider)
		}
		if _, exists := normalized[provider]; exists {
			return nil, nil, fmt.Errorf("import: duplicate provider %q after normalization", provider)
		}
		normalized[provider] = key
	}
	providers := make([]string, 0, len(normalized))
	for provider := range normalized {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return normalized, providers, nil
}

func lookupSoleOwner(ctx context.Context, db *sql.DB, expectedEmail string) (int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, email FROM users ORDER BY id LIMIT 2`)
	if err != nil {
		return 0, fmt.Errorf("import: query owner: %w", err)
	}
	defer rows.Close()
	type owner struct {
		id    int64
		email string
	}
	var owners []owner
	for rows.Next() {
		var candidate owner
		if err := rows.Scan(&candidate.id, &candidate.email); err != nil {
			return 0, fmt.Errorf("import: scan owner: %w", err)
		}
		owners = append(owners, candidate)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("import: iterate owners: %w", err)
	}
	if len(owners) != 1 {
		return 0, fmt.Errorf("import: target must contain exactly one owner")
	}
	if owners[0].email != expectedEmail {
		return 0, fmt.Errorf("import: owner email mismatch")
	}
	return owners[0].id, nil
}

func readTargetCounts(ctx context.Context, db *sql.DB, ownerID int64) (categoryCounts, error) {
	var c categoryCounts
	queries := []struct {
		name  string
		query string
		args  []any
		dest  *int
	}{
		{"profile", `SELECT count(*) FROM profiles WHERE user_id = $1`, []any{ownerID}, &c.Profile},
		{"postings", `SELECT count(*) FROM postings`, nil, &c.Postings},
		{"scores", `SELECT count(*) FROM scores WHERE user_id = $1`, []any{ownerID}, &c.Scores},
		{"bookmarks", `SELECT count(*) FROM bookmarks WHERE user_id = $1`, []any{ownerID}, &c.Bookmarks},
		{"not_interested", `SELECT count(*) FROM not_interested WHERE user_id = $1`, []any{ownerID}, &c.NotInterested},
		{"ai_extractions", `SELECT count(*) FROM ai_extractions`, nil, &c.AIExtractions},
		{"ai_scores", `SELECT count(*) FROM ai_scores WHERE user_id = $1`, []any{ownerID}, &c.AIScores},
		{"ai_usage", `SELECT count(*) FROM ai_usage WHERE user_id = $1`, []any{ownerID}, &c.AIUsage},
		{"ai_providers", `SELECT count(*) FROM user_ai_credentials WHERE user_id = $1`, []any{ownerID}, &c.AIProviders},
	}
	for _, item := range queries {
		if err := db.QueryRowContext(ctx, item.query, item.args...).Scan(item.dest); err != nil {
			return c, fmt.Errorf("import: count target %s: %w", item.name, err)
		}
	}
	return c, nil
}

func readCollisionCounts(ctx context.Context, source, target *sql.DB, ownerID int64, providers []string) (categoryCounts, error) {
	var c categoryCounts
	var sourceProfile bool
	if err := source.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM profile WHERE id = 1)`).Scan(&sourceProfile); err != nil {
		return c, fmt.Errorf("import: inspect source profile collision: %w", err)
	}
	if sourceProfile {
		var targetProfile bool
		if err := target.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM profiles WHERE user_id = $1)`, ownerID).Scan(&targetProfile); err != nil {
			return c, fmt.Errorf("import: inspect target profile collision: %w", err)
		}
		if targetProfile {
			c.Profile = 1
		}
	}

	checks := []struct {
		name        string
		sourceQuery string
		targetQuery string
		columns     int
		prefix      []any
		dest        *int
	}{
		{"postings", `SELECT id, source, source_posting_id FROM postings`, `SELECT EXISTS(SELECT 1 FROM postings WHERE id = $1 OR (source = $2 AND source_posting_id = $3))`, 3, nil, &c.Postings},
		{"scores", `SELECT posting_id FROM scores`, `SELECT EXISTS(SELECT 1 FROM scores WHERE user_id = $1 AND posting_id = $2)`, 1, []any{ownerID}, &c.Scores},
		{"bookmarks", `SELECT posting_id FROM bookmarks`, `SELECT EXISTS(SELECT 1 FROM bookmarks WHERE user_id = $1 AND posting_id = $2)`, 1, []any{ownerID}, &c.Bookmarks},
		{"not_interested", `SELECT posting_id FROM not_interested`, `SELECT EXISTS(SELECT 1 FROM not_interested WHERE user_id = $1 AND posting_id = $2)`, 1, []any{ownerID}, &c.NotInterested},
		{"ai_extractions", `SELECT posting_id, content_hash, ai_version FROM ai_extractions`, `SELECT EXISTS(SELECT 1 FROM ai_extractions WHERE posting_id = $1 AND content_hash = $2 AND ai_version = $3)`, 3, nil, &c.AIExtractions},
		{"ai_scores", `SELECT posting_id, ai_input_hash, ai_version FROM ai_scores`, `SELECT EXISTS(SELECT 1 FROM ai_scores WHERE user_id = $1 AND posting_id = $2 AND ai_input_hash = $3 AND ai_version = $4)`, 3, []any{ownerID}, &c.AIScores},
		{"ai_usage", `SELECT day FROM ai_usage`, `SELECT EXISTS(SELECT 1 FROM ai_usage WHERE user_id = $1 AND day = $2)`, 1, []any{ownerID}, &c.AIUsage},
	}
	for _, check := range checks {
		count, err := countKeyCollisions(ctx, source, target, check.sourceQuery, check.targetQuery, check.columns, check.prefix)
		if err != nil {
			return c, fmt.Errorf("import: inspect %s collisions: %w", check.name, err)
		}
		*check.dest = count
	}
	for _, provider := range providers {
		var exists bool
		if err := target.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_ai_credentials WHERE user_id = $1 AND provider = $2)`, ownerID, provider).Scan(&exists); err != nil {
			return c, fmt.Errorf("import: inspect AI provider collisions: %w", err)
		}
		if exists {
			c.AIProviders++
		}
	}
	return c, nil
}

func countKeyCollisions(ctx context.Context, source, target *sql.DB, sourceQuery, targetQuery string, columns int, prefix []any) (int, error) {
	rows, err := source.QueryContext(ctx, sourceQuery)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		values := make([]any, columns)
		destinations := make([]any, columns)
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return 0, err
		}
		args := append(append([]any{}, prefix...), values...)
		var exists bool
		if err := target.QueryRowContext(ctx, targetQuery, args...).Scan(&exists); err != nil {
			return 0, err
		}
		if exists {
			count++
		}
	}
	return count, rows.Err()
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

func copyScores(ctx context.Context, source *sql.DB, tx *sql.Tx, ownerID int64) error {
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
INSERT INTO scores (user_id, posting_id, profile_hash, total, breakdown_json, computed_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, posting_id) DO UPDATE SET
    profile_hash = EXCLUDED.profile_hash,
    total = EXCLUDED.total,
    breakdown_json = EXCLUDED.breakdown_json,
    computed_at = EXCLUDED.computed_at`,
			ownerID, postingID, profileHash, total, breakdownJSON, computedAt.UTC()); err != nil {
			return fmt.Errorf("import: write score for posting %d: %w", postingID, err)
		}
	}
	return rows.Err()
}

func copyBookmarks(ctx context.Context, source *sql.DB, tx *sql.Tx, ownerID int64) error {
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
INSERT INTO bookmarks (user_id, posting_id, bookmarked_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, posting_id) DO UPDATE SET bookmarked_at = EXCLUDED.bookmarked_at`,
			ownerID, postingID, bookmarkedAt.UTC()); err != nil {
			return fmt.Errorf("import: write bookmark for posting %d: %w", postingID, err)
		}
	}
	return rows.Err()
}

func copyNotInterested(ctx context.Context, source *sql.DB, tx *sql.Tx, ownerID int64) error {
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
INSERT INTO not_interested (user_id, posting_id, muted_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, posting_id) DO UPDATE SET muted_at = EXCLUDED.muted_at`,
			ownerID, postingID, mutedAt.UTC()); err != nil {
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

func copyAIScores(ctx context.Context, source *sql.DB, tx *sql.Tx, ownerID int64) error {
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
INSERT INTO ai_scores (user_id, posting_id, ai_input_hash, ai_version, items_json, net_delta, computed_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (user_id, posting_id, ai_input_hash, ai_version) DO UPDATE SET
    items_json = EXCLUDED.items_json,
    net_delta = EXCLUDED.net_delta,
    computed_at = EXCLUDED.computed_at`,
			ownerID, postingID, inputHash, aiVersion, itemsJSON, netDelta, computedAt.UTC()); err != nil {
			return fmt.Errorf("import: write ai_score for posting %d: %w", postingID, err)
		}
	}
	return rows.Err()
}

func copyAIUsage(ctx context.Context, source *sql.DB, tx *sql.Tx, ownerID int64) error {
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
INSERT INTO ai_usage (user_id, day, input_tokens, output_tokens)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, day) DO UPDATE SET
    input_tokens = GREATEST(ai_usage.input_tokens, EXCLUDED.input_tokens),
    output_tokens = GREATEST(ai_usage.output_tokens, EXCLUDED.output_tokens)`,
			ownerID, day, inputTokens, outputTokens); err != nil {
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
