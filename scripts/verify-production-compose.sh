#!/bin/sh

set -eu

fail() {
	printf '%s\n' "production Compose contract failed: $1" >&2
	exit 1
}

[ -n "${JOBCRON_IMAGE:-}" ] || fail "required variable JOBCRON_IMAGE"
[ -n "${JOBCRON_STAGE1_SPONSOR_USER_ID:-}" ] ||
	fail "required variable JOBCRON_STAGE1_SPONSOR_USER_ID"

command -v docker >/dev/null 2>&1 || fail "docker compose command"
command -v jq >/dev/null 2>&1 || fail "jq structured Compose inspector"

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
compose_file="$repo_root/deploy/production/compose.yaml"
[ -f "$compose_file" ] || fail "deploy/production/compose.yaml"

umask 077
rendered_compose=$(mktemp "${TMPDIR:-/tmp}/jobcron-production-compose.XXXXXX" 2>/dev/null) ||
	fail "private rendered Compose file"
legacy_volume_probe=""
cleanup() {
	rm -f "$rendered_compose" >/dev/null 2>&1 || :
	if [ -n "$legacy_volume_probe" ]; then
		rm -f "$legacy_volume_probe" >/dev/null 2>&1 || :
	fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

if ! docker compose -f "$compose_file" config --format json 2>/dev/null >"$rendered_compose"; then
	fail "rendered Compose configuration"
fi

# Compose drops unused top-level volumes from its normalized model. Reference the
# retired volume from a private override: a successful render proves the base
# file still declares it, while an undefined-volume failure proves it is absent.
legacy_volume_probe=$(mktemp "${TMPDIR:-/tmp}/jobcron-production-compose.legacy.XXXXXX" 2>/dev/null) ||
	fail "private legacy-volume probe"
if ! cat 2>/dev/null >"$legacy_volume_probe" <<'EOF'
services:
  production_contract_probe:
    image: scratch
    volumes:
      - jobcron_config:/contract-probe
EOF
then
	fail "private legacy-volume probe"
fi
if docker compose -f "$compose_file" -f "$legacy_volume_probe" config --quiet \
	>/dev/null 2>&1; then
	fail "volumes.jobcron_config must be absent"
fi

# Compose v2 omits false create_host_path; v5 omits true instead. Determine
# which representation this renderer uses without running or mounting anything.
# Reuse the private probe file; no application environment is rendered here.
cat >"$legacy_volume_probe" <<'EOF'
services:
  production_contract_probe:
    image: scratch
    volumes:
      - type: bind
        source: /contract-false
        target: /contract-false
        bind:
          create_host_path: false
      - type: bind
        source: /contract-true
        target: /contract-true
        bind:
          create_host_path: true
EOF
if ! omitted_host_path_is_false=$(docker compose -f "$legacy_volume_probe" config --format json 2>/dev/null |
	jq -r '
		.services.production_contract_probe.volumes as $v |
		($v[] | select(.target == "/contract-false").bind) as $f |
		($v[] | select(.target == "/contract-true").bind) as $t |
		if ($f | type) != "object" or ($t | type) != "object" then error("bind options")
		elif ($f | has("create_host_path") | not) and $t.create_host_path == true then true
		elif $f.create_host_path == false and
			(($t | has("create_host_path") | not) or $t.create_host_path == true) then false
		else error("bind options") end
	' 2>/dev/null); then
	fail "Compose bind create_host_path representation"
fi
[ "$omitted_host_path_is_false" = true ] || [ "$omitted_host_path_is_false" = false ] ||
	fail "Compose bind create_host_path representation"

check_contract() {
	contract=$1
	filter=$2
	if ! jq --argjson omitted_host_path_is_false "$omitted_host_path_is_false" \
		-e "$filter" "$rendered_compose" >/dev/null 2>&1; then
		fail "$contract"
	fi
}

check_contract "services.app" '.services.app | type == "object"'
check_contract "services.caddy" '.services.caddy | type == "object"'
check_contract "services.app.volumes must be only read-only runtime secrets and RDS CA" '
	.services.app.volumes as $v | ($v | length) == 2 and
	all($v[]; .type == "bind" and .read_only == true and .source == .target) and
	([$v[].source] | sort) == ["/run/jobcron/rds-ca.pem", "/run/jobcron/secrets"] and
	all($v[] | select(.source == "/run/jobcron/rds-ca.pem");
		(.bind | type == "object") and
		(.bind.create_host_path == false or
			($omitted_host_path_is_false and (.bind | has("create_host_path") | not))))
'
check_contract "services.app.ports must bind only loopback 7777" '
	(.services.app.ports // []) as $ports |
	($ports | length) == 1 and
	$ports[0].host_ip == "127.0.0.1" and
	($ports[0].published | tostring) == "7777" and
	$ports[0].target == 7777 and
	$ports[0].protocol == "tcp"
'
check_contract "services.caddy.ports must publish only host TCP 443" '
	(.services.caddy.ports // []) as $ports |
	($ports | length) == 1 and
	$ports[0].host_ip == "0.0.0.0" and
	($ports[0].published | tostring) == "443" and
	$ports[0].target == 443 and
	$ports[0].protocol == "tcp"
'
check_contract "networks must retain internal runtime and outbound bridge for app egress and published HTTPS" '
	.networks.runtime.internal == true and
	.networks.outbound.driver == "bridge" and
	((.networks.outbound.internal // false) == false) and
	((.services.app.networks | keys | sort) == ["outbound", "runtime"]) and
	((.services.caddy.networks | keys | sort) == ["outbound", "runtime"])
'
check_contract "services.caddy.volumes must mount transient Origin CA read-only" '
	[.services.caddy.volumes[] |
	 select(.type == "bind" and
	        .source == "/run/jobcron/caddy" and
	        .target == "/run/jobcron/caddy" and
	        .read_only == true)] | length == 1
'
check_contract "services.app.logging must rotate local JSON logs" '
	.services.app.logging.driver == "json-file" and
	.services.app.logging.options["max-size"] == "10m" and
	.services.app.logging.options["max-file"] == "3"
'
check_contract "services.caddy.logging must rotate local JSON logs" '
	.services.caddy.logging.driver == "json-file" and
	.services.caddy.logging.options["max-size"] == "10m" and
	.services.caddy.logging.options["max-file"] == "3"
'
check_contract "services.app.ulimits.core must disable core dumps" '
	(.services.app.ulimits | has("core")) and
	(.services.app.ulimits.core | type == "object") and
	((.services.app.ulimits.core.hard // 0) == 0) and
	((.services.app.ulimits.core.soft // 0) == 0)
'
check_contract "services.caddy.ulimits.core must disable core dumps" '
	(.services.caddy.ulimits | has("core")) and
	(.services.caddy.ulimits.core | type == "object") and
	((.services.caddy.ulimits.core.hard // 0) == 0) and
	((.services.caddy.ulimits.core.soft // 0) == 0)
'

for name in DATABASE_URL SESSION_SECRET JOBCRON_CREDENTIAL_ENCRYPTION_KEY JOBCRON_PROXY_SECRET JOBCRON_SIGNUP_ACCESS_CODE; do
	check_contract "services.app.environment.${name}_FILE" \
		".services.app.environment.${name}_FILE == \"/run/jobcron/secrets/$name\" and (.services.app.environment | has(\"$name\") | not)"
done
check_contract "services.caddy.environment.JOBCRON_PROXY_SECRET must be absent" \
	'(.services.caddy.environment | has("JOBCRON_PROXY_SECRET") | not)'
check_contract "services.caddy state must be volatile" '
	.services.caddy.read_only == true and
	(.services.caddy.tmpfs | sort) == ["/config:mode=0700", "/data:mode=0700", "/tmp:mode=0700"] and
	all(.services.caddy.volumes[]; .type == "bind" and .read_only == true) and
	(.services.caddy.volumes | length) == 2
'
check_contract "services.app.environment.JOBCRON_ENV must be production" \
	'.services.app.environment.JOBCRON_ENV == "production"'
check_contract "services.app.environment.JOBCRON_HOST must be 0.0.0.0" \
	'.services.app.environment.JOBCRON_HOST == "0.0.0.0"'
check_contract "services.app.environment.JOBCRON_PORT must be 7777" \
	'.services.app.environment.JOBCRON_PORT == "7777"'
check_contract "services.app.environment.JOBCRON_NO_OPEN must be 1" \
	'.services.app.environment.JOBCRON_NO_OPEN == "1"'
check_contract "services.app.environment.AWS_EC2_METADATA_DISABLED must be true" \
	'.services.app.environment.AWS_EC2_METADATA_DISABLED == "true"'
check_contract "services.app.environment.JOBCRON_DEMO must be absent" \
	'(.services.app.environment | has("JOBCRON_DEMO")) | not'
check_contract "services.app.environment.JOBCRON_ADMIN_TOKEN must be absent" \
	'(.services.app.environment | has("JOBCRON_ADMIN_TOKEN")) | not'
check_contract "services.app.environment.JOBCRON_WORKNET_KEY must be absent" \
	'(.services.app.environment | has("JOBCRON_WORKNET_KEY")) | not'

check_contract "services.caddy.environment.AWS_EC2_METADATA_DISABLED must be true" \
	'.services.caddy.environment.AWS_EC2_METADATA_DISABLED == "true"'
check_contract "services.app.environment.JOBCRON_SCHEDULER_ENABLED must be 1" \
	'.services.app.environment.JOBCRON_SCHEDULER_ENABLED == "1"'
check_contract "services.app.environment.JOBCRON_DAILY_SCRAPE_TIME must preserve the same-name input or default to 05:00" '
	(env.JOBCRON_DAILY_SCRAPE_TIME // "") as $daily_time |
	.services.app.environment.JOBCRON_DAILY_SCRAPE_TIME ==
	(if $daily_time == "" then "05:00" else $daily_time end)
'

check_contract "services.app.environment.JOBCRON_STAGE1_SPONSOR_USER_ID" \
	'.services.app.environment.JOBCRON_STAGE1_SPONSOR_USER_ID == env.JOBCRON_STAGE1_SPONSOR_USER_ID'
check_contract "services.app.command must enforce no-open host and port" '
	.services.app.command == ["--no-open", "--host", "0.0.0.0", "--port", "7777"]
'
check_contract "services.app.image must equal JOBCRON_IMAGE" \
	'.services.app.image == env.JOBCRON_IMAGE'
check_contract "services.app.pull_policy must be never" \
	'.services.app.pull_policy == "never"'
check_contract "services environment must contain only reviewed keys" '
	(.services.app.environment | keys | sort) == ([
		"AWS_EC2_METADATA_DISABLED", "DATABASE_URL_FILE",
		"JOBCRON_CREDENTIAL_ENCRYPTION_KEY_FILE", "JOBCRON_DAILY_SCRAPE_TIME",
		"JOBCRON_ENV", "JOBCRON_HOST", "JOBCRON_NO_OPEN", "JOBCRON_PORT",
		"JOBCRON_PROXY_SECRET_FILE", "JOBCRON_SCHEDULER_ENABLED",
		"JOBCRON_SIGNUP_ACCESS_CODE_FILE", "JOBCRON_STAGE1_SPONSOR_USER_ID",
		"SESSION_SECRET_FILE"] | sort) and
	(.services.caddy.environment | keys) == ["AWS_EC2_METADATA_DISABLED"]
'
check_contract "services.app.image must use private GHCR sha256 digest" \
	'(.services.app.image | type == "string") and
	 (.services.app.image |
	  test("^ghcr[.]io/[a-z0-9._-]+/jobcron@sha256:[0-9a-f]{64}$"))'

printf '%s\n' "production Compose contract verified"
