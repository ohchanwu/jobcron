package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
)

// queryReader is only a SQL execution boundary, not a repository interface.
// Both the ordinary concrete Store and the read transaction use the same parsers.
type queryReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// PostingViewData contains one committed view. No encrypted credential fields,
// materialized score or runtime are needed for deterministic read composition.
type PostingViewData struct {
	ProfileJSON               string
	ProfileHash               string
	HasProfile                bool
	CredentialPresent         bool
	Postings                  []scraper.Posting
	Bookmarks                 map[int64]bool
	BookmarkOrder             []int64
	Muted                     map[int64]bool
	DuplicateSources          map[int64][]string
	ContentHashes             map[int64]string
	Extractions               map[int64]ai.Extraction
	Validations               map[int64]map[string]AIDealbreakerValidation
	Fresh, Latest, AnyVersion map[int64]ai.Delta
	Outcomes                  map[int64]AIScoreOutcome
}

// PostingViewRead intentionally exposes no writer. Begin performs a real SELECT;
// callers can release their commit/read barrier only AFTER it returns. Load must
// finish on this same tx, and Close precedes templates and network output.
type PostingViewRead struct {
	store  *Store
	tx     *sql.Tx
	userID int64
	data   PostingViewData
}

func (s *Store) BeginPostingView(ctx context.Context, userID int64) (*PostingViewRead, error) {
	if s.dialect == DialectPostgres && userID <= 0 {
		return nil, errors.New("storage: posting view user required")
	}
	opts := &sql.TxOptions{ReadOnly: true}
	if s.dialect == DialectPostgres {
		opts.Isolation = sql.LevelRepeatableRead
	}
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	v := &PostingViewRead{store: s, tx: tx, userID: userID}
	query := `SELECT profile_json,profile_hash FROM profile WHERE id=1`
	args := []any{}
	if s.dialect == DialectPostgres {
		query = s.query(`SELECT profile_json,profile_hash FROM profiles WHERE user_id=?`)
		args = append(args, userID)
	}
	err = tx.QueryRowContext(ctx, query, args...).Scan(&v.data.ProfileJSON, &v.data.ProfileHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		return nil, err
	}
	v.data.HasProfile = err == nil
	return v, nil
}

func (v *PostingViewRead) Close() error {
	err := v.tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return err
}

func (v *PostingViewRead) Load(ctx context.Context, goalHash, scoreVersion string, enabled, demo bool) (PostingViewData, error) {
	d := v.data
	s := v.store
	fail := func(err error) (PostingViewData, error) {
		return PostingViewData{}, fmt.Errorf("storage: posting view unavailable: %w", err)
	}
	rows, err := v.tx.QueryContext(ctx, `SELECT `+selectColumns+` FROM postings ORDER BY first_seen_at DESC,id DESC`)
	if err != nil {
		return fail(err)
	}
	d.Postings, err = scanPostings(rows)
	closeErr := rows.Close()
	if err != nil {
		return fail(err)
	}
	if closeErr != nil {
		return fail(closeErr)
	}
	d.Bookmarks = map[int64]bool{}
	d.Muted = map[int64]bool{}
	for _, table := range []string{"bookmarks", "not_interested"} {
		column := "bookmarked_at"
		if table == "not_interested" {
			column = "muted_at"
		}
		query := `SELECT posting_id FROM ` + table
		args := []any{}
		if s.dialect == DialectPostgres {
			query += ` WHERE user_id=?`
			args = append(args, v.userID)
		}
		query += ` ORDER BY ` + column + ` DESC,posting_id DESC`
		rows, err := v.tx.QueryContext(ctx, s.query(query), args...)
		if err != nil {
			return fail(err)
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return fail(err)
			}
			if table == "bookmarks" {
				d.Bookmarks[id] = true
				d.BookmarkOrder = append(d.BookmarkOrder, id)
			} else {
				d.Muted[id] = true
			}
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return fail(err)
		}
		if closeErr != nil {
			return fail(closeErr)
		}
	}
	d.DuplicateSources = map[int64][]string{}
	rows, err = v.tx.QueryContext(ctx, `SELECT duplicate_of,source FROM postings WHERE duplicate_of IS NOT NULL ORDER BY duplicate_of,id`)
	if err != nil {
		return fail(err)
	}
	for rows.Next() {
		var id int64
		var source string
		if err := rows.Scan(&id, &source); err != nil {
			rows.Close()
			return fail(err)
		}
		list := d.DuplicateSources[id]
		found := false
		for _, old := range list {
			if old == source {
				found = true
				break
			}
		}
		if !found {
			d.DuplicateSources[id] = append(list, source)
		}
	}
	err = rows.Err()
	closeErr = rows.Close()
	if err != nil {
		return fail(err)
	}
	if closeErr != nil {
		return fail(closeErr)
	}
	d.ContentHashes = make(map[int64]string, len(d.Postings))
	for _, p := range d.Postings {
		_, d.ContentHashes[p.ID], _ = ai.ModelInput(p)
	}
	d.Extractions, err = s.aiExtractionsByPostingID(ctx, v.tx, d.ContentHashes, ai.ExtractionContractVersion())
	if err != nil {
		return fail(err)
	}
	uid := v.userID
	if s.dialect == DialectSQLite {
		uid = 1
	}
	if d.HasProfile && s.dialect == DialectPostgres {
		prof, err := profile.Unmarshal(d.ProfileJSON)
		if err != nil {
			return fail(err)
		}
		if prof.AIProvider != "" {
			err = v.tx.QueryRowContext(ctx, s.query(`SELECT EXISTS(SELECT 1 FROM user_ai_credentials WHERE user_id=? AND provider=?)`), uid, prof.AIProvider).Scan(&d.CredentialPresent)
			if err != nil {
				return fail(err)
			}
		}
		if enabled {
			model := prof.AIModel
			if model == "" {
				model = ai.DefaultModel(prof.AIProvider)
			}
			d.Validations, err = s.aiDealbreakerValidationsByPostingID(ctx, v.tx, uid, ai.DealbreakerVersion(prof.AIProvider, model))
			if err != nil {
				return fail(err)
			}
		}
	}
	if enabled {
		d.Fresh, err = s.aiScoresByPostingID(ctx, v.tx, uid, goalHash, scoreVersion)
		if err != nil {
			return fail(err)
		}
		d.Latest, err = s.latestAIScoresByPostingID(ctx, v.tx, uid, scoreVersion)
		if err != nil {
			return fail(err)
		}
		d.Outcomes, err = s.aiScoreOutcomesByPostingID(ctx, v.tx, uid, goalHash, scoreVersion)
		if err != nil {
			return fail(err)
		}
	}
	if enabled || demo {
		d.AnyVersion, err = s.latestAIScoresAnyVersionByPostingID(ctx, v.tx, uid)
		if err != nil {
			return fail(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return d, nil
}

func (v *PostingViewRead) Profile() (string, string, bool) {
	return v.data.ProfileJSON, v.data.ProfileHash, v.data.HasProfile
}

// CredentialPresence checks only configuration in the established read view.
func (v *PostingViewRead) CredentialPresence(ctx context.Context, provider string) (bool, error) {
	if provider == "" || v.store.dialect == DialectSQLite {
		return false, nil
	}
	normalized, err := validateCredentialKey(v.userID, provider)
	if err != nil {
		return false, err
	}
	var present bool
	err = v.tx.QueryRowContext(ctx, v.store.query(`SELECT EXISTS(SELECT 1 FROM user_ai_credentials WHERE user_id=? AND provider=?)`), v.userID, normalized).Scan(&present)
	return present, err
}
