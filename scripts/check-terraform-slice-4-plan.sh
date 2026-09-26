#!/bin/bash -p
# Privileged startup ignores exported functions, BASH_ENV and shell options.
# Always cross a clean process boundary; no environment/argument sentinel can
# skip it. Invoke this entry point directly, not through an ambient shell.
# Initial-deployment additionally requires five read-only evidence paths:
# TF_INITIAL_LIVE_HOST_JSON: AWS describe-instances, exactly one reservation/host
# TF_INITIAL_LIVE_RDS_JSON: AWS describe-db-instances, exactly one DB
# TF_INITIAL_LIVE_ADDRESSES_JSON: complete DescribeAddresses {"Addresses": [...]}
# in the reconciled account/region; no matching state-bound origin AllocationId
# when creating the absent EIP. Not a NotFound or paginated/partial response.
# TF_INITIAL_CURRENT_STATE_JSON: pre-refresh Terraform show JSON or raw v4 state
# TF_INITIAL_TFVARS_JSON: independently reconstructed production input JSON
# Use owner-owned 0600 regular non-symlink files in owner-owned 0700 directories.
# Only paths are forwarded; payloads never enter argv or diagnostics. Python 3
# (stdlib only) must be available on the body's trusted system PATH.
# Preserved observations bind a frozen saved-plan review, not live freshness:
# revalidate state/live resources in the later attended pre-apply gate. These
# inputs do not authorize apply, change the release SHA, or regenerate a plan.
exec /usr/bin/env -i PATH=/usr/bin:/bin \
  TF_INITIAL_LIVE_HOST_JSON="${TF_INITIAL_LIVE_HOST_JSON-}" \
  TF_INITIAL_LIVE_RDS_JSON="${TF_INITIAL_LIVE_RDS_JSON-}" \
  TF_INITIAL_LIVE_ADDRESSES_JSON="${TF_INITIAL_LIVE_ADDRESSES_JSON-}" \
  TF_INITIAL_CURRENT_STATE_JSON="${TF_INITIAL_CURRENT_STATE_JSON-}" \
  TF_INITIAL_TFVARS_JSON="${TF_INITIAL_TFVARS_JSON-}" \
  /bin/bash --noprofile --norc -p \
  "${BASH_SOURCE[0]%/*}/check-terraform-slice-4-plan-body.sh" "$@"
