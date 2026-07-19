# Terraform AWS Foundation And Cloudflare Ingress Automation

**Status:** Approved design, awaiting implementation planning

**Recorded:** 2026-07-19

**Scope:** First production infrastructure only

## Purpose

Move Jobcron's first production infrastructure into Terraform before launch. The first
implementation must fix the disconnected EC2/RDS network, make the host reproducible, and keep
origin ingress synchronized with Cloudflare's published IPv4 ranges without routine operator
edits.

This specification replaces the infrastructure assumptions in the current production rollout
guide. It does not authorize an AWS apply or production cutover by itself.

## Operator-Reported Current State

The following inventory was reported from the AWS console on 2026-07-19. Exact account IDs,
resource IDs, addresses, names, endpoints, and credentials remain outside tracked documentation.
Implementation must verify every item through the authenticated AWS CLI before planning changes.

### EC2

- One standalone `t4g.micro` instance runs in `ap-northeast-2a`.
- It uses an Amazon Linux 2023 arm64 AMI and an Elastic IPv4 address.
- Its custom VPC has four public subnets across `ap-northeast-2a` through
  `ap-northeast-2d`. All four use a default route to an internet gateway.
- Its root volume is an unencrypted 8 GiB `gp3` volume.
- No IAM instance profile, launch template, Auto Scaling group, load balancer, or target group
  exists.
- The security group has two SSH rules sourced from private operator IPv4 addresses. It has no
  web ingress.
- Docker and Jobcron are not installed. The only application material is an untracked `.env`
  file.

### RDS

- One disposable pre-launch PostgreSQL 18.4 `db.t4g.micro` instance exists.
- It is Single-AZ, not publicly accessible, encrypted, deletion-protected, and backed up for
  seven days.
- It uses 20 GiB of `gp3` storage.
- Its subnet group contains four public subnets in a different VPC from EC2.
- The EC2 and RDS VPCs have no peering, Transit Gateway, or other private route between them.
- Its security group includes PostgreSQL rules referring to the EC2 security group and one
  operator IPv4 address. Neither rule creates reachability across the isolated VPCs.

The configured `DATABASE_URL` therefore does not make the current topology functional. Network
reachability must be fixed before the first application start.

### Automation And Edge

- No Terraform code, remote state bucket, state locking, GitHub OIDC provider, or deployment
  role exists.
- `jobcron.app` is not yet a proxied Cloudflare record.
- The intended edge mode is Cloudflare Full (strict), with Caddy terminating origin TLS.
- The production repository currently assumes manual host preparation, a persistent EC2
  `.env`, ports 80 and 443, and Caddy-managed public certificates.

## Goals

1. Establish protected remote Terraform state and short-lived GitHub Actions AWS access.
2. Adopt one existing VPC as the canonical application VPC.
3. Recreate disposable RDS in private subnets inside that VPC.
4. Provision a reproducible replacement EC2 host in the same VPC.
5. Expose only Caddy port 443, and only to current Cloudflare origin IPv4 ranges.
6. Replace inbound SSH with AWS Systems Manager Session Manager.
7. Keep runtime secrets, database passwords, and TLS private keys out of Git, Terraform state,
   EC2 user data, and persistent host files.
8. Preserve a safe, explicit rollback boundary until the new stack passes production checks.

## Non-Goals

- Managing Cloudflare DNS, encryption mode, Origin CA issuance, or HSTS through Terraform.
- Enabling HSTS during the first cutover.
- Creating an ALB, Auto Scaling group, launch template, NAT gateway, or private app tier.
- Enabling RDS Multi-AZ, read replicas, enhanced monitoring, or paid observability.
- Replacing Docker Compose as the production process boundary.
- Automating the verified SQLite-to-PostgreSQL data import or owner creation.
- Importing the disposable old EC2 instance, its VPC, or the old RDS instance.
- Adding IPv6 origin ingress while the origin has only an Elastic IPv4 address.

## Approved Architecture

```text
Visitor
  |
  v
Cloudflare proxy
  |
  | HTTPS :443, Cloudflare IPv4 prefix list only
  v
Elastic IPv4 -> EC2 public subnet -> Caddy -> app:7777
                                      |
                                      | PostgreSQL :5432, TLS
                                      v
                              RDS private subnets

GitHub schedule -> OIDC edge role -> Terraform edge state -> prefix-list entries
Human-approved run -> OIDC production role -> Terraform production state
EC2 instance role -> Session Manager + one runtime Secrets Manager secret
```

The existing RDS VPC becomes the canonical VPC. Its public subnets remain available for the
single EC2 origin. Terraform adds two private database subnets in distinct Availability Zones,
with no internet-gateway route, and places the replacement RDS subnet group there.

The old EC2 VPC is not peered. The old EC2 instance remains outside Terraform until the rollback
window closes, then the operator removes it and its unused network resources explicitly.

## Terraform Layout And Ownership

Use three small root configurations rather than a module hierarchy:

```text
infra/terraform/
  bootstrap/   # state bucket, GitHub OIDC provider, and automation roles
  production/  # adopted VPC/EIP plus RDS, EC2, IAM, secrets, and security groups
  edge/        # Cloudflare IPv4 prefix list and the origin :443 ingress rule
```

Do not add one-implementation modules. Extract a module only after a second real environment or
repeated resource group exists.

Each root has its own state key and lock file. The edge root owns the port 443 ingress rule; the
production root must not declare the same rule. Production tags the canonical VPC and origin
security group so the edge root can discover them without copying resource IDs into Git.

Exact backend bucket names, role ARNs, account IDs, resource IDs, and import IDs live in
untracked backend files or protected GitHub environment variables.

## State Bootstrap And Human Identity

The AWS root user is not used for the CLI, Terraform, or GitHub Actions. Root MFA is already
enabled and remains reserved for account-level recovery and billing tasks.

Before the first Terraform run, the operator must:

1. enable IAM Identity Center for the account;
2. create a named administrator identity with MFA;
3. install AWS CLI v2 and Terraform on the trusted Mac;
4. authenticate with temporary credentials through `aws configure sso`; and
5. verify the expected account and `ap-northeast-2` region without printing credentials.

The bootstrap root initially uses local state on the trusted Mac. Its first apply creates:

- one private S3 state bucket with public access blocked;
- bucket versioning and server-side encryption;
- a bucket policy requiring TLS;
- GitHub's AWS OIDC provider, if the account does not already have one;
- a human-approved production role; and
- a narrow unattended edge role.

After creation, migrate bootstrap state into the same bucket. Enable S3 native locking with
`use_lockfile = true`. Do not create a DynamoDB lock table; HashiCorp now marks DynamoDB-based
locking as deprecated.

All three state resources use `prevent_destroy`. The bucket retains old object versions so an
operator can recover from an accidental state overwrite. State may contain sensitive metadata,
so read and write access is limited to the Identity Center administrator and the matching OIDC
role and state key.

## Existing Resource Adoption

Adopt only the infrastructure that belongs in the final layout:

- the existing RDS VPC;
- its internet gateway;
- its four public subnets and associated public route table; and
- the existing Elastic IPv4 allocation.

Use temporary Terraform import blocks or equivalent CLI imports with IDs supplied outside Git.
The first adoption plan must show imports only, plus explicitly approved tag additions. It must
show no replacement, destruction, route change, subnet change, or EIP reassociation.

Apply adoption separately from new resource creation. Remove temporary import inputs after state
adoption. Do not import the old EC2 instance, old EC2 VPC, current RDS instance, current RDS
subnet group, or their stale security groups.

## Network Requirements

### Public Origin Tier

- Reuse one canonical-VPC public subnet for the replacement EC2 instance.
- Keep the existing Elastic IPv4 allocation, but do not re-associate it until cutover.
- The origin security group has no SSH, HTTP, or general public ingress.
- Port 443 ingress is owned by the edge stack and references one customer-managed Cloudflare
  IPv4 prefix list.
- The host retains outbound internet access for scraping, image pulls, package updates, AWS API
  calls, DNS, and time synchronization.
- Require IMDSv2. Containers do not receive instance-metadata access.

### Private Database Tier

- Create two private subnets in distinct Availability Zones using verified unused CIDR ranges.
- Associate them with a route table that has only local VPC routing.
- Do not create a NAT gateway for database subnets.
- The RDS security group accepts TCP 5432 only from the origin application security group.
- Remove all personal-IP and cross-VPC database rules with the disposable old RDS stack.

The implementation plan must include a CIDR-capacity check before assigning new subnet ranges.
Exact VPC and subnet CIDRs stay in the protected operator inventory, not tracked prose.

## RDS Requirements

Create a new Terraform-managed RDS for PostgreSQL instance with these launch settings:

- PostgreSQL 18.4 with automatic minor-version upgrades enabled;
- Single-AZ `db.t4g.micro`;
- 20 GiB encrypted `gp3` storage;
- seven-day automated backup retention;
- an off-hours backup window;
- deletion protection enabled;
- `publicly_accessible = false`;
- TLS required by the application connection string; and
- a DB subnet group containing the two new private subnets.

Add Terraform `prevent_destroy` in addition to RDS deletion protection. Any later replacement
must remove both safeguards deliberately and require a final snapshot.

Set RDS to manage its master password in its own Secrets Manager secret. Terraform and the EC2
instance role never receive that password. After EC2 has Session Manager access, the operator opens
an SSM port-forward from the trusted Mac to private RDS. The operator retrieves the master secret
locally with the Identity Center session, creates a lower-privilege Jobcron database role over the
TLS tunnel, then stores only that application's TLS-required `DATABASE_URL` in the runtime secret.

The current RDS is disposable. Delete it only after the new database, import, application checks,
and rollback checkpoint are complete.

## EC2 And Runtime Requirements

Provision one replacement host with:

- `t4g.micro` and an approved Amazon Linux 2023 arm64 AMI;
- an encrypted 8 GiB `gp3` root volume;
- no EC2 key pair and no inbound SSH;
- an IAM instance profile;
- Systems Manager Agent enabled; and
- Docker Engine and the Compose v2 plugin installed by value-blind bootstrap code.

The host must never build images. It pulls only the approved immutable arm64 image. Configure
Docker log rotation, retain the current and previous rollback images, and prune older unreferenced
images after a successful deployment.

Eight GiB remains the initial ceiling. Expand the volume when free space falls below 2 GiB after
normal pruning. Expansion is one-way; do not pre-allocate 20 GiB without observed need.

The instance profile grants only:

- the standard permissions required for Session Manager; and
- `secretsmanager:GetSecretValue` for the one Jobcron runtime secret.

It must not read the RDS master secret or mutate the runtime secret.

## Runtime Secret And Fail-Closed Startup

Terraform creates the runtime Secrets Manager container but never creates a secret version. The
operator populates it outside Terraform after the new application database role exists.

One secret version contains the existing production environment values plus:

- the approved immutable image reference;
- the production credential-encryption master key;
- the lower-privilege application `DATABASE_URL`;
- the Cloudflare Origin CA certificate; and
- its private key.

No real value appears in Git, Terraform variables, Terraform plans, Terraform state, EC2 user
data, chat, or logs.

A systemd preparation service fetches the secret before each Jobcron start. With `umask 077`, it
materializes the Compose environment and Caddy certificate files below `/run/jobcron`. `/run` is
memory-backed and cleared at reboot. The service extracts expected fields explicitly, validates
that every required value is present, and never prints values.

Jobcron and Caddy fail closed if retrieval or validation fails. Restarting Jobcron fetches the
latest secret version before Compose starts. The EC2 filesystem contains no persistent `.env` or
TLS private key.

## Caddy And Cloudflare Boundary

Caddy is the only public container and publishes host port 443 only. It loads the Origin CA
certificate and private key from read-only `/run/jobcron` mounts and proxies to `app:7777` on the
private Docker network.

The Cloudflare launch steps remain human-controlled:

1. issue an Origin CA certificate covering the apex and `www` hostnames;
2. populate the runtime secret before Caddy starts;
3. set the apex record to the Elastic IPv4 address and enable proxying;
4. keep `www` proxied and routed to the canonical apex behavior;
5. set encryption mode to Full (strict); and
6. verify the proxied user path before considering HSTS.

Port 80 remains closed. A browser connecting directly to the origin would not trust a Cloudflare
Origin CA certificate; that is expected because the security group accepts only Cloudflare.

Cloudflare does not notify customers when Origin CA certificates approach expiration. Record the
certificate expiry in the private operator calendar and schedule renewal before it lapses.

HSTS is a later human decision. Enable it only after stable HTTPS, redirect, subdomain, and
rollback behavior has been verified because browsers cache the policy.

## Cloudflare IPv4 Automation

The edge root reads Cloudflare's official `https://www.cloudflare.com/ips-v4` endpoint. On
2026-07-19 it published 15 IPv4 CIDRs. The origin has no IPv6 address, so the seven published IPv6
ranges are not applied.

Normalize the response into a unique set and fail before planning an update unless all checks
pass:

- the HTTP request succeeds;
- every non-empty line is a valid IPv4 CIDR;
- the set contains between 10 and 20 entries;
- no entry is the default route, loopback, link-local, multicast, or RFC 1918 space; and
- no duplicate survives normalization.

Create one regional customer-managed prefix list with `max_entries = 20`. The fixed ceiling
provides modest growth room while limiting its security-group quota weight. If Cloudflare exceeds
the ceiling, automation fails safely and requires an explicit quota and design review.

The origin ingress rule references the prefix list on TCP 443. Updating the list automatically
updates the effective source ranges for that rule.

## GitHub Actions And IAM

Use two AWS OIDC roles. Never store AWS access keys in GitHub.

### Production Role

- Trust only this repository's protected production GitHub environment.
- Require `aud = sts.amazonaws.com` and the expected GitHub OIDC subject.
- Run only through a manually dispatched workflow with environment approval.
- Scope state access to the bootstrap and production keys.
- Grant only the resource actions required by the reviewed production configuration.

### Edge Role

- Trust only the main branch's protected edge environment.
- Scope state access to the edge key and its lock file.
- Permit read-only discovery of the tagged VPC and origin security group.
- Permit changes only to the tagged Cloudflare prefix list and its one tagged ingress rule.
- Do not grant EC2 instance, RDS, IAM, Secrets Manager, or general security-group administration.

Pin third-party workflow actions by full commit SHA. Grant workflows `id-token: write` and
`contents: read` only where required.

The edge workflow runs daily and supports manual dispatch. It:

1. initializes the edge backend;
2. runs formatting and validation checks;
3. creates and saves a plan;
4. stops on fetch or validation failure;
5. applies that exact saved plan only when changes exist; and
6. uses a concurrency group so runs cannot overlap.

The scheduled workflow may apply only the edge root. Full production Terraform is never applied
on a schedule.

## Cutover Sequence

1. Create the Identity Center administrator and install the local CLI tools.
2. Apply bootstrap locally, migrate its state to S3, and verify state recovery.
3. Inventory and import the canonical VPC, public networking, and Elastic IPv4 allocation.
4. Apply the new private subnets, security groups, RDS, runtime-secret container, IAM resources,
   and replacement EC2 host. Jobcron and Caddy remain stopped while the secret is empty.
5. Confirm Session Manager access, then open an SSM port-forward from the trusted Mac to RDS.
6. Create the lower-privilege database role and populate the runtime secret without displaying
   values or granting the instance access to the RDS master secret.
7. Verify secret retrieval, Docker, Compose, Caddy, migrations, owner creation, and the data
   import through private paths.
8. Apply the edge root and confirm the security group has only Cloudflare port 443 ingress.
9. Re-associate the Elastic IPv4 address, enable the proxied Cloudflare records, and select Full
   (strict).
10. Walk the real production user path and record private evidence.
11. Keep old resources through the rollback window, then remove them explicitly.

The implementation plan must separate each state-changing phase with a saved plan and a human
approval checkpoint. No checkpoint may print a secret, endpoint, account identifier, or personal
address.

## Failure And Rollback Rules

- A failed Cloudflare fetch or validation leaves the last prefix-list version active.
- A failed edge apply is retried only after inspecting the saved plan and workflow logs.
- A missing or malformed runtime secret keeps Jobcron and Caddy stopped.
- Before EIP cutover, destroy only newly created disposable resources after reviewing a plan.
- During cutover, re-associate the EIP to the old instance and disable proxying if origin routing
  must be reversed.
- After new PostgreSQL writes begin, preserve the new RDS instance and use snapshots or
  point-in-time recovery rather than guessing at rollback.
- Recover Terraform state from S3 object versions; never reconstruct state by applying blindly.
- Do not delete the old EC2 instance, old VPC, or old RDS until the explicit rollback checkpoint
  expires.

## Security And Publication Rules

- Treat every tracked file and workflow log as public.
- Never track backend bucket names, AWS account IDs, ARNs, resource IDs, IP addresses, VPC CIDRs,
  database endpoints, usernames, passwords, certificate bodies, private keys, or secret values.
- Do not put secret values in Terraform `sensitive` variables; `sensitive` hides display but does
  not keep values out of state.
- Use partial backend configuration and protected GitHub environment variables for identifiers.
- Inspect every Terraform plan for destructive actions and unexpected broad IAM permissions.
- Run Gitleaks and a manual publication review before each infrastructure documentation commit.

## Required Documentation Changes During Implementation

Implementation is incomplete until it updates:

- `docs/architecture.md` with the final Terraform, network, identity, and secret boundaries;
- `deploy/production/README.md` with the Terraform-managed host model;
- `deploy/production/HUMAN_DEPLOY_GUIDE.md` with bootstrap, secret, and cutover steps;
- the first-production human-blocked specification;
- the Caddy and Compose references that currently assume ports 80 and 443 and automatic public
  certificates; and
- `docs/superpowers/README.md`, archiving completed tracked work when appropriate.

## Verification Requirements

### Static And Plan Gates

- `terraform fmt -check -recursive`, `terraform init -backend=false`, and `terraform validate`
  pass for every root.
- Provider and action versions are pinned and reviewed.
- The adoption plan contains no replacement or destruction.
- The production plan contains only approved imports, new resources, and the explicit EIP
  cutover action in its own phase.
- The edge plan can change only the prefix list and its port 443 rule.
- No plan, state inspection, user data, or workflow log exposes runtime secret values.

### AWS And User-Path Gates

- A browser-based Session Manager shell works and port 22 is absent.
- Direct internet connections to the origin EIP on ports 22, 80, 7777, and 5432 fail.
- The origin security group accepts port 443 only through the Cloudflare prefix list.
- RDS is not publicly accessible and accepts 5432 only from the application security group.
- The application connects to RDS with TLS through the lower-privilege database role.
- Rebooting EC2 recreates transient files, starts the approved image, and leaves no persistent
  `.env` or origin private key.
- Docker logs rotate, the current and previous images remain, and older unused images are pruned.
- Cloudflare Full (strict) serves the apex and `www` behavior through a real browser without a
  certificate error or redirect loop.
- A direct-origin request remains blocked after proxying is enabled.
- A no-change scheduled edge run is a no-op; a controlled fixture change updates only prefix-list
  entries; malformed input fails without changing AWS.

## Acceptance Criteria

This specification is implemented when:

1. protected remote state and OIDC roles exist without long-lived AWS keys;
2. the canonical VPC and Elastic IPv4 allocation are safely adopted;
3. EC2 and private RDS share the canonical VPC and communicate only through their security groups;
4. RDS, EC2 storage, state, and secrets are encrypted;
5. the host has no SSH ingress or key pair and is manageable through Session Manager;
6. runtime values and the Origin CA private key are transient and absent from Terraform state;
7. Cloudflare IPv4 changes update the AWS prefix list automatically through the narrow edge
   workflow;
8. only Cloudflare can reach Caddy on port 443;
9. the real proxied application passes the production browser checks; and
10. stale deployment documentation and superseded manual assumptions are updated or archived.

## Implementation Slices

```text
1. Identity, state bootstrap, and Terraform CI
   |
2. Canonical VPC and EIP adoption
   |
3. Private database tier and secret containers
   |
4. Replacement EC2, Session Manager, transient runtime, and Caddy
   |
5. Cloudflare prefix-list automation
   |
6. Data bootstrap, EIP/DNS cutover, verification, and documentation
```

Each slice requires its own implementation plan and verification checkpoint. Autoscaling, ALB,
private app subnets, and Multi-AZ RDS are separate future designs triggered by observed availability
or capacity needs.

## Authoritative References

- [Terraform S3 backend and native lock files](https://developer.hashicorp.com/terraform/language/backend/s3)
- [Terraform resource import workflow](https://developer.hashicorp.com/terraform/language/import)
- [AWS customer-managed prefix lists](https://docs.aws.amazon.com/vpc/latest/userguide/managed-prefix-lists.html)
- [AWS Systems Manager Session Manager](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager.html)
- [RDS-managed master passwords](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/rds-secrets-manager.html)
- [GitHub Actions OIDC for AWS](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-aws)
- [Cloudflare IP ranges](https://www.cloudflare.com/ips/)
- [Cloudflare Origin CA](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/)
- [Cloudflare Full strict mode](https://developers.cloudflare.com/ssl/origin-configuration/ssl-modes/full-strict/)
