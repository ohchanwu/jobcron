# Production Deploy Prep Report

Date: 2026-07-10 KST

## Summary

Prepared a production deployment stack for `jobcron.app` that is separate from
the demo stack. The new production stack pulls a prebuilt registry image,
connects to AWS RDS PostgreSQL through `DATABASE_URL`, runs the private owner
app instead of demo mode, and uses Caddy as the only public entry point.

No production secrets were added to Git.

## Files Added

- `deploy/production/Dockerfile`
  - Builds the `job-scraper` web binary for Linux arm64.
  - Intended for MacBook build-and-push, not EC2 image builds.
    - **overseer feedback** actually, I want to standardize the name to "jobcron" now, to prevent future confusion.
- `deploy/production/compose.yaml`
  - Runs the app from `JOBSCRAPER_IMAGE`.
    - **overseer feedback** I want to standardize the name to "jobcron" now, to prevent future confusion.
  - Requires `DATABASE_URL` and `SESSION_SECRET`.
  - Sets `JOBSCRAPER_ENV=production`, `JOBSCRAPER_NO_OPEN=1`, and the daily
    scrape scheduler at `09:00`.
    - **OF**: Let's set it to `05:00` actually. We want the stuff to be scraped by the time the early birds wake up.
  - Does not set `JOBSCRAPER_DEMO`, `JOBSCRAPER_ADMIN_TOKEN`,
    `JOBSCRAPER_WORKNET_KEY`, or `JOBSCRAPER_PROXY_SECRET`.
- `deploy/production/Caddyfile`
  - Serves `jobcron.app`.
  - Redirects `www.jobcron.app` to `jobcron.app`.
  - Proxies to the private Docker service `app:7777`.
- `deploy/production/.env.example`
  - Documents required placeholders without real secrets.
- `deploy/production/README.md`
  - Explains the production stack and local validation command.
- `deploy/production/HUMAN_DEPLOY_GUIDE.md`
  - Step-by-step production deployment guide for Mac image build, registry push,
    EC2 pull, compose startup, owner creation, and final browser checks.

## Decisions Recorded

- Worknet stays disabled for the first production pass.
- `JOBSCRAPER_PROXY_SECRET` stays unset for the first production pass.
- Caddy is the only public entry point and owns forwarded headers.
- Production uses PostgreSQL/RDS, not SQLite.
- The EC2 instance pulls a prebuilt image; it should not build the image.

## Verification Run

From `/Users/chanbla11mit/gt/jobscraper/mayor/rig`:

```sh
JOBSCRAPER_IMAGE=example/job-scraper:prod \
DATABASE_URL='postgres://jobcron_admin:dummy@example-rds.ap-northeast-2.rds.amazonaws.com:5432/jobcron?sslmode=require' \
SESSION_SECRET=dummy-session-secret \
docker compose -f deploy/production/compose.yaml config
```

Result: passed. The rendered config included production env values and did not
include demo/admin-token/Worknet/proxy-secret variables.

```sh
docker buildx build --platform linux/arm64 \
  -f deploy/production/Dockerfile \
  -t jobcron-prod-local:verify \
  --load .
```

Result: passed. Docker built and loaded `jobcron-prod-local:verify`.

```sh
docker run --rm \
  -v "$PWD/deploy/production/Caddyfile:/etc/caddy/Caddyfile:ro" \
  caddy:2.8-alpine \
  caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
```

Result: passed. Caddy reported `Valid configuration`.

```sh
go test ./...
```

Result: passed.

Runtime probe:

```sh
docker run --rm jobcron-prod-local:verify --help
```

Result: the container invoked the app binary and exited with `flag: help
requested`. That is acceptable for this probe; it proves the built image can
start the binary, not that production can connect to RDS.

## Human-Blocked Next Steps

- Enter the real `DATABASE_URL` password and `SESSION_SECRET` only on the
  production EC2 instance.
  - **overseer feedback** I already did this.
- Choose the real registry image name/tag for `JOBSCRAPER_IMAGE`.
  - **OF** could we make the env variable name JOBCRON_IMAGE instead? For standardization.
  - And let's make it `ohchanwu/jobcron:0.2-linuxarm64`, unless you have objections
- Build and push the production image from the MacBook using the chosen image
  name.
- Pull and start the image on EC2 with `docker compose --env-file .env up -d`.
- Run the owner account command for `ohchanwu@gmail.com` with the real owner
  password.
- Open `https://jobcron.app/` and `https://www.jobcron.app/` in a browser after
  Caddy starts and certificates are issued.
- Run or wait for the first production scrape so the briefing page has data.
- After initial production data exists, run the first backup and restore
  verification from `/Users/chanbla11mit/mystuff/projects/job-scraper/backups`.

## Notes

The first attempted polecat, `jobscraper/radrat`, created a useful draft but
stalled before verification and left recovery-needed state. I recovered the
usable pieces into the mayor clone, corrected placeholders and proxy decisions,
and completed verification locally.
