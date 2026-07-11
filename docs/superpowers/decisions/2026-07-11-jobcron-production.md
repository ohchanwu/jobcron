# Jobcron Production And Rename Decisions

- The active product, Go module, commands, deployment identity, browser storage,
  and environment-variable prefix are `jobcron` / `JOBCRON_*`.
- The Gas Town rig remains named `jobscraper`; changing that recovery-sensitive
  identity is a separate task.
- On macOS, first normal startup atomically renames
  `~/Library/Application Support/job-scraper` to `jobcron`. Startup refuses a
  collision when both directories exist and does not modify either directory.
- `jobcron --version` does not inspect or migrate application data.
- Production uses PostgreSQL 18 through `DATABASE_URL`. Caddy is the only public
  entry point.
- The MacBook builds and pushes the Linux arm64 image; EC2 verifies and pulls the
  reviewed image. EC2 does not build it.
- Worknet remains disabled for the first production pass. Do not set
  `JOBCRON_PROXY_SECRET` for production.
- Secrets stay outside Git. The production Compose example intentionally leaves
  `SESSION_SECRET` empty so configuration fails until a real secret is supplied.
- Autonomous agents may commit locally but must not push.

