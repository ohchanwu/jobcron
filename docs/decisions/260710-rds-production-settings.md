# RDS Production Settings

## Decision

Use a private Amazon RDS for PostgreSQL 18 instance for the first production
deployment:

- Single-AZ `db.t4g.micro`
- 20 GiB General Purpose SSD storage (`gp3` when available)
- deletion protection and storage encryption enabled
- automated backups retained for seven days
- automated-backup window set to off-hours, for example `18:00-18:30 UTC`
  (`03:00-03:30 KST`)
- automatic minor-version upgrades enabled
- no public access; PostgreSQL ingress is limited to the application instance's
  security group inside the same VPC
- password authentication, with the generated password kept outside Git

The production connection string is assembled on the server from the RDS
endpoint and stored only in the untracked production environment file.
RDS automated backups provide short-term point-in-time recovery; a separate
weekly, human-controlled MacBook `pg_dump` remains the independent backup layer.

## Why

Jobcron currently serves one owner, writes a small dataset, and runs one daily
scrape. Managed durability matters now; Multi-AZ availability, larger instances,
and paid monitoring do not yet justify their recurring cost.

## Operations

- Start without Performance Insights, enhanced monitoring, or log exports.
- Use an off-hours maintenance window.
- Reach the private database for administration through the application host,
  such as an SSH tunnel, instead of exposing PostgreSQL publicly.
- Upgrade to Multi-AZ and deeper monitoring when uptime requirements or measured
  query behavior justify them.
