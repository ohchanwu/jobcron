#!/bin/bash -p
# Privileged startup ignores exported functions, BASH_ENV and shell options.
# Always cross a clean process boundary; no environment/argument sentinel can
# skip it. Invoke this entry point directly, not through an ambient shell.
exec /usr/bin/env -i PATH=/usr/bin:/bin \
  /bin/bash --noprofile --norc -p \
  "${BASH_SOURCE[0]%/*}/check-terraform-slice-4-plan-body.sh" "$@"
