#!/bin/bash -p
set -euo pipefail

fail() {
  printf 'Terraform saved plan violates the Slice 4 contract\n' >&2
  exit 1
}

# Internal implementation: use check-terraform-slice-4-plan.sh, which always
# starts this body in a cleared environment with privileged Bash startup.
# Resolve tools only from root-owned, non-group/other-writable directories.
# Absolute-path stat probes cannot be replaced by caller PATH wrappers.
slice4_stat_bin=/usr/bin/stat
[[ -x "$slice4_stat_bin" ]] || slice4_stat_bin=/bin/stat
[[ -x "$slice4_stat_bin" ]] || fail
if "$slice4_stat_bin" -f '%u' / >/dev/null 2>&1; then
  slice4_stat_bsd=yes
else
  slice4_stat_bsd=no
fi

slice4_dir_trusted() {
  local dir="$1" uid mode

  if [[ "$slice4_stat_bsd" == yes ]]; then
    uid="$("$slice4_stat_bin" -L -f '%u' "$dir" 2>/dev/null)" || return 1
    mode="$("$slice4_stat_bin" -L -f '%Lp' "$dir" 2>/dev/null)" || return 1
  else
    uid="$("$slice4_stat_bin" -c '%u' "$dir" 2>/dev/null)" || return 1
    mode="$("$slice4_stat_bin" -c '%a' "$dir" 2>/dev/null)" || return 1
  fi
  [[ "$uid" == 0 ]] || return 1
  case "$mode" in
    '' | *[!0-7]*) return 1 ;;
  esac
  (( (8#$mode & 8#22) == 0 )) || return 1
  return 0
}

slice4_trusted_path() {
  local dir sanitized=''

  for dir in /usr/local/bin /usr/bin /bin; do
    if [[ -d "$dir" ]] && slice4_dir_trusted "$dir"; then
      if [[ -n "$sanitized" ]]; then
        sanitized="$sanitized:$dir"
      else
        sanitized="$dir"
      fi
    fi
  done
  case ":$sanitized:" in
    *:/usr/bin:* | *:/bin:*) ;;
    *) return 1 ;;
  esac
  printf '%s\n' "$sanitized"
}

trusted_path="$(slice4_trusted_path)" || fail
export PATH="$trusted_path"

[[ "$#" -eq 3 || "$#" -eq 5 || "$#" -eq 6 ]] || fail

plan_json="$1"
cost_json="$2"
checkpoint_json="$3"
mode=create
expected_user_data_hash=
reviewed_sha=

file_mode() {
  local mode

  if mode="$(stat -f '%Lp' "$1" 2>/dev/null)"; then
    printf '%s\n' "$mode"
  else
    stat -c '%a' "$1" 2>/dev/null
  fi
}

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

if [[ "$#" -ge 5 ]]; then
  mode=replacement
  rendered_user_data="$4"
  reviewed_sha="$5"
  [[ "$reviewed_sha" =~ ^[0-9a-f]{40}$ ]] || fail

  if [[ "$#" -eq 6 ]]; then
    [[ "$6" == combined-recovery ]] || fail
    mode=combined-recovery
  fi

  repo_root="$(CDPATH='' cd -P "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)" || fail

  run_git() {
    env -i PATH=/usr/bin:/bin HOME=/dev/null \
      GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_SYSTEM=/dev/null \
      GIT_CONFIG_GLOBAL=/dev/null GIT_NO_REPLACE_OBJECTS=1 \
      git -c core.hooksPath=/dev/null \
        -c core.attributesFile=/dev/null \
        -c core.excludesFile=/dev/null \
        -c core.fsmonitor=false \
        "$@"
  }

  git_root="$(run_git -C "$repo_root" rev-parse --show-toplevel 2>/dev/null)" || fail
  [[ "$git_root" == "$repo_root" ]] || fail
  if run_git -C "$repo_root" config --local --name-only --get-regexp \
    '^(alias\.|core\.(attributesfile|excludesfile|hookspath|sshcommand|fsmonitor|worktree)$|diff\.external$|filter\.|include\.|includeif\.|extensions\.|status\.showuntrackedfiles$|remote\..*\.(promisor|partialclonefilter)$)' \
    >/dev/null 2>&1; then
    fail
  fi
  git_dir="$(run_git -C "$repo_root" rev-parse --git-dir 2>/dev/null)" || fail
  case "$git_dir" in
    /*) ;;
    *) git_dir="$repo_root/$git_dir" ;;
  esac
  [[ ! -e "$git_dir/config.worktree" ]] || fail
  [[ -f "$repo_root/scripts/check-terraform-slice-4-plan.sh" &&
    ! -L "$repo_root/scripts/check-terraform-slice-4-plan.sh" ]] || fail
  run_git -C "$repo_root" ls-files --error-unmatch -- \
    scripts/check-terraform-slice-4-plan.sh >/dev/null 2>&1 || fail
  [[ -f "$repo_root/scripts/check-terraform-slice-4-plan-body.sh" &&
    ! -L "$repo_root/scripts/check-terraform-slice-4-plan-body.sh" ]] || fail
  run_git -C "$repo_root" ls-files --error-unmatch -- \
    scripts/check-terraform-slice-4-plan-body.sh >/dev/null 2>&1 || fail
  [[ "$(run_git -C "$repo_root" rev-parse HEAD 2>/dev/null)" == \
    "$reviewed_sha" ]] || fail
  run_git -C "$repo_root" cat-file -e "$reviewed_sha^{commit}" \
    2>/dev/null || fail
  [[ -z "$(run_git -C "$repo_root" status --porcelain=v1 \
    --untracked-files=all 2>/dev/null)" ]] || fail
  [[ -z "$(run_git -C "$repo_root" for-each-ref \
    --format='%(refname)' refs/replace 2>/dev/null)" ]] || fail

  [[ -f "$rendered_user_data" && ! -L "$rendered_user_data" ]] || fail
  [[ "$(file_mode "$rendered_user_data")" == 600 ]] || fail
  [[ -s "$rendered_user_data" ]] || fail

  while IFS='|' read -r target source; do
    [[ -f "$repo_root/$source" && ! -L "$repo_root/$source" ]] || fail
    run_git -C "$repo_root" ls-files --error-unmatch -- "$source" \
      >/dev/null 2>&1 || fail
    asset_digest="$(digest "$repo_root/$source")" || fail
    grep -F "$target" "$rendered_user_data" >/dev/null 2>&1 || fail
    grep -F "$asset_digest" "$rendered_user_data" >/dev/null 2>&1 || fail
  done <<'ASSETS'
/opt/jobcron/compose.yaml|deploy/production/compose.yaml
/opt/jobcron/Caddyfile|deploy/production/Caddyfile
/opt/jobcron/jobcron-runtime.sh|deploy/production/jobcron-runtime.sh
/etc/systemd/system/jobcron.service|deploy/production/systemd/jobcron.service
/etc/systemd/system/jobcron-recovery.service|deploy/production/systemd/jobcron-recovery.service
/etc/systemd/system/jobcron-recovery.timer|deploy/production/systemd/jobcron-recovery.timer
ASSETS

  for marker in \
    docker-compose-linux-aarch64 \
    ff42489f5a9b879d5d117c5ffea6defc27390b3286da8ad52cbc9c6ab5df590e \
    'sha256sum -c -' \
    /etc/jobcron/runtime-secret-id \
    'systemctl enable docker.service' \
    'systemctl start docker.service' \
    'systemctl stop jobcron.service'; do
    grep -F "$marker" "$rendered_user_data" >/dev/null 2>&1 || fail
  done
  ! grep -F 'required deployment asset missing' \
    "$rendered_user_data" >/dev/null 2>&1 || fail

  if command -v sha1sum >/dev/null 2>&1; then
    expected_user_data_hash="$(sha1sum "$rendered_user_data" | awk '{print $1}')"
  else
    expected_user_data_hash="$(shasum "$rendered_user_data" | awk '{print $1}')"
  fi
fi

command -v jq >/dev/null 2>&1 || fail
[[ -f "$plan_json" && -f "$cost_json" && -f "$checkpoint_json" ]] || fail

# Slurp forces one evaluation even on empty input (jq 1.6 otherwise exits 0),
# and rejects streams before applying policy to the same parsed document.
jq -es --arg mode "$mode" --arg expected_user_data_hash "$expected_user_data_hash" '
  (if length == 1 then .[0] else error("invalid") end) |
  [
    "aws_iam_role.replacement_host",
    "aws_iam_role_policy_attachment.replacement_host_ssm",
    "aws_iam_role_policy.replacement_host_runtime",
    "aws_iam_instance_profile.replacement_host",
    "aws_instance.replacement_host"
  ] as $allowed_creates |
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
  ] as $allowed_noops |
  ([.resource_changes[] | select(.address == "aws_instance.replacement_host")][0] // {}) as $instance |
  ([.resource_changes[] | select(.address == "aws_eip.origin")][0] // {}) as $eip |
  (.resource_changes | type == "array") and
  ([.resource_changes[].address] | length == (unique | length)) and
  (
    if $mode == "create" then
      (
        [
          .resource_changes[] |
          select(.change.actions == ["create"]) |
          .address
        ] | sort == ($allowed_creates | sort)
      ) and
      all(
        .resource_changes[];
        (.address | type == "string") and
        (.previous_address // null) == null and
        (.change | type == "object") and
        (.change.importing // null) == null and
        (
          (
            .change.actions == ["create"] and
            (.address as $address | $allowed_creates | index($address)) != null
          ) or
          (
            .change.actions == ["no-op"] and
            (.address as $address | $allowed_noops | index($address)) != null
          )
        )
      )
    elif ($mode == "replacement" or $mode == "combined-recovery") then
      ([.resource_changes[].address] | sort == (($allowed_creates + $allowed_noops) | sort)) and
      all(
        .resource_changes[];
        (.address | type == "string") and
        (.previous_address // null) == null and
        (.change | type == "object") and
        (.change.importing // null) == null and
        (
          if .address == "aws_instance.replacement_host" then
            (.action_reason == "replace_by_request") and
            (.change.actions == ["delete", "create"])
          elif ($mode == "combined-recovery" and .address == "aws_eip.origin") then
            (.action_reason // null) == null and
            (.change.actions == ["create"])
          else
            .change.actions == ["no-op"]
          end
        )
      ) and
      (
        if $mode == "combined-recovery" then
          ($eip.change | has("before")) and
          ($eip.change.before == null) and
          ($eip.change.after | type == "object") and
          ($eip.change.after | has("domain")) and
          ($eip.change.after.domain == "vpc") and
          ($eip.change.after | has("instance")) and
          ($eip.change.after.instance == null) and
          ($eip.change.after | has("network_interface")) and
          ($eip.change.after.network_interface == null) and
          ($eip.change.after | has("associate_with_private_ip")) and
          ($eip.change.after.associate_with_private_ip == null) and
          (($eip.change.after_unknown.domain // false) == false) and
          (($eip.change.after_unknown.instance // false) == false) and
          (($eip.change.after_unknown.network_interface // false) == false) and
          (($eip.change.after_unknown.associate_with_private_ip // false) == false)
        else
          true
        end
      ) and
      ($instance.change.before | type == "object") and
      ($instance.change.after | type == "object") and
      ($instance.change.before.ami | type == "string") and
      ($instance.change.before.ami | length > 0) and
      ($instance.change.after.ami == $instance.change.before.ami) and
      ($instance.change.before.instance_type | type == "string") and
      ($instance.change.before.instance_type | length > 0) and
      ($instance.change.after.instance_type == $instance.change.before.instance_type) and
      ($instance.change.before.subnet_id | type == "string") and
      ($instance.change.before.subnet_id | length > 0) and
      ($instance.change.after.subnet_id == $instance.change.before.subnet_id) and
      ($instance.change.before.iam_instance_profile | type == "string") and
      ($instance.change.before.iam_instance_profile | length > 0) and
      ($instance.change.after.iam_instance_profile == $instance.change.before.iam_instance_profile) and
      ($instance.change.before.key_name == null) and
      ($instance.change.after.key_name == null) and
      ($instance.change.before.associate_public_ip_address == true) and
      ($instance.change.after.associate_public_ip_address == true) and
      ($instance.change.before.vpc_security_group_ids | type == "array") and
      ($instance.change.after.vpc_security_group_ids | type == "array") and
      ($instance.change.after.vpc_security_group_ids | length == 1) and
      ($instance.change.after.vpc_security_group_ids == $instance.change.before.vpc_security_group_ids) and
      ($instance.change.after.metadata_options | type == "array") and
      ($instance.change.after.metadata_options | length == 1) and
      ($instance.change.after.metadata_options[0].http_endpoint == "enabled") and
      ($instance.change.after.metadata_options[0].http_tokens == "required") and
      ($instance.change.after.metadata_options[0].http_put_response_hop_limit == 1) and
      ($instance.change.after.root_block_device | type == "array") and
      ($instance.change.after.root_block_device | length == 1) and
      ($instance.change.after.root_block_device[0].encrypted == true) and
      ($instance.change.after.root_block_device[0].volume_type == "gp3") and
      ($instance.change.after.root_block_device[0].volume_size == 8) and
      ($instance.change.after.root_block_device[0].delete_on_termination == true) and
      ($instance.change.before.user_data | type == "string") and
      ($instance.change.after.user_data == $expected_user_data_hash) and
      ($instance.change.before.user_data != $instance.change.after.user_data) and
      (($instance.change.after_unknown.key_name // false) == false) and
      (($instance.change.after_unknown.associate_public_ip_address // false) == false) and
      (($instance.change.after_unknown.ami // false) == false) and
      (($instance.change.after_unknown.instance_type // false) == false) and
      (($instance.change.after_unknown.subnet_id // false) == false) and
      (($instance.change.after_unknown.iam_instance_profile // false) == false) and
      (($instance.change.after_unknown.vpc_security_group_ids // false) == false) and
      (($instance.change.after_unknown.metadata_options // false) == false) and
      (($instance.change.after_unknown.root_block_device // false) == false) and
      (($instance.change.after_unknown.user_data // false) == false) and
      ((.diagnostics // []) == [])
    else
      false
    end
  ) and
  all(
    .resource_changes[];
    all(
      [(.change.after // {}) | .. | objects | .from_port?, .to_port?][];
      . != 22
    )
  ) and
  (.output_changes | type == "object") and
  (.output_changes | keys == ["replacement_instance_id"]) and
  (.output_changes.replacement_instance_id | type == "object") and
  (
    if $mode == "create" then
      .output_changes.replacement_instance_id.actions == ["create"]
    else
      (.output_changes.replacement_instance_id.actions == ["update"]) and
      (.output_changes.replacement_instance_id.before | type == "string") and
      (.output_changes.replacement_instance_id.after == null) and
      (.output_changes.replacement_instance_id.after_unknown == true) and
      (.output_changes.replacement_instance_id.before_sensitive == true)
    end
  ) and
  (.output_changes.replacement_instance_id.after_sensitive == true) and
  (
    (.diagnostics // []) |
    type == "array" and
    all(
      .[];
      (
        tostring |
        test("private[-_ ]?value|secret[-_ ]?value|sensitive[-_ ]?value"; "i") |
        not
      )
    )
  )
' "$plan_json" >/dev/null 2>&1 || fail

jq -es '
  (if length == 1 then .[0] else error("invalid") end) |
  [
    "aws_compute",
    "public_ipv4",
    "database",
    "storage",
    "backup",
    "registry",
    "cloudflare"
  ] as $required |
  . as $cost |
  (try ($cost.checked_at | fromdateiso8601) catch null) as $checked |
  ([$cost.categories[]?.recurring_monthly_upper_bound] | add) as $recurring_sum |
  ([$cost.categories[]?.one_time_upper_bound] | add) as $one_time_sum |
  ($cost.checked_at | type == "string") and
  (
    ($checked != null) and
    ((now - $checked) >= 0) and
    ((now - $checked) <= 86400)
  ) and
  ($cost.currency == "USD") and
  ($cost.categories | type == "array") and
  ($cost.categories | length == 7) and
  ([$cost.categories[].name] | sort == ($required | sort)) and
  ([$cost.categories[].name] | length == (unique | length)) and
  all(
    $cost.categories[];
    (.name | type == "string") and
    (.source | type == "string") and
    (.source | test("\\S")) and
    (.quantity | type == "number") and
    (.quantity >= 0) and
    (.recurring_monthly_upper_bound | type == "number") and
    (.recurring_monthly_upper_bound >= 0) and
    (.one_time_upper_bound | type == "number") and
    (.one_time_upper_bound >= 0)
  ) and
  ($cost.aggregate | type == "object") and
  ($cost.aggregate.recurring_monthly_upper_bound | type == "number") and
  ($cost.aggregate.one_time_upper_bound | type == "number") and
  ($cost.aggregate.recurring_monthly_upper_bound >= $recurring_sum) and
  ($cost.aggregate.one_time_upper_bound >= $one_time_sum) and
  ($cost.aggregate.recurring_monthly_upper_bound <= 100) and
  ($cost.aggregate.one_time_upper_bound <= 200)
' "$cost_json" >/dev/null 2>&1 || fail

if [[ "$mode" == create ]]; then
  jq -es '
    (if length == 1 then .[0] else error("invalid") end) |
    [
      "aws_db_instance.production",
      "aws_security_group.origin",
      "aws_security_group.database",
      "aws_vpc_security_group_ingress_rule.database_postgresql_from_origin",
      "aws_secretsmanager_secret.runtime",
      "aws_s3_bucket.recovery",
      "aws_s3_bucket_lifecycle_configuration.recovery"
    ] as $required_addresses |
    (.checked_at | type == "string") and
    ((try (.checked_at | fromdateiso8601) catch null) as $checked |
      ($checked != null) and
      ((now - $checked) >= 0) and
      ((now - $checked) <= 86400)) and
    (.commit | type == "string") and
    (.commit | test("^[0-9a-f]{40}$")) and
    (.verdict == "PASS") and
    (.post_apply_plan == "clean") and
    (.addresses | type == "array") and
    ([.addresses[]] | sort == ($required_addresses | sort)) and
    ([.addresses[]] | length == (unique | length)) and
    (.private_rds == true) and
    (.runtime_secret_versions == 0) and
    (.recovery_bucket | type == "object") and
    (.recovery_bucket.encrypted == true) and
    (.recovery_bucket.versioned == true) and
    (.recovery_bucket.public_access_blocked == true) and
    (.recovery_bucket.verified_retention_days >= 14) and
    (.recovery_bucket.unverified_retention_days >= 90) and
    (.recovery_lifecycle_verdict == "PASS") and
    (.destroy_or_replace == 0) and
    (.old_resource_changes == 0)
  ' "$checkpoint_json" >/dev/null 2>&1 || fail
else
  jq -es --arg mode "$mode" --arg reviewed_sha "$reviewed_sha" '
    (if length == 1 then .[0] else error("invalid") end) |
    (. | type == "object") and
    (keys == [
      "bootstrap_host",
      "checked_at",
      "commit",
      "legacy_rollback_host",
      "managed_eip",
      "origin",
      "public_cutover",
      "rds",
      "recovery_bucket",
      "runtime_secret",
      "schema_version",
      "selected_state_resources",
      "state_backend"
    ]) and
    (.schema_version == "human-assisted-reconciliation-v1") and
    (.checked_at | type == "string") and
    ((try (.checked_at | fromdateiso8601) catch null) as $checked |
      ($checked != null) and
      ((now - $checked) >= 0) and
      ((now - $checked) <= 86400)) and
    (.commit | type == "object") and
    (.commit | keys == ["clean", "exact", "sha"]) and
    (.commit.sha | type == "string") and
    (.commit.sha | test("^[0-9a-f]{40}$")) and
    (.commit.sha == $reviewed_sha) and
    (.commit.exact == true) and
    (.commit.clean == true) and
    (.selected_state_resources | type == "object") and
    (.selected_state_resources | keys == ["bootstrap_host", "database"]) and
    (.selected_state_resources.bootstrap_host | type == "object") and
    (.selected_state_resources.bootstrap_host | keys == ["managed", "terraform_address"]) and
    (.selected_state_resources.bootstrap_host.terraform_address == "aws_instance.replacement_host") and
    (.selected_state_resources.bootstrap_host.managed == true) and
    (.selected_state_resources.database | type == "object") and
    (.selected_state_resources.database | keys == ["managed", "terraform_address"]) and
    (.selected_state_resources.database.terraform_address == "aws_db_instance.production") and
    (.selected_state_resources.database.managed == true) and
    (.bootstrap_host | type == "object") and
    (.bootstrap_host | keys == ["disposition", "human_approved"]) and
    (.bootstrap_host.disposition == "replace") and
    (.bootstrap_host.human_approved == true) and
    (.legacy_rollback_host | type == "object") and
    (.legacy_rollback_host | keys == ["retained"]) and
    (.legacy_rollback_host.retained == true) and
    (.managed_eip | type == "object") and
    (.managed_eip | keys == ["state_presence", "terraform_address", "unattached"]) and
    (.managed_eip.terraform_address == "aws_eip.origin") and
    (.managed_eip.unattached == true) and
    (if $mode == "combined-recovery" then
       .managed_eip.state_presence == "absent"
     else
       .managed_eip.state_presence == "present"
     end) and
    (.origin | type == "object") and
    (.origin | keys == ["ingress_rule_count", "phase"]) and
    (.origin.ingress_rule_count == 0) and
    (.origin.phase == "private") and
    (.rds | type == "object") and
    (.rds | keys == [
      "backup_retention_days",
      "deletion_protection",
      "latest_restorable_time_observed",
      "publicly_accessible",
      "status",
      "storage_encrypted"
    ]) and
    (.rds.status == "available") and
    (.rds.publicly_accessible == false) and
    (.rds.storage_encrypted == true) and
    (.rds.deletion_protection == true) and
    (.rds.backup_retention_days | type == "number") and
    (.rds.backup_retention_days | floor == .) and
    (.rds.backup_retention_days >= 1) and
    (.rds.latest_restorable_time_observed == true) and
    (.runtime_secret | type == "object") and
    (.runtime_secret | keys == [
      "container_exists",
      "observed_version_count",
      "terraform_manages_versions"
    ]) and
    (.runtime_secret.container_exists == true) and
    (.runtime_secret.terraform_manages_versions == false) and
    (.runtime_secret.observed_version_count | type == "number") and
    (.runtime_secret.observed_version_count | floor == .) and
    (.runtime_secret.observed_version_count >= 0) and
    (.recovery_bucket | type == "object") and
    (.recovery_bucket | keys == [
      "encrypted",
      "lifecycle_verified",
      "public_access_blocked",
      "tls_only",
      "versioned"
    ]) and
    (.recovery_bucket.encrypted == true) and
    (.recovery_bucket.versioned == true) and
    (.recovery_bucket.public_access_blocked == true) and
    (.recovery_bucket.tls_only == true) and
    (.recovery_bucket.lifecycle_verified == true) and
    (.state_backend | type == "object") and
    (.state_backend | keys == ["encrypted", "lockfile_enabled", "remote"]) and
    (.state_backend.remote == true) and
    (.state_backend.encrypted == true) and
    (.state_backend.lockfile_enabled == true) and
    (.public_cutover | type == "object") and
    (.public_cutover | keys == ["approved", "performed"]) and
    (.public_cutover.approved == false) and
    (.public_cutover.performed == false)
  ' "$checkpoint_json" >/dev/null 2>&1 || fail
fi

if [[ "$mode" == combined-recovery ]]; then
  resource_changes=2
  destroy_or_replace=1
elif [[ "$mode" == replacement ]]; then
  resource_changes=1
  destroy_or_replace=1
else
  resource_changes=5
  destroy_or_replace=0
fi

if [[ "$mode" == create ]]; then
  checkpoint_label=slice3_checkpoint
else
  checkpoint_label=current_reconciliation_checkpoint
fi

printf '%s\n' \
  "resource_changes=$resource_changes" \
  'output_changes=1' \
  'sensitive_outputs=1' \
  "destroy_or_replace=$destroy_or_replace" \
  'aggregate_cost=PASS' \
  "$checkpoint_label=PASS" \
  'PASS'
