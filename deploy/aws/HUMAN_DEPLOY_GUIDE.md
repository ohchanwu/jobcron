# Human deploy guide for demo.jobcron.app

This guide deploys the read-only job-scraper demo to AWS.

The deploy files in this directory are configured for:

- Public URL: `https://demo.jobcron.app`
- App data file on the server: `/srv/job-scraper/data/jobs.db`
- App config directory on the server: `/srv/job-scraper/app`
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
6. Allocate an Elastic IP.
7. Associate the Elastic IP with the instance.

## 2. Point DNS at the server

At your DNS provider for `jobcron.app`, create this record:

```text
Type: A
Name: demo
Value: <Elastic IP>
```

Verify DNS from your local machine:

```sh
dig demo.jobcron.app
```

The answer should include your Elastic IP before you start Caddy.

## 3. Copy the deploy files and database

On your Mac, set these variables:

```sh
EC2_HOST=<elastic-ip-or-ec2-host>
KEY=~/path/to/your-key.pem
```

Create the server directories:

```sh
ssh -i "$KEY" ec2-user@$EC2_HOST 'sudo mkdir -p /srv/job-scraper/app /srv/job-scraper/data && sudo chown -R ec2-user:ec2-user /srv/job-scraper'
```

Copy the app deploy files:

```sh
scp -i "$KEY" -r /Users/chanbla11mit/gt/jobscraper/polecats/chrome/jobscraper/deploy/aws/* ec2-user@$EC2_HOST:/srv/job-scraper/app/
```

Copy only the prepared SQLite database:

```sh
scp -i "$KEY" /tmp/jobs.db ec2-user@$EC2_HOST:/srv/job-scraper/data/jobs.db
```

Do not copy:

```text
~/Library/Application Support/job-scraper/ai_keys.json
```

## 4. Install Docker on the server

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

## 5. Create the environment file

On the server:

```sh
cd /srv/job-scraper/app
openssl rand -hex 32
```

Create `.env`:

```sh
nano .env
```

Add this line, using the random token from `openssl`:

```sh
JOBSCRAPER_ADMIN_TOKEN=<paste-random-token-here>
```

The token is a safety hatch for operator-triggered `/api/scrape` in demo mode. Visitors still cannot write profile, bookmark, hide, or AI re-rate data.

## 6. Start the app

On the server:

```sh
cd /srv/job-scraper/app
docker compose --env-file .env up -d --build
docker compose logs -f
```

Expected behavior:

- The `app` container starts and listens on port `7777` inside Docker.
- Port `7777` is bound only to server loopback: `127.0.0.1:7777`.
- The `caddy` container listens on public ports `80` and `443`.
- Caddy requests and stores the HTTPS certificate automatically.

## 7. Final checks

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
/tmp/job-scraper-linux-arm64
```

The deployment uses `/tmp/jobs.db`. The Dockerfile builds its own arm64 binary, so `/tmp/job-scraper-linux-arm64` is only a verification artifact.

