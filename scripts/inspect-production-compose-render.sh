#!/bin/sh

set -u

fail() {
	printf '%s\n' "production Compose validation failed: $1" >&2
	exit 1
}

[ "$#" -eq 1 ] || fail "rendered Compose input"

if grep -E -q 'jobcron_config|/root/\.config/jobcron' "$1" 2>/dev/null; then
	fail "legacy volume"
else
	inspection_status=$?
	[ "$inspection_status" -eq 1 ] || fail "rendered Compose inspection"
fi
