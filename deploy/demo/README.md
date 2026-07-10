# AWS demo deploy: demo.jobcron.app

This stack runs the read-only job-scraper demo on one AWS EC2 t4g.micro arm64
instance. AI is off on the server. Upload only `jobs.db`; never upload
`ai_keys.json`.

This directory is **demo-only**. It is not the production deployment for
`jobcron.app`. The production app should use a separate production deploy
configuration with PostgreSQL/RDS, login sessions, and no `--demo` flag.

## DNS and instance assumptions

- Hostname: `demo.jobcron.app`
- DNS: create an `A` record for `demo.jobcron.app` pointing to the instance's
  current public IPv4 address. This guide does not use an Elastic IP. If the
  instance is stopped and AWS assigns a new public IPv4 address, update DNS.
- Instance: Amazon Linux 2023 kernel-6.18 arm64 AMI, t4g.micro.
- Security group: allow 80 and 443 from the internet; allow 22 only from your IP.

## Files on the instance

Clone or pull this repository to `/srv/job-scraper/app`, then run Compose from
`/srv/job-scraper/app/deploy/demo`.

Put the prepared database at:

```sh
/srv/job-scraper/data/jobs.db
```

Create `/srv/job-scraper/app/deploy/demo/.env`:

```sh
JOBSCRAPER_IMAGE=<registry>/<namespace>/job-scraper-demo:<tag>
JOBSCRAPER_ADMIN_TOKEN=<long random string>
JOBSCRAPER_PROXY_SECRET=<another long random string>
```

The token is only a safety hatch for operator-triggered `/api/scrape` in demo
mode. Visitors cannot write profile, bookmark, hide, or AI re-rate data.
The proxy secret lets the app trust Caddy's forwarded client-IP header for
login rate limiting; do not reuse the admin token.

## Build and push the app image from your Mac

Do not build the app image on the EC2 instance. A t4g.micro does not have enough
memory for the Docker build.

Choose a registry image name first. For GitHub Container Registry, it will look
like `ghcr.io/<github-user>/job-scraper-demo:latest`. For Docker Hub, it will
look like `<dockerhub-user>/job-scraper-demo:latest`.

From the project root on your Mac, build the arm64 image and push it:

```sh
cd /Users/chanbla11mit/gt/jobscraper/polecats/chrome/jobscraper
IMAGE=<registry>/<namespace>/job-scraper-demo:latest
docker buildx build --platform linux/arm64 -f deploy/demo/Dockerfile -t "$IMAGE" --push .
```

On the EC2 instance, pull the image before starting Compose:

```sh
cd /srv/job-scraper/app/deploy/demo
set -a
. ./.env
set +a
docker pull "$JOBSCRAPER_IMAGE"
```

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
cd /srv/job-scraper/app/deploy/demo
set -a
. ./.env
set +a
docker pull "$JOBSCRAPER_IMAGE"
docker compose --env-file .env up -d
docker compose logs -f
```

Do not run `docker compose build` or `docker compose up --build` on the EC2
instance. The compose file expects the image named by `JOBSCRAPER_IMAGE` to
already exist locally after `docker pull`, and it will not build a replacement
image.

Caddy handles HTTPS certificates automatically. The Caddyfile intentionally
does not enable gzip, because compression can buffer Server-Sent Events.

## Local database preparation

After the final local scrape and AI re-rate, stop the local app and create a
clean SQLite backup:

```sh
sqlite3 "$HOME/Library/Application Support/job-scraper/jobs.db" ".backup '/tmp/jobs.db'"
```

Upload only `/tmp/jobs.db` to `/srv/job-scraper/data/jobs.db`.
