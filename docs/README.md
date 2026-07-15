# Documentation Index

Use this index instead of recursively loading the entire documentation tree.

## Product

- [Parked feature ideas](product/feature-ideas.md)
- [보류된 기능 아이디어](product/feature-ideas.ko.md)

## Scraping

- [Source catalog and roadmap](scraping/source-catalog.md)

## Research

- [Browser-driven and fingerprint-blocked scrapers](research/2026-06-06-browser-driven-scrapers.md)
- [Bundled local model versus bring-your-own-key AI](research/2026-06-09-local-model-bundled-vs-byok.md)
- [Job-platform comparison](research/job-platforms-comparison.md)
- [채용 플랫폼 비교](research/job-platforms-comparison.ko.md)

## Implementation Work

- [Active Superpowers work and context policy](superpowers/README.md)
- [PostgreSQL local convergence and per-user AI credentials](superpowers/specs/260714-postgresql-local-convergence-user-ai-credentials.md)
- [Slice 2: User-scoped AI runtime](superpowers/plans/260715-postgresql-convergence-slice-2-user-scoped-ai-runtime.md)
- [Slice 3: Local PostgreSQL bootstrap](superpowers/plans/260715-postgresql-convergence-slice-3-local-postgresql-bootstrap.md)
- [Slice 4: Verified SQLite import](superpowers/plans/260715-postgresql-convergence-slice-4-verified-sqlite-import.md)
- [Slice 5: First production deployment](superpowers/plans/260715-postgresql-convergence-slice-5-first-production-deployment.md)
- [Multi-user account expansion follow-up](superpowers/specs/260715-multi-user-account-expansion.md)
- [PostgreSQL credential foundation: Slice 1 verification](superpowers/archive/2026-07-14-postgresql-credential-foundation/260714-postgresql-credential-foundation-verification.md)
- [Alpha pre-launch fixes verification](superpowers/archive/2026-07-13-alpha-pre-launch-fixes/260713-alpha-pre-launch-fixes-verification.md)
- [Alpha milestone A polishes verification](superpowers/archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes-verification.md)
- [No browser-driven scraping for v1.x](superpowers/decisions/260606-no-browser-driven-scraping.md)
- [RDS production settings](superpowers/decisions/260710-rds-production-settings.md)
- [Jobcron production and rename decisions](superpowers/decisions/260711-jobcron-production.md)
- [Hosted-first product and local database convergence](superpowers/decisions/260714-hosted-first-local-database-convergence.md)

## Deployment

Deployment configuration remains at the repository root because Docker,
Compose, Caddy, CI, and EC2 commands consume those paths directly.

- [Local PostgreSQL](../deploy/local/README.md)
- [Public demo deployment](../deploy/demo/README.md)
- [Public demo human guide](../deploy/demo/HUMAN_DEPLOY_GUIDE.md)
- [Production deployment](../deploy/production/README.md)
- [Production human guide](../deploy/production/HUMAN_DEPLOY_GUIDE.md)

## Assets

- `assets/screenshots/` contains images embedded by the root README files.
