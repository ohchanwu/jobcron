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
rendered_user_data=/dev/null
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
    [[ "$6" == combined-recovery || "$6" == initial-deployment ]] || fail
    mode="$6"
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

if [[ "$mode" != create ]]; then
  # Provider 6.33.0 exposes plaintext user_data. Hash its exact decoded bytes,
  # without jq/shell adding or stripping newlines; never print the payload.
  planned_user_data_hash="$(
    jq -esj '
      if length != 1 then error("invalid") else .[0] end |
      [.resource_changes[] | select(.address == "aws_instance.replacement_host")] |
      if length != 1 then error("invalid") else .[0].change.after.user_data end |
      if type == "string" and length > 0 then . else error("invalid") end
    ' "$plan_json" 2>/dev/null | {
      if command -v sha1sum >/dev/null 2>&1; then sha1sum; else shasum; fi
    } | awk '{print $1}'
  )" || fail
  [[ "$planned_user_data_hash" == "$expected_user_data_hash" ]] || fail
fi

if [[ "$mode" == initial-deployment ]]; then
  # The lean lane removes invented history, not private-evidence protections.
  for artifact in "$plan_json" "$cost_json" "$checkpoint_json" "$rendered_user_data"; do
    [[ -f "$artifact" && ! -L "$artifact" ]] || fail
    [[ "$(file_mode "$artifact")" == 600 ]] || fail
    artifact_dir="$(dirname "$artifact")"
    [[ "$(file_mode "$artifact_dir")" == 700 ]] || fail
    if [[ "$slice4_stat_bsd" == yes ]]; then
      artifact_owner="$(stat -f '%u' "$artifact")" || fail
    else
      artifact_owner="$(stat -c '%u' "$artifact")" || fail
    fi
    [[ "$artifact_owner" == "$EUID" ]] || fail
  done
fi

# Slurp forces one evaluation even on empty input (jq 1.6 otherwise exits 0),
# and rejects streams before applying policy to the same parsed document.
jq -es --arg mode "$mode" --rawfile expected_user_data "$rendered_user_data" \
  --slurpfile checkpoint "$checkpoint_json" '
  (if length == 1 then .[0] else error("invalid") end) |
  (if ($checkpoint | length) == 1 then $checkpoint[0] else error("invalid") end) as $checkpoint |
  ($mode == "combined-recovery" or
    ($mode == "initial-deployment" and $checkpoint.managed_eip.state_presence == "absent")) as $create_eip |
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
    elif ($mode == "replacement" or $mode == "combined-recovery" or $mode == "initial-deployment") then
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
          elif ($create_eip and .address == "aws_eip.origin") then
            (.action_reason // null) == null and
            (.change.actions == ["create"])
          else
            .change.actions == ["no-op"]
          end
        )
      ) and
      (
        if $create_eip then
          ($eip.change | has("before")) and
          ($eip.change.before == null) and
          ($eip.change.after | type == "object") and
          ($eip.change.after | has("domain")) and
          ($eip.change.after.domain == "vpc") and
          (if $mode == "initial-deployment" then true else $eip.change.after | has("instance") end) and
          ($eip.change.after.instance == null) and
          (if $mode == "initial-deployment" then true else $eip.change.after | has("network_interface") end) and
          ($eip.change.after.network_interface == null) and
          ($eip.change.after | has("associate_with_private_ip")) and
          ($eip.change.after.associate_with_private_ip == null) and
          (($eip.change.after_unknown.domain // false) == false) and
          (if $mode == "initial-deployment" then true else
            (($eip.change.after_unknown.instance // false) == false) and
            (($eip.change.after_unknown.network_interface // false) == false)
          end) and
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
      (($instance.change.before.key_name == null) or
        ($mode == "initial-deployment" and $instance.change.before.key_name == "" and
          $instance.change.after_unknown.key_name == true)) and
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
      # Exact equality in addition to the digest binding above; no hash-only
      # fallback, base64 decoding, whitespace trimming, or normalization.
      ($instance.change.after.user_data == $expected_user_data) and
      ($instance.change.before.user_data != $instance.change.after.user_data) and
      ((($instance.change.after_unknown.key_name // false) == false) or
        ($mode == "initial-deployment" and $instance.change.before.key_name == "" and
          $instance.change.after_unknown.key_name == true)) and
      (($instance.change.after_unknown.associate_public_ip_address // false) == false) and
      (($instance.change.after_unknown.ami // false) == false) and
      (($instance.change.after_unknown.instance_type // false) == false) and
      (($instance.change.after_unknown.subnet_id // false) == false) and
      (($instance.change.after_unknown.iam_instance_profile // false) == false) and
      (if $mode == "initial-deployment" then true else
        (($instance.change.after_unknown.vpc_security_group_ids // false) == false) and
        (($instance.change.after_unknown.metadata_options // false) == false) and
        (($instance.change.after_unknown.root_block_device // false) == false)
      end) and
      (($instance.change.after_unknown.user_data // false) == false) and
      ((.diagnostics // []) == [])
    else
      false
    end
  ) and
  (if $mode == "initial-deployment" then
    def after($address): [.resource_changes[] | select(.address == $address)][0].change.after;
    def no_unknown: type == "object" and all(.. | scalars; . == false);
    after("aws_security_group.origin") as $origin |
    after("aws_security_group.database") as $database_sg |
    after("aws_vpc_security_group_ingress_rule.database_postgresql_from_origin") as $ingress |
    after("aws_db_instance.production") as $rds |
    all(.resource_changes[] | select(.change.actions == ["no-op"]);
      (.change.before | type == "object" and length > 0) and
      .change.before == .change.after and
      ((.change.after_unknown // {}) | no_unknown)) and
    ($origin.id | type == "string" and length > 0) and
    ($origin.ingress == []) and
    ($database_sg.id | type == "string" and length > 0) and
    ($database_sg.ingress | type == "array" and length <= 1 and
      all(.[]; .from_port == 5432 and .to_port == 5432 and .protocol == "tcp" and
        .security_groups == [$origin.id] and .cidr_blocks == [] and
        .ipv6_cidr_blocks == [] and .prefix_list_ids == [] and .self == false)) and
    ($instance.change.after.vpc_security_group_ids == [$origin.id]) and
    ($ingress.security_group_id == $database_sg.id) and
    ($ingress.referenced_security_group_id == $origin.id) and
    ($ingress.from_port == 5432 and $ingress.to_port == 5432 and $ingress.ip_protocol == "tcp") and
    ($ingress.cidr_ipv4 == null and $ingress.cidr_ipv6 == null and $ingress.prefix_list_id == null) and
    ($rds.publicly_accessible == false and $rds.storage_encrypted == true and
      $rds.deletion_protection == true and $rds.manage_master_user_password == true) and
    ($rds.backup_retention_period | type == "number" and floor == . and . >= 1) and
    ($rds.vpc_security_group_ids == [$database_sg.id]) and
    ($eip.change.after.domain == "vpc") and
    (if $create_eip then true else
      $eip.change.after | has("instance") and has("network_interface")
    end) and
    ($eip.change.after | has("associate_with_private_ip")) and
    ($eip.change.after.instance == null and $eip.change.after.network_interface == null and
      $eip.change.after.associate_with_private_ip == null) and
    # Refresh is checked separately against independent owner-only observations
    # below. Planned actions remain subject to the exact allowlist above.
    # Defense in depth over all plan sections (including embedded prior state).
    # Arbitrary secret detection still requires the independent private review.
    all(.. | objects | to_entries[];
      if (.key | test("^(password|master_password|secret_string|secret_binary|private_key|token|session_secret|database_url)$"; "i")) then
        # Terraform sensitivity masks contain booleans, never credential bytes.
        (.value == null or (.value | type) == "boolean" or .value == "")
      else true end) and
    all(.. | strings; test("-----BEGIN ([A-Z ]+ )?PRIVATE KEY-----|postgres(ql)?://[^ /:]+:[^ /@]+@"; "i") | not)
  else true end) and
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

if [[ "$mode" == initial-deployment ]]; then
  # Only these five paths cross the launcher's cleared environment. No values
  # are printed or passed as arguments. Frozen observations are review evidence,
  # not a substitute for attended freshness revalidation before apply.
  python3 -I - "$plan_json" "$checkpoint_json" >/dev/null 2>&1 <<'PY' || fail
import datetime
import ipaddress
import json
import os
import re
import stat
import sys


def require(value):
    if not value:
        raise ValueError("invalid")


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result)
        result[key] = value
    return result


def equal(left, right):
    # Python equates True with 1; Terraform JSON security controls must not.
    return json.dumps(left, sort_keys=True) == json.dumps(right, sort_keys=True)


def load(path):
    # Open without following a final symlink, then check the opened inode.
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd) as stream:
        info = os.fstat(stream.fileno())
        require(stat.S_ISREG(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o600)
        require(info.st_uid == os.geteuid())
        parent = os.stat(os.path.dirname(os.path.abspath(path)))
        require(parent.st_uid == os.geteuid() and stat.S_IMODE(parent.st_mode) == 0o700)
        value = json.load(stream, object_pairs_hook=unique_object,
                          parse_constant=lambda _: require(False))
    require(type(value) is dict)
    secret_free(value)
    return value


def secret_free(value):
    if type(value) is dict:
        for key, item in value.items():
            if re.fullmatch(r'password|master_password|secret_string|secret_binary|private_key|token|session_secret|database_url', key, re.I):
                require(item is None or type(item) is bool or item == "")
            secret_free(item)
    elif type(value) is list:
        for item in value:
            secret_free(item)
    elif type(value) is str:
        require(not re.search(r'-----BEGIN ([A-Z ]+ )?PRIVATE KEY-----|postgres(ql)?://[^ /:]+:[^ /@]+@', value, re.I))


def text(value):
    require(type(value) is str and bool(value.strip()))
    return value


def one(values):
    require(type(values) is list and len(values) == 1 and type(values[0]) is dict)
    return values[0]


def indexed(records):
    require(type(records) is list)
    result = {}
    for record in records:
        address = text(record['address'])
        require(address not in result)
        result[address] = record
    return result


def instant(value):
    require(re.fullmatch(r'\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,6})?(Z|[+-]\d{2}:\d{2})', text(value)))
    return datetime.datetime.fromisoformat(value.replace('Z', '+00:00'))


def known(value):
    if type(value) is dict:
        return all(known(v) for v in value.values())
    if type(value) is list:
        return all(known(v) for v in value)
    return value is False


def unconfigured_tags(value):
    if type(value) is dict:
        return 'tags' not in value and all(unconfigured_tags(v) for v in value.values())
    if type(value) is list:
        return all(unconfigured_tags(v) for v in value)
    return True


def has_key(value, key):
    if type(value) is dict:
        return key in value or any(has_key(v, key) for v in value.values())
    if type(value) is list:
        return any(has_key(v, key) for v in value)
    return False


def unknown_mask(mask, after, allowed, expressions, path=()):
    # A partially known collection is a structure, not a boolean. Only exact
    # computed paths may be true; known controls remain checked by jq above.
    if type(mask) is bool:
        if mask:
            require(path in allowed and after is None)
            if path == ('key_name',):
                require(expressions.get('key_name', {'constant_value': None}) == {'constant_value': None})
            else:
                require(not has_key(expressions.get(path[0], {}), path[-1]) if len(path) > 1
                        else path[0] not in expressions)
        return
    if type(mask) is dict:
        require(type(after) is dict)
        for key, item in mask.items():
            child = path + (key,)
            require(key in after or child in allowed)
            unknown_mask(item, after.get(key), allowed, expressions, child)
    elif type(mask) is list:
        require(type(after) is list and len(mask) == len(after))
        for index, item in enumerate(mask):
            unknown_mask(item, after[index], allowed, expressions, path + (index,))
    else:
        require(False)


plan = load(sys.argv[1])
checkpoint = load(sys.argv[2])
live_host = one(one(load(os.environ['TF_INITIAL_LIVE_HOST_JSON'])['Reservations'])['Instances'])
live_db = one(load(os.environ['TF_INITIAL_LIVE_RDS_JSON'])['DBInstances'])
state = load(os.environ['TF_INITIAL_CURRENT_STATE_JSON'])
inputs = load(os.environ['TF_INITIAL_TFVARS_JSON'])
live_addresses = load(os.environ['TF_INITIAL_LIVE_ADDRESSES_JSON'])
require(set(live_addresses) == {'Addresses'} and type(live_addresses['Addresses']) is list)
allocations = set()
for address in live_addresses['Addresses']:
    require(type(address) is dict)
    allocation = text(address['AllocationId'])
    require(re.fullmatch(r'eipalloc-[0-9a-f]+', allocation) and allocation not in allocations)
    allocations.add(allocation)
changes = indexed(plan['resource_changes'])
drifts = indexed(plan.get('resource_drift', []))
config = indexed(plan['configuration']['root_module']['resources'])
require(not plan['configuration']['root_module'].get('module_calls'))
# Accept the two Terraform snapshot representations, preserving attribute
# values exactly. This never rewrites/normalizes the saved plan or evidence.
if 'values' in state:
    require(state['format_version'] == '1.0' and 'resources' not in state)
    module = state['values']['root_module']
    require(not module.get('child_modules'))
    resources = indexed(module['resources'])
else:
    require(type(state['version']) is int and state['version'] == 4)
    records = []
    groups = set()
    for resource in state['resources']:
        require(not resource.get('module') and resource['mode'] in ('managed', 'data'))
        base = ('data.' if resource['mode'] == 'data' else '') + text(resource['type']) + '.' + text(resource['name'])
        require(base not in groups)
        groups.add(base)
        if resource['mode'] == 'data':
            require(base == 'data.aws_ssm_parameter.amazon_linux_2023_arm64')
            instance = one(resource['instances'])
            require('index_key' not in instance and not instance.get('deposed'))
            require(instance.get('status', 'ready') == 'ready')
            # The provider marks SSM value sensitive even for this public AMI.
            # Admit only its canonical path; the public-value gates below still apply.
            require(instance['sensitive_attributes'] == [[{'type': 'get_attr', 'value': 'value'}]])
            attrs = instance['attributes']
            public_ami = '/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64'
            require(type(attrs) is dict and set(attrs) <= {
                'id', 'name', 'arn', 'data_type', 'region', 'type', 'value',
                'insecure_value', 'version', 'with_decryption'})
            require(attrs['id'] == attrs['name'] == public_ami and attrs['type'] == 'String')
            require(text(attrs['value']) == text(inputs['replacement_host_ami_id']))
            require(re.fullmatch(r'ami-([0-9a-f]{8}|[0-9a-f]{17})', attrs['value']))
            require(attrs.get('insecure_value') in (None, '', attrs['value']))
            require(type(attrs['with_decryption']) is bool)
            continue
        require(type(resource['instances']) is list and resource['instances'])
        for instance in resource['instances']:
            require(not instance.get('deposed') and instance.get('status', 'ready') == 'ready')
            address = base
            if 'index_key' in instance:
                require(type(instance['index_key']) in (str, int))
                address += '[' + json.dumps(instance['index_key']) + ']'
            records.append({'address': address, 'mode': 'managed', 'values': instance['attributes']})
    resources = indexed(records)
require(set(resources) - {'aws_eip.origin'} == set(changes) - {'aws_eip.origin'})
require(all(r['mode'] == 'managed' for r in resources.values()))
eip = 'aws_eip.origin'
if changes[eip]['change']['actions'] == ['create']:
    allocation = text(resources[eip]['values']['id'])
    require(re.fullmatch(r'eipalloc-[0-9a-f]+', allocation))
    require(resources[eip]['values']['allocation_id'] == allocation and allocation not in allocations)
    require(config[eip]['expressions'] == {'domain': {'constant_value': 'vpc'}})
    require(not any(a.startswith('aws_eip_association.') for a in set(config) | set(changes) | set(resources)))
    eip_computed = {(k,) for k in (
        'id', 'allocation_id', 'arn', 'association_id', 'carrier_ip', 'customer_owned_ip',
        'instance', 'network_interface', 'ipam_pool_id', 'network_border_group',
        'private_dns', 'private_ip', 'ptr_record', 'public_dns', 'public_ip',
        'public_ipv4_pool', 'tags_all')}
    unknown_mask(changes[eip]['change'].get('after_unknown', {}),
                 changes[eip]['change']['after'], eip_computed, config[eip]['expressions'])
    require(changes[eip]['change']['after'].get('association_id') is None)
for address, resource in resources.items():
    if address != 'aws_eip.origin' and address not in drifts:
        require(equal(resource['values'], changes[address]['change']['before']))
expected_inputs = {'canonical_network_config', 'private_database_config',
                   'replacement_host_ami_id', 'replacement_public_subnet_key'}
require(set(inputs) in (expected_inputs, expected_inputs - {'replacement_public_subnet_key'}))
declared = dict(inputs)
declared.setdefault('replacement_public_subnet_key', 'public_a')
require(set(plan['variables']) == expected_inputs)
require(all(equal(plan['variables'][k]['value'], v) for k, v in declared.items()))
host = 'aws_instance.replacement_host'
db = 'aws_db_instance.production'
role = 'aws_iam_role.replacement_host'
profile = 'aws_iam_instance_profile.replacement_host'
attachment = 'aws_iam_role_policy_attachment.replacement_host_ssm'
runtime_policy = 'aws_iam_role_policy.replacement_host_runtime'
host_before = changes[host]['change']['before']
db_before = changes[db]['change']['before']
# Pinned AWS 6.33.0 computed fields, excluding the configured AMI, instance
# size, profile, subnet, public-IP switch and all IMDS/root security controls.
# Whole unconfigured computed blocks may be unknown; partial root/metadata
# blocks admit only the named computed leaves, never the whole block.
host_computed = {(k,) for k in (
    'id', 'arn', 'availability_zone', 'capacity_reservation_specification',
    'cpu_options', 'disable_api_stop', 'disable_api_termination', 'ebs_block_device',
    'ebs_optimized', 'enclave_options', 'enable_primary_ipv6', 'ephemeral_block_device',
    'host_id', 'host_resource_group_arn', 'instance_initiated_shutdown_behavior',
    'instance_lifecycle', 'instance_market_options', 'instance_state', 'ipv6_address_count',
    'ipv6_addresses', 'key_name', 'maintenance_options', 'monitoring', 'network_interface',
    'outpost_arn', 'password_data', 'placement_group', 'placement_group_id',
    'placement_partition_number', 'primary_network_interface_id', 'primary_network_interface',
    'private_dns', 'private_dns_name_options', 'private_ip', 'public_dns', 'public_ip',
    'secondary_private_ips', 'secondary_network_interface', 'security_groups',
    'spot_instance_request_id', 'tags_all', 'tenancy', 'user_data_base64')}
host_computed |= {('root_block_device', 0, k) for k in
                  ('device_name', 'iops', 'kms_key_id', 'tags_all', 'throughput', 'volume_id')}
host_computed.add(('metadata_options', 0, 'instance_metadata_tags'))
unknown_mask(changes[host]['change'].get('after_unknown', {}),
             changes[host]['change']['after'], host_computed, config[host]['expressions'])
if host_before.get('key_name') == '':
    require('KeyName' not in live_host)
    require(config[host]['expressions'].get('key_name', {'constant_value': None}) == {'constant_value': None})
    require(not any(a.startswith('aws_key_pair.') for a in set(config) | set(changes) | set(resources)))
require(text(live_host['InstanceId']) == text(host_before['id']) == resources[host]['values']['id'])
require(text(live_db['DBInstanceIdentifier']) == text(db_before['identifier']) == resources[db]['values']['identifier'])
require(text(live_db['DbiResourceId']) == text(db_before['id']) == resources[db]['values']['id'])
ipaddress.IPv4Address(text(live_host['PublicIpAddress']))
text(live_host['PublicDnsName'])
instant(live_db['LatestRestorableTime'])
require(host_before['ami'] == text(declared['replacement_host_ami_id']))
require(db_before['identifier'] == declared['private_database_config']['database_identifier'])
subnet = 'aws_subnet.public[' + json.dumps(declared['replacement_public_subnet_key']) + ']'
require(host_before['subnet_id'] == changes[subnet]['change']['before']['id'])

allowed = {db: {'latest_restorable_time'}, profile: {'tags'},
           role: {'tags', 'inline_policy', 'managed_policy_arns'},
           host: {'public_ip', 'public_dns', 'root_block_device', 'tags'}}
for address, drift in drifts.items():
    require(drift.get('previous_address') is None and drift.get('action_reason') is None)
    change = drift['change']
    require(change.get('importing') is None and not change.get('replace_paths'))
    require(known(change.get('after_unknown', {})))
    before, after = change['before'], change['after']
    if address == 'aws_eip.origin':
        require(checkpoint['managed_eip']['state_presence'] == 'absent')
        require(change['actions'] == ['delete'] and type(before) is dict and after is None)
        require(changes[address]['change']['actions'] == ['create'])
        require(equal(before, resources[address]['values']))
        continue
    require(address in allowed and change['actions'] == ['update'])
    require(type(before) is dict and type(after) is dict and set(before) == set(after))
    require(text(before['id']) == after['id'])
    require(equal(before, resources[address]['values']))
    require(equal(after, changes[address]['change']['before']))
    if address != host:
        require(changes[address]['change']['actions'] == ['no-op'])
        require(equal(after, changes[address]['change']['after']))
    else:
        require(changes[address]['change']['actions'] == ['delete', 'create'])
        require(changes[address]['action_reason'] == 'replace_by_request')
    changed = {k for k in before if not equal(before[k], after[k])}
    require(changed and changed <= allowed[address])
    if address == role:
        require(before['inline_policy'] in (None, []))
        ssm = 'arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore'
        require(after['managed_policy_arns'] == [ssm])
        attached = changes[attachment]['change']['after']
        policy = changes[runtime_policy]['change']['after']
        if after['inline_policy'] != []:
            # AWS 6.33.0 also projects the separately managed runtime policy
            # into this unconfigured computed block. Bind it to independent
            # pre-refresh state, not merely another value in the same plan.
            projected = one(after['inline_policy'])
            require(set(projected) == {'name', 'policy'})
            require(runtime_policy not in drifts)
            require(changes[runtime_policy]['change']['actions'] == ['no-op'])
            source_policy = resources[runtime_policy]['values']
            require(equal(source_policy, policy))
            require(text(projected['name']) == text(source_policy['name']))
            def policy_document(value):
                document = json.loads(text(value), object_pairs_hook=unique_object,
                                      parse_constant=lambda _: require(False))
                require(type(document) is dict)
                secret_free(document)
                return document
            require(equal(policy_document(projected['policy']),
                          policy_document(source_policy['policy'])))
        require(after['name'] == after['id'] == 'jobcron-replacement-host')
        require(attached['role'] == policy['role'] == after['name'])
        require(attached['policy_arn'] == ssm)
        require(changes[profile]['change']['after']['role'] == after['name'])
        require(host_before['iam_instance_profile'] == changes[profile]['change']['after']['name'])
        require(config[attachment]['expressions']['policy_arn'] == {'constant_value': ssm})
        require(config[attachment]['expressions']['role']['references'] ==
                ['aws_iam_role.replacement_host.name', 'aws_iam_role.replacement_host'])
        require(config[runtime_policy]['expressions']['role']['references'] ==
                ['aws_iam_role.replacement_host.id', 'aws_iam_role.replacement_host'])
    expressions = config[address]['expressions']
    require(type(expressions) is dict)
    for key in changed:
        # These fields are unconfigured in the pinned production module. A
        # configured value is not a computed-refresh exemption.
        if key != 'root_block_device':
            require(key not in expressions)
        if key == 'tags':
            require(before[key] is None and after[key] == {})
        elif key == 'root_block_device':
            require(unconfigured_tags(expressions.get(key, {})))
            old, new = one(before[key]), one(after[key])
            require(set(old) == set(new) and old['tags'] is None and new['tags'] == {})
            require(equal({k: v for k, v in old.items() if k != 'tags'},
                          {k: v for k, v in new.items() if k != 'tags'}))
        elif key == 'inline_policy':
            # Exact empty/singleton projection checked above, including the
            # independent source policy and unchanged role/policy actions.
            require(address == role and before[key] in (None, []))
        elif key == 'managed_policy_arns':
            require(before[key] in (None, []))
        elif key == 'public_ip':
            require(after[key] == live_host['PublicIpAddress'])
        elif key == 'public_dns':
            require(after[key] == live_host['PublicDnsName'])
        elif key == 'latest_restorable_time':
            require(instant(live_db['LatestRestorableTime']) <= instant(after[key]) <= instant(plan['timestamp']))
PY
fi

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
    (keys == ([
      "bootstrap_host",
      "checked_at",
      "commit",
      "managed_eip",
      "origin",
      "public_cutover",
      "rds",
      "recovery_bucket",
      "runtime_secret",
      "schema_version",
      "selected_state_resources",
      "state_backend"
    ] + (if $mode == "initial-deployment" then
      ["evidence_security", "initial_deployment"] else ["legacy_rollback_host"] end) | sort)) and
    (.schema_version == (if $mode == "initial-deployment" then
      "human-assisted-initial-deployment-v1" else "human-assisted-reconciliation-v1" end)) and
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
    (.bootstrap_host.disposition == (if $mode == "initial-deployment" then
      "replace-disposable-bootstrap" else "replace" end)) and
    (.bootstrap_host.human_approved == true) and
    (if $mode == "initial-deployment" then
      (.initial_deployment == {
        prior_service_exists: false,
        prior_production_data_exists: false,
        prior_image_exists: false,
        legacy_rollback_host_exists: false,
        bootstrap_service_running: false,
        bootstrap_contains_production_data: false,
        database_unused: true,
        rollback: "stop-private-runtime-leave-public-routing-unchanged"
      }) and
      (.evidence_security == {
        state_secret_free: true,
        plan_secret_free: true,
        shared_evidence_secret_free: true
      })
    else
      (.legacy_rollback_host | type == "object") and
      (.legacy_rollback_host | keys == ["retained"]) and
      (.legacy_rollback_host.retained == true)
    end) and
    (.managed_eip | type == "object") and
    (.managed_eip | keys == ["state_presence", "terraform_address", "unattached"]) and
    (.managed_eip.terraform_address == "aws_eip.origin") and
    (.managed_eip.unattached == true) and
    (if $mode == "initial-deployment" then
       (.managed_eip.state_presence == "absent" or .managed_eip.state_presence == "present")
     elif $mode == "combined-recovery" then
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

if [[ "$mode" == initial-deployment ]]; then
  resource_changes="$(jq -er '[.resource_changes[] | select(.change.actions != ["no-op"])] | length' "$plan_json")" || fail
  destroy_or_replace=1
elif [[ "$mode" == combined-recovery ]]; then
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
