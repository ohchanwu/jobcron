# Human deploy guide for demo.jobcron.app

This guide deploys the read-only job-scraper demo to AWS.

The deploy files in this directory are configured for:

- Public URL: `https://demo.jobcron.app`
- App data file on the server: `/srv/job-scraper/data/jobs.db`
- App repository directory on the server: `/srv/job-scraper/app`
- App compose directory on the server: `/srv/job-scraper/app/deploy/aws`
- Local prepared database backup: `/tmp/jobs.db`

Do not upload `ai_keys.json`. AI runs locally before deployment. The server reads cached AI results from `jobs.db`.

## 1. Create the AWS server

In AWS EC2:

1. Launch a new instance.
2. Choose an Amazon Linux 2023 arm64 AMI.
3. Choose instance type `t4g.micro`.
4. Create or select an SSH key pair.
5. Configure the security group:
   - Allow TCP `80` from `0.0.0.0/0`
   - Allow TCP `443` from `0.0.0.0/0`
   - Allow TCP `22` only from your current IP address
6. Do not allocate an Elastic IP for this demo. Use the instance's current public IPv4 address or public DNS name.

## 2. Point DNS at the server

In the EC2 console, copy the instance's current public IPv4 address. At your DNS provider for `jobcron.app`, create this record:

```text
Type: A
Name: demo
Value: <current EC2 public IPv4 address>
```

Verify DNS from your local machine:

```sh
dig demo.jobcron.app
```

The answer should include the instance's current public IPv4 address before you start Caddy. If the instance is stopped and AWS assigns a new public IPv4 address later, update this DNS record again.

## 3. Build and push the Docker image on your Mac

Do not build the app image on the EC2 instance. A `t4g.micro` does not have enough memory for the Docker build.

Build the arm64 Linux image and push it from your Mac:

```sh
cd /Users/chanbla11mit/gt/jobscraper/polecats/chrome/jobscraper
docker buildx build --platform linux/arm64 -f deploy/aws/Dockerfile -t ohchanwu/jobcron:0.1-linuxarm64 --push .
```

This creates an arm64 Linux image that can run on the AWS instance, then stores it in your registry.

## 4. Clone or update the repo on the server

On your Mac, set these variables:

```sh
EC2_HOST=<current-ec2-public-ip-or-public-dns-name>
KEY=~/path/to/your-key.pem
```

Create the server directories, then clone the repository if this is the first deploy:

```sh
ssh -i "$KEY" ec2-user@$EC2_HOST 'sudo mkdir -p /srv/job-scraper/app /srv/job-scraper/data && sudo chown -R ec2-user:ec2-user /srv/job-scraper'
ssh -i "$KEY" ec2-user@$EC2_HOST 'git clone <repo-url> /srv/job-scraper/app'
```

For later deploys, pull the latest code instead:

```sh
ssh -i "$KEY" ec2-user@$EC2_HOST 'cd /srv/job-scraper/app && git pull --ff-only'
```

Using Git for the app files is safe for this repo because the tracked files do not include the runtime database, `.env`, or `ai_keys.json`. Those stay local to your Mac or the server.

## 5. Copy only the database

The database is runtime data, not source code. Copy only the prepared SQLite backup:

```sh
scp -i "$KEY" /tmp/jobs.db ec2-user@$EC2_HOST:/srv/job-scraper/data/jobs.db
```

Do not copy:

```text
~/Library/Application Support/job-scraper/ai_keys.json
```

Do not commit or upload:

```text
.env
*.db
*.db-shm
*.db-wal
```

## 6. Install Docker on the server

SSH into the instance:

```sh
ssh -i "$KEY" ec2-user@$EC2_HOST
```

Install and start Docker:

```sh
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker ec2-user
exit
```

SSH back in so the Docker group membership applies:

```sh
ssh -i "$KEY" ec2-user@$EC2_HOST
```

Install the Docker Compose plugin:

```sh
sudo mkdir -p /usr/local/lib/docker/cli-plugins
sudo curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-aarch64 -o /usr/local/lib/docker/cli-plugins/docker-compose
sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
docker compose version
```

If your registry requires login, log in on the EC2 instance:

```sh
docker login <registry>
```

## 7. Create the environment file

On the server:

```sh
cd /srv/job-scraper/app/deploy/aws
openssl rand -hex 32
```

Create `.env`:

```sh
nano .env
```

Add these lines. Use the same image name you pushed from your Mac, and use the random token from `openssl`:

```sh
JOBSCRAPER_IMAGE=ohchanwu/jobcron:0.1-linuxarm64
JOBSCRAPER_ADMIN_TOKEN=<paste-random-token-here>
```

The token is a safety hatch for operator-triggered `/api/scrape` in demo mode. Visitors still cannot write profile, bookmark, hide, or AI re-rate data.

## 8. Pull the image and start the app

On the server:

```sh
cd /srv/job-scraper/app/deploy/aws
set -a
. ./.env
set +a
docker pull "$JOBSCRAPER_IMAGE"
docker compose --env-file .env up -d
docker compose logs -f
```

Do not run `docker compose build` or `docker compose up --build` on the EC2 instance. The compose file uses the prebuilt image named by `JOBSCRAPER_IMAGE`, and it will not build a replacement image.

Expected behavior:

- The `app` container starts and listens on port `7777` inside Docker.
- Port `7777` is bound only to server loopback: `127.0.0.1:7777`.
- The `caddy` container listens on public ports `80` and `443`.
- Caddy requests and stores the HTTPS certificate automatically.

## 9. Final checks

From a phone on cellular, open:

```text
https://demo.jobcron.app/
```

Check:

- HTTPS works with a valid certificate.
- `http://demo.jobcron.app/` redirects to HTTPS.
- Archive has postings.
- `AI 분석` chips open their evidence popovers.
- Bookmark works and survives refresh.
- Hide works and survives refresh.
- `/bookmarks` and `/hidden` show browser-local state only.
- Settings page is visible but disabled.
- No scrape button appears in the UI.

From your laptop, also confirm the write guards:

```sh
curl -i https://demo.jobcron.app/api/scrape
curl -i -X POST https://demo.jobcron.app/profile
curl -i -X PUT https://demo.jobcron.app/api/bookmark/1
curl -i -X PUT https://demo.jobcron.app/api/not-interested/1
curl -i 'https://demo.jobcron.app/api/rerate?surface=archive'
```

Expected result: each request is refused with HTTP `403` unless `/api/scrape` uses the correct admin token.

## Local files prepared by the final check

These files were prepared locally:

```text
/tmp/jobs.db
```

The deployment uses `/tmp/jobs.db` for the read-only data. The app container image is pushed from your Mac and pulled by the EC2 instance.
