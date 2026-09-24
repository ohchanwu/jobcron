# Jobcron Hermes Orchestration Transition

> **Archived 2026-09-24 — do not follow for current work.** The transition this
> adapter records is completed and validated. Details below are obsolete: the
> active Superpowers work index no longer exists after the docs taxonomy
> migration, and Ponytail `full` inheritance is no longer current — Hermes
> profiles do not run Ponytail as a plugin, and current task-scoped policy
> lives in the canonical `multi-agent-coding-orchestrator` skill. Current
> workers follow repository `AGENTS.md` and that canonical skill; live profile
> selection and authority remain Kanban/task policy, not this document.

## Status and scope

**Status: archived; the transition it records is complete and was validated.**

This document is Jobcron's repository adapter for the canonical Hermes skill
`multi-agent-coding-orchestrator`. The skill defines shared multi-agent policy;
this document adds only constraints that differ for Jobcron. Profile selection
and live authority are recorded on each Kanban card, not in this specification.
All tracked records remain public-safe: no credentials, account identifiers, or
machine-local paths.

## Shared procedure

Follow `multi-agent-coding-orchestrator` for card lifecycle, authority,
workspace handling, verification, review, and external-action controls.
Ponytail `full` is inherited from that shared procedure; this repository does
not duplicate its roster or routing rules.

## Repository-specific constraints

- Adoption is local-only. Review or local integration does not authorize a
  push, publication, deployment, production change, or other external action;
  production and deployment authorization remain separate.
- Autonomous runs invoke Jobcron with `--no-open` and must not launch the
  user's default browser. Required UI smoke checks use a headless browser only
  against the local service.
- Jobcron v1.x forbids browser-driven scraping and browser-fingerprint bypass.
  Do not add or use Playwright, chromedp, or similar browser automation for
  scraping; the local UI-smoke exception does not change this constraint.
- Follow the repository documentation lifecycle: read
  `docs/README.md`, keep active work in the appropriate active
  record, and follow `docs/AGENTS.md` for completion and archival.
- Kanban cards, documentation, and review records are public-safe. Keep
  credentials, account identifiers, machine-local paths, and raw sensitive logs
  out of them.

## Adoption status

- [x] Jobcron's adapter is linked from the active Superpowers work index.
- [x] Jobcron-specific no-browser, `--no-open`, local UI-smoke, local-only, and
  public-safe constraints are recorded.
- [x] End-to-end Jobcron local orchestration lifecycle completed through this
  Kanban dry run: `go test ./...`, `go vet ./...`, and `gofmt -l .` passed; the
  card recorded baseline evidence and completed local review handoff.

## Acceptance and rollback

The transition is accepted when an implementation card records its assigned
profile and authority, completes the shared procedure, and satisfies the
Jobcron constraints above without relying on chat or an external notification.
If the adapter proves insufficient, stop the affected card, record the gap on
Kanban, and revert this documentation commit to restore the prior repository
record.
