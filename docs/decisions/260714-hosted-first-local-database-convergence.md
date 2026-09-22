# Hosted-First Product And Local Database Convergence

**Date:** 2026-07-14
**Status:** Accepted

## Decision

- `jobcron.app` is the primary full-product path once it launches.
- `demo.jobcron.app` remains the immediate read-only evaluation path.
- Release binaries, build-from-source instructions, and the writable localhost
  preview remain available, but the READMEs present them as advanced paths for
  contributors and self-hosters rather than the default user journey.
- SQLite and the isolated SQLite preview remain supported through the alpha
  launch so database work does not delay go-live.
- Local runs must converge on PostgreSQL 18 immediately after launch. This is
  the first post-launch persistence task and must finish before adding another
  database-backed product feature.
- After a documented migration path preserves existing local state, remove the
  SQLite runtime path and its SQLite-specific maintenance burden.

## Why

Hosted-first positioning gives ordinary users one clear path while retaining a
useful development and self-hosting workflow. Keeping SQLite only through launch
avoids a late storage migration, but maintaining SQLite and PostgreSQL
indefinitely would duplicate schema, query, migration, and test work and allow
the two modes to drift.

## Post-Launch Completion Contract

The PostgreSQL convergence follow-up must:

1. make the repository's PostgreSQL 18 Compose service the supported local data
   store;
2. make the isolated writable preview use disposable PostgreSQL state without
   touching the developer's normal database;
3. provide and verify an SQLite-to-PostgreSQL migration path for existing local
   profiles, postings, scores, bookmarks, hidden state, AI caches, and AI usage;
4. update release, development, CI, and test documentation to exercise the same
   persistence backend; and
5. remove the SQLite runtime only after migration and rollback verification pass.

This follow-up is intentionally not a pre-launch blocker.
