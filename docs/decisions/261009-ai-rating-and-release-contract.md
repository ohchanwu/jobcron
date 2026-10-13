# AI-Rating and Existing-Service Release Contract

Date: 2026-10-09. Status: maintained technical reference, not operational authority.

## Accepted baseline and historical status

The AI-rating outcomes / stronger-weighting upgrade is implemented and its release
was accepted at `4701eba23989b2dc4eac93ea12f1896ea16b6066`. The local Jobcron
release closeout (`t_21922811`) and private final acceptance receipt record the
exact forward identity, one additive migration invocation, preserved data,
authenticated functional checks and a 37.8-minute observation with zero restarts.
This is scoped historical evidence, not a new live observation or permission.

The [completed launch and release documents](../archive/2026-10-09-alpha-launch-and-ai-rating-release/README.md)
retain original requirements and rationale. Their initial-deployment assumptions,
deadlines, special sequencing and approvals must not be reused as ordinary-release
authority. Current work concerns an upgrade of an existing working service with
valuable data, not an empty bootstrap. The separate compatibility recovery work
remains separate; this document neither merges nor deploys it.

## Outcome and cache semantics to preserve

- **Rated:** at least one accepted evidence item; includes partial acceptance and
  a nonempty result with net zero. Retain cited signed adjustments.
- **No signal:** a structurally valid explicit empty result, a cached success
  with neutral explanatory copy, not a scored LineItem invented from failure.
- **Rejected:** nonempty proposals all fail validation/evidence grouping. Persist
  only bounded status; do not turn rejection into an empty successful score.
- **Failed:** provider, parsing or persistence did not complete successfully.
  No new successful score. Distinguish actual failed attempts from work never
  reached because of caps or budgets.
- Fresh N counts committed current-identity rated/no-signal successes. Processing
  progress, rejected work and failed attempts do not count as fresh success.
- Cache identity is user + posting + goal hash + score version for PostgreSQL.
  Successful cache hits spend nothing. Existing deliberate manual retry eligibility
  does not authorize a new live experiment or an automatic in-run retry.
- Existing prior supported scores may remain faded; failure/rejection cannot
  fabricate a new adjustment. Startup, rendering, polling and reconnect do not
  initiate paid analysis. Keep the established cap, token accounting and exclusion
  rules. Default cap 200 does not overwrite an explicitly saved positive cap.
- Evidence remains posting-cited and goal-linked, with the existing contiguous
  quote/token checks, canonical grouping, per-item/net delta bounds, total clamp
  and hard-exclusion short-circuit. Pacing changes must not alter these rules.

Implementation details and current user-facing distinctions remain in
[re-rate notes](../../internal/server/RERATE_NOTES.md). Those notes and the code
are maintained references; the archived execution plan is not a queue of new work.

## Existing-service release safety

- Bind a reviewed full source commit, successful exact-head CI, immutable artifact
  and platform to an authoritative target/current image and a compatible recovery.
- Preserve complete migration filename/digest/tree and applied-ledger validation.
  The outcome upgrade added SQLite 0013 and PostgreSQL 0020. The untouched older
  binary is not assumed compatible with the resulting ledger. Verify complete
  compatibility, not only that a SQL change is additive.
- App-only releases have unchanged complete migration manifests and no newly
  applied migration, privileged migration command, infrastructure mutation or
  non-image configuration change. Existing startup/ledger validation stays intact. A mismatch
  leaves that lane; it is not an exception to be waived inside the runner.
- Protect the existing data and required pre-change recovery point. Reuse accepted
  restore evidence until relevant schema/tool/target changes invalidate it. Never
  restore a stale writable database after new writes to make recovery start.
- Change only the approved image selector/runtime app. Preserve Caddy, mounts,
  secrets, database, DNS/routing and one active scheduler. Whole-stack Compose
  shutdown or systemd restart is not an app-only replacement.
- Verify exact external readback, functional behavior and required stability.
  Authentication expiry alone does not invalidate completed evidence; affected
  drift must be reobserved before a write materially depending on it.
- Publication, credentials, live reads/writes, deployment and recovery each need
  applicable explicit authority. Ambiguous submissions require reconciliation,
  not blind retry. Global user stop remains global.

These are reused requirements. The [runner is now locally accepted](261012-app-only-runner-local-acceptance.md)
at `095bc775610656f3cbe74a076c5be495c30d22b5`; its
[operator reference](../../scripts/app_release_fixture/README.md) maintains actual
invocation/custody and limits. Local acceptance is not production proof or authority.
The [approved campaign](../specs/261012-ai-rating-live-updates-campaign.md) orders
Gemini pacing/quota then live UI updates, with separate exact-code acceptance.
The owner's later 2026-10-12 instruction authorized local implementation. Gemini
Slice 1 is locally accepted/integrated at `8c369d3cf19a4c966fe4745dfa99738c3fe78367`;
the live-view candidate awaits its own implementation acceptance. A possible
single release after both, or a live experiment, still needs separate authority.
Page-document and new observation paths are strictly read-only; only the two
legacy status GETs retain their session-activity exception chosen by the owner.
