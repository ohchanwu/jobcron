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

## Build the app image on your Mac

Do not build the app image on the EC2 instance. A t4g.micro does not have enough
memory for the Docker build.

From the project root on your Mac:

```sh
cd /Users/chanbla11mit/gt/jobscraper/polecats/chrome/jobscraper
docker buildx build --platform linux/arm64 -f deploy/aws/Dockerfile -t job-scraper-demo:latest --load .
docker save job-scraper-demo:latest | gzip > /tmp/job-scraper-demo-linux-arm64.tar.gz
```

Copy the archive to the instance:

```sh
scp -i "$KEY" /tmp/job-scraper-demo-linux-arm64.tar.gz ec2-user@$EC2_HOST:/srv/job-scraper/app/
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
cd /srv/job-scraper/app
gunzip -c job-scraper-demo-linux-arm64.tar.gz | docker load
docker compose --env-file .env up -d
docker compose logs -f
```

Do not run `docker compose build` or `docker compose up --build` on the EC2
instance. The compose file expects the prebuilt `job-scraper-demo:latest` image
to already exist locally after `docker load`, and it will not pull or build a
replacement image.

Caddy handles HTTPS certificates automatically. The Caddyfile intentionally
does not enable gzip, because compression can buffer Server-Sent Events.

## Local database preparation

After the final local scrape and AI re-rate, stop the local app and create a
clean SQLite backup:

```sh
sqlite3 "$HOME/Library/Application Support/job-scraper/jobs.db" ".backup '/tmp/jobs.db'"
```

Upload only `/tmp/jobs.db` to `/srv/job-scraper/data/jobs.db`.
