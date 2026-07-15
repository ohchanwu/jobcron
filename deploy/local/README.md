# Local PostgreSQL

The managed local lifecycle uses Docker Compose project `jobcron-local`,
PostgreSQL 18 on `127.0.0.1:55432`, and the named volume
`jobcron-postgres18-cluster`. That volume is separate from the older local
PostgreSQL volume. PostgreSQL 18 mounts it at `/var/lib/postgresql`, allowing the
image to keep major-version-specific data directories internally.

This lifecycle is implemented in Slice 3 and starts automatically for the
interactive preview. Ordinary no-URL `jobcron` startup still uses SQLite until
Slice 4 completes and verifies the import before activating PostgreSQL as the
only writable runtime. Until then, `--db` and the SQLite import warning remain.

## Prerequisites and startup

Install Docker Engine with the `docker compose` plugin and start the Docker
daemon. These diagnostics must all succeed:

```sh
docker --version
docker compose version
docker info
```

Start the local PostgreSQL service:

```sh
docker compose -p jobcron-local -f deploy/local/compose.yaml \
  up -d --wait --wait-timeout 60
```

The fixed local URL is:

```sh
postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable
```

The managed path runs the same fixed Compose project automatically and creates
the synthetic no-login owner only when the `users` table is empty. It reuses one
existing positive owner, including a verified imported owner, and refuses a
database containing multiple users.

Setting `DATABASE_URL` explicitly bypasses managed Docker startup. The target
must already contain exactly one user; the explicit path never creates one.

Run PostgreSQL integration tests:

```sh
JOBCRON_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable' go test ./internal/storage -run Postgres -count=1
```

Run the app against local PostgreSQL:

```sh
DATABASE_URL='postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable' go run ./cmd/jobcron --no-open
```

## Diagnostics and recovery

Compose health and Docker `HostConfig` or label inspection are insufficient by
themselves. The service is usable only when the host can open a real TCP
connection to `127.0.0.1:55432`. On failure, run the exact `ps` and
`logs postgres` commands printed by the launcher. It preserves the container,
volume, and private Compose file needed by those commands; it never runs `down`,
removes a container, or removes a volume automatically.

If the first container creation happened while port `55432` was occupied,
Docker can leave `jobcron-local-postgres-1` without a working published port.
After freeing `55432`, recreate only that new container:

```sh
docker rm -f jobcron-local-postgres-1
docker compose -p jobcron-local -f deploy/local/compose.yaml \
  up -d --wait --wait-timeout 60 postgres
nc -z 127.0.0.1 55432
```

Do not remove the older `local-postgres-1` container. Preserve both
`local_jobcron-postgres18-cluster` and `jobcron-postgres18-cluster`; recreating the
new container does not require deleting either volume.

## Stop versus reset

Stopping `jobcron` does not stop PostgreSQL. To stop only the managed service
without deleting its data:

```sh
docker compose -p jobcron-local -f deploy/local/compose.yaml stop postgres
```

An explicit reset is destructive. It removes the managed container and
`jobcron-postgres18-cluster`; back up anything needed first:

```sh
docker compose -p jobcron-local -f deploy/local/compose.yaml down -v
```

The older local PostgreSQL container and volume are not part of this Compose
project and must remain available for rollback.

## Isolated interactive preview

`scripts/preview-interactive.sh [port]` (default `17778`) acquires an atomic
per-user/per-port lock and refuses any unrelated listener on that HTTP port. It
also refuses an inherited `DATABASE_URL`. After verifying the exact
`jobcron-local` container and real host TCP reachability, it creates a unique
`jobcron_preview_*` database, private state directory, and disposable master
key. The app runs in non-production mode with the scheduler disabled.

Normal cleanup drops only that unique database and removes only its private
state. The shared Compose service and named volume stay running. With
`JOBCRON_PREVIEW_KEEP=1`, the launcher retains the database and state and prints
exact manual `dropdb` and `rm -rf` commands. Run those commands after inspecting
the printed names; never use Compose `down` to clean up a preview.

## SQLite migration boundary

The future `jobcron-import` command lands in Slice 4 and is not available yet.
After it ships, its planned syntax for importing an old SQLite `jobs.db` will be:

```text
jobcron-import \
  --sqlite "$HOME/Library/Application Support/jobcron/jobs.db" \
  --postgres 'postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable'
```

Do not run that future example or treat the writable cutover as complete yet.
Avoid a raw `sqlite3 .dump | psql` transfer: SQLite-only details such as FTS5
virtual tables and type differences need project-aware conversion.
