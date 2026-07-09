# Local PostgreSQL

Start the local PostgreSQL service:

```sh
docker compose -f deploy/local/compose.yaml up -d
```

Use this local `DATABASE_URL`:

```sh
postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable
```

Run PostgreSQL integration tests:

```sh
JOBSCRAPER_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable' go test ./internal/storage -run Postgres -count=1
```

Reset the local database:

```sh
docker compose -f deploy/local/compose.yaml down -v
```

Import from an old SQLite `jobs.db` after starting PostgreSQL:

```sh
sqlite3 "$HOME/Library/Application Support/job-scraper/jobs.db" .dump > /tmp/jobscraper-sqlite.sql
psql 'postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable' -f /tmp/jobscraper-sqlite.sql
```

The dump may need manual adjustment for SQLite-only features such as FTS5 virtual tables. The production PostgreSQL schema uses ordinary tables plus a PostgreSQL full-text index on posting title, company, and description.
