# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This changelog begins at the **v2.0 epoch**, which marks the introduction of
the BYOK AI integration. Earlier work (multi-source scraping, cross-portal
dedup, the scoring engine, the daily-briefing UI) is captured in git history;
versioned releases start with v2.0.0.

## How to cut a release

1. Move entries out of `[Unreleased]` into a new section dated today,
   e.g. `## [2.0.0] - 2026-06-15`.
2. Open a new empty `[Unreleased]` block at the top.
3. `git tag -a vX.Y.Z -m "Release X.Y.Z"` and `git push origin vX.Y.Z`.
   GoReleaser (`.goreleaser.yml`) reads the tag and bakes it into the binary
   via `-ldflags "-X main.version={{ .Version }}"`; the `--version` CLI flag
   then surfaces it.

## [Unreleased]
