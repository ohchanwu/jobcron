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

# AWS provider 6.33.0 plans contain plaintext, not the pre-v6 SHA-1 value.
# --rawfile preserves the final newline that shell substitution would remove.
jq --rawfile user_data "$fixture_root/replacement-user-data" '
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
          user_data: "#!/bin/bash\n# disposable bootstrap\n"
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
          user_data: $user_data
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
# Synthetic independent observations and pre-refresh state, never real evidence.
export TF_INITIAL_LIVE_HOST_JSON="$fixture_root/live-host.json"
export TF_INITIAL_LIVE_RDS_JSON="$fixture_root/live-rds.json"
export TF_INITIAL_LIVE_ADDRESSES_JSON="$fixture_root/live-addresses.json"
export TF_INITIAL_CURRENT_STATE_JSON="$fixture_root/current-state.json"
export TF_INITIAL_TFVARS_JSON="$fixture_root/reconstructed.tfvars.json"
python3 - "$fixture_root" <<'PY'
import copy
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
def save(name, value):
    (root / name).write_text(json.dumps(value))

p = json.loads((root / 'plan-initial-valid.json').read_text())
p['timestamp'] = '2026-09-25T01:08:00Z'
changes = {r['address']: r['change'] for r in p['resource_changes']}
host = 'aws_instance.replacement_host'
db = 'aws_db_instance.production'
role = 'aws_iam_role.replacement_host'
profile = 'aws_iam_instance_profile.replacement_host'
attachment = 'aws_iam_role_policy_attachment.replacement_host_ssm'
policy = 'aws_iam_role_policy.replacement_host_runtime'
ssm = 'arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore'
for address, change in changes.items():
    if change['before'] is not None:
        change['before'].setdefault('id', address)
        if address != host:
            change['after'] = copy.deepcopy(change['before'])
changes[host]['before'].update(id='i-synthetic', public_ip='192.0.2.1',
    public_dns='old.example.invalid', tags=None,
    root_block_device=[dict(changes[host]['after']['root_block_device'][0], tags=None)])
changes[host]['before']['ami'] = changes[host]['after']['ami'] = 'ami-0123456789abcdef0'
changes[db]['before'].update(id='db-synthetic-resource', identifier='synthetic-db',
    latest_restorable_time='2026-09-25T01:00:00Z')
changes[role]['before'].update(name='jobcron-replacement-host', id='jobcron-replacement-host',
    tags=None, inline_policy=None, managed_policy_arns=None)
changes[profile]['before'].update(name='jobcron-replacement-host', role='jobcron-replacement-host', tags=None)
changes[attachment]['before'].update(role='jobcron-replacement-host', policy_arn=ssm)
changes[policy]['before'].update(role='jobcron-replacement-host', name='jobcron-replacement-host-runtime')
for address in (db, role, profile, attachment, policy):
    changes[address]['after'] = copy.deepcopy(changes[address]['before'])
inputs = {'replacement_host_ami_id': 'ami-0123456789abcdef0',
    'replacement_public_subnet_key': 'public_a',
    'canonical_network_config': {'synthetic': True},
    'private_database_config': {'database_identifier': 'synthetic-db'}}
changes['aws_subnet.public["public_a"]']['before']['id'] = 'subnet-reviewed-public'
changes['aws_subnet.public["public_a"]']['after']['id'] = 'subnet-reviewed-public'
p['variables'] = {k: {'value': v} for k, v in inputs.items()}
p['configuration'] = {'root_module': {'resources': [
    {'address': a, 'expressions': {}} for a in changes]}}
config = {r['address']: r['expressions'] for r in p['configuration']['root_module']['resources']}
config['aws_eip.origin']['domain'] = {'constant_value': 'vpc'}
config[attachment].update(policy_arn={'constant_value': ssm},
    role={'references': [role + '.name', role]})
config[policy]['role'] = {'references': [role + '.id', role]}
save('plan-initial-valid.json', p)
save('current-state.json', {'format_version': '1.0', 'values': {'root_module': {'resources': [
    {'address': a, 'mode': 'managed', 'values': c['before']}
    for a, c in changes.items() if c['before'] is not None] + [
    {'address': 'aws_eip.origin', 'mode': 'managed', 'values': {
        'id': 'eipalloc-0123456789abcdef0', 'allocation_id': 'eipalloc-0123456789abcdef0',
        'domain': 'vpc'}}]}}})
save('live-addresses.json', {'Addresses': []})
save('live-host.json', {'Reservations': [{'Instances': [{'InstanceId': 'i-synthetic',
    'PublicIpAddress': '192.0.2.2', 'PublicDnsName': 'new.example.invalid'}]}]})
save('live-rds.json', {'DBInstances': [{'DBInstanceIdentifier': 'synthetic-db',
    'DbiResourceId': 'db-synthetic-resource', 'LatestRestorableTime': '2026-09-25T01:05:00+00:00'}]})
save('reconstructed.tfvars.json', inputs)
drifts = []
for address in (db, role, profile, host):
    before = copy.deepcopy(changes[address]['before'])
    after = copy.deepcopy(before)
    if address == db:
        after['latest_restorable_time'] = '2026-09-25T01:05:00Z'
    else:
        after['tags'] = {}
    if address == role:
        after.update(inline_policy=[], managed_policy_arns=[ssm])
    if address == host:
        after.update(public_ip='192.0.2.2', public_dns='new.example.invalid')
        after['root_block_device'][0]['tags'] = {}
    drifts.append({'address': address, 'change': {'actions': ['update'],
        'before': before, 'after': after, 'after_unknown': {}}})
    changes[address]['before'] = after
    if address != host:
        changes[address]['after'] = copy.deepcopy(after)
p['resource_drift'] = drifts
save('plan-initial-refresh.json', p)
PY
expect_initial_verified
expect_initial_verified "$fixture_root/plan-initial-refresh.json"

# Mutation tests keep drift.after and planned before/no-op values consistent so
# source binding and the attribute allowlist, not an incidental mismatch, decide.
python3 - "$fixture_root" "$checker" "$reviewed_sha" <<'PY'
import copy
import json
import os
from pathlib import Path
import subprocess
import sys

root, checker, sha = Path(sys.argv[1]), sys.argv[2], sys.argv[3]
baseline = json.loads((root / 'plan-initial-refresh.json').read_text())
host, db = 'aws_instance.replacement_host', 'aws_db_instance.production'
role, profile = 'aws_iam_role.replacement_host', 'aws_iam_instance_profile.replacement_host'
attachment = 'aws_iam_role_policy_attachment.replacement_host_ssm'
policy = 'aws_iam_role_policy.replacement_host_runtime'
env_files = {'TF_INITIAL_LIVE_HOST_JSON': 'live-host.json',
    'TF_INITIAL_LIVE_RDS_JSON': 'live-rds.json',
    'TF_INITIAL_LIVE_ADDRESSES_JSON': 'live-addresses.json',
    'TF_INITIAL_CURRENT_STATE_JSON': 'current-state.json',
    'TF_INITIAL_TFVARS_JSON': 'reconstructed.tfvars.json'}
evidence = {k: json.loads((root / v).read_text()) for k, v in env_files.items()}

def record(p, section, address):
    return next(r for r in p[section] if r['address'] == address)

def drift_after(p, address, key, value):
    record(p, 'resource_drift', address)['change']['after'][key] = value

def planned(p, address, key, value):
    change = record(p, 'resource_changes', address)['change']
    change['before'][key] = value
    change['after'][key] = value

def run(name, mutate, accept=False):
    p, e = copy.deepcopy(baseline), copy.deepcopy(evidence)
    mutate(p, e)
    for d in p['resource_drift']:
        c = record(p, 'resource_changes', d['address'])['change'] if d['address'] in {r['address'] for r in p['resource_changes']} else None
        if c:
            c['before'] = copy.deepcopy(d['change']['after'])
            if c['actions'] == ['no-op']:
                c['after'] = copy.deepcopy(c['before'])
    path = root / 'refresh-mutation.json'
    path.write_text(json.dumps(p))
    env = dict(os.environ)
    for key, value in e.items():
        f = root / ('mutation-' + env_files[key])
        f.write_text(json.dumps(value))
        env[key] = str(f)
    result = subprocess.run([checker, str(path), str(root / 'cost-valid.json'),
        str(root / 'current-checkpoint-initial-valid.json'), str(root / 'replacement-user-data'),
        sha, 'initial-deployment'], env=env, capture_output=True, text=True)
    valid = result.returncode == 0 if accept else (
        result.returncode != 0 and result.stdout == '' and
        result.stderr == 'Terraform saved plan violates the Slice 4 contract\n')
    if not valid:
        sys.exit('FAIL: refresh mutation ' + name)
    print('PASS: refresh mutation ' + name)

run('independent four-resource refresh', lambda p, e: None, True)
def inline_projection(p, e):
    # Independently managed policy projected into the provider's computed role
    # block, not a new IAM permission or an inline policy configured on the role.
    payload = json.dumps({'Version': '2012-10-17', 'Statement': [
        {'Effect': 'Allow', 'Action': ['secretsmanager:GetSecretValue'],
         'Resource': ['arn:aws:secretsmanager:us-east-1:111122223333:secret:synthetic']}]})
    planned(p, policy, 'policy', payload)
    state = e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources']
    next(r for r in state if r['address'] == policy)['values']['policy'] = payload
    next(r for r in state if r['address'] == role)['values']['inline_policy'] = []
    record(p, 'resource_drift', role)['change']['before']['inline_policy'] = []
    drift_after(p, role, 'inline_policy', [
        {'name': 'jobcron-replacement-host-runtime', 'policy': payload}])
run('independent runtime policy computed projection', inline_projection, True)
def projection_run(name, mutate, accept=False):
    run(name, lambda p, e: (inline_projection(p, e), mutate(p, e)), accept)
def projected_policy(p):
    return record(p, 'resource_drift', role)['change']['after']['inline_policy'][0]
projection_run('equivalent policy JSON formatting', lambda p, e:
    projected_policy(p).update(policy=json.dumps(json.loads(projected_policy(p)['policy']), indent=2, sort_keys=True)), True)
projection_run('null-to-singleton policy projection', lambda p, e: (
    record(p, 'resource_drift', role)['change']['before'].update(inline_policy=None),
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources']
         if r['address'] == role)['values'].update(inline_policy=None)), True)
for value in (None, {}, 'policy', [None], [{'name': 'unexpected'}]):
    projection_run('malformed projection ' + repr(value), lambda p, e, v=value:
        drift_after(p, role, 'inline_policy', v))
projection_run('duplicate projected policy', lambda p, e:
    record(p, 'resource_drift', role)['change']['after']['inline_policy'].append(copy.deepcopy(projected_policy(p))))
for key, value in [('name', 'other'), ('name', None), ('extra', True),
        ('policy', '{}'), ('policy', '{'), ('policy', '[]'), ('policy', 'null'),
        ('policy', '{"Statement":[],"Statement":[]}'), ('policy', '{"x":NaN}'),
        ('policy', '{"token":"PRIVATE_SYNTHETIC_CANARY"}')]:
    projection_run('invalid projected policy ' + key + repr(value), lambda p, e, k=key, v=value:
        projected_policy(p).update({k: v}))
projection_run('configured role inline policy', lambda p, e:
    record(p['configuration']['root_module'], 'resources', role)['expressions'].update(inline_policy=[]))
projection_run('unknown projected policy', lambda p, e:
    record(p, 'resource_drift', role)['change']['after_unknown'].update(inline_policy=[{'policy': True}]))
projection_run('changed standalone policy action', lambda p, e:
    record(p, 'resource_changes', policy)['change'].update(actions=['update']))
projection_run('independent policy payload mismatch', lambda p, e:
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources']
         if r['address'] == policy)['values'].update(policy='{}'))
projection_run('matching plan policy cannot replace source evidence', lambda p, e: (
    projected_policy(p).update(policy='{}'), planned(p, policy, 'policy', '{}')))
projection_run('independent policy name mismatch', lambda p, e:
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources']
         if r['address'] == policy)['values'].update(name='different'))
projection_run('policy role substitution', lambda p, e: planned(p, policy, 'role', 'different'))
projection_run('preexisting nonempty projection', lambda p, e: (
    record(p, 'resource_drift', role)['change']['before'].update(inline_policy=[copy.deepcopy(projected_policy(p))]),
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources']
         if r['address'] == role)['values'].update(inline_policy=[copy.deepcopy(projected_policy(p))])))
run('RDS recovery progresses before plan generation', lambda p, e:
    drift_after(p, db, 'latest_restorable_time', '2026-09-25T01:06:00Z'), True)
def keyless(p, e):
    d = record(p, 'resource_drift', host)['change']
    d['before']['key_name'] = d['after']['key_name'] = ''
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources']
         if r['address'] == host)['values']['key_name'] = ''
    record(p, 'resource_changes', host)['change']['after_unknown']['key_name'] = True
    # Terraform may serialize an explicit null expression or omit it.
    record(p['configuration']['root_module'], 'resources', host)['expressions']['key_name'] = {'constant_value': None}
run('provider keyless empty-to-unknown', keyless, True)
def collection_masks(p, e):
    c = record(p, 'resource_changes', host)['change']
    c['after_unknown'].update(
        vpc_security_group_ids=[False],
        metadata_options=[{'http_endpoint': False, 'http_tokens': False,
            'http_put_response_hop_limit': False, 'instance_metadata_tags': True}],
        root_block_device=[{'encrypted': False, 'volume_size': False,
            'volume_type': False, 'delete_on_termination': False,
            'device_name': True, 'volume_id': True, 'iops': True,
            'kms_key_id': True, 'throughput': True, 'tags_all': True}])
run('provider collection-shaped computed masks', collection_masks, True)
def raw_state(p, e):
    resources = []
    for r in e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources']:
        base = r['address'].split('[')[0]
        kind, name = base.split('.')
        instance = {'attributes': r['values']}
        if '[' in r['address']:
            instance['index_key'] = json.loads(r['address'].split('[', 1)[1][:-1])
        existing = next((x for x in resources if x['type'] == kind and x['name'] == name), None)
        if existing:
            existing['instances'].append(instance)
        else:
            resources.append({'mode': 'managed', 'type': kind, 'name': name, 'instances': [instance]})
    e['TF_INITIAL_CURRENT_STATE_JSON'] = {'version': 4, 'resources': resources}
run('raw Terraform state snapshot', raw_state, True)
def ami_data(p, e):
    raw_state(p, e)
    e['TF_INITIAL_CURRENT_STATE_JSON']['resources'].append({
        'mode': 'data', 'type': 'aws_ssm_parameter', 'name': 'amazon_linux_2023_arm64',
        'instances': [{'attributes': {
            'id': '/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64',
            'name': '/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64',
            'type': 'String', 'value': 'ami-0123456789abcdef0', 'insecure_value': None,
            'with_decryption': True},
            'sensitive_attributes': [[{'type': 'get_attr', 'value': 'value'}]]}]})
run('preserved public AMI data source with canonical value annotation', ami_data, True)
def unassociated_eip(p, e):
    c = record(p, 'resource_changes', 'aws_eip.origin')['change']
    for key in ('instance', 'network_interface'):
        c['after'].pop(key)
        c['after_unknown'][key] = True
run('provider unknown unattached EIP with live absence', unassociated_eip, True)
run('provider omitted EIP associations without unknown mask', lambda p, e: (
    record(p, 'resource_changes', 'aws_eip.origin')['change']['after'].pop('instance'),
    record(p, 'resource_changes', 'aws_eip.origin')['change']['after'].pop('network_interface')), True)

# Each relaxed provider representation remains source-bound and fail-closed.
def provider_shape(p, e):
    inline_projection(p, e)
    keyless(p, e)
    collection_masks(p, e)
    unassociated_eip(p, e)
    ami_data(p, e)
    drift_after(p, db, 'latest_restorable_time', '2026-09-25T01:06:00Z')
run('combined provider-v6 shape', provider_shape, True)
def provider_run(name, mutate, accept=False):
    run(name, lambda p, e: (provider_shape(p, e), mutate(p, e)), accept)
provider_run('omitted null key configuration', lambda p, e:
    record(p['configuration']['root_module'], 'resources', host)['expressions'].pop('key_name'), True)
for value in ('synthetic-key', '', None, False):
    provider_run('live KeyName present ' + repr(value), lambda p, e, v=value:
        e['TF_INITIAL_LIVE_HOST_JSON']['Reservations'][0]['Instances'][0].update(KeyName=v))
for expr in ({'constant_value': 'key'}, {'references': ['aws_key_pair.other.key_name']}, {}):
    provider_run('configured key ' + str(expr), lambda p, e, v=expr:
        record(p['configuration']['root_module'], 'resources', host)['expressions'].update(key_name=v))
for address in ('aws_key_pair.other', 'aws_eip_association.other'):
    provider_run('extra configuration ' + address, lambda p, e, a=address:
        p['configuration']['root_module']['resources'].append({'address': a, 'expressions': {}}))
    provider_run('extra planned action ' + address, lambda p, e, a=address:
        p['resource_changes'].append({'address': a, 'change': {'actions': ['create'], 'after': {}}}))
for key, value in [('metadata_options', True), ('root_block_device', True),
        ('vpc_security_group_ids', [True]), ('vpc_security_group_ids', []),
        ('root_block_device', {}), ('root_block_device', [False, False]),
        ('root_block_device', [{'encrypted': True}]),
        ('root_block_device', [{'volume_size': True}]),
        ('root_block_device', [{'unexpected': True}]),
        ('metadata_options', [{'http_tokens': True}]),
        ('metadata_options', [{'instance_metadata_tags': 1}]),
        ('metadata_options', [{'http_endpoint': 'false'}]),
        ('unexpected', True), ('unexpected', {'leaf': True}), ('user_data', True)]:
    provider_run('invalid unknown mask ' + key + str(value), lambda p, e, k=key, v=value:
        record(p, 'resource_changes', host)['change']['after_unknown'].update({k: v}))
provider_run('configured computed root leaf', lambda p, e:
    record(p['configuration']['root_module'], 'resources', host)['expressions'].update(
        root_block_device=[{'iops': {'constant_value': 3000}}]))
provider_run('unknown field with known value', lambda p, e:
    record(p, 'resource_changes', host)['change']['after'].update(public_ip='192.0.2.9'))
for key in ('instance', 'network_interface', 'associate_with_private_ip', 'association_id'):
    provider_run('EIP attached ' + key, lambda p, e, k=key:
        record(p, 'resource_changes', 'aws_eip.origin')['change']['after'].update({k: 'attached'}))
provider_run('EIP unexpected unknown', lambda p, e:
    record(p, 'resource_changes', 'aws_eip.origin')['change']['after_unknown'].update(unexpected=True))
provider_run('configured EIP attachment', lambda p, e:
    record(p['configuration']['root_module'], 'resources', 'aws_eip.origin')['expressions'].update(
        instance={'references': [host + '.id', host]}))
provider_run('live allocation still exists', lambda p, e:
    e['TF_INITIAL_LIVE_ADDRESSES_JSON']['Addresses'].append({'AllocationId': 'eipalloc-0123456789abcdef0'}))
provider_run('unrelated live allocation', lambda p, e:
    e['TF_INITIAL_LIVE_ADDRESSES_JSON']['Addresses'].append({'AllocationId': 'eipalloc-fedcba98765432100'}), True)
provider_run('state allocation identity mismatch', lambda p, e:
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['resources'] if r['type'] == 'aws_eip')
        ['instances'][0]['attributes'].update(allocation_id='eipalloc-fedcba98765432100'))
provider_run('missing state allocation identity', lambda p, e:
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['resources'] if r['type'] == 'aws_eip')
        ['instances'][0]['attributes'].pop('allocation_id'))
for records in ([{}], [{'AllocationId': None}], [{'AllocationId': 1}],
        [{'AllocationId': 'not-an-allocation'}], [{'AllocationId': 'eipalloc-ab'}, {'AllocationId': 'eipalloc-ab'}]):
    provider_run('ambiguous allocation records ' + str(records), lambda p, e, v=records:
        e['TF_INITIAL_LIVE_ADDRESSES_JSON'].update(Addresses=v))
for key in ('NextToken', 'IsTruncated', 'Error', 'unexpected'):
    provider_run('partial or invalid address response ' + key, lambda p, e, k=key:
        e['TF_INITIAL_LIVE_ADDRESSES_JSON'].update({k: 'synthetic'}))
def data_record(e):
    return next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['resources'] if r['mode'] == 'data')
for key, value in [('name', 'other'), ('type', 'aws_secretsmanager_secret_version'), ('module', 'module.other')]:
    provider_run('extra or moved data source ' + key, lambda p, e, k=key, v=value: data_record(e).update({k: v}))
provider_run('duplicate data source', lambda p, e:
    e['TF_INITIAL_CURRENT_STATE_JSON']['resources'].append(copy.deepcopy(data_record(e))))
provider_run('extra data source instance', lambda p, e:
    data_record(e)['instances'].append(copy.deepcopy(data_record(e)['instances'][0])))
for key, value in [('value', 'ami-wrong'), ('value', None), ('type', 'SecureString'),
        ('name', '/private/parameter'), ('id', '/private/parameter'),
        ('insecure_value', 'PRIVATE_VALUE'), ('password', 'PRIVATE_VALUE'), ('sensitive', True)]:
    provider_run('unsafe AMI data ' + key, lambda p, e, k=key, v=value:
        data_record(e)['instances'][0]['attributes'].update({k: v}))
for value in (None, True, [], [[]], [['value']], [{'type': 'get_attr', 'value': 'value'}],
        [[{'type': 'get_attr', 'value': 'name'}]],
        [[{'type': 'get_attr', 'value': '*'}]],
        [[{'type': 'index', 'value': 'value'}]],
        [[{'type': 'get_attr', 'value': 0}]],
        [[{'type': 'get_attr', 'value': 'value', 'extra': True}]],
        [[{'type': 'get_attr'}]], [[{'value': 'value'}]],
        [[{'type': 'get_attr', 'value': 'value'}, {'type': 'index', 'value': 0}]],
        [[{'type': 'get_attr', 'value': 'value'}], [{'type': 'get_attr', 'value': 'value'}]],
        [[{'type': 'get_attr', 'value': 'value'}], [{'type': 'get_attr', 'value': 'insecure_value'}]]):
    provider_run('sensitive AMI data ' + str(value), lambda p, e, v=value:
        data_record(e)['instances'][0].update(sensitive_attributes=v))
provider_run('missing AMI sensitivity annotation', lambda p, e:
    data_record(e)['instances'][0].pop('sensitive_attributes'))
provider_run('valid but mismatched AMI data value', lambda p, e:
    data_record(e)['instances'][0]['attributes'].update(value='ami-fedcba98765432100'))
for timestamp in ('2026-09-25T01:05:59Z', 'not-a-time', None):
    provider_run('invalid plan generation time ' + str(timestamp), lambda p, e, v=timestamp: p.update(timestamp=v))
provider_run('missing plan generation time', lambda p, e: p.pop('timestamp'))
provider_run('observed recovery later than refresh', lambda p, e:
    e['TF_INITIAL_LIVE_RDS_JSON']['DBInstances'][0].update(LatestRestorableTime='2026-09-25T01:07:00Z'))
provider_run('refresh recovery after plan generation', lambda p, e:
    drift_after(p, db, 'latest_restorable_time', '2026-09-25T01:08:01Z'))
provider_run('time ordering with equivalent offsets', lambda p, e:
    p.update(timestamp='2026-09-25T10:08:00+09:00'), True)
def non_ami_value(p, e):
    value = 'PRIVATE_VALUE'
    data_record(e)['instances'][0]['attributes']['value'] = value
    e['TF_INITIAL_TFVARS_JSON']['replacement_host_ami_id'] = value
    p['variables']['replacement_host_ami_id']['value'] = value
    for section in ('resource_drift', 'resource_changes'):
        c = record(p, section, host)['change']
        c['before']['ami'] = c['after']['ami'] = value
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['resources']
         if r['type'] == 'aws_instance')['instances'][0]['attributes']['ami'] = value
provider_run('self-consistent non-AMI data value', non_ami_value)
run('unchanged unbound managed policy', lambda p, e: (
    record(p, 'resource_drift', role)['change']['before'].update(managed_policy_arns=['unexpected']),
    drift_after(p, role, 'managed_policy_arns', ['unexpected']),
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources']
         if r['address'] == role)['values'].update(managed_policy_arns=['unexpected'])))
run('inline policy retained nonempty', lambda p, e: (
    record(p, 'resource_drift', role)['change']['before'].update(inline_policy=[{'name': 'unexpected'}]),
    drift_after(p, role, 'inline_policy', [{'name': 'unexpected'}]),
    next(r for r in e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources']
         if r['address'] == role)['values'].update(inline_policy=[{'name': 'unexpected'}])))
run('unrefreshed IAM attachment state substitution', lambda p, e:
    planned(p, attachment, 'id', 'different-attachment'))
run('configured root tags', lambda p, e:
    next(r for r in p['configuration']['root_module']['resources'] if r['address'] == host)
        ['expressions'].update(root_block_device=[{'tags': {'constant_value': {}}}]))
for address, key, value in [
    (host, 'public_ip', '192.0.2.99'), (host, 'public_dns', 'other.example.invalid'),
    (host, 'tags', {'Name': 'unexpected'}), (profile, 'tags', {'Name': 'unexpected'}),
    (role, 'tags', {'Name': 'unexpected'}), (role, 'inline_policy', [{'name': 'unexpected'}]),
    (role, 'managed_policy_arns', ['arn:aws:iam::aws:policy/AdministratorAccess']),
    (role, 'managed_policy_arns', []), (db, 'latest_restorable_time', '2026-09-25T01:04:59Z'),
    (db, 'latest_restorable_time', '2026-09-25T01:08:01Z'),
    (db, 'latest_restorable_time', '2999-01-01T00:00:00Z'),
    (db, 'latest_restorable_time', 'not-a-time'), (db, 'allocated_storage', 21),
    (db, 'publicly_accessible', True), (db, 'storage_encrypted', 1),
    (db, 'identifier', 'other'), (host, 'iam_instance_profile', 'other'),
    (profile, 'role', 'other'), (role, 'assume_role_policy', 'unexpected'),
    (role, 'password', 'PRIVATE_VALUE'), (host, 'user_data', '# changed'),
]:
    run(address + ' ' + key, lambda p, e, a=address, k=key, v=value: drift_after(p, a, k, v))
for key, value in [('volume_size', 9), ('encrypted', 1), ('tags', {'Name': 'unexpected'}),
                   ('volume_id', 'other')]:
    run('root disk ' + key, lambda p, e, k=key, v=value:
        record(p, 'resource_drift', host)['change']['after']['root_block_device'][0].update({k: v}))
for address in (host, db, role, profile):
    run('extra drift attr ' + address, lambda p, e, a=address: drift_after(p, a, 'unexpected', True))
    run('unknown drift ' + address, lambda p, e, a=address:
        record(p, 'resource_drift', a)['change'].update(after_unknown={'id': True}))
    run('drift import ' + address, lambda p, e, a=address:
        record(p, 'resource_drift', a)['change'].update(importing={'id': 'unexpected'}))
    run('drift move ' + address, lambda p, e, a=address:
        record(p, 'resource_drift', a).update(previous_address='old'))
    run('coupled planned update ' + address, lambda p, e, a=address:
        record(p, 'resource_changes', a)['change'].update(actions=['update']))
run('database replacement', lambda p, e:
    record(p, 'resource_changes', db)['change'].update(actions=['delete', 'create']))
run('extra drift resource', lambda p, e: p['resource_drift'].append({
    'address': 'aws_route53_record.origin', 'change': {'actions': ['update'], 'before': {}, 'after': {}}}))
run('duplicate drift', lambda p, e: p['resource_drift'].append(copy.deepcopy(p['resource_drift'][0])))
run('missing drift with changed state', lambda p, e: p['resource_drift'].pop())
run('altered attachment binding', lambda p, e: planned(p, attachment, 'role', 'other'))
run('altered runtime policy binding', lambda p, e: planned(p, policy, 'role', 'other'))
run('configured public IP', lambda p, e:
    next(r for r in p['configuration']['root_module']['resources'] if r['address'] == host)
        ['expressions'].update(public_ip={'constant_value': '192.0.2.2'}))
for key in env_files:
    run('extra evidence secret ' + key, lambda p, e, k=key: e[k].update(password='PRIVATE_VALUE'))
    run('missing evidence structure ' + key, lambda p, e, k=key: e[k].clear())
run('host identity', lambda p, e: e['TF_INITIAL_LIVE_HOST_JSON']['Reservations'][0]['Instances'][0].update(InstanceId='other'))
run('extra reservation', lambda p, e: e['TF_INITIAL_LIVE_HOST_JSON']['Reservations'].append({'Instances': []}))
run('extra instance', lambda p, e: e['TF_INITIAL_LIVE_HOST_JSON']['Reservations'][0]['Instances'].append({}))
run('missing host IP', lambda p, e: e['TF_INITIAL_LIVE_HOST_JSON']['Reservations'][0]['Instances'][0].pop('PublicIpAddress'))
run('malformed host DNS', lambda p, e: e['TF_INITIAL_LIVE_HOST_JSON']['Reservations'][0]['Instances'][0].update(PublicDnsName=[]))
run('database identity', lambda p, e: e['TF_INITIAL_LIVE_RDS_JSON']['DBInstances'][0].update(DBInstanceIdentifier='other'))
run('database resource identity', lambda p, e: e['TF_INITIAL_LIVE_RDS_JSON']['DBInstances'][0].update(DbiResourceId='other'))
run('extra database', lambda p, e: e['TF_INITIAL_LIVE_RDS_JSON']['DBInstances'].append({}))
run('missing database time', lambda p, e: e['TF_INITIAL_LIVE_RDS_JSON']['DBInstances'][0].pop('LatestRestorableTime'))
run('normalized RFC3339 timezone', lambda p, e:
    e['TF_INITIAL_LIVE_RDS_JSON']['DBInstances'][0].update(LatestRestorableTime='2026-09-25T10:05:00.000000+09:00'), True)
run('wrong reconstructed AMI', lambda p, e: e['TF_INITIAL_TFVARS_JSON'].update(replacement_host_ami_id='other'))
run('unknown reconstructed input', lambda p, e: e['TF_INITIAL_TFVARS_JSON'].update(other=True))
run('omitted default subnet input', lambda p, e: e['TF_INITIAL_TFVARS_JSON'].pop('replacement_public_subnet_key'), True)
run('missing state resource', lambda p, e: e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources'].pop())
run('duplicate state resource', lambda p, e: e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources'].append(
    copy.deepcopy(e['TF_INITIAL_CURRENT_STATE_JSON']['values']['root_module']['resources'][0])))

# Each environment input has the same generic, value-blind filesystem/JSON gate.
args = [checker, str(root / 'plan-initial-refresh.json'), str(root / 'cost-valid.json'),
    str(root / 'current-checkpoint-initial-valid.json'), str(root / 'replacement-user-data'), sha, 'initial-deployment']
for key, filename in env_files.items():
    for case in ('missing', 'unset', 'directory', 'symlink', 'public', 'empty', 'stream', 'duplicate-key', 'malformed', 'null'):
        env = dict(os.environ)
        bad = root / 'bad-evidence'
        if bad.exists() or bad.is_symlink():
            bad.unlink()
        if case == 'unset':
            env.pop(key)
        else:
            env[key] = str(bad)
        if case == 'directory':
            env[key] = str(root)
        elif case == 'symlink':
            bad.symlink_to(root / filename)
        elif case not in ('missing', 'unset'):
            content = {'empty': '', 'stream': '{}\n{}', 'duplicate-key': '{"x":1,"x":2}',
                'malformed': '{"PRIVATE_VALUE":', 'null': 'null'}.get(case, (root / filename).read_text())
            bad.write_text(content)
            bad.chmod(0o644 if case == 'public' else 0o600)
        result = subprocess.run(args, env=env, capture_output=True, text=True)
        if result.returncode == 0 or result.stdout or result.stderr != 'Terraform saved plan violates the Slice 4 contract\n':
            sys.exit('FAIL: evidence boundary ' + key + ' ' + case)
        print('PASS: evidence boundary ' + key + ' ' + case)
PY

jq '.resource_drift = [{address:"aws_eip.origin", change:{actions:["delete"],before:{domain:"vpc",id:"eipalloc-0123456789abcdef0",allocation_id:"eipalloc-0123456789abcdef0"},after:null}}]' \
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
initial_mutation 0 'EIP unknown private-IP control' '(.resource_changes[] | select(.address == "aws_eip.origin") | .change.after_unknown.associate_with_private_ip) = true'
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

# Exercise the same provider-v6 boundary in all three replacement lanes.
# In particular, the old hash-only fixture must now fail rather than silently
# admitting a second representation. Every rejection must remain value-blind.
for user_data_filter in \
  ".change.after.user_data = \"$replacement_user_data_hash\"" \
  '.change.after.user_data |= . + "# PRIVATE_VALUE injected command\n"' \
  '.change.after.user_data |= sub("systemctl stop"; "systemctl start")' \
  '.change.after.user_data |= rtrimstr("\n")' \
  '.change.after.user_data |= . + "\n"' \
  '.change.after.user_data |= gsub("\n"; "\r\n")' \
  '.change.after.user_data |= @base64' \
  '.change.after.user_data = null' \
  'del(.change.after.user_data)' \
  '.change.after.user_data = {}' \
  '.change.after.user_data = ""' \
  '.change.after_unknown.user_data = true' \
  '.change.before.user_data = .change.after.user_data'; do
  user_data_mutation="(.resource_changes[] | select(.address == \"aws_instance.replacement_host\")) |= ($user_data_filter)"
  replacement_plan_mutation 'provider-v6-user-data' "$user_data_mutation"
  combined_recovery_plan_mutation 'provider-v6-user-data' "$user_data_mutation"
  initial_mutation 0 'provider-v6-user-data' "$user_data_mutation"
done

# A matching render cannot legitimize a recognizable secret in initial mode.
cp "$fixture_root/replacement-user-data" "$fixture_root/user-data-secret"
printf '\n# postgresql://synthetic:PRIVATE_VALUE@invalid/db\n' >>"$fixture_root/user-data-secret"
jq --rawfile payload "$fixture_root/user-data-secret" \
  '(.resource_changes[] | select(.address == "aws_instance.replacement_host") |
    .change.after.user_data) = $payload' \
  "$fixture_root/plan-initial-valid.json" >"$fixture_root/plan-user-data-secret.json"
expect_recovery_invocation_rejected 'initial matching user-data secret' \
  "$fixture_root/plan-user-data-secret.json" "$fixture_root/cost-valid.json" \
  "$fixture_root/current-checkpoint-initial-valid.json" "$fixture_root/user-data-secret" \
  "$reviewed_sha" initial-deployment

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
