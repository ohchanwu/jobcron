# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This changelog begins at the **v1.0 epoch** — the first versioned release of the
multi-source scraping, cross-portal dedup, scoring engine, and daily-briefing UI
that already shipped. Earlier development is captured in git history; versioned
releases start with v1.0.0. The BYOK AI integration, when it lands, is a v1.1
feature (see `CLAUDE.md`).

## How to cut a release

1. Move entries out of `[Unreleased]` into a new section dated today,
   e.g. `## [1.0.0] - 2026-06-15`.
2. Open a new empty `[Unreleased]` block at the top.
3. `git tag -a vX.Y.Z -m "Release X.Y.Z"` and `git push origin vX.Y.Z`.
   GoReleaser (`.goreleaser.yml`) reads the tag and bakes it into the binary
   via `-ldflags "-X main.version={{ .Version }}"`; the `--version` CLI flag
   then surfaces it.

## [Unreleased]
