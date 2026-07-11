# Local PostgreSQL

The local development database runs PostgreSQL 18 on port `55432`. Its Docker
volume is separate from the older PostgreSQL 16 local volume, so switching to 18
does not delete the old local dev data. PostgreSQL 18 Docker images use a
cluster-style mount at `/var/lib/postgresql`, which lets the image keep
major-version-specific data directories internally.

Start the local PostgreSQL service:

```sh
docker compose -f deploy/local/compose.yaml up -d
```

Use this local `DATABASE_URL`:

```sh
postgres://postgres@localhost:55432/jobcron_dev?sslmode=disable
```

Run PostgreSQL integration tests:

```sh
JOBCRON_TEST_POSTGRES_URL='postgres://postgres@localhost:55432/jobcron_dev?sslmode=disable' go test ./internal/storage -run Postgres -count=1
```

Run the app against local PostgreSQL:

```sh
DATABASE_URL='postgres://postgres@localhost:55432/jobcron_dev?sslmode=disable' go run ./cmd/jobcron --no-open
```

Reset the local database:

```sh
docker compose -f deploy/local/compose.yaml down -v
```

That removes only the fresh `jobcron-postgres18-cluster` volume declared in
`deploy/local/compose.yaml`. The legacy `jobscraper-postgres18-cluster` volume
is not part of this stack and remains available for rollback.

Import from an old SQLite `jobs.db` after starting PostgreSQL with the planned import command:

```sh
jobcron-import \
  --sqlite "$HOME/Library/Application Support/jobcron/jobs.db" \
  --postgres 'postgres://postgres@localhost:55432/jobcron_dev?sslmode=disable'
```

The importer is planned for the PostgreSQL migration line. Avoid treating a raw `sqlite3 .dump | psql` transfer as reliable because SQLite-only details such as FTS5 virtual tables and type differences need project-aware conversion.
