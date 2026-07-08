# AWS demo deploy: demo.jobcron.app

This stack runs the read-only job-scraper demo on one AWS EC2 t4g.micro arm64
instance. AI is off on the server. Upload only `jobs.db`; never upload
`ai_keys.json`.

## DNS and instance assumptions

- Hostname: `demo.jobcron.app`
- DNS: create an `A` record for `demo.jobcron.app` pointing to the instance
  Elastic IP.
- Instance: Amazon Linux 2023 kernel-6.18 arm64 AMI, t4g.micro.
- Security group: allow 80 and 443 from the internet; allow 22 only from your IP.

## Files on the instance

Copy this directory to `/srv/job-scraper/app` and put the prepared database at:

```sh
/srv/job-scraper/data/jobs.db
```

Create `/srv/job-scraper/app/.env`:

```sh
JOBSCRAPER_ADMIN_TOKEN=<long random string>
```

The token is only a safety hatch for operator-triggered `/api/scrape` in demo
mode. Visitors cannot write profile, bookmark, hide, or AI re-rate data.

## Docker setup on Amazon Linux 2023

```sh
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker ec2-user
```

Log out and back in so the group change applies. Amazon Linux 2023 does not
ship the Compose plugin; install the linux-aarch64 binary from Docker Compose
GitHub releases into:

```sh
/usr/local/lib/docker/cli-plugins/docker-compose
```

## Start

```sh
cd /srv/job-scraper/app
docker compose --env-file .env up -d --build
docker compose logs -f
```

Caddy handles HTTPS certificates automatically. The Caddyfile intentionally
does not enable gzip, because compression can buffer Server-Sent Events.

## Local database preparation

After the final local scrape and AI re-rate, stop the local app and create a
clean SQLite backup:

```sh
sqlite3 "$HOME/Library/Application Support/job-scraper/jobs.db" ".backup '/tmp/jobs.db'"
```

Upload only `/tmp/jobs.db` to `/srv/job-scraper/data/jobs.db`.
