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

Import from an old SQLite `jobs.db` after starting PostgreSQL with the planned import command:

```sh
job-scraper-import \
  --sqlite "$HOME/Library/Application Support/job-scraper/jobs.db" \
  --postgres 'postgres://postgres@localhost:55432/jobscraper_dev?sslmode=disable'
```

The importer is planned for the PostgreSQL migration line. Avoid treating a raw `sqlite3 .dump | psql` transfer as reliable because SQLite-only details such as FTS5 virtual tables and type differences need project-aware conversion.
