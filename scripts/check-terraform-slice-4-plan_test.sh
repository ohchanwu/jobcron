#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_checker="$repo_root/scripts/check-terraform-slice-4-plan.sh"
ci_workflow="$repo_root/.github/workflows/ci.yml"
fixture_root="$(mktemp -d)"
trap 'rm -rf "$fixture_root"' EXIT

checked_repo="$fixture_root/checked-repo"
mkdir -p "$checked_repo/scripts" \
  "$checked_repo/deploy/production/systemd"
cp "$source_checker" "$checked_repo/scripts/check-terraform-slice-4-plan.sh"
for asset in \
  deploy/production/compose.yaml \
  deploy/production/Caddyfile \
  deploy/production/jobcron-runtime.sh \
  deploy/production/systemd/jobcron.service \
  deploy/production/systemd/jobcron-recovery.service \
  deploy/production/systemd/jobcron-recovery.timer; do
  cp "$repo_root/$asset" "$checked_repo/$asset"
done
git -C "$checked_repo" init -q
git -C "$checked_repo" add .
git -C "$checked_repo" \
  -c user.name='Slice 4 test' -c user.email='slice4@example.invalid' \
  commit -qm 'reviewed fixture checkout'
checker="$checked_repo/scripts/check-terraform-slice-4-plan.sh"
reviewed_sha="$(git -C "$checked_repo" rev-parse HEAD)"

grep -Fqx \
  '        run: bash scripts/check-terraform-slice-4-plan_test.sh' \
  "$ci_workflow"

failures=0
generic_error="Terraform saved plan violates the Slice 4 contract"
expected_output='resource_changes=5
output_changes=1
sensitive_outputs=1
destroy_or_replace=0
aggregate_cost=PASS
slice3_checkpoint=PASS
PASS'
expected_replacement_output='resource_changes=1
output_changes=1
sensitive_outputs=1
destroy_or_replace=1
aggregate_cost=PASS
current_reconciliation_checkpoint=PASS
PASS'
expected_combined_recovery_output='resource_changes=2
output_changes=1
sensitive_outputs=1
destroy_or_replace=1
aggregate_cost=PASS
current_reconciliation_checkpoint=PASS
PASS'

expect_verified() {
  local name="$1"
  local plan="$2"
  local cost="$3"
  local checkpoint="$4"
  local output

  if ! output="$("$checker" "$plan" "$cost" "$checkpoint" 2>&1)"; then
    printf 'FAIL: rejected valid %s fixture\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  if [[ "$output" != "$expected_output" ]]; then
    printf 'FAIL: valid %s fixture emitted unexpected output\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  printf 'PASS: verified %s fixture\n' "$name"
}

expect_rejected() {
  local name="$1"
  local plan="$2"
  local cost="$3"
  local checkpoint="$4"
  local output
  local rc

  set +e
  output="$("$checker" "$plan" "$cost" "$checkpoint" 2>&1)"
  rc=$?
  set -e

  if [[ "$rc" -eq 0 ]]; then
    printf 'FAIL: accepted %s\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  if [[ "$output" != "$generic_error" ]]; then
    printf 'FAIL: %s disclosed input or emitted a non-generic error\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  printf 'PASS: rejected %s without private output\n' "$name"
}

expect_replacement_verified() {
  local name="$1"
  local plan="$2"
  local cost="$3"
  local checkpoint="$4"
  local user_data="$5"
  local output

  if ! output="$("$checker" "$plan" "$cost" "$checkpoint" "$user_data" \
    "$reviewed_sha" 2>&1)"; then
    printf 'FAIL: rejected valid %s fixture\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  if [[ "$output" != "$expected_replacement_output" ]]; then
    printf 'FAIL: valid %s fixture emitted unexpected output\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  printf 'PASS: verified %s fixture\n' "$name"
}

expect_replacement_verified_with_noisy_failed_stat_probe() {
  local stat_bin="$fixture_root/noisy-stat-bin"
  local output

  mkdir -p "$stat_bin"
  cat >"$stat_bin/stat" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == -f ]]; then
  printf 'synthetic GNU stat filesystem output\n'
  exit 1
fi
if [[ "$1" == -c && "$2" == '%a' ]]; then
  printf '600\n'
  exit 0
fi
exit 2
EOF
  chmod +x "$stat_bin/stat"

  if ! output="$(PATH="$stat_bin:$PATH" \
    "$checker" \
    "$fixture_root/plan-replacement-valid.json" \
    "$fixture_root/cost-valid.json" \
    "$fixture_root/current-checkpoint-replacement-valid.json" \
    "$fixture_root/replacement-user-data" \
    "$reviewed_sha" 2>&1)"; then
    printf 'FAIL: failed stat probe stdout contaminated file mode\n' >&2
    failures=$((failures + 1))
    return
  fi
  if [[ "$output" != "$expected_replacement_output" ]]; then
    printf 'FAIL: noisy failed stat probe emitted unexpected output\n' >&2
    failures=$((failures + 1))
    return
  fi
  printf 'PASS: ignored stdout from failed stat probe\n'
}

expect_replacement_rejected() {
  local name="$1"
  local plan="$2"
  local user_data="${3:-$fixture_root/replacement-user-data}"
  local output
  local rc

  set +e
  output="$("$checker" \
    "$plan" \
    "$fixture_root/cost-valid.json" \
    "$fixture_root/current-checkpoint-replacement-valid.json" \
    "$user_data" \
    "$reviewed_sha" 2>&1)"
  rc=$?
  set -e

  if [[ "$rc" -eq 0 ]]; then
    printf 'FAIL: accepted %s\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  if [[ "$output" != "$generic_error" ]]; then
    printf 'FAIL: %s disclosed input or emitted a non-generic error\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  printf 'PASS: rejected %s without private output\n' "$name"
}

expect_combined_recovery_verified() {
  local output

  if ! output="$("$checker" \
    "$fixture_root/plan-combined-recovery-valid.json" \
    "$fixture_root/cost-valid.json" \
    "$fixture_root/current-checkpoint-combined-valid.json" \
    "$fixture_root/replacement-user-data" \
    "$reviewed_sha" \
    combined-recovery 2>&1)"; then
    printf 'FAIL: rejected valid combined recovery fixture\n' >&2
    failures=$((failures + 1))
    return
  fi
  if [[ "$output" != "$expected_combined_recovery_output" ]]; then
    printf 'FAIL: valid combined recovery fixture emitted unexpected output\n' >&2
    failures=$((failures + 1))
    return
  fi
  printf 'PASS: verified exact combined EIP recovery and host replacement fixture\n'
}

expect_combined_recovery_rejected() {
  local name="$1"
  local plan="$2"
  local mode="${3:-combined-recovery}"
  local output
  local rc

  set +e
  output="$("$checker" \
    "$plan" \
    "$fixture_root/cost-valid.json" \
    "$fixture_root/current-checkpoint-combined-valid.json" \
    "$fixture_root/replacement-user-data" \
    "$reviewed_sha" \
    "$mode" 2>&1)"
  rc=$?
  set -e

  if [[ "$rc" -eq 0 ]]; then
    printf 'FAIL: accepted %s\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  if [[ "$output" != "$generic_error" ]]; then
    printf 'FAIL: %s disclosed input or emitted a non-generic error\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  printf 'PASS: rejected %s without private output\n' "$name"
}

expect_recovery_invocation_rejected() {
  local name="$1"
  shift
  local output
  local rc

  set +e
  output="$("$checker" "$@" 2>&1)"
  rc=$?
  set -e

  if [[ "$rc" -eq 0 ]]; then
    printf 'FAIL: accepted %s\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  if [[ "$output" != "$generic_error" ]]; then
    printf 'FAIL: %s disclosed input or emitted a non-generic error\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  printf 'PASS: rejected %s without private output\n' "$name"
}

now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

jq -n '{
  format_version: "1.2",
  resource_changes: ((
    [
      "aws_iam_role.replacement_host",
      "aws_iam_role_policy_attachment.replacement_host_ssm",
      "aws_iam_role_policy.replacement_host_runtime",
      "aws_iam_instance_profile.replacement_host",
      "aws_instance.replacement_host"
    ] |
    map({
      address: .,
      previous_address: null,
      change: {
        actions: ["create"],
        importing: null,
        before: null,
        after: {synthetic: true}
      }
    })
  ) + (
    [
      "aws_vpc.canonical",
      "aws_internet_gateway.canonical",
      "aws_subnet.public[\"public_a\"]",
      "aws_subnet.public[\"public_b\"]",
      "aws_subnet.public[\"public_c\"]",
      "aws_subnet.public[\"public_d\"]",
      "aws_route_table.public",
      "aws_route.public_ipv4_default",
      "aws_eip.origin",
      "aws_subnet.database[\"database_a\"]",
      "aws_subnet.database[\"database_b\"]",
      "aws_route_table.database",
      "aws_route_table_association.database[\"database_a\"]",
      "aws_route_table_association.database[\"database_b\"]",
      "aws_security_group.origin",
      "aws_security_group.database",
      "aws_vpc_security_group_ingress_rule.database_postgresql_from_origin",
      "aws_db_subnet_group.production",
      "aws_db_parameter_group.production",
      "aws_db_instance.production",
      "aws_secretsmanager_secret.runtime",
      "aws_s3_bucket.recovery",
      "aws_s3_bucket_public_access_block.recovery",
      "aws_s3_bucket_versioning.recovery",
      "aws_s3_bucket_server_side_encryption_configuration.recovery",
      "aws_s3_bucket_policy.recovery",
      "aws_s3_bucket_lifecycle_configuration.recovery"
    ] |
    map({
      address: .,
      previous_address: null,
      change: {
        actions: ["no-op"],
        importing: null,
        before: {synthetic: true},
        after: {synthetic: true}
      }
    })
  )),
  output_changes: {
    replacement_instance_id: {
      actions: ["create"],
      before: null,
      after: "synthetic-sensitive-selector",
      after_sensitive: true
    }
  },
  diagnostics: []
}' >"$fixture_root/plan-valid.json"

cat >"$fixture_root/replacement-user-data" <<'EOF'
#!/bin/bash
/opt/jobcron/compose.yaml 28470a096ae9430633174abf7b631c0b41540dfed9003a07a91280a8c703d5a8
/opt/jobcron/Caddyfile 3b52c1f296b4fa709ae4ce50aced61f23a4d1d00ff7939d5f6b0f8a193c863f6
/opt/jobcron/jobcron-runtime.sh 854a72aadb5dfa5717956131cb6db37430a7fba49beafac92010db977cbbe91f
/etc/systemd/system/jobcron.service f450f85dd50c75250b0c5ae4c40cbcc61da8453356b3b89cec8abe9c647e10dd
/etc/systemd/system/jobcron-recovery.service f1ead8f00c5cdf8dab3b1dd2564b3e20956fa38f388fe82016098383ea3e361c
/etc/systemd/system/jobcron-recovery.timer 4b9831517333fcb689dd40e7db55faf1773495bc85f54612e0ab8db6e75a834c
docker-compose-linux-aarch64 ff42489f5a9b879d5d117c5ffea6defc27390b3286da8ad52cbc9c6ab5df590e
sha256sum -c -
/etc/jobcron/runtime-secret-id
systemctl enable docker.service
systemctl start docker.service
systemctl stop jobcron.service
EOF
chmod 0600 "$fixture_root/replacement-user-data"

# terraform console prints the jsonencode result as a quoted Terraform string,
# so recovering the raw bootstrap requires decoding both string layers.
jq -Rs '@json' "$fixture_root/replacement-user-data" \
  >"$fixture_root/replacement-user-data.console"
jq -ejr '
  fromjson |
  if type == "string" and length > 0 then . else error("invalid") end
' "$fixture_root/replacement-user-data.console" \
  >"$fixture_root/replacement-user-data.decoded"
if ! cmp -s \
  "$fixture_root/replacement-user-data" \
  "$fixture_root/replacement-user-data.decoded"; then
  printf 'FAIL: Terraform console output did not decode to raw user-data bytes\n' >&2
  failures=$((failures + 1))
else
  printf 'PASS: decoded Terraform console output to raw user-data bytes\n'
fi

if command -v sha1sum >/dev/null 2>&1; then
  replacement_user_data_hash="$(sha1sum "$fixture_root/replacement-user-data" | awk '{print $1}')"
else
  replacement_user_data_hash="$(shasum "$fixture_root/replacement-user-data" | awk '{print $1}')"
fi

jq --arg user_data_hash "$replacement_user_data_hash" '
  .resource_changes |= map(
    if .address == "aws_instance.replacement_host" then
      .action_reason = "replace_by_request" |
      .change = {
        actions: ["delete", "create"],
        importing: null,
        before: {
          ami: "ami-reviewed-arm64",
          instance_type: "t4g.micro",
          subnet_id: "subnet-reviewed-public",
          iam_instance_profile: "jobcron-replacement-host",
          key_name: null,
          associate_public_ip_address: true,
          vpc_security_group_ids: ["sg-origin"],
          user_data: "old-user-data-hash"
        },
        after: {
          ami: "ami-reviewed-arm64",
          instance_type: "t4g.micro",
          subnet_id: "subnet-reviewed-public",
          iam_instance_profile: "jobcron-replacement-host",
          key_name: null,
          associate_public_ip_address: true,
          vpc_security_group_ids: ["sg-origin"],
          metadata_options: [{
            http_endpoint: "enabled",
            http_tokens: "required",
            http_put_response_hop_limit: 1
          }],
          root_block_device: [{
            encrypted: true,
            volume_type: "gp3",
            volume_size: 8,
            delete_on_termination: true
          }],
          user_data: $user_data_hash
        },
        after_unknown: {
          id: true,
          arn: true,
          public_dns: true,
          public_ip: true
        }
      }
    elif .change.actions == ["create"] then
      .change = {
        actions: ["no-op"],
        importing: null,
        before: {synthetic: true},
        after: {synthetic: true},
        after_unknown: {}
      }
    else
      .change.after_unknown = {}
    end
  ) |
  .output_changes.replacement_instance_id = {
    actions: ["update"],
    before: "old-sensitive-selector",
    after: null,
    after_unknown: true,
    before_sensitive: true,
    after_sensitive: true
  }
' "$fixture_root/plan-valid.json" >"$fixture_root/plan-replacement-valid.json"

jq '
  .resource_changes |= map(
    if .address == "aws_eip.origin" then
      .change = {
        actions: ["create"],
        importing: null,
        before: null,
        after: {
          domain: "vpc",
          instance: null,
          network_interface: null,
          associate_with_private_ip: null
        },
        after_unknown: {
          id: true,
          allocation_id: true,
          public_ip: true
        }
      }
    else
      .
    end
  )
' "$fixture_root/plan-replacement-valid.json" \
  >"$fixture_root/plan-combined-recovery-valid.json"

jq -n --arg checked_at "$now" '{
  checked_at: $checked_at,
  currency: "USD",
  aggregate: {
    recurring_monthly_upper_bound: 98,
    one_time_upper_bound: 190
  },
  categories: [
    {
      name: "aws_compute",
      source: "AWS pricing",
      quantity: 1,
      recurring_monthly_upper_bound: 20,
      one_time_upper_bound: 10
    },
    {
      name: "public_ipv4",
      source: "AWS pricing",
      quantity: 2,
      recurring_monthly_upper_bound: 10,
      one_time_upper_bound: 0
    },
    {
      name: "database",
      source: "AWS pricing",
      quantity: 1,
      recurring_monthly_upper_bound: 30,
      one_time_upper_bound: 0
    },
    {
      name: "storage",
      source: "AWS pricing",
      quantity: 1,
      recurring_monthly_upper_bound: 8,
      one_time_upper_bound: 10
    },
    {
      name: "backup",
      source: "AWS pricing",
      quantity: 1,
      recurring_monthly_upper_bound: 10,
      one_time_upper_bound: 20
    },
    {
      name: "registry",
      source: "GitHub pricing",
      quantity: 1,
      recurring_monthly_upper_bound: 10,
      one_time_upper_bound: 50
    },
    {
      name: "cloudflare",
      source: "Cloudflare pricing",
      quantity: 1,
      recurring_monthly_upper_bound: 10,
      one_time_upper_bound: 100
    }
  ]
}' >"$fixture_root/cost-valid.json"

jq -n --arg checked_at "$now" '{
  checked_at: $checked_at,
  commit: "0123456789abcdef0123456789abcdef01234567",
  verdict: "PASS",
  post_apply_plan: "clean",
  addresses: [
    "aws_db_instance.production",
    "aws_security_group.origin",
    "aws_security_group.database",
    "aws_vpc_security_group_ingress_rule.database_postgresql_from_origin",
    "aws_secretsmanager_secret.runtime",
    "aws_s3_bucket.recovery",
    "aws_s3_bucket_lifecycle_configuration.recovery"
  ],
  private_rds: true,
  runtime_secret_versions: 0,
  recovery_bucket: {
    encrypted: true,
    versioned: true,
    public_access_blocked: true,
    verified_retention_days: 14,
    unverified_retention_days: 90
  },
  recovery_lifecycle_verdict: "PASS",
  destroy_or_replace: 0,
  old_resource_changes: 0
}' >"$fixture_root/checkpoint-valid.json"

jq -n --arg checked_at "$now" --arg reviewed_sha "$reviewed_sha" '{
  schema_version: "human-assisted-reconciliation-v1",
  checked_at: $checked_at,
  commit: {
    sha: $reviewed_sha,
    exact: true,
    clean: true
  },
  selected_state_resources: {
    bootstrap_host: {
      terraform_address: "aws_instance.replacement_host",
      managed: true
    },
    database: {
      terraform_address: "aws_db_instance.production",
      managed: true
    }
  },
  bootstrap_host: {
    disposition: "replace",
    human_approved: true
  },
  legacy_rollback_host: {retained: true},
  managed_eip: {
    terraform_address: "aws_eip.origin",
    state_presence: "present",
    unattached: true
  },
  origin: {
    ingress_rule_count: 0,
    phase: "private"
  },
  rds: {
    status: "available",
    publicly_accessible: false,
    storage_encrypted: true,
    deletion_protection: true,
    backup_retention_days: 7,
    latest_restorable_time_observed: true
  },
  runtime_secret: {
    container_exists: true,
    terraform_manages_versions: false,
    observed_version_count: 1
  },
  recovery_bucket: {
    encrypted: true,
    versioned: true,
    public_access_blocked: true,
    tls_only: true,
    lifecycle_verified: true
  },
  state_backend: {
    remote: true,
    encrypted: true,
    lockfile_enabled: true
  },
  public_cutover: {
    approved: false,
    performed: false
  }
}' >"$fixture_root/current-checkpoint-replacement-valid.json"

jq '.managed_eip.state_presence = "absent"' \
  "$fixture_root/current-checkpoint-replacement-valid.json" \
  >"$fixture_root/current-checkpoint-combined-valid.json"

plan_mutation() {
  local name="$1"
  local filter="$2"

  jq "$filter" "$fixture_root/plan-valid.json" \
    >"$fixture_root/plan-$name.json"
  expect_rejected \
    "$name" \
    "$fixture_root/plan-$name.json" \
    "$fixture_root/cost-valid.json" \
    "$fixture_root/checkpoint-valid.json"
}

cost_mutation() {
  local name="$1"
  local filter="$2"

  jq "$filter" "$fixture_root/cost-valid.json" \
    >"$fixture_root/cost-$name.json"
  expect_rejected \
    "$name" \
    "$fixture_root/plan-valid.json" \
    "$fixture_root/cost-$name.json" \
    "$fixture_root/checkpoint-valid.json"
}

checkpoint_mutation() {
  local name="$1"
  local filter="$2"

  jq "$filter" "$fixture_root/checkpoint-valid.json" \
    >"$fixture_root/checkpoint-$name.json"
  expect_rejected \
    "$name" \
    "$fixture_root/plan-valid.json" \
    "$fixture_root/cost-valid.json" \
    "$fixture_root/checkpoint-$name.json"
}

current_checkpoint_mutation() {
  local name="$1"
  local filter="$2"
  local mode="${3:-replacement}"
  local source="$fixture_root/current-checkpoint-replacement-valid.json"
  local plan="$fixture_root/plan-replacement-valid.json"
  local output
  local rc

  if [[ "$mode" == combined-recovery ]]; then
    source="$fixture_root/current-checkpoint-combined-valid.json"
    plan="$fixture_root/plan-combined-recovery-valid.json"
  fi
  jq "$filter" "$source" >"$fixture_root/current-checkpoint-$mode-$name.json"

  set +e
  if [[ "$mode" == combined-recovery ]]; then
    output="$("$checker" "$plan" "$fixture_root/cost-valid.json" \
      "$fixture_root/current-checkpoint-$mode-$name.json" \
      "$fixture_root/replacement-user-data" "$reviewed_sha" \
      combined-recovery 2>&1)"
  else
    output="$("$checker" "$plan" "$fixture_root/cost-valid.json" \
      "$fixture_root/current-checkpoint-$mode-$name.json" \
      "$fixture_root/replacement-user-data" "$reviewed_sha" 2>&1)"
  fi
  rc=$?
  set -e

  if [[ "$rc" -eq 0 ]]; then
    printf 'FAIL: accepted %s current reconciliation checkpoint\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  if [[ "$output" != "$generic_error" ]]; then
    printf 'FAIL: %s current checkpoint emitted non-generic output\n' "$name" >&2
    failures=$((failures + 1))
    return
  fi
  printf 'PASS: rejected %s current reconciliation checkpoint\n' "$name"
}

expect_verified \
  "exact Slice 4" \
  "$fixture_root/plan-valid.json" \
  "$fixture_root/cost-valid.json" \
  "$fixture_root/checkpoint-valid.json"

expect_replacement_verified \
  "exact Slice 4 replacement" \
  "$fixture_root/plan-replacement-valid.json" \
  "$fixture_root/cost-valid.json" \
  "$fixture_root/current-checkpoint-replacement-valid.json" \
  "$fixture_root/replacement-user-data"

expect_replacement_verified_with_noisy_failed_stat_probe

expect_combined_recovery_verified

recovery_args=(
  "$fixture_root/plan-replacement-valid.json"
  "$fixture_root/cost-valid.json"
  "$fixture_root/current-checkpoint-replacement-valid.json"
  "$fixture_root/replacement-user-data"
)
expect_recovery_invocation_rejected \
  "ambiguous historical four-argument recovery invocation" \
  "${recovery_args[@]}"
expect_recovery_invocation_rejected \
  "mode token in reviewed-SHA position" \
  "${recovery_args[@]}" combined-recovery
expect_recovery_invocation_rejected \
  "malformed reviewed SHA" \
  "${recovery_args[@]}" not-a-commit
wrong_reviewed_sha=0123456789abcdef0123456789abcdef01234567
if [[ "$wrong_reviewed_sha" == "$reviewed_sha" ]]; then
  wrong_reviewed_sha=89abcdef0123456789abcdef0123456789abcdef
fi
expect_recovery_invocation_rejected \
  "reviewed SHA different from checkout" \
  "${recovery_args[@]}" "$wrong_reviewed_sha"

jq --arg sha "$wrong_reviewed_sha" '.commit.sha = $sha' \
  "$fixture_root/current-checkpoint-replacement-valid.json" \
  >"$fixture_root/current-checkpoint-mismatched-sha.json"
expect_recovery_invocation_rejected \
  "checkpoint SHA different from reviewed SHA" \
  "$fixture_root/plan-replacement-valid.json" \
  "$fixture_root/cost-valid.json" \
  "$fixture_root/current-checkpoint-mismatched-sha.json" \
  "$fixture_root/replacement-user-data" \
  "$reviewed_sha"

printf 'untracked\n' >"$checked_repo/untracked"
expect_recovery_invocation_rejected \
  "untracked reviewed checkout" "${recovery_args[@]}" "$reviewed_sha"
rm "$checked_repo/untracked"

printf '\n' >>"$checked_repo/deploy/production/Caddyfile"
expect_recovery_invocation_rejected \
  "dirty reviewed checkout" "${recovery_args[@]}" "$reviewed_sha"
git -C "$checked_repo" checkout -q -- deploy/production/Caddyfile

printf '\n' >>"$checked_repo/deploy/production/Caddyfile"
git -C "$checked_repo" add deploy/production/Caddyfile
expect_recovery_invocation_rejected \
  "staged reviewed checkout" "${recovery_args[@]}" "$reviewed_sha"
git -C "$checked_repo" reset -q --hard HEAD

replacement_tree="$(git -C "$checked_repo" rev-parse 'HEAD^{tree}')"
replacement_commit="$(printf 'replacement fixture\n' | git -C "$checked_repo" \
  -c user.name='Slice 4 test' -c user.email='slice4@example.invalid' \
  commit-tree "$replacement_tree")"
git -C "$checked_repo" replace "$reviewed_sha" "$replacement_commit"
expect_recovery_invocation_rejected \
  "replacement ref in reviewed repository" \
  "${recovery_args[@]}" "$reviewed_sha"
git -C "$checked_repo" replace -d "$reviewed_sha" >/dev/null

git -C "$checked_repo" config core.attributesFile /tmp/hostile-attributes
expect_recovery_invocation_rejected \
  "hostile local Git configuration" \
  "${recovery_args[@]}" "$reviewed_sha"
git -C "$checked_repo" config --unset core.attributesFile

git -C "$checked_repo" config extensions.worktreeConfig true
printf '[core]\n\texcludesFile = /tmp/hostile-worktree-excludes\n' \
  >"$checked_repo/.git/config.worktree"
expect_recovery_invocation_rejected \
  "hostile worktree Git configuration" \
  "${recovery_args[@]}" "$reviewed_sha"
rm -f "$checked_repo/.git/config.worktree"
expect_recovery_invocation_rejected \
  "enabled worktree Git configuration extension" \
  "${recovery_args[@]}" "$reviewed_sha"
git -C "$checked_repo" config --unset extensions.worktreeConfig

git -C "$checked_repo" config status.showUntrackedFiles no
expect_recovery_invocation_rejected \
  "hostile status control" \
  "${recovery_args[@]}" "$reviewed_sha"
git -C "$checked_repo" config --unset status.showUntrackedFiles

git -C "$checked_repo" config remote.origin.promisor true
expect_recovery_invocation_rejected \
  "promisor remote control" \
  "${recovery_args[@]}" "$reviewed_sha"
git -C "$checked_repo" config --unset remote.origin.promisor

expect_recovery_invocation_rejected \
  "unknown six-argument recovery mode" \
  "${recovery_args[@]}" "$reviewed_sha" combined-recover

# Caller-controlled PATH tool wrappers must never be able to forge a PASS:
# every external tool the checker consults is resolved from root-owned system
# directories, so a hostile wrapper directory on PATH is silently ignored and
# the real tools still reject hostile inputs.
hostile_wrapper_bin="$fixture_root/hostile-tool-wrappers"
mkdir -p "$hostile_wrapper_bin"
for tool in jq grep stat sha256sum sha1sum shasum awk; do
  printf '#!/bin/sh\nexit 0\n' >"$hostile_wrapper_bin/$tool"
  chmod 0755 "$hostile_wrapper_bin/$tool"
done
cat >"$hostile_wrapper_bin/grep" <<'EOF'
#!/bin/sh
for arg in "$@"; do
  if [ "$arg" = "required deployment asset missing" ]; then
    exit 1
  fi
done
exit 0
EOF
cat >"$hostile_wrapper_bin/stat" <<'EOF'
#!/bin/sh
printf '600\n'
exit 0
EOF
for tool in sha256sum sha1sum shasum; do
  cat >"$hostile_wrapper_bin/$tool" <<'EOF'
#!/bin/sh
printf 'forged  wrapped-input\n'
exit 0
EOF
done
cat >"$hostile_wrapper_bin/awk" <<'EOF'
#!/bin/sh
read -r first _rest || exit 1
printf '%s\n' "$first"
EOF
cat >"$hostile_wrapper_bin/env" <<'EOF'
#!/bin/sh
exec /usr/bin/env "$@"
EOF
chmod 0755 "$hostile_wrapper_bin/grep" "$hostile_wrapper_bin/stat" \
  "$hostile_wrapper_bin/sha256sum" "$hostile_wrapper_bin/sha1sum" \
  "$hostile_wrapper_bin/shasum" "$hostile_wrapper_bin/awk" \
  "$hostile_wrapper_bin/env"

jq '(.resource_changes[] |
  select(.address == "aws_instance.replacement_host") |
  .change.after.user_data) = "forged"' \
  "$fixture_root/plan-replacement-valid.json" \
  >"$fixture_root/plan-wrapper-forged.json"

wrapper_output=
wrapper_rc=0
set +e
wrapper_output="$(PATH="$hostile_wrapper_bin:$PATH" \
  "$checker" \
  "$fixture_root/plan-wrapper-forged.json" \
  "$fixture_root/cost-valid.json" \
  "$fixture_root/current-checkpoint-replacement-valid.json" \
  "$fixture_root/replacement-user-data" \
  "$reviewed_sha" 2>&1)"
wrapper_rc=$?
set -e
if [[ "$wrapper_rc" -eq 0 ]]; then
  printf 'FAIL: accepted hostile plan through caller PATH tool wrappers\n' >&2
  failures=$((failures + 1))
elif [[ "$wrapper_output" != "$generic_error" ]]; then
  printf 'FAIL: PATH wrapper rejection disclosed a non-generic error\n' >&2
  failures=$((failures + 1))
else
  printf 'PASS: rejected hostile plan despite caller PATH tool wrappers\n'
fi

if wrapper_output="$(PATH="$hostile_wrapper_bin:$PATH" \
  "$checker" \
  "$fixture_root/plan-replacement-valid.json" \
  "$fixture_root/cost-valid.json" \
  "$fixture_root/current-checkpoint-replacement-valid.json" \
  "$fixture_root/replacement-user-data" \
  "$reviewed_sha" 2>&1)"; then
  if [[ "$wrapper_output" != "$expected_replacement_output" ]]; then
    printf 'FAIL: valid fixture through PATH wrappers emitted unexpected output\n' >&2
    failures=$((failures + 1))
  else
    printf 'PASS: verified valid fixture while ignoring caller PATH tool wrappers\n'
  fi
else
  printf 'FAIL: rejected valid fixture because of caller PATH tool wrappers\n' >&2
  failures=$((failures + 1))
fi

combined_recovery_plan_mutation() {
  local name="$1"
  local filter="$2"

  jq "$filter" "$fixture_root/plan-combined-recovery-valid.json" \
    >"$fixture_root/plan-combined-recovery-$name.json"
  expect_combined_recovery_rejected \
    "$name combined recovery plan" \
    "$fixture_root/plan-combined-recovery-$name.json"
}

combined_recovery_plan_mutation "missing-eip-create" \
  '.resource_changes |= map(select(.address != "aws_eip.origin"))'
combined_recovery_plan_mutation "eip-no-op" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.actions) = ["no-op"]'
combined_recovery_plan_mutation "eip-replace" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.actions) = ["delete", "create"]'
combined_recovery_plan_mutation "eip-has-before-state" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.before) = {domain: "vpc"}'
combined_recovery_plan_mutation "eip-missing-before" \
  'del(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.before)'
combined_recovery_plan_mutation "eip-wrong-domain" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.after.domain) = "standard"'
combined_recovery_plan_mutation "eip-missing-domain" \
  'del(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.after.domain)'
combined_recovery_plan_mutation "eip-unknown-domain" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.after_unknown.domain) = true'
combined_recovery_plan_mutation "eip-action-reason" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .action_reason) = "replace_by_request"'
combined_recovery_plan_mutation "eip-instance-association" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.after.instance) = "i-synthetic"'
combined_recovery_plan_mutation "eip-missing-instance-control" \
  'del(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.after.instance)'
combined_recovery_plan_mutation "eip-network-interface-association" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.after.network_interface) = "eni-synthetic"'
combined_recovery_plan_mutation "eip-private-ip-association" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.after.associate_with_private_ip) = "10.0.0.1"'
combined_recovery_plan_mutation "eip-unknown-instance-association" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.after_unknown.instance) = true'
combined_recovery_plan_mutation "eip-import" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .change.importing) = {id: "eipalloc-synthetic"}'
combined_recovery_plan_mutation "eip-move" \
  '(.resource_changes[] | select(.address == "aws_eip.origin") |
    .previous_address) = "aws_eip.old"'
combined_recovery_plan_mutation "host-no-op" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.actions) = ["no-op"]'
combined_recovery_plan_mutation "host-wrong-replace-reason" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .action_reason) = "replace_because_cannot_update"'
combined_recovery_plan_mutation "host-ami-change" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.ami) = "ami-unreviewed"'
combined_recovery_plan_mutation "additional-create" \
  '(.resource_changes[] | select(.address == "aws_iam_role.replacement_host") |
    .change.actions) = ["create"]'
combined_recovery_plan_mutation "additional-replacement" \
  '(.resource_changes[] | select(.address == "aws_iam_role.replacement_host") |
    .change.actions) = ["delete", "create"]'
combined_recovery_plan_mutation "unexpected-output" \
  '.output_changes.unexpected = .output_changes.replacement_instance_id'
combined_recovery_plan_mutation "diagnostic" \
  '.diagnostics = [{summary: "synthetic warning"}]'
expect_combined_recovery_rejected \
  "unknown combined recovery mode" \
  "$fixture_root/plan-combined-recovery-valid.json" \
  combined-recover

replacement_plan_mutation() {
  local name="$1"
  local filter="$2"

  jq "$filter" "$fixture_root/plan-replacement-valid.json" \
    >"$fixture_root/plan-replacement-$name.json"
  expect_replacement_rejected \
    "$name replacement plan" \
    "$fixture_root/plan-replacement-$name.json"
}

replacement_plan_mutation "wrong-action-order" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.actions) = ["create", "delete"]'
replacement_plan_mutation "second-action" \
  '(.resource_changes[] | select(.address == "aws_iam_role.replacement_host") |
    .change.actions) = ["update"]'
replacement_plan_mutation "missing-replace-request" \
  'del(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .action_reason)'
replacement_plan_mutation "wrong-replace-reason" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .action_reason) = "replace_because_cannot_update"'
replacement_plan_mutation "instance-profile-change" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.iam_instance_profile) = "unexpected-profile"'
replacement_plan_mutation "ami-change" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.ami) = "ami-unreviewed"'
replacement_plan_mutation "instance-type-change" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.instance_type) = "t4g.small"'
replacement_plan_mutation "subnet-change" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.subnet_id) = "subnet-unreviewed"'
replacement_plan_mutation "old-key-pair" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.before.key_name) = "unexpected-key"'
replacement_plan_mutation "old-public-ip-disabled" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.before.associate_public_ip_address) = false'
replacement_plan_mutation "key-pair" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.key_name) = "unexpected-key"'
replacement_plan_mutation "security-group-change" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.vpc_security_group_ids) = ["sg-unexpected"]'
replacement_plan_mutation "metadata-v1" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.metadata_options[0].http_tokens) = "optional"'
replacement_plan_mutation "unencrypted-root" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.root_block_device[0].encrypted) = false'
replacement_plan_mutation "unknown-security-control" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after_unknown.key_name) = true'
replacement_plan_mutation "unknown-recovery-field" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after_unknown.iam_instance_profile) = true'
replacement_plan_mutation "known-output" \
  '.output_changes.replacement_instance_id.after_unknown = false'
replacement_plan_mutation "created-output" \
  '.output_changes.replacement_instance_id.actions = ["create"]'
replacement_plan_mutation "diagnostic" \
  '.diagnostics = [{summary: "synthetic warning"}]'

cp "$fixture_root/replacement-user-data" \
  "$fixture_root/replacement-user-data-missing-asset"
sed -i.bak '/jobcron-recovery.timer/d' \
  "$fixture_root/replacement-user-data-missing-asset"
rm -f "$fixture_root/replacement-user-data-missing-asset.bak"
expect_replacement_rejected \
  "missing reviewed user-data asset" \
  "$fixture_root/plan-replacement-valid.json" \
  "$fixture_root/replacement-user-data-missing-asset"

chmod 0644 "$fixture_root/replacement-user-data"
expect_replacement_rejected \
  "non-private rendered user-data" \
  "$fixture_root/plan-replacement-valid.json"
chmod 0600 "$fixture_root/replacement-user-data"

plan_mutation "unexpected address" \
  '.resource_changes[0].address = "aws_instance.unexpected"'
plan_mutation "unexpected no-op address" \
  '.resource_changes += [{
    address: "aws_instance.old",
    previous_address: null,
    change: {
      actions: ["no-op"],
      importing: null,
      before: {synthetic: true},
      after: {synthetic: true}
    }
  }]'
for action in update delete forget mystery; do
  plan_mutation "$action action" \
    ".resource_changes[0].change.actions = [\"$action\"]"
done
plan_mutation "replace action" \
  '.resource_changes[0].change.actions = ["delete", "create"]'
plan_mutation "import action" \
  '.resource_changes[0].change.importing = {id: "synthetic-private-import"}'
plan_mutation "move action" \
  '.resource_changes[0].previous_address = "aws_iam_role.old"'
plan_mutation "port 22" \
  '.resource_changes[4].change.after.from_port = 22'

for address in \
  aws_security_group.unexpected \
  aws_vpc_security_group_ingress_rule.unexpected \
  aws_eip_association.unexpected \
  aws_key_pair.unexpected \
  aws_db_instance.unexpected \
  aws_secretsmanager_secret_version.unexpected \
  aws_instance.old \
  cloudflare_record.unexpected \
  aws_route53_record.unexpected; do
  jq --arg address "$address" \
    '.resource_changes += [{
      address: $address,
      previous_address: null,
      change: {
        actions: ["create"],
        importing: null,
        before: null,
        after: {synthetic: true}
      }
    }]' "$fixture_root/plan-valid.json" \
    >"$fixture_root/plan-forbidden.json"
  expect_rejected \
    "forbidden $address" \
    "$fixture_root/plan-forbidden.json" \
    "$fixture_root/cost-valid.json" \
    "$fixture_root/checkpoint-valid.json"
done

plan_mutation "missing output" \
  'del(.output_changes.replacement_instance_id)'
plan_mutation "renamed output" \
  '.output_changes.renamed = .output_changes.replacement_instance_id |
   del(.output_changes.replacement_instance_id)'
plan_mutation "unexpected output" \
  '.output_changes.unexpected = .output_changes.replacement_instance_id'
plan_mutation "non-sensitive output" \
  '.output_changes.replacement_instance_id.after_sensitive = false'
plan_mutation "updated output" \
  '.output_changes.replacement_instance_id.actions = ["update"]'
plan_mutation "private diagnostic marker" \
  '.diagnostics = [{summary: "PRIVATE_VALUE must never be printed"}]'

cost_mutation "recurring ceiling" \
  '.aggregate.recurring_monthly_upper_bound = 101'
cost_mutation "one-time ceiling" \
  '.aggregate.one_time_upper_bound = 201'
cost_mutation "understated aggregate" \
  '.aggregate.recurring_monthly_upper_bound = 1'
cost_mutation "stale cost evidence" \
  '.checked_at = "2000-01-01T00:00:00Z"'
cost_mutation "missing cost category" \
  '.categories |= map(select(.name != "cloudflare"))'
cost_mutation "duplicate cost category" \
  '.categories += [.categories[0]]'
cost_mutation "missing pricing source" \
  '.categories[0].source = ""'

checkpoint_mutation "stale Slice 3 checkpoint" \
  '.checked_at = "2000-01-01T00:00:00Z"'
checkpoint_mutation "missing Slice 3 address" \
  '.addresses |= map(select(. != "aws_db_instance.production"))'
checkpoint_mutation "renamed Slice 3 address" \
  '.addresses[0] = "aws_db_instance.renamed"'
checkpoint_mutation "dirty Slice 3 post-apply plan" \
  '.post_apply_plan = "changes"'
checkpoint_mutation "failed recovery lifecycle" \
  '.recovery_lifecycle_verdict = "FAIL"'
checkpoint_mutation "short verified retention" \
  '.recovery_bucket.verified_retention_days = 13'
checkpoint_mutation "short unverified retention" \
  '.recovery_bucket.unverified_retention_days = 89'
checkpoint_mutation "Slice 3 destroy" \
  '.destroy_or_replace = 1'
checkpoint_mutation "old resource change" \
  '.old_resource_changes = 1'

current_checkpoint_mutation "stale evidence" \
  '.checked_at = "2000-01-01T00:00:00Z"'
current_checkpoint_mutation "future evidence" \
  '.checked_at = "2999-01-01T00:00:00Z"'
current_checkpoint_mutation "wrong schema" \
  '.schema_version = "human-assisted-reconciliation-v2"'
current_checkpoint_mutation "dirty commit" '.commit.clean = false'
current_checkpoint_mutation "inexact commit" '.commit.exact = false'
current_checkpoint_mutation "malformed commit" '.commit.sha = "not-a-commit"'
current_checkpoint_mutation "unmanaged bootstrap host" \
  '.selected_state_resources.bootstrap_host.managed = false'
current_checkpoint_mutation "renamed bootstrap address" \
  '.selected_state_resources.bootstrap_host.terraform_address = "aws_instance.renamed"'
current_checkpoint_mutation "unmanaged database" \
  '.selected_state_resources.database.managed = false'
current_checkpoint_mutation "renamed database address" \
  '.selected_state_resources.database.terraform_address = "aws_db_instance.renamed"'
current_checkpoint_mutation "unapproved host disposition" \
  '.bootstrap_host.human_approved = false'
current_checkpoint_mutation "wrong host disposition" \
  '.bootstrap_host.disposition = "retain"'
current_checkpoint_mutation "rollback host not retained" \
  '.legacy_rollback_host.retained = false'
current_checkpoint_mutation "replacement EIP absent" \
  '.managed_eip.state_presence = "absent"'
current_checkpoint_mutation "renamed EIP address" \
  '.managed_eip.terraform_address = "aws_eip.renamed"'
current_checkpoint_mutation "combined EIP present" \
  '.managed_eip.state_presence = "present"' combined-recovery
current_checkpoint_mutation "EIP attached" '.managed_eip.unattached = false'
current_checkpoint_mutation "origin ingress" '.origin.ingress_rule_count = 1'
current_checkpoint_mutation "public origin phase" '.origin.phase = "public"'
current_checkpoint_mutation "RDS unavailable" '.rds.status = "stopped"'
current_checkpoint_mutation "public RDS" '.rds.publicly_accessible = true'
current_checkpoint_mutation "unencrypted RDS" '.rds.storage_encrypted = false'
current_checkpoint_mutation "unprotected RDS" '.rds.deletion_protection = false'
current_checkpoint_mutation "RDS backups disabled" '.rds.backup_retention_days = 0'
current_checkpoint_mutation "missing RDS restorable metadata" \
  '.rds.latest_restorable_time_observed = false'
current_checkpoint_mutation "missing runtime secret container" \
  '.runtime_secret.container_exists = false'
current_checkpoint_mutation "Terraform-managed secret versions" \
  '.runtime_secret.terraform_manages_versions = true'
current_checkpoint_mutation "nonnumeric secret version count" \
  '.runtime_secret.observed_version_count = "1"'
current_checkpoint_mutation "negative secret version count" \
  '.runtime_secret.observed_version_count = -1'
current_checkpoint_mutation "unsafe recovery bucket" \
  '.recovery_bucket.public_access_blocked = false'
current_checkpoint_mutation "unencrypted recovery bucket" \
  '.recovery_bucket.encrypted = false'
current_checkpoint_mutation "unversioned recovery bucket" \
  '.recovery_bucket.versioned = false'
current_checkpoint_mutation "non-TLS recovery bucket" \
  '.recovery_bucket.tls_only = false'
current_checkpoint_mutation "unverified recovery lifecycle" \
  '.recovery_bucket.lifecycle_verified = false'
current_checkpoint_mutation "unsafe state backend" \
  '.state_backend.lockfile_enabled = false'
current_checkpoint_mutation "local state backend" '.state_backend.remote = false'
current_checkpoint_mutation "unencrypted state backend" \
  '.state_backend.encrypted = false'
current_checkpoint_mutation "public cutover approved" '.public_cutover.approved = true'
current_checkpoint_mutation "public cutover performed" '.public_cutover.performed = true'
current_checkpoint_mutation "missing field" 'del(.rds.status)'
current_checkpoint_mutation "renamed field" \
  '.runtime_secret.version_count = .runtime_secret.observed_version_count |
   del(.runtime_secret.observed_version_count)'
current_checkpoint_mutation "unexpected top-level field" '.unexpected = true'
current_checkpoint_mutation "unexpected nested field" '.rds.unexpected = true'

printf '{malformed' >"$fixture_root/plan-malformed.json"
expect_rejected \
  "malformed Terraform output" \
  "$fixture_root/plan-malformed.json" \
  "$fixture_root/cost-valid.json" \
  "$fixture_root/checkpoint-valid.json"

printf '{malformed' >"$fixture_root/cost-malformed.json"
expect_rejected \
  "malformed cost evidence" \
  "$fixture_root/plan-valid.json" \
  "$fixture_root/cost-malformed.json" \
  "$fixture_root/checkpoint-valid.json"

printf '{malformed' >"$fixture_root/checkpoint-malformed.json"
expect_rejected \
  "malformed Slice 3 checkpoint" \
  "$fixture_root/plan-valid.json" \
  "$fixture_root/cost-valid.json" \
  "$fixture_root/checkpoint-malformed.json"

if [[ "$failures" -ne 0 ]]; then
  printf '%d Slice 4 checker test(s) failed\n' "$failures" >&2
  exit 1
fi

printf 'All Terraform Slice 4 plan checker tests passed\n'
