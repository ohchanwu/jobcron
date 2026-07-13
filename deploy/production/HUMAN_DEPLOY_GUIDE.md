# Human deploy guide for jobcron.app production

This guide deploys the production jobcron app to AWS using an image built on
your Mac, an AWS EC2 host, AWS RDS PostgreSQL 18, and Caddy-managed HTTPS.

The production deploy files are configured for:

- Public URLs: `https://jobcron.app` and `https://www.jobcron.app`
- App compose directory on EC2: `/srv/jobcron/deploy/production`
- Database: AWS RDS PostgreSQL 18 through `DATABASE_URL`
- App port inside Docker: `7777`

Do not put real secrets in Git. The real `DATABASE_URL` password and
`SESSION_SECRET` are entered by a human on EC2.

## 1. Build and publish the approved immutable image

From the approved release checkout on the operator's Mac:

```sh
git status --short
git rev-parse HEAD

RELEASE_TAG="sha-$(git rev-parse --short=12 HEAD)"
IMAGE="<dockerhub-user>/jobcron:$RELEASE_TAG"

docker login
docker buildx build \
  --platform linux/arm64 \
  -f deploy/production/Dockerfile \
  -t "$IMAGE" \
  --push .
docker buildx imagetools inspect "$IMAGE"
```

Stop if `git status` is not clean or the inspected image does not match the
approved release architecture and tag. Set `JOBCRON_IMAGE` in the EC2 `.env`
file to this exact immutable image.

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
JOBCRON_IMAGE=<dockerhub-user>/jobcron:sha-<12-character-commit>
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

The app must start successfully once before owner creation or import because app
startup applies PostgreSQL migrations.

After startup, verify `docker volume inspect jobcron_config` succeeds. Routine
deploys may use `docker compose down`, but must not use `docker compose down -v`.

## 8. Open a localhost-only tunnel to private RDS

Keep RDS public access disabled. In a dedicated terminal on the operator's Mac:

```sh
ssh -o ExitOnForwardFailure=yes -N \
  -L 127.0.0.1:15432:<rds-endpoint>:5432 \
  ec2-user@<ec2-public-host>
```

Leave that terminal running only for owner creation and optional import.

In a trusted local source checkout, export private values in the current shell:

```sh
export TUNNELED_DATABASE_URL='<postgresql-url-using-127.0.0.1:15432-and-sslmode-require>'
export OWNER_EMAIL='<owner-email>'
```

Do not paste the real URL or email into Git, issues, chat, or shared logs.

## 9. Create the owner before any import

Let `jobcron-user` prompt for the password so it does not enter shell history:

```sh
go run ./cmd/jobcron-user create-owner \
  --database-url "$TUNNELED_DATABASE_URL" \
  --email "$OWNER_EMAIL"
```

## 10. Optionally import the recovered SQLite state

Skip this section unless the human approved import and a current RDS snapshot
exists. Use the exact same `OWNER_EMAIL` as owner creation.

Run the source-count dry run first:

```sh
go run ./cmd/jobcron-import \
  --sqlite '<recovered-sqlite-path>' \
  --postgres "$TUNNELED_DATABASE_URL" \
  --owner-email "$OWNER_EMAIL" \
  --dry-run
```

Review profile, postings, scores, bookmarks, not-interested state,
AI extractions, AI scores, and AI usage counts. If approved, run:

```sh
go run ./cmd/jobcron-import \
  --sqlite '<recovered-sqlite-path>' \
  --postgres "$TUNNELED_DATABASE_URL" \
  --owner-email "$OWNER_EMAIL"
```

The importer preserves the existing owner's password hash when the email
matches. It does not import `ai_keys.json`, sessions, owner passwords, or
production secrets.

Close the SSH tunnel and clear the private shell variables:

```sh
unset TUNNELED_DATABASE_URL OWNER_EMAIL
```

## 11. Enter the Anthropic key after volume durability exists

Sign in to `jobcron.app` and save the key through the application UI. The key
stays in `/root/.config/jobcron/ai_keys.json` inside `jobcron_config`. Re-enter
it after host loss unless a separate secure backup exists.

## 12. Final checks

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
