# Human deploy guide for jobcron.app production

This guide deploys the production jobcron app to AWS using an image built on
your Mac, an AWS EC2 host, AWS RDS PostgreSQL 18, and Caddy-managed HTTPS.

The production deploy files are configured for:

- Public URLs: `https://jobcron.app` and `https://www.jobcron.app`
- App compose directory on EC2: `/srv/jobcron/deploy/production`
- Database: AWS RDS PostgreSQL 18 through `DATABASE_URL`
- App port inside Docker: `7777`

Do not put real secrets in Git. The real `DATABASE_URL` password,
`SESSION_SECRET`, and owner password are entered by a human on EC2.

## 1. Build and push the app image from your Mac

Do not build the app image on the EC2 instance. Build an arm64 Linux image on
your Mac and push it to your registry:

```sh
cd /path/to/jobcron
IMAGE=ohchanwu/jobcron:0.2-linuxarm64
docker buildx build --platform linux/arm64 -f deploy/production/Dockerfile -t "$IMAGE" --push .
```

Use the same image name later as `JOBCRON_IMAGE`.

## 2. Confirm AWS RDS PostgreSQL 18

The production RDS database must exist before the app starts.

Expected first-pass settings:

- Database name: `jobcron`
- App database user: `jobcron_admin`
- Public access: off unless there is a specific temporary maintenance need
- Security group: allow PostgreSQL `5432` only from the EC2 instance security
  group
- TLS: require TLS in the connection string with `sslmode=require`

The EC2 `.env` file uses this shape:

```sh
DATABASE_URL=postgres://jobcron_admin:<database-password>@<rds-endpoint>:5432/jobcron?sslmode=require
```

## 3. Confirm DNS points at the EC2 host

Cloudflare proxy is off for this first pass, so Caddy talks directly to Let's
Encrypt.

At your DNS provider for `jobcron.app`, these records should exist:

```text
Type: A      Name: @    Value: <EC2 public IPv4 address>    Proxy: off
Type: CNAME  Name: www  Value: jobcron.app                  Proxy: off
```

Verify from your Mac:

```sh
dig jobcron.app
dig www.jobcron.app
```

The answers should route to the EC2 host before Caddy starts.

## 4. Install Docker on EC2

SSH into the instance, then install and start Docker:

```sh
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker ec2-user
exit
```

SSH back in so the Docker group change applies, then install Docker Compose:

```sh
sudo mkdir -p /usr/local/lib/docker/cli-plugins
sudo curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-aarch64 -o /usr/local/lib/docker/cli-plugins/docker-compose
sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
docker compose version
```

If your registry requires authentication:

```sh
docker login <registry>
```

## 5. Place the app files on EC2

Create the app directory and clone or update the repo:

```sh
sudo mkdir -p /srv/jobcron
sudo chown -R ec2-user:ec2-user /srv/jobcron
git clone <repo-url> /srv/jobcron
```

For later deploys:

```sh
cd /srv/jobcron
git pull --ff-only
```

## 6. Create the production environment file

On EC2:

```sh
cd /srv/jobcron/deploy/production
cp .env.example .env
openssl rand -base64 48
nano .env
```

Fill in:

```sh
JOBCRON_IMAGE=ohchanwu/jobcron:0.2-linuxarm64
DATABASE_URL=postgres://jobcron_admin:<database-password>@<rds-endpoint>:5432/jobcron?sslmode=require
SESSION_SECRET=<paste-random-session-secret-here>
```

Do not add these variables for this first production pass:

```text
JOBCRON_DEMO
JOBCRON_ADMIN_TOKEN
JOBCRON_PROXY_SECRET
JOBCRON_WORKNET_KEY
```

The compose file already sets:

```text
JOBCRON_ENV=production
JOBCRON_HOST=0.0.0.0
JOBCRON_PORT=7777
JOBCRON_NO_OPEN=1
JOBCRON_SCHEDULER_ENABLED=1
JOBCRON_DAILY_SCRAPE_TIME=05:00
```

## 7. Pull the image and start production

On EC2:

```sh
cd /srv/jobcron/deploy/production
docker compose --env-file .env pull
docker compose --env-file .env up -d
docker compose logs -f
```

Do not run `docker compose build` or `docker compose up --build` on EC2. The
compose file uses the image named by `JOBCRON_IMAGE`.

Expected behavior:

- Caddy listens on public ports `80` and `443`.
- The app is reachable only inside the Docker network as `app:7777`.
- Caddy requests and stores HTTPS certificates automatically.
- Caddy owns forwarded headers. There is no shared proxy secret in this pass.

## 8. Create the owner account

From a source checkout with Go installed and network access to RDS:

```sh
export DATABASE_URL='postgres://jobcron_admin:<database-password>@<rds-endpoint>:5432/jobcron?sslmode=require'
export JOBCRON_OWNER_PASSWORD='<temporary-owner-password>'
go run ./cmd/jobcron-user create-owner \
  --database-url "$DATABASE_URL" \
  --email 'owner@example.com'
unset JOBCRON_OWNER_PASSWORD
```

## 9. Final checks

Open these URLs in a browser:

```text
https://jobcron.app/
https://www.jobcron.app/
```

Check:

- HTTPS works with a valid certificate.
- `http://jobcron.app/` redirects to HTTPS.
- `https://www.jobcron.app/` redirects to `https://jobcron.app/`.
- The owner can log in.
- The app can load its production PostgreSQL-backed data.
- The daily scrape is scheduled for `05:00` KST, or a first manual scrape is run
  after deploy to populate the briefing.
- Worknet is absent unless a human later adds `JOBCRON_WORKNET_KEY`.

If the app cannot connect to RDS, check the RDS security group, the database
endpoint, the username and password in `DATABASE_URL`, and `sslmode=require`.
