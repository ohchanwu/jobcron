package storage

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestScanPostingsReturnsScanError(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	if _, _, err := st.UpsertPosting(ctx, samplePosting()); err != nil {
		t.Fatalf("UpsertPosting valid row: %v", err)
	}
	bad := samplePosting()
	bad.SourcePostingID += "-bad"
	badID, _, err := st.UpsertPosting(ctx, bad)
	if err != nil {
		t.Fatalf("UpsertPosting bad row: %v", err)
	}

	badColumns := strings.Replace(
		postingColumns,
		"education, education_name",
		"CASE WHEN id = ? THEN 'bad' ELSE education END, education_name",
		1,
	)
	rows, err := st.db.QueryContext(ctx,
		`SELECT id, `+badColumns+`, duplicate_of FROM postings ORDER BY id`, badID)
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	defer rows.Close()

	got, err := scanPostings(rows)
	if got != nil {
		t.Fatalf("scanPostings result = %#v, want nil on scan error", got)
	}
	if err == nil || !strings.Contains(err.Error(), "storage: scan posting:") {
		t.Fatalf("scanPostings error = %v, want scan error", err)
	}
}

func TestScanPostingsReturnsRowsError(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	want := samplePosting()
	id, _, err := st.UpsertPosting(ctx, want)
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	want.ID = id

	rows, err := st.db.QueryContext(ctx,
		`WITH RECURSIVE sequence(n) AS (
VALUES (1)
UNION ALL
SELECT json_extract('not-json', '$') FROM sequence WHERE n = 1
)
SELECT p.id, `+postingColumns+`, p.duplicate_of
FROM sequence CROSS JOIN postings p`)
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	defer rows.Close()

	got, err := scanPostings(rows)
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("scanPostings result = %#v, want only %#v", got, want)
	}
	if err == nil || !strings.Contains(err.Error(), "malformed JSON") {
		t.Fatalf("scanPostings error = %v, want malformed JSON", err)
	}
}
