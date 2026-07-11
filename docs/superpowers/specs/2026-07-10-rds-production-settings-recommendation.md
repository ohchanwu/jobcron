# RDS Production Settings Recommendation

Date: 2026-07-10 KST

This is the current recommended Amazon RDS setup for the first production
`jobcron.app` deployment. It is intentionally small, private, and easy to
upgrade later.

## Recommended Choice

Create a normal Amazon RDS for PostgreSQL instance:

- Engine: PostgreSQL
- Version: PostgreSQL 18, latest minor available in the selected AWS region
- Deployment: Single-AZ DB instance
- Instance class: `db.t4g.micro`
- Storage: 20 GiB General Purpose SSD (`gp3` if available)
- Public access: No
- Access path: app EC2 instance only, through security groups

Why: this app currently serves one owner, runs one daily scrape, stores a small
job-posting dataset, and has low write volume. The main production value is
durable managed PostgreSQL, not high availability yet.

## Create Database Screen

- Creation method: Standard create
- Engine type: PostgreSQL
- Engine version: PostgreSQL 18, latest minor AWS offers
- Template: Free tier if available; otherwise Dev/Test
- Availability and durability: Single DB instance

Do not choose Multi-AZ for this first cut. Multi-AZ is useful when downtime is
expensive, but it adds meaningful monthly cost. For this project, a brief
database outage is acceptable while we are still validating the production app.

## Settings

- DB instance identifier: `jobcron-prod`
- Master username: `jobcron_admin`
- Credentials management: Self managed
- Initial database name: `jobcron`
- Port: `5432`

Use a generated password and store it in a password manager. Do not commit it to
the repo or paste it into long-lived docs.

Production `DATABASE_URL` shape:

```sh
postgres://jobcron_admin:<password>@<rds-endpoint>:5432/jobcron?sslmode=require
```

## Instance And Storage

- DB instance class: `db.t4g.micro`
- Storage type: General Purpose SSD, preferably `gp3`
- Allocated storage: 20 GiB
- Storage autoscaling: off for now

Education note: RDS storage can be increased later, but it is not normally
shrunk in place. Starting at 20 GiB keeps the blast radius and cost small while
we learn the app's real data growth.

## Connectivity

- VPC: same VPC as the EC2 app instance
- Public access: No
- Subnet group: default/private subnets in that VPC are fine for this stage
- Security group: create/use `jobcron-rds-sg`
- Inbound rule:
  - Type: PostgreSQL
  - Port: 5432
  - Source: the EC2 app instance security group, not `0.0.0.0/0`
- Database authentication: Password authentication

This is the most important security setting. The database should not be exposed
to the public internet. The app server connects to it inside the VPC.

## Backups

- Automated backups: enabled
- Backup retention: 7 days
- Backup window: off-hours, for example `18:00-18:30 UTC` (`03:00-03:30 KST`)
- Copy tags to snapshots: enabled
- Deletion protection: enabled

RDS automated backups cover short-term mistakes and point-in-time recovery. The
weekly MacBook `pg_dump` pull remains the separate, human-controlled backup
layer.

MacBook backup path currently proposed by overseer feedback:

```text
/Users/chanbla11mit/mystuff/projects/job-scraper/backups
```

## Security

- Encryption at rest: enabled
- KMS key: AWS managed key is fine for this stage
- Certificate/TLS: use `sslmode=require` in `DATABASE_URL`
- IAM DB authentication: off for now

Education note: RDS encryption at rest protects storage, snapshots, automated
backups, and read replicas. It does not replace database credentials; anyone
with the DB password can still read the data through PostgreSQL.

## Monitoring And Maintenance

- Performance Insights: off for now
- Enhanced monitoring: off for now
- CloudWatch log exports: skip initially unless AWS makes it easy and free in
  the selected template
- Auto minor version upgrade: on
- Maintenance window: off-hours, for example `19:00-20:00 UTC`
  (`04:00-05:00 KST`)

We can turn on deeper monitoring after the app is live if we need query
diagnostics. For now, fewer paid extras keeps the bill predictable.

## Values Needed After Creation

Record these after AWS finishes creating the instance:

```text
RDS endpoint:
Port: 5432
Database name: jobcron
Username: jobcron_admin
```

Do not write the password into this file. Keep it in a password manager and use
it only when assembling `DATABASE_URL` on the EC2 instance.

## Known Debates

### PostgreSQL 18 vs 16

Decision: PostgreSQL 18.

Reason: local dev/test has now been upgraded to PostgreSQL 18.4 and re-tested.
The app passed storage integration tests, importer/storage tests, full uncached
Go tests, `go vet`, binary builds, and a browser runtime check against a clean
PostgreSQL 18.4 database.

### Single-AZ vs Multi-AZ

Recommendation: Single-AZ.

Reason: the current app is not yet a revenue-critical production service.
Single-AZ gives managed PostgreSQL durability and backups at much lower cost.
Multi-AZ is a later upgrade if uptime becomes important.

### Public vs Private RDS

Recommendation: private RDS.

Reason: only the EC2 app needs database access. Local MacBook administration can
use SSH tunneling through the EC2 instance later if needed, instead of exposing
PostgreSQL to the internet.

### RDS Free Tier vs Dev/Test

Recommendation: choose Free tier if AWS offers it for the account/region and it
still allows PostgreSQL 18 plus the required settings. Otherwise choose Dev/Test
and keep the same small instance/storage choices.

## Source Notes

Checked against current AWS documentation/pages on 2026-07-10:

- Amazon RDS pricing and free tier pages
- Amazon RDS for PostgreSQL version/release notes
- Amazon RDS storage docs
- Amazon RDS VPC/security group docs
- Amazon RDS automated backup retention docs
- Amazon RDS encryption docs
