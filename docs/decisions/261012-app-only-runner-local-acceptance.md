# App-Only Runner — Local Acceptance and Evidence Limits

Date: 2026-10-12. Status: maintained acceptance record, not operational authority.

## Accepted exact implementation

The local runner card `t_6e112520` is complete. Worker run 166 and independent
reviewer run 167 accepted R1–R5, including pending durable-success/postimage-pair
failed-app recovery and completed-receipt digest/semantic binding. The accepted
integrated source is `095bc775610656f3cbe74a076c5be495c30d22b5`, tree
`3a963914ba415838a11412a122458111cd144a15`.

Canonical integration checked 69 Python tests and the complete 27-package Go
matrix: 1,783 passing tests, 247 optional skips, zero failures, plus build/vet,
276 tracked Go files formatted and 103 Markdown destinations. Literal canonical
`gofmt -l .` reported one preexisting ignored historical accessor fixture; it was
not repaired or promoted into the acceptance diff. This is inherited exact-code
acceptance, not a claim that every optional integration or ignored file passed.

The owned native app artifacts used source `689129b`, NOT `095bc775`.
Runner/controller acceptance identity and native app artifact identity are distinct.
Local native journeys, seeded post-change writes, singleton app replacement,
proxy preservation and recovery/reconciliation are valid only within their
recorded local fixtures. Historical rejected candidates and unknown effects do
not become successes when a later correction passes.

## Recovery facts to preserve

- Automatic replacement continuation requires a durable native-success/exact-
  postimage pair with exact runtime/selector/protected-state bindings. Unbound
  replies stay unknown/no-replay; a separate trusted attended settled-exclusive
  disposition is not invented causal provenance.
- Same-container failed/stopped/restarted recovery includes the interrupted
  pending-pair case, under separately named compatible-recovery authority and
  the same target lock. Retain the original evidence; never accept the failed
  release or overwrite unknown history to recover it.
- Completed resume binds receipt digest AND semantic inputs, runtime and acceptance.
  Missing/copied/conflicting receipts and receipt-publication crash windows fail
  closed; historical successful receipts are not regenerated from current state.
- The public disk Caddyfile remains immutable and distinct from owner-only
  secret tmpfs/no-swap custody. Local fixtures do not prove the production positive.

After ownership checks, three disposable app/Caddy/database containers were removed;
local fixture ports 17777/17780 were free. The 23 scoped checkpoint/receipt/handoff
records were unchanged. Volumes, networks, images and recovery evidence were
retained; unknown markers were NOT cleared. These are recorded closeout facts,
not a current port inventory or authorization to clean anything else.

## Current lifecycle and future gates

Useful completed runner spec/plan are [archived with an index](../archive/2026-10-12-app-only-runner-local-acceptance/README.md).
Tiny old-path pointers remain because repository instructions name them; no code
or test changes were needed. `AGENTS.md`/`CLAUDE.md` are not edited by this docs
closeout. Use `docs/README.md` for current status, not their historical draft label.
The [native invocation/custody guide](../../scripts/app_release_fixture/README.md)
and [existing-service release safety contract](261009-ai-rating-and-release-contract.md)
remain maintained requirements for future separately authorized operations.

No production no-swap positive, real 1,800-second stability, IAM/provider ability,
hosted publication or public production acceptance was established by the local
rehearsal. No Gemini call, quota-safety conclusion or model comparison follows.
Do not restart rehearsal, reopen the runner card or discard recovery evidence
merely to author the [new campaign proposal](../specs/261012-ai-rating-live-updates-campaign.md).
Application coding and every applicable live/external effect require separate
owner approval; approved documentation integration is local documentation only.
