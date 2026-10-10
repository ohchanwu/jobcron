# App-Only Release Runner v1

Created: 2026-10-09. Updated: 2026-10-10. Status: local implementation/rehearsal,
independent exact-candidate acceptance pending. Readiness review approved the
integrated documentation baseline `37b589bc671fe33e9928c492a1a5d29170a29484`;
the owner separately authorized local Phase 1/2 on card `t_6e112520`.
This is NOT production, publication, credential or provider authority.
Companion: [phased plan](../plans/261009-app-only-release-runner-plan.md).
Source baseline: `4701eba23989b2dc4eac93ea12f1896ea16b6066`.

## Purpose and lifecycle

The runnable target-host entry point is `scripts/app_release.py`. See
[invocation, schema, custody and limitations](../../scripts/app_release_fixture/README.md).
It implements default input-only prepare, authorized preflight, release,
resume/reconcile and separately approved compatible recovery using fixed native
operations. The owned local rehearsal executed real app/DB/proxy replacement,
authenticated HTTPS journeys, retained new writes and lost-reply reconciliation.
Local gate records/short observation do not prove hosted CI, IAM/provider
permissions, public production routing or the 30-minute production gate.
The corrected local rehearsal also exercises failed acceptance followed by an
identifiable stopped app and separately approved native recovery. Read-only
failure history, explicit observed lock handoff, exact committed/completed resume
identity and bounded non-disk stdout are part of the implementation; a prior
receipt never approves a different container/version or protected value.
Review164's consolidated local correction covers pending durable-success/postimage
followed by same-container failure and explicitly approved same-lock recovery,
without rewriting the original pair or accepting the failed release. Completed
receipts bind a durable digest plus semantic input/runtime/acceptance facts;
missing/copied/conflicting receipts and incomplete publication refuse. These
regressions and affected owned native recovery are local evidence only; independent
exact-new-candidate acceptance is still pending.

Make ordinary upgrades of the existing working service easier and faster without
weakening recovery, custody, review or external-write gates. This is a small
reusable operational capability, not another initial launch. Valuable existing
data and service continuity must be protected. Short app interruption is allowed;
this design does not promise zero downtime.

Implement and independently accept the runner on disposable local infrastructure
first. The [Gemini pacing/classification change](261009-gemini-pacing-and-quota-classification.md)
is the proposed first payload afterward. No live investigation, provider call,
publication or deployment follows from this local implementation or these docs.

Reuse the [maintained release safety contract](../decisions/261009-ai-rating-and-release-contract.md),
[production runtime reference](../../deploy/production/README.md), supported
`jobcron-user` operations and native Session Manager route. The historical
initial-launch plans and emergency grant are not current authority.

## Scope and explicit exclusions

V1 has one lane: change the immutable application image while keeping the complete
migration manifests/ledger requirements and all non-image configuration unchanged.
A migration, schema/ledger mismatch, target/infrastructure change, secret rotation,
provider/model-setting edit or DNS/routing change exits this lane. Do not silently
upgrade the lane or append an exception.

Use existing publisher, Compose, runtime helper, secure file-input custody and
operator commands. No generic arbitrary-command framework, daemon, credential
broker, new deployment platform, Terraform execution or secret-store redesign.
Keep the existing recovery line separate; do not merge a recovery PR into the app.

## Inputs, evidence and authority

One release descriptor must bind:

- Full reviewed source SHA, exact successful CI/review evidence and immutable
  candidate image digest/platform, connected through the publication provenance.
- Exact selected target and authoritative expected/current image identity; tags
  alone or a target asserted by the descriptor itself are insufficient.
- Proven compatible recovery source/image, complete migration-manifest equality
  and required ledger predicates for candidate/current/recovery.
- Explicit scope of proposed effects and separate applicable owner approvals,
  including publication, selector persistence, app replacement and recovery.
- Existing-runtime configuration identity, recovery-point requirements, operation
  timeout policy and the required observation/acceptance policy.

Resolve private identifiers/paths in owner-only operator inputs, never tracked
prose. Keep review/CI/target observations independent of the candidate's assertions.
A generated plan or receipt cannot approve itself. The attended operator supplies
approval via the established trusted custody channel; v1 is not a new signature,
anti-replay or permission service. Describe that trust boundary honestly.

## Runnable stages

Stage names may match existing conventions. The implementation must actually
execute supported native operations under explicit authority, not emit a script
for someone else to run while claiming completion.

1. **Prepare:** validate the descriptor, provenance, compatibility and local
   prerequisites. Default is local/nonmutating preparation. Separately authorized
   provider reads bind live target identity/drift; a local plan is not live proof.
   Present named proposed effects and short interruption/recovery behavior.
2. **Preflight:** acquire the per-target release lock, establish applicable
   credentials without logging them, and reobserve the affected target/preimage
   immediately before each materially dependent write. Validate candidate and
   recovery availability and pre-pull before app interruption. Establish the
   required pre-change recovery point through supported operations. Reuse accepted
   restore evidence unless relevant schema/tool/target changes invalidate it.
3. **Persist selector:** change only the image field through the existing runtime
   persistence contract. Bind the native preimage/version, preserve every other
   value and verify exact authoritative readback without outputting secrets.
   Persisting a selector is a protected write with its own named authority.
4. **Replace app:** replace only the app using the existing runtime/Compose
   contract. Never Compose down or the whole-stack systemd restart path. Do not
   remove/recreate mounted secret directories. Preserve Caddy container/config,
   mounts, database, DNS/routing and non-image settings. There must never be two
   active application schedulers. Report the actual interruption.
5. **Verify and observe:** confirm running digest/readiness, public TLS/HTTPS and
   authenticated core journey under applicable observation authority, exact
   migration ledger and protected-state preservation. No scrape/re-rate/provider
   call is a deployment smoke check. Observe at least **30 minutes**, with initial,
   intermediate and final samples, stable app identity and zero unexpected
   restarts; this carries forward the accepted existing-service minimum, not a
   campaign deadline. Relevant failure prevents success; invalidated observation
   restarts only the affected interval/check. A shorter local rehearsal is clearly
   local evidence and cannot satisfy production stability.
6. **Receipt:** record sanitized source/artifact/runtime identities, approvals'
   scopes, stage outcomes, checks, interruption, observation and cleanup. Read
   actual state before declaring success. Partial acceptance is not deployment done.

Retain real maintenance windows, credential expiry and per-operation watchdogs;
do not add blanket authorization expiry or campaign elapsed-time admission tests.
The operator/controller executes live effects. Local workers/reviewers receive
neither production credentials nor live authority.

## Checkpoints, reconciliation and recovery

Use one lightweight owner-only durable checkpoint/receipt and a per-target lock.
Bound native waits/watchdogs and safe backoff; stale locks require observed-state
reconciliation, not automatic deletion because an arbitrary TTL elapsed.

For each submitted effect, retain its native operation identity and expected
pre/postconditions. Interrupted/lost replies are **unknown**, not failed/no-op.
Resume reads authoritative state first: verified committed effect is not repeated;
verified unsubmitted work may continue under applicable authority; conflicting or
unprovable state stops for reconciliation. Retry at most once only after proven
pre-provider/no-submission failure. Do not infer that a local timeout prevented
remote completion. Global owner stop halts the whole campaign; no scoped stop.

The eligible local approach keeps native singleton Compose replacement. Before
every write (including committed-backup/selector crash windows), require the
recorded complete stage-preimage. Automatic replacement continuation needs a
durable successful-command/exact-postimage pair and unchanged CID/image/start/
restart/selector-version/protected state; Compose has no create-ID return channel.
A missing native result OR first durable postidentity remains unknown, never
same-image adoption or automatic replay. A separate trusted attended disposition
must bind the original checkpoint digest/sequence, operation/target/artifact and
independent settled-exclusive effective-state evidence plus named scope. Append
that disposition and preserve unknown history; do not invent native causal proof.
Still-in-flight or transient observations cannot authorize racing recovery.
Changed host/DB/schema/protected/routing state remains outside this app-only lane.
Actual [input and resume commands](../../scripts/app_release_fixture/README.md)
define the runnable custody boundary, including explicit partial/absent recovery.
Public disk Caddyfile stays immutable and need not be tmpfs; known secret routes,
payloads and tool scratch remain owner-only tmpfs/no-swap in production.

Recovery requires explicit named authority and a proven compatible image. It
changes only the selector/app while retaining the authoritative database and new
writes. Never delete ledger entries, weaken validation or restore an older
writable database after new writes. A backup is disaster recovery material, not
permission for destructive app rollback. Unknown compatibility/identity blocks
recovery; a failed acceptance does not automatically authorize it.

Clean only exact owned temporary resources and sessions. Retain checkpoints and
needed evidence securely; never expose credentials in argv, raw errors, receipts,
logs or tracked files. Reuse valid custody/evidence; authentication lapse alone is
not a reason to repeat completed gates.

## Acceptance before any production use

- Focused fail-closed tests cover the ordinary lane plus wrong target/current
  digest, tag-only image, complete manifest/ledger mismatch, absent approval or
  evidence, protected-field change, unsafe custody, concurrent release and missing
  native prerequisites. Test representative failures, not a generic checker platform.
- An owned disposable Compose fixture executes actual native adapters and the
  current -> candidate -> compatible recovery -> candidate sequence. Keep the
  exact proxy container and secret mounts intact, seeded post-change data readable
  and one active scheduler; exercise a real HTTP/authenticated app journey. A
  mocked command trace alone is insufficient. Label local evidence as local.
- Inject one representative lost reply locally and demonstrate reconciliation
  without duplicate committed effects, plus refusal when state cannot be proven.
- No shared local Compose project/database or user DB is a fixture. Allocate a
  unique project, owned volumes/database and nonconflicting loopback ports;
  standard preview is `127.0.0.1:17777` with `--no-open`. Record any alternative
  before use. Broad subprocesses remove optional PostgreSQL/browser-probe inputs.
- Build/test/vet/format gates, focused runner/syntax/Linux command-shape checks and
  owned rehearsal pass; independently review the exact candidate and evidence.
  Controller integration is local. Live target/auth semantics still require their
  separately authorized preflight; local mocks cannot prove provider permissions.
- Maintained operator docs state real invocation, custody, no-schema-change limit,
  approval, interruption/resume/recovery and known limitations. Do not document an
  unimplemented command as available.

## Non-goals

No new cloud topology, public cutover, concurrent blue/green schedulers, unattended
credential acquisition, migration controller, Gemini changes in the runner card,
paid acceptance or new inference retry loop. All such effects need separate scope.
