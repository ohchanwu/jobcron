# Superpowers Work Index

Read this file before opening plans, specifications, or implementation reports.
Future sessions should load only the active files listed below and the concise
decision records needed for the current task.

## Active Work

- [Alpha pre-launch fixes](specs/260713-alpha-pre-launch-fixes.md)
- [Alpha pre-launch fixes implementation plan](plans/260713-alpha-pre-launch-fixes-implementation-plan.md)
- [Alpha launch human-blocked steps](specs/260713-alpha-launch-human-blocked-steps.md)

## Recently Archived

- [README deployment status refresh design](archive/2026-07-13-readme-deployment-status-refresh/260713-readme-deployment-status-refresh.md)
- [README deployment status refresh implementation plan](archive/2026-07-13-readme-deployment-status-refresh/260713-readme-deployment-status-refresh-implementation-plan.md)
- [Alpha milestone A polishes specification](archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes.md)
- [Alpha milestone A polishes implementation plan](archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes-implementation-plan.md)
- [Alpha milestone A polishes verification](archive/2026-07-12-alpha-milestone-a-polishes/260712-alpha-milestone-a-polishes-verification.md)
- [Interactive local preview, first-run guidance, and navigation](archive/2026-07-11-interactive-preview-navigation/260711-interactive-preview-navigation.md)
- [Integrated verification report](archive/2026-07-11-interactive-preview-navigation/260711-interactive-preview-navigation-verification.md)

## Stable Decisions

- [No browser-driven scraping for v1.x](decisions/260606-no-browser-driven-scraping.md)
- [RDS production settings](decisions/260710-rds-production-settings.md)
- [Jobcron production and rename decisions](decisions/260711-jobcron-production.md)

## Context Policy

- `plans/` contains only work that is not implemented yet.
- `specs/` contains only designs that are still active or awaiting approval.
- `decisions/` contains short durable facts needed by future work.
- `archive/` contains completed evidence. Do not scan or load it unless an active
  plan or a human explicitly names a specific archived file.
- `.superpowers/sdd/` is ephemeral and must contain only the current execution's
  ignored briefs, reports, and progress ledger.
- When work completes, distill stable facts into `decisions/`, move verbose
  tracked artifacts to a dated archive workstream, and move ignored local-only
  artifacts to `.superpowers/archive/`.
- Git history is the authoritative fallback for old detail. Do not keep verbose
  completed reports active merely for discoverability.
