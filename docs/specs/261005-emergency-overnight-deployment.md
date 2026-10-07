# Emergency Overnight Alpha Deployment

**Status:** Owner-approved for autonomous execution on 2026-10-05. The emergency scope controls this launch; exact operational plans still require independent review and fresh per-operation envelopes.
**Deadline:** 2026-10-06 06:00 KST. **Operator handoff:** 2026-10-05 22:40 KST.
**Success:** A usable invite-only app at https://jobcron.app, not a plan, image or running container.

## Scope and authority

The owner's approval replaces the attended sequencing in [the alpha spec](260923-human-assisted-alpha-deployment.md) for this launch. Keep one existing-size EC2 host, existing private RDS, Caddy, one app/scheduler, access-code signup and an immutable private arm64 image. No architecture/product redesign.

Settle one owner-approved scope before sleep: exact account/region/resources, existing cost ceiling (or one explicit ceiling), release-selection rule, resource start, locked planning, conditional private apply, migration/role setup, secret publication, runtime start, conditional public cutover and rollback. Explicitly include EIP association, apex/`www` DNS, Cloudflare-origin HTTPS ingress and strict TLS. This changes the old requirement for later attended apply/cutover approvals; it is not inferred from this draft. No repeat permission requests for covered operations.

The inherited cost envelope is **USD 85/month upper bound and USD 150 one-time**, including reserves, researched **2026-09-30**. This is inherited evidence, not current pricing or approval of a particular plan. The controller must rebind it to the selected resources and aggregate costs before any paid resource expansion; preserve existing sizing and do not invent a fresh price or plan approval.

Use only a fresh independently reviewed saved plan within that footprint. Reuse the host if viable; replace only the proven disposable managed bootstrap if necessary; create at most one missing origin EIP. Retain RDS and existing sizing/network/IAM bindings. No unrelated destruction, public database/SSH/app port, force push, repository-rule bypass, IAM expansion or unapproved paid service. Secret versions stay outside Terraform.

Controller owns live operations. Local repairs use board `jobcron`, workers `jobcron-worker`, `jobcron-worker-glm-1` or `jobcron-worker-glm-2`, isolated worktrees and same-card `jobcron-reviewer` review. Pin repository/profile/worktree/baseline/checks/completion contract on dispatch. Workers get no live authority or credentials. Local previews: loopback `17777`, `--no-open`.

## Cut the fat

- Reuse unaffected accepted evidence and the existing green, published image. Documentation changes do not require republishing the app.
- No new checker platform, structural-inventory pipeline, launcher generations, recursive attestation, blanket re-audits, exhaustive fault injection, HA, unnecessary local-data import, or unrelated refactoring/cleanup.
- Use existing commands/runtime scripts. Resolve checker representation mismatches by direct independent review of the complete real plan and a recorded disposition, not falsified evidence or generalized checker development. Keep mandatory hard stops.
- With explicit risk acceptance, defer effective TEMP removal and exact global `search_path` conformance until **2026-10-08 06:00 KST**. Still require safe effective object resolution, no untrusted schema writers and a restricted runtime role. Let ordinary post-migration role setup apply straightforward corrections; do not restart the separate hardening campaign.
- Worknet, paid-AI acceptance, broad observability and exhaustive recovery matrices are deferred. No new secret-manager plugin.

## Critical path

1. **By 22:40: eliminate human dependencies.** Settle scope/cost/risk once; verify credential permissions/expiry, actual headless secret delivery, narrow command approvals and durable continuation. Keep Mac/gateway powered, awake and online; Docker available where required. Saved logins are not proof of unattended readiness.
2. **Target 00:00: release and reconciliation.** Bind clean release SHA, genuinely complete green CI, private digest and arm64 platform. Reuse unchanged evidence. Reconcile selected cloud/state/DNS targets and data once before dependent changes. Generate/review fresh plans only where needed.
3. **Target 02:00: private deployment.** Start/apply scoped resources, migrate, provision runtime role/owner, publish the existing runtime secret, pull the pinned image, start systemd/Compose. Keep public routing off. Existing-data changes require a recovery point; a proven empty unused DB needs no empty snapshot.
4. **Target 03:00: acceptance and recovery.** Run acceptance below. Produce one encrypted off-host dump, checksum it, restore into a disposable DB and compare schema/representative counts. Back up the credential-encryption key separately. No exhaustive log/backup matrix.
5. **Target 04:00: automatic cutover.** After private acceptance and restore PASS, use the upfront conditional grant for approved EIP/ingress, proxied apex/`www`, Full (strict) TLS. Read back changes; run public browser acceptance.
6. **By 05:30: stabilize; by 06:00: deliver.** Observe at least 30 minutes, verify service/restart state and a post-cutover backup. Report URL, release/digest, real acceptance/restore results and exceptions. Earlier targets trigger reprioritization, not campaign abandonment.

## Minimum verification and recovery

Require valid public HTTPS; owner login/logout and rejected logged-out session; access-code rejection; profile save/read; representative scrape/scoring with visible real postings; persistence after one controlled restart; one scheduler; private verified-TLS RDS using the non-admin role; no public DB/SSH/app/admin listener; successful off-host restore. Exercise the real UI, not just `/health`. Repeat only affected paths after fixes.

Unchanged code reuses complete green CI. A launch-blocking code fix requires independent review, focused regression and required build/test/vet/gofmt/UI gates, then exact-head CI and a rebound image. Authorize push/PR/publication contingencies upfront if wanted; never bypass repository protections.

Continue after card completion and ordinary failures. Diagnose the failed operation, make the smallest reviewed repair and retry with bounded backoff only after prior effects are known. Never blindly repeat ambiguous writes or reuse consumed/expired packets. Cosmetic/redundant-proof findings cannot open another critical-path project. The deadline is not automatic campaign revocation; completion or explicit owner revocation ends the campaign.

Before public writes, keep failures isolated. After public writes, preserve RDS as authoritative; close new exposure/stop unsafe runtime, retain backups, fix forward or use a verified schema-compatible image. Never restore a stale writable DB. Safety/capability failure is a missed deadline, not permission to fabricate success.

## Complete unattended-access checklist

Reuse existing credentials; supply missing ones **locally, never in Telegram**. Everything must work without approval/touch/unlock after handoff and remain usable through **06:30 KST**.

- **AWS:** Selected account/role/region; refresh-capable CLI SSO authenticated before sleep with verified session horizon, or scoped access-key ID/secret plus session token when temporary. Permissions: approved EC2/EIP/network operations, SSM, RDS start/describe, existing Secrets Manager/KMS, Terraform backend state/lock and recovery bucket. Host instance role must already support runtime secrets/backups. No root credential or broad IAM expansion.
- **GitHub API:** Existing authenticated CLI or repository-scoped token for commit/CI reads; Actions dispatch if publishing. Conditional repairs need Contents write and Pull requests write only where rules require PRs, with upfront write authority. Resolve organization SSO/token approval and workflow-environment approval before sleep.
- **Private GHCR:** GitHub username plus PAT **classic**, `read:packages`, access to the selected private image, and organization SSO authorization if applicable. Existing Actions `GITHUB_TOKEN` handles workflow publication; no manual package-write/delete token needed. Delete host login/token after pull; retain the pinned local image for reboot.
- **Cloudflare:** Existing-zone API token: Zone read, DNS edit, Zone Settings edit for strict TLS if needed; SSL and Certificates edit if issuing Origin CA. Existing valid certificate/private key can replace issuance permission. Change only approved records/settings. Registrar credentials/nameserver changes are outside scope.
- **RDS administrator:** Selected DB/admin identity and password, or authorized existing managed-password retrieval. Migration/bootstrap only; never app runtime credentials. Reuse runtime-role password, or authorize its creation/rotation if unavailable. No reset of credentials used by an existing service.
- **App owner:** Selected owner email/password for bootstrap and acceptance; authorize generation/private retention of a strong password if absent. No phone/email verification dependency.

**Generate/reuse privately, not operator chores:** runtime-role password, `SESSION_SECRET`, `JOBCRON_CREDENTIAL_ENCRYPTION_KEY`, `JOBCRON_SIGNUP_ACCESS_CODE`, `JOBCRON_PROXY_SECRET`, Origin CA key/certificate. Preserve existing encryption keys/valid secrets. Exact runtime-secret keys: `DATABASE_URL`, `SESSION_SECRET`, `JOBCRON_CREDENTIAL_ENCRYPTION_KEY`, `JOBCRON_SIGNUP_ACCESS_CODE`, `JOBCRON_PROXY_SECRET`, `ORIGIN_CA_CERT`, `ORIGIN_CA_KEY`, `JOBCRON_IMAGE`, `JOBCRON_STAGE1_SPONSOR_USER_ID`. Image/sponsor ID are bindings; derive sponsor ID from the actual owner row. No Worknet/admin/paid-AI token required.

**Storage:** Only `jobcron-orchestrator` credential configuration. Hermes' encrypted browser vault handles website passwords/saved TOTP, not native CLI injection. Native credentials use existing profile secret sources or an explicitly accepted local `0600` profile `.env` (**plaintext**, not an encrypted vault). Forward only declared controller child credentials, never workers. Use existing `_FILE` inputs and tmpfs runtime secrets. No values in model output/logs/argv/Git/Terraform. Prove actual headless retrieval before handoff.

**Browser fallback:** AWS identity-provider/GitHub/Cloudflare exact-origin logins plus saved TOTP seeds where applicable. SMS/email codes, hardware keys, passkeys, push approvals or expired SSO can still need a human. Native API access is therefore the critical path; do not disable MFA or depend on overnight browser re-login.

## Before-sleep receipt

One short receipt: explicit scope/cost/conditional apply/cutover approval, accepted exceptions, credential/permission/expiry PASS, real native-delivery PASS, narrow command approvals and durable-continuation PASS. No blanket approval-mode disablement. Use the existing Jobcron board, not a recreated diagnostic graph. Card completion is a checkpoint, not deployment completion.

The owner approved this specification's scope/cost and autonomous execution on 2026-10-05. Credential availability is recorded separately; approval does not turn unexecuted checks into PASS. Fresh exact operational plans and code changes require independent review. Only the public app satisfying the acceptance criteria counts as deployment success.
