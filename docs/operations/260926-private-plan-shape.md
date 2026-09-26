# Private saved-plan structural inventory (D1)

This diagnostic does not validate a deployment or authorize any mutation. The
canonical Slice 4 checker remains unchanged and must consume the original plan
JSON and independent evidence. No real-derived inventory belongs in Git, CI,
Kanban, chat, screenshots, or shared logs. Even value-blind topology is private.

## Trust and authorization boundaries

Use only an attended owner-controlled workstation and independently reviewed
code/tool bindings. Root or a compromised owner account is outside this model.
The operator selects the already trusted Terraform executable and a dedicated,
preinitialized public tooling checkout containing AWS provider 6.33.0. Neither
the saved plan nor an inventory may select an executable. No automatic tool
installation or network fallback exists.

Implementation review is not authorization to read production artifacts. A
separate explicit private-read authorization is required before generating an
inventory from a real plan. Reviewers must regenerate from the same binary and
code bindings, not approve an editable fixture merely because it matches itself.
Any semantic-checker change requires a separate reviewed disposition; this tool
cannot edit predicates, create allowlists, replace plan JSON, or make a failed
canonical check pass.

The independently approved release stays distinct from checker-only commits.
The initial D1 command binds the checker to the reviewed `9963a4c` baseline and
the application release to `e8e1053`; full identifiers are checked in code. The
extractor and shape-contract commits must both equal the clean current HEAD.
Changing any member invalidates the previous generation's review binding.

## Closed inventory contract

`private_plan_shape.py` defines `jobcron-private-plan-shape-v1`. Its exact envelope
is `schema`, `mode`, `binding`, `sections`, `resources`, `nodes`, `relations`,
`shape_diff`, and `coverage`. Mode is always `diagnostic-only`.

The binding has exactly:

- `binary_plan_sha256` (full lowercase SHA-256 of the saved binary);
- `checker_commit`, `extractor_commit`, `shape_contract_commit`, and
  `approved_release_commit` (full lowercase Git commits);
- `terraform_version` = `1.15.8`, `terraform_json_format` = `1.2`;
- `aws_provider_version` = `6.33.0`; and
- `aws_schema_contract` = `aws-6.33.0-plan-1.2-v1`.

These provenance identifiers are exceptions, not permission to hash payload
values. Resource schema versions are independently pinned from the public AWS
provider schema, not learned from a candidate plan.

The fixture retains only reviewed literal logical addresses/types, action and
reason enums, public context-specific keys, JSON types, container cardinalities,
missing/present records, nested unknown/sensitive mask booleans, replacement-path
field tokens, and explicit configuration-reference descriptors. It preserves
array order. Approved keys are sorted; unknown keys use source-encounter ordinals.
Unknown names, map keys, resource addresses, references, and path steps never
escape as raw strings, hashes, or stable pseudonyms. They remain structural
records and mismatches, rather than disappearing from traversal.

Ordinary scalar values, scalar lengths, identifiers, addresses, policies,
user-data, diagnostics text, source timestamps, costs, image selectors, secrets,
encodings and payload fingerprints are not exported. Ordinary booleans and
numbers remain distinct types without exporting their values. Only mask booleans
have value-bearing tokens. A closed output-schema scan rejects additional fields
or arbitrary strings before publication; keyword-based secret detection is not
the privacy boundary.

### Shape-support obligations

The small code tables are conservative structural classifications, not acceptance
predicates. Fields without an explicit public dictionary remain unclassified,
even if Terraform understands them. All mismatches require independent review;
there is no `PASS` or automatic expected-fixture update.

| Rule/table | Independent source | Representative synthetic verification |
| --- | --- | --- |
| `ADDRESSES`, `required_resources`, `initial_actions` | Canonical checker address/action inventory | Missing inventory, forbidden action, duplicate identity |
| `CONTEXTS`, `CONTEXT_REQUIRED`, `REQUIRED` | Terraform JSON 1.2 container layout and canonical inputs | Missing/extra fields, wrong types, multiple distant mismatches |
| `ATTRS`, `REQUIRED_ATTRS`, `required_fields` | Explicit checker host/RDS/network structure | Missing security controls, collection cardinality, scalar types |
| `PROVIDER_SCHEMAS`, `provider_schema` | Offline public AWS 6.33.0 schema | Wrong provider/schema identity |
| `move_import`, replacement paths, mask records | Checker move/import rejection and structural masks | Opaque path canaries, nested true/false masks, malformed masks |
| Configuration references | Explicit configuration expressions only | Resolved public targets and unresolved opaque targets |

The walker visits all decoded sections, including unsupported sections and
subtrees, in one traversal. Each node belongs to exactly one classified,
opaque-leaf, or unsupported partition. Missing expected obligations are recorded
as well as present mismatches. The output validator checks node IDs, parent/child
conservation, collection counts, partitions, and closed record fields. Limits
are 32 MiB per input, 200,000 visited nodes, depth 80, and ten seconds of traversal;
exceeding a limit fails rather than publishing a truncated complete report.

`coverage.complete` means complete **binary-document traversal**, not semantic
validation or complete evidence coverage. `coverage.source_companions` is always
`missing`: raw Terraform state and the five independent evidence interfaces are
not authenticated by the binary digest. This is a blocker to combined evidence-
interface completeness. Separate source-companion access requires separate
private authorization; never infer the raw-state sensitivity path from a plan.

Identically shaped safe and unsafe CIDRs, IAM policies, AMIs, bootstrap contents,
RDS controls or source identities can have identical structural output. The
unchanged canonical checker and private semantic review still bind those values
to original state, live host/RDS/address observations, reconstructed inputs,
reviewed bootstrap bytes and release evidence. Shape comparison proves none of
those facts.

## Generation and review-status commands

After the separate authorization, put the reviewed binding JSON and existing
binary in owner-owned 0600 regular files beneath owner-only 0700 directories.
Select a trusted private output root. Path components may not be symlinks or
writable by another user; extended ACL access grants are rejected. Hard-linked,
nonregular and changed source files fail. Use an isolated trusted Python 3:

```sh
python3 -I -B scripts/private_plan_shape_cli.py inventory \
  "$PRIVATE_BINARY" "$REVIEWED_BINDING_JSON" "$PRIVATE_ROOT" \
  "$NEW_GENERATION_NAME" "$TRUSTED_TERRAFORM" "$OFFLINE_TOOLING_DIRECTORY"
```

The decoder currently supports the reviewed macOS sandbox path only. It copies
through checked source descriptors, binds the expected binary digest, runs only
`version -json` and `show -json` on the staged binary, clears the child environment,
disables checkpoints/metadata lookup, denies Internet sockets, and permits local
Unix-socket plugin IPC. It rechecks source identity and content after decoding.
Raw stderr is discarded. Output is bounded, private, strictly parsed, and never
forwarded to shared stdout. Unsupported platform/tool/format fails closed.

Publication uses a directory-descriptor lock, owner-only staging, file and
directory fsync, no-clobber atomic directory rename, and readback. Existing
artifacts are never overwritten. Failed stages are private recovery evidence,
not complete inventories; do not automatically reuse or share them. Successful
generation contains `manifest.json` and reports only
`inventory_complete_unreviewed`. That outcome never creates or activates a
register entry. Review every unsupported shape in one batch before requesting
any semantic changes.

The operator-selected `TF_PLAN_SHAPE_REVIEW_REGISTER_JSON` must be the current,
non-rolled-back register under attended custody. This is a mandatory trust
precondition, not something a stateless consumer can prove:

```sh
TF_PLAN_SHAPE_REVIEW_REGISTER_JSON="$AUTHORITATIVE_PRIVATE_REGISTER" \
  python3 -I -B scripts/private_plan_shape_cli.py review-status \
  "$PRIVATE_GENERATION" "$REVIEWED_BINDING_JSON"
```

Only an exact latest `approved` record permits the diagnostic review-status
result. The result explicitly includes `rollback_detection_unavailable`.
Replaying an older otherwise valid register snapshot is outside the stateless
guarantee. No monotonic-head service, caller-supplied anti-rollback hash, or claim
of rollback detection is introduced. Missing records, changed tuples/manifests,
retirement/invalidation, unsafe files and unknown schemas fail closed.

## Isolated operator register writer

`private_plan_shape_review.py` is read-only. The extractor and review-status
command never import or invoke the separate writer. Only `default-orchestrator`
may invoke `private_plan_shape_register.py`, after documented independent
`reviewer-sol`/human disposition. Role enums record this attended authority;
they are not an authentication mechanism against an owner-account compromise.
Do not run the helper as part of generation or automated approval.

The closed register schema is `jobcron-private-plan-shape-review-register-v1`,
with only `schema` and `events`. Each event contains `sequence`, `status`, the
exact `binding`, `manifest_sha256`, `reviewer`, `writer`, `timestamp`, and `reason`.
Sequence numbers strictly increase; latest status is selected per exact tuple
and manifest digest. The manifest digest is expressly permitted because the
manifest is already value-blind. Timestamps are RFC3339 operator audit events,
never copied plan timestamps. No free text or extra metadata is accepted.

Statuses/reasons are closed:

- `approved`: `independent_review`;
- `retired`: `superseded`, `abandoned`;
- `invalidated`: `freshness_failed`, `custody_lost`, `parser_defect`,
  `regeneration_failed`, `incomplete_coverage`, `source_changed`.

The operator command takes the expected last global sequence (zero for a new
register), rejects stale concurrent writers, preserves all previous events, and
atomically replaces the owner-only whole file under the directory lock:

```sh
python3 -I -B scripts/private_plan_shape_register.py \
  operator-disposition-confirmed "$AUTHORITATIVE_PRIVATE_REGISTER" \
  "$PRIVATE_MANIFEST" "$EXPECTED_SEQUENCE" "$STATUS" "$REASON" \
  reviewer-sol default-orchestrator "$AUDIT_TIMESTAMP"
```

These commands document the interface; they do not authorize real register writes.
Synthetic tests alone exercise writer behavior during implementation.

## Freshness and downstream checklist

Before using a generation, independently confirm exact binary/code/tool/release
bindings, deterministic regeneration, current-register custody, latest approval,
and complete review of mismatches. Before apply, also perform attended fresh
state/live reconciliation and every canonical checkpoint/cost/release gate.
Historical structural reproducibility never refreshes observations or approvals.

A changed tuple, source evidence, lost custody, parser/redaction defect,
regeneration mismatch, incomplete coverage, supersession or freshness failure
requires explicit retirement/invalidation and human disposition. Preserve the
old immutable generation as private audit evidence. Never edit timestamps or
bindings to revive it. If a new plan is required, obtain separate authorization,
generate new bound evidence, repeat independent review, and obtain separate apply
approval. Private acceptance, recovery proof before public writes, and separate
attended public cutover approval remain mandatory.

## Offline verification

Run `PYTHONDONTWRITEBYTECODE=1 python3 scripts/private_plan_shape_test.py` on the
reviewed macOS workstation with Terraform 1.15.8 and an already-installed AWS
6.33.0 plugin. `D1_TEST_PROVIDER_DIR` can select the read-only local provider
installation; the default is the production checkout's installed provider
**binaries**, not its state or private inputs. The test creates a separate
synthetic local configuration and saved binary, using fake fixture credentials
and network-denied init/plan/show. Missing tools/plugins fail rather than skip
real-binary verification or download dependencies. No apply runs.

Tests cover deterministic/value-blind shape pairs and canaries, malformed input,
missing/extra structure, masks/references/schema metadata, closed output/register
schemas, node/depth/time/byte budgets, wrong digest, real binary mutation,
mode/ACL/symlink/hardlink/FIFO/path substitution, atomic publication and collision,
lifecycle invalidation/retirement, stale sequence and register-snapshot replay
limitations. Canonical initial/replacement/combined regression suites remain a
separate required check. Real private regeneration and independent private
semantic review remain downstream and are not proved by synthetic tests.
