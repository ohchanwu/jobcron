package storage

import (
	"context"
	"testing"
)

func TestPostgresMigrationsCreateCoreTables(t *testing.T) {
	st, schema := newPostgresTestStoreWithSchema(t)

	for _, table := range []string{
		"users",
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
