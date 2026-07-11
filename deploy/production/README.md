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

## Files

- `compose.yaml` runs a prebuilt registry image and Caddy. It has no local
  build section and no local database service.
- `Caddyfile` redirects `www.jobcron.app` to `jobcron.app` and proxies the
  canonical host to the private app container.
- `.env.example` documents required human-entered values without real secrets.
- `Dockerfile` is for building the arm64 image on your Mac before pushing it to
  a registry.
- `HUMAN_DEPLOY_GUIDE.md` is the step-by-step production handoff.

## Validate the compose file locally

Use dummy values only:

```sh
cd deploy/production
JOBCRON_IMAGE=ohchanwu/jobcron:0.2-linuxarm64 \
DATABASE_URL='postgres://jobcron_admin:dummy@example-rds.ap-northeast-2.rds.amazonaws.com:5432/jobcron?sslmode=require' \
SESSION_SECRET=dummy-session-secret \
docker compose config
```

The rendered config should include `DATABASE_URL`, `SESSION_SECRET`,
`JOBCRON_ENV=production`, `JOBCRON_SCHEDULER_ENABLED=1`, and
`JOBCRON_DAILY_SCRAPE_TIME=05:00`. It should not include demo mode, an admin
token, a Worknet key, or a proxy secret.
