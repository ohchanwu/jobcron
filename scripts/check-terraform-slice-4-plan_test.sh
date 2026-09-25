#!/usr/bin/env bash
set -euo pipefail
umask 077

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_checker="$repo_root/scripts/check-terraform-slice-4-plan.sh"
ci_workflow="$repo_root/.github/workflows/ci.yml"
fixture_root="$(mktemp -d)"
trap 'rm -rf "$fixture_root"' EXIT

checked_repo="$fixture_root/checked-repo"
mkdir -p "$checked_repo/scripts" \
  "$checked_repo/deploy/production/systemd"
cp "$source_checker" "$checked_repo/scripts/check-terraform-slice-4-plan.sh"
cp "$repo_root/scripts/check-terraform-slice-4-plan-body.sh" \
  "$checked_repo/scripts/check-terraform-slice-4-plan-body.sh"
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

# Initial deployment is deliberately not a retained-legacy-host assertion.
jq '
  del(.legacy_rollback_host) |
  .schema_version = "human-assisted-initial-deployment-v1" |
  .bootstrap_host.disposition = "replace-disposable-bootstrap" |
  .initial_deployment = {
    prior_service_exists: false,
    prior_production_data_exists: false,
    prior_image_exists: false,
    legacy_rollback_host_exists: false,
    bootstrap_service_running: false,
    bootstrap_contains_production_data: false,
    database_unused: true,
    rollback: "stop-private-runtime-leave-public-routing-unchanged"
  } |
  .evidence_security = {
    state_secret_free: true,
    plan_secret_free: true,
    shared_evidence_secret_free: true
  }
' "$fixture_root/current-checkpoint-combined-valid.json" \
  >"$fixture_root/current-checkpoint-initial-valid.json"

# A real initial plan still contains the full protected no-op inventory.
jq '
  .resource_changes |= map(
    if .address == "aws_security_group.origin" then
      .change.before = {id: "sg-origin", ingress: []} | .change.after = .change.before
    elif .address == "aws_security_group.database" then
      .change.before = {id: "sg-database", ingress: [{from_port:5432,to_port:5432,
        protocol:"tcp",security_groups:["sg-origin"],cidr_blocks:[],ipv6_cidr_blocks:[],
        prefix_list_ids:[],self:false}]} | .change.after = .change.before
    elif .address == "aws_vpc_security_group_ingress_rule.database_postgresql_from_origin" then
      .change.before = {security_group_id: "sg-database", referenced_security_group_id: "sg-origin",
        from_port: 5432, to_port: 5432, ip_protocol: "tcp", cidr_ipv4: null, cidr_ipv6: null,
        prefix_list_id: null} | .change.after = .change.before
    elif .address == "aws_db_instance.production" then
      .change.before = {publicly_accessible: false, storage_encrypted: true,
        deletion_protection: true, backup_retention_period: 7, manage_master_user_password: true,
        password: null, vpc_security_group_ids: ["sg-database"]} | .change.after = .change.before |
      .change.before_sensitive = {password:true} | .change.after_sensitive = {password:true}
    else . end
  )
' "$fixture_root/plan-combined-recovery-valid.json" >"$fixture_root/plan-initial-valid.json"

expect_initial_verified() {
  local output plan="${1:-$fixture_root/plan-initial-valid.json}"
  local checkpoint="${2:-$fixture_root/current-checkpoint-initial-valid.json}"
  local expected="${3:-$expected_combined_recovery_output}"
  if ! output="$("$checker" "$plan" \
    "$fixture_root/cost-valid.json" \
    "$checkpoint" \
    "$fixture_root/replacement-user-data" "$reviewed_sha" \
    initial-deployment 2>&1)" || [[ "$output" != "$expected" ]]; then
    printf 'FAIL: rejected truthful initial deployment\n' >&2
    failures=$((failures + 1))
  else
    printf 'PASS: verified truthful initial deployment\n'
  fi
}
expect_initial_verified

jq '.resource_drift = [{address:"aws_eip.origin", change:{actions:["delete"],before:{domain:"vpc"},after:null}}]' \
  "$fixture_root/plan-initial-valid.json" >"$fixture_root/plan-initial-drift.json"
expect_initial_verified "$fixture_root/plan-initial-drift.json"
jq '(.resource_changes[] | select(.address == "aws_eip.origin") | .change) |=
  (.actions = ["no-op"] | .before = .after | .after_unknown = {})' \
  "$fixture_root/plan-initial-valid.json" >"$fixture_root/plan-initial-present.json"
jq '.managed_eip.state_presence = "present"' \
  "$fixture_root/current-checkpoint-initial-valid.json" >"$fixture_root/checkpoint-initial-present.json"
expect_initial_verified "$fixture_root/plan-initial-present.json" \
  "$fixture_root/checkpoint-initial-present.json" "$expected_replacement_output"

initial_mutation() {
  local slot="$1" name="$2" filter="$3"
  local args=("$fixture_root/plan-initial-valid.json" "$fixture_root/cost-valid.json"
    "$fixture_root/current-checkpoint-initial-valid.json"
    "$fixture_root/replacement-user-data" "$reviewed_sha" initial-deployment)
  jq "$filter" "${args[$slot]}" >"$fixture_root/initial-mutation.json"
  args[$slot]="$fixture_root/initial-mutation.json"
  expect_recovery_invocation_rejected "initial $name" "${args[@]}"
}
initial_mutation 0 'broad no-op ingress' '(.resource_changes[] | select(.address == "aws_security_group.origin") | .change) |= (.before.ingress = [{from_port:443,to_port:443,cidr_blocks:["0.0.0.0/0"]}] | .after = .before)'
initial_mutation 0 'no-op hides a change' '.resource_changes[0].change.after.extra = true'
initial_mutation 0 'no-op unknowns' '.resource_changes[0].change.after_unknown.policy = true'
initial_mutation 0 'malformed no-op unknowns' '.resource_changes[0].change.after_unknown.policy = "unknown"'
initial_mutation 0 'public no-op RDS' '(.resource_changes[] | select(.address == "aws_db_instance.production") | .change) |= (.before.publicly_accessible = true | .after = .before)'
initial_mutation 0 'credential in plan state' '.prior_state.values.root_module.resources = [{values:{password:"PRIVATE_VALUE"}}]'
initial_mutation 0 'unrelated refreshed drift' '.resource_drift = [{address:"aws_db_instance.production",change:{actions:["update"]}}]'
initial_mutation 0 'database replacement' '(.resource_changes[] | select(.address == "aws_db_instance.production") | .change.actions) = ["delete","create"]'
initial_mutation 0 'unrequested host replacement' '(.resource_changes[] | select(.address == "aws_instance.replacement_host") | .action_reason) = "replace_because_cannot_update"'
initial_mutation 0 'public database CIDR' '(.resource_changes[] | select(.address == "aws_security_group.database") | .change) |= (.before.ingress[0].cidr_blocks = ["0.0.0.0/0"] | .after = .before)'
initial_mutation 0 'EIP association' '(.resource_changes[] | select(.address == "aws_eip.origin") | .change.after.instance) = "i-private"'
initial_mutation 0 'EIP unknown association' '(.resource_changes[] | select(.address == "aws_eip.origin") | .change.after_unknown.instance) = true'
initial_mutation 0 'unexpected output' '.output_changes.secret = {actions:["create"],after:"PRIVATE_VALUE",after_sensitive:true}'
initial_mutation 0 'secret version' '.resource_changes += [{address:"aws_secretsmanager_secret_version.runtime",change:{actions:["create"]}}]'
for address in aws_eip_association.origin cloudflare_record.origin aws_route53_record.origin aws_vpc_security_group_ingress_rule.public aws_instance.unrelated; do
  initial_mutation 0 "$address" ".resource_changes += [{address:\"$address\",change:{actions:[\"create\"]}}]"
done
initial_mutation 1 'stale cost' '.checked_at = "2000-01-01T00:00:00Z"'
initial_mutation 1 'cost ceiling' '.aggregate.recurring_monthly_upper_bound = 101'

for field in prior_service_exists prior_production_data_exists prior_image_exists legacy_rollback_host_exists bootstrap_service_running bootstrap_contains_production_data; do
  initial_mutation 2 "$field" ".initial_deployment.$field = true"
done
initial_mutation 2 'used database' '.initial_deployment.database_unused = false'
initial_mutation 2 'invented legacy host' '.legacy_rollback_host = {retained:true}'
initial_mutation 2 'secret evidence' '.evidence_security.state_secret_free = false'
initial_mutation 2 'missing fact' 'del(.initial_deployment.bootstrap_service_running)'
initial_mutation 2 'extra fact' '.initial_deployment.provenance = "invented"'
initial_mutation 2 'legacy schema' '.schema_version = "human-assisted-reconciliation-v1"'
initial_mutation 2 'legacy disposition' '.bootstrap_host.disposition = "replace"'
initial_mutation 2 'wrong EIP presence' '.managed_eip.state_presence = "present"'
initial_mutation 2 'stale reconciliation' '.checked_at = "2000-01-01T00:00:00Z"'
initial_mutation 2 'wrong SHA' '.commit.sha = "0000000000000000000000000000000000000000"'
initial_mutation 2 'unapproved destruction' '.bootstrap_host.human_approved = false'
initial_mutation 2 'state locking disabled' '.state_backend.lockfile_enabled = false'
initial_mutation 2 'public cutover' '.public_cutover.approved = true'
initial_mutation 2 'unknown field' '.unexpected = true'

# Initial evidence cannot be substituted into either historical recovery lane.
expect_recovery_invocation_rejected 'initial checkpoint in replacement mode' \
  "$fixture_root/plan-replacement-valid.json" "$fixture_root/cost-valid.json" \
  "$fixture_root/current-checkpoint-initial-valid.json" "$fixture_root/replacement-user-data" "$reviewed_sha"
expect_recovery_invocation_rejected 'initial checkpoint in combined mode' \
  "$fixture_root/plan-combined-recovery-valid.json" "$fixture_root/cost-valid.json" \
  "$fixture_root/current-checkpoint-initial-valid.json" "$fixture_root/replacement-user-data" "$reviewed_sha" combined-recovery

chmod 0644 "$fixture_root/current-checkpoint-initial-valid.json"
expect_recovery_invocation_rejected 'initial non-private checkpoint' \
  "$fixture_root/plan-initial-valid.json" "$fixture_root/cost-valid.json" \
  "$fixture_root/current-checkpoint-initial-valid.json" \
  "$fixture_root/replacement-user-data" "$reviewed_sha" initial-deployment
chmod 0600 "$fixture_root/current-checkpoint-initial-valid.json"

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

# Exercise the exported-function bypass with both possible trusted PATHs.
# Use malformed nonempty JSON so jq 1.6 and newer have the same rejection.
printf '{invalid' >"$fixture_root/ambient-invalid.json"
cat >"$fixture_root/ambient-caller.sh" <<'EOF'
#!/bin/bash
jq() { return 0; }
export -f jq
exec "$@"
EOF
for ambient_path in /usr/bin:/bin /usr/local/bin:/usr/bin:/bin; do
  ambient_output=
  ambient_rc=0
  ambient_output="$(PATH="$ambient_path" /bin/bash \
    "$fixture_root/ambient-caller.sh" "$checker" \
    "$fixture_root/ambient-invalid.json" \
    "$fixture_root/ambient-invalid.json" \
    "$fixture_root/ambient-invalid.json" 2>&1)" || ambient_rc=$?
  if [[ "$ambient_rc" -eq 0 || "$ambient_output" != "$generic_error" ]]; then
    printf 'FAIL: exported jq function bypassed generic rejection\n' >&2
    failures=$((failures + 1))
  else
    printf 'PASS: rejected malformed inputs despite exported jq function\n'
  fi
done

: >"$fixture_root/empty-input"
expect_rejected "empty plan/cost/checkpoint (jq 1.6)" \
  "$fixture_root/empty-input" "$fixture_root/empty-input" \
  "$fixture_root/empty-input"

# Each artifact must contain exactly one JSON document, not a stream whose
# last result can hide an earlier failure. Keep the other inputs valid.
for json_mode in create replacement combined-recovery initial-deployment; do
  json_plan="$fixture_root/plan-valid.json"
  json_checkpoint="$fixture_root/checkpoint-valid.json"
  if [[ "$json_mode" == replacement ]]; then
    json_plan="$fixture_root/plan-replacement-valid.json"
    json_checkpoint="$fixture_root/current-checkpoint-replacement-valid.json"
  elif [[ "$json_mode" == combined-recovery ]]; then
    json_plan="$fixture_root/plan-combined-recovery-valid.json"
    json_checkpoint="$fixture_root/current-checkpoint-combined-valid.json"
  elif [[ "$json_mode" == initial-deployment ]]; then
    json_plan="$fixture_root/plan-initial-valid.json"
    json_checkpoint="$fixture_root/current-checkpoint-initial-valid.json"
  fi
  for json_slot in 0 1 2; do
    json_args=("$json_plan" "$fixture_root/cost-valid.json" "$json_checkpoint")
    json_source="${json_args[$json_slot]}"
    if [[ "$json_mode" != create ]]; then
      json_args+=("$fixture_root/replacement-user-data" "$reviewed_sha")
    fi
    if [[ "$json_mode" == combined-recovery || "$json_mode" == initial-deployment ]]; then
      json_args+=("$json_mode")
    fi
    for json_shape in empty whitespace malformed valid-then-malformed duplicate invalid-then-valid valid-then-invalid null array; do
      json_bad="$fixture_root/json-boundary-input"
      case "$json_shape" in
        empty) : >"$json_bad" ;;
        whitespace) printf ' \t\r\n' >"$json_bad" ;;
        malformed) printf '{"PRIVATE_VALUE":' >"$json_bad" ;;
        valid-then-malformed)
          cp "$json_source" "$json_bad"
          printf '\n{"PRIVATE_VALUE":' >>"$json_bad" ;;
        duplicate) jq -s '.[]' "$json_source" "$json_source" >"$json_bad" ;;
        invalid-then-valid)
          printf '{"PRIVATE_VALUE":true}\n' >"$json_bad"
          jq . "$json_source" >>"$json_bad" ;;
        valid-then-invalid)
          cp "$json_source" "$json_bad"
          printf '\n{"PRIVATE_VALUE":true}\n' >>"$json_bad" ;;
        null) printf 'null\n' >"$json_bad" ;;
        array) jq -s . "$json_source" >"$json_bad" ;;
      esac
      json_args[json_slot]="$json_bad"
      expect_recovery_invocation_rejected \
        "$json_mode JSON slot $json_slot $json_shape" "${json_args[@]}"
    done
  done
done

# Direct execution must ignore startup files, imported functions and exported
# shell options before even the launcher's first command. A marker catches
# startup execution even if its stdout would be swallowed by a later command.
cat >"$fixture_root/hostile-startup.sh" <<'EOF'
printf 'PRIVATE_VALUE startup executed\n'
printf 'executed\n' >"$AMBIENT_MARKER"
jq() { return 0; }
export -f jq
EOF
cat >"$fixture_root/ambient-state-caller.sh" <<'EOF'
#!/bin/bash
jq() { printf 'PRIVATE_VALUE forged jq\n'; return 0; }
export -f jq
exec /usr/bin/env SHELLOPTS=xtrace:verbose:nounset \
  BASHOPTS=extdebug:failglob BASH_ENV="$AMBIENT_STARTUP" \
  ENV="$AMBIENT_STARTUP" "$@"
EOF
for ambient_mode in create replacement combined-recovery; do
  ambient_plan="$fixture_root/plan-valid.json"
  ambient_checkpoint="$fixture_root/checkpoint-valid.json"
  ambient_expected="$expected_output"
  if [[ "$ambient_mode" == replacement ]]; then
    ambient_plan="$fixture_root/plan-replacement-valid.json"
    ambient_checkpoint="$fixture_root/current-checkpoint-replacement-valid.json"
    ambient_expected="$expected_replacement_output"
  elif [[ "$ambient_mode" == combined-recovery ]]; then
    ambient_plan="$fixture_root/plan-combined-recovery-valid.json"
    ambient_checkpoint="$fixture_root/current-checkpoint-combined-valid.json"
    ambient_expected="$expected_combined_recovery_output"
  fi
  for ambient_case in valid invalid; do
    ambient_args=("$ambient_plan" "$fixture_root/cost-valid.json" "$ambient_checkpoint")
    if [[ "$ambient_case" == invalid ]]; then
      ambient_args[0]="$fixture_root/ambient-invalid.json"
    fi
    if [[ "$ambient_mode" != create ]]; then
      ambient_args+=("$fixture_root/replacement-user-data" "$reviewed_sha")
    fi
    if [[ "$ambient_mode" == combined-recovery ]]; then
      ambient_args+=(combined-recovery)
    fi
    ambient_rc=0
    ambient_output="$(AMBIENT_STARTUP="$fixture_root/hostile-startup.sh" \
      AMBIENT_MARKER="$fixture_root/startup-executed" PATH=/usr/bin:/bin \
      /bin/bash "$fixture_root/ambient-state-caller.sh" \
      "$checker" "${ambient_args[@]}" 2>&1)" || ambient_rc=$?
    if [[ -e "$fixture_root/startup-executed" ]] ||
      { [[ "$ambient_case" == valid ]] &&
        [[ "$ambient_rc" -ne 0 || "$ambient_output" != "$ambient_expected" ]]; } ||
      { [[ "$ambient_case" == invalid ]] &&
        [[ "$ambient_rc" -eq 0 || "$ambient_output" != "$generic_error" ]]; }; then
      printf 'FAIL: %s %s ambient startup boundary\n' "$ambient_mode" "$ambient_case" >&2
      failures=$((failures + 1))
    else
      printf 'PASS: %s %s ignores ambient startup state\n' "$ambient_mode" "$ambient_case"
    fi
  done
done

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
