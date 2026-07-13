# AWS production deploy: jobcron.app

This stack is for the private production `jobcron.app` app on AWS. It is
separate from the read-only demo stack in `deploy/demo`.

Production assumptions:

- Public hostnames: `jobcron.app` and `www.jobcron.app`
- Caddy is the only public entry point and terminates HTTPS directly.
- Cloudflare proxy is off for this first pass.
- The app uses AWS RDS PostgreSQL 18 through `DATABASE_URL`.
- Worknet stays disabled until a human explicitly adds `JOBCRON_WORKNET_KEY`.
- `JOBCRON_PROXY_SECRET`, `JOBCRON_DEMO`, and `JOBCRON_ADMIN_TOKEN`
  are intentionally unset.
- The app stores BYOK credentials below `/root/.config/jobcron` in the explicitly
  named `jobcron_config` Docker volume.
- Normal container recreation and `docker compose down` preserve the volume on
  the same EC2 host. `docker compose down -v` deletes it and is not a routine
  deploy command.
- Host loss is outside the volume's durability boundary; recover through a
  secure backup or re-enter the API key.

## Files

- `compose.yaml` mounts `jobcron_config` into the app and retains the existing
  Caddy volumes. It has no local build section and no local database service.
- `Caddyfile` redirects `www.jobcron.app` to `jobcron.app` and proxies the
  canonical host to the private app container.
- `.env.example` documents required human-entered values without real secrets.
- `Dockerfile` is for building the arm64 image on your Mac before pushing it to
  a registry.
- `HUMAN_DEPLOY_GUIDE.md` keeps RDS private by using a localhost SSH tunnel for
  owner creation and optional import.

## Validate the compose file locally

Use dummy values only:

```sh
cd deploy/production
JOBCRON_IMAGE=example/jobcron:sha-test \
DATABASE_URL='postgres://jobcron_admin:<database-password>@db.example.invalid:5432/jobcron?sslmode=require' \
SESSION_SECRET=dummy-session-secret \
docker compose config
```

The rendered config must mount `jobcron_config` at `/root/.config/jobcron`,
retain Caddy's `caddy_data` and `caddy_config` volumes, and include
`DATABASE_URL`, `SESSION_SECRET`, `JOBCRON_ENV=production`,
`JOBCRON_SCHEDULER_ENABLED=1`, and `JOBCRON_DAILY_SCRAPE_TIME=05:00`. It must
not include demo mode, an admin token, a Worknet key, or a proxy secret.
