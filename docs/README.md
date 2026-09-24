# Documentation Index

Use this index instead of recursively loading the entire documentation tree.

## Architecture

- [Current system architecture](architecture.md)

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

## Learnings

- [Brainstorming skill and autonomous-mode question gates][brainstorming-autonomous-gate]
- [Campaign scope must outlive slice completion][campaign-scope-outlives-slice]
- [Caller-managed convoys require Mayor closeout][caller-managed-convoy-closeout]
- [Local-only polecat work must not use an auto-submit formula][local-only-submit-formula]

## Implementation Work

- Read this index before opening plans, specifications, or implementation
  reports. Load only the active files listed here and the concise decision
  records needed for the current task.

- [Human-assisted alpha deployment][human-assisted-alpha-deployment]
- [Gemini BYOK onboarding](specs/260924-gemini-byok-onboarding.md)

## Recently Archived

Archived files are completed evidence. Do not scan this tree; open an entry
only when the active task or a human explicitly names it. See
[archive guidance](archive/AGENTS.md).

- [Completed PostgreSQL convergence specification][postgresql-convergence-spec-archive]
- [Completed Hermes orchestration transition adapter][hermes-orchestration-transition]
- [Superseded first-production deployment records][simplified-alpha-deployment-archive]
- [Terraform Slice 3 implementation archive][terraform-slice-3-plan]
- [Terraform Slice 3 verification][terraform-slice-3-verification]
- [Terraform Slice 2: canonical VPC and EIP adoption][terraform-slice-2-spec]
- [Terraform Slice 2 implementation][terraform-slice-2-plan]
- [Terraform Slice 2 verification][terraform-slice-2-verification]
- [Terraform Slice 1: identity, state bootstrap, and CI][terraform-slice-1-plan]
- [Terraform Slice 1 verification][terraform-slice-1-verification]
- [Superseded first production human steps][archived-first-production-human-steps]
- [PostgreSQL account-mutation clock source][account-mutation-clock-plan]
- [Contextual dealbreaker match-provenance contract][dealbreaker-provenance-spec]
- [Contextual dealbreaker match-provenance implementation][dealbreaker-provenance-plan]
- [AI re-rate blocker surfacing](archive/2026-07-25-ai-rerate-blocker-surfacing/260725-ai-rerate-blocker-surfacing-plan.md)
- [Multi-user account expansion specification][multi-user-account-spec]
- [Multi-user account expansion implementation][multi-user-account-plan]
- [Contextual dealbreaker validation specification][contextual-dealbreaker-spec]
- [Contextual dealbreaker validation implementation][contextual-dealbreaker-plan]
- [Ponytail codebase reduction campaign][ponytail-campaign-plan]
- [Ponytail reduction candidate ledger][ponytail-campaign-ledger]
- [Ponytail campaign verification][ponytail-campaign-verification]
- [Daangn canonical role URLs][daangn-canonical-spec]
- [Daangn canonical role URL implementation][daangn-canonical-plan]
- [PostgreSQL convergence Slice 4 plan](archive/2026-07-16-postgresql-convergence-slice-4/260715-postgresql-convergence-slice-4-verified-sqlite-import.md)
- [PostgreSQL convergence Slice 4 verification](archive/2026-07-16-postgresql-convergence-slice-4/260715-postgresql-convergence-slice-4-verification.md)
- [PostgreSQL convergence Slice 3 plan](archive/2026-07-15-postgresql-convergence-slice-3/260715-postgresql-convergence-slice-3-local-postgresql-bootstrap.md)
- [PostgreSQL convergence Slice 3 verification](archive/2026-07-15-postgresql-convergence-slice-3/260715-postgresql-convergence-slice-3-verification.md)
- [PostgreSQL convergence Slice 2 plan](archive/2026-07-15-postgresql-convergence-slice-2/260715-postgresql-convergence-slice-2-user-scoped-ai-runtime.md)
- [PostgreSQL convergence Slice 2 verification](archive/2026-07-15-postgresql-convergence-slice-2/260715-postgresql-convergence-slice-2-verification.md)
- [PostgreSQL credential foundation: Slice 1 implementation plan](archive/2026-07-14-postgresql-credential-foundation/260714-postgresql-credential-foundation-implementation-plan.md)
- [PostgreSQL credential foundation: Slice 1 verification](archive/2026-07-14-postgresql-credential-foundation/260714-postgresql-credential-foundation-verification.md)
- [Alpha pre-launch fixes specification](archive/2026-07-13-alpha-pre-launch-fixes/260713-alpha-pre-launch-fixes.md)
- [Alpha pre-launch fixes implementation plan](archive/2026-07-13-alpha-pre-launch-fixes/260713-alpha-pre-launch-fixes-implementation-plan.md)
- [Alpha pre-launch fixes verification](archive/2026-07-13-alpha-pre-launch-fixes/260713-alpha-pre-launch-fixes-verification.md)
- [README deployment status refresh design](archive/2026-07-13-readme-deployment-status-refresh/260713-readme-deployment-status-refresh.md)
- [README deployment status refresh implementation plan](archive/2026-07-13-readme-deployment-status-refresh/260713-readme-deployment-status-refresh-implementation-plan.md)
- [Alpha milestone A polishes specification](archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes.md)
- [Alpha milestone A polishes implementation plan](archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes-implementation-plan.md)
- [Alpha milestone A polishes verification](archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes-verification.md)
- [Interactive local preview, first-run guidance, and navigation](archive/2026-07-11-interactive-preview-navigation/260711-interactive-preview-navigation.md)
- [Integrated verification report](archive/2026-07-11-interactive-preview-navigation/260711-interactive-preview-navigation-verification.md)

## Explanations

- [How Gas Town communicates with agents and why startup/nudging broke](explanations/260803-gastown-communication-and-startup-repair-explained.md)

## Stable Decisions

- [No browser-driven scraping for v1.x](decisions/260606-no-browser-driven-scraping.md)
- [RDS production settings](decisions/260710-rds-production-settings.md)
- [Jobcron production and rename decisions](decisions/260711-jobcron-production.md)
- [Hosted-first product and local database convergence](decisions/260714-hosted-first-local-database-convergence.md)

## Context Policy

- `plans/` contains only work that is not implemented yet.
- `specs/` contains only designs that are still active or awaiting approval.
- `decisions/` contains short durable facts needed by future work.
- `archive/` contains completed evidence. Do not scan or load it unless an
  active plan or a human explicitly names a specific archived file.
- `.superpowers/sdd/` is ephemeral and must contain only the current
  execution's ignored briefs, reports, and progress ledger.
- When work completes, distill stable facts into `decisions/`, move verbose
  tracked artifacts to a dated archive workstream, and move ignored local-only
  artifacts to `.superpowers/archive/`.
- Git history is the authoritative fallback for old detail. Do not keep verbose
  completed reports active merely for discoverability.
- Update this index and repository `AGENTS.md` when adding, moving, or removing
  durable records.

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

[brainstorming-autonomous-gate]: learnings/260717-brainstorming-autonomous-mode-question-gate.md
[campaign-scope-outlives-slice]: learnings/260718-campaign-scope-outlives-slice-completion.md
[caller-managed-convoy-closeout]: learnings/260718-caller-managed-convoy-closeout.md
[local-only-submit-formula]: learnings/260719-local-only-polecat-submit-formula.md
[human-assisted-alpha-deployment]:
  specs/260923-human-assisted-alpha-deployment.md
[simplified-alpha-deployment-archive]:
  archive/2026-09-23-simplified-alpha-deployment/README.md
[terraform-slice-3-plan]:
  archive/2026-07-28-terraform-slice-3/260728-terraform-slice-3-private-database-secret-containers-implementation.md
[terraform-slice-3-verification]:
  archive/2026-07-28-terraform-slice-3/260728-terraform-slice-3-verification.md
[terraform-slice-2-spec]:
  archive/2026-07-27-terraform-slice-2/260727-terraform-slice-2-canonical-vpc-eip-adoption.md
[terraform-slice-2-plan]:
  archive/2026-07-27-terraform-slice-2/260727-terraform-slice-2-canonical-vpc-eip-adoption-implementation.md
[terraform-slice-2-verification]:
  archive/2026-07-27-terraform-slice-2/260727-terraform-slice-2-verification.md
[terraform-slice-1-plan]:
  archive/2026-07-26-terraform-slice-1/260726-terraform-slice-1-identity-state-bootstrap-ci.md
[terraform-slice-1-verification]:
  archive/2026-07-26-terraform-slice-1/260726-terraform-slice-1-verification.md
[archived-first-production-human-steps]:
  archive/2026-07-26-first-production-launch-human-blocked-steps/260716-first-production-launch-human-blocked-steps.md
[account-mutation-clock-plan]:
  archive/2026-07-26-postgresql-account-mutation-clock-source/260726-postgresql-account-mutation-clock-source.md
[ponytail-campaign-plan]:
  archive/2026-07-18-ponytail-codebase-reduction/260717-campaign-plan.md
[ponytail-campaign-ledger]:
  archive/2026-07-18-ponytail-codebase-reduction/260717-candidate-ledger.md
[ponytail-campaign-verification]:
  archive/2026-07-18-ponytail-codebase-reduction/260718-verification.md
[hermes-orchestration-transition]:
  archive/2026-09-24-hermes-orchestration-transition/260919-hermes-orchestration-transition.md
[postgresql-convergence-spec-archive]:
  archive/2026-09-24-postgresql-convergence-spec/260714-postgresql-local-convergence-user-ai-credentials.md
[contextual-dealbreaker-spec]:
  archive/2026-07-18-contextual-dealbreaker-validation/260718-stage-1-contextual-dealbreaker-validation-and-exclusion-evidence.md
[contextual-dealbreaker-plan]:
  archive/2026-07-18-contextual-dealbreaker-validation/260718-stage-1-contextual-dealbreaker-validation-and-exclusion-evidence-plan.md
[dealbreaker-provenance-spec]:
  archive/2026-07-25-contextual-dealbreaker-match-provenance/260725-contextual-dealbreaker-match-provenance-contract.md
[dealbreaker-provenance-plan]:
  archive/2026-07-25-contextual-dealbreaker-match-provenance/260725-contextual-dealbreaker-match-provenance-implementation.md
[multi-user-account-spec]:
  archive/2026-07-22-multi-user-account-expansion/260715-multi-user-account-expansion.md
[multi-user-account-plan]:
  archive/2026-07-22-multi-user-account-expansion/260722-multi-user-account-expansion.md
[daangn-canonical-spec]:
  archive/2026-07-17-daangn-canonical-role-urls/260717-daangn-canonical-role-urls.md
[daangn-canonical-plan]:
  archive/2026-07-17-daangn-canonical-role-urls/260717-daangn-canonical-role-urls-plan.md
