# Superpowers Work Index

Read this file before opening plans, specifications, or implementation reports.
Future sessions should load only the active files listed below and the concise
decision records needed for the current task.

## Active Work

- None.

## Recently Archived

- [Interactive local preview, first-run guidance, and navigation](archive/2026-07-11-interactive-preview-navigation/260711-interactive-preview-navigation.md)
- [Integrated verification report](archive/2026-07-11-interactive-preview-navigation/260711-interactive-preview-navigation-verification.md)

## Stable Decisions

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
