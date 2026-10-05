#!/bin/sh
set -eu
umask 077

run_dir=${JOBCRON_RUN_DIR:-/run/jobcron}
etc_dir=${JOBCRON_ETC_DIR:-/etc/jobcron}
deploy_dir=${JOBCRON_DEPLOY_DIR:-/opt/jobcron}
docker_config=$run_dir/docker
export DOCKER_CONFIG=$docker_config
secret_id_file=$etc_dir/runtime-secret-id

fail() {
	printf '%s\n' "jobcron runtime operation failed" >&2
	exit 1
}

mode() {
	stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"
}

owner() {
	stat -c '%u' "$1" 2>/dev/null || stat -f '%u' "$1"
}

validate_database_url() {
	printf '%s\n' "$1" |
		grep -Eq '^postgres://[A-Za-z_][A-Za-z0-9_]*:([A-Za-z0-9._~-]|%[0-9A-Fa-f]{2})+@[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+\.rds\.amazonaws\.com:[0-9]+/[A-Za-z_][A-Za-z0-9_]*\?sslmode=verify-full&sslrootcert=/run/jobcron/rds-ca\.pem$' || fail
	endpoint=${1#*@}
	endpoint=${endpoint%%/*}
	port=${endpoint##*:}
	[ "$port" -ge 1 ] 2>/dev/null && [ "$port" -le 65535 ] 2>/dev/null || fail
}

check_runtime_custody() {
	swap=$(swapon --noheadings --show=NAME 2>/dev/null) || fail
	[ -z "$swap" ] || fail
	case $run_dir in /*) ;; *) fail ;; esac
	for directory in "$run_dir" "$run_dir/secrets" "$run_dir/caddy" "$run_dir/docker" "$run_dir/archive"; do
		[ ! -L "$directory" ] || fail
		if [ -e "$directory" ]; then
			[ -d "$directory" ] || fail
			[ "$(mode "$directory")" = 700 ] || fail
			[ "$(owner "$directory")" = "$(id -u)" ] || fail
			[ "$(findmnt -n -o FSTYPE --target "$directory" 2>/dev/null)" = tmpfs ] || fail
		fi
	done
	parent=$(dirname "$run_dir")
	[ "$(findmnt -n -o FSTYPE --target "$parent" 2>/dev/null)" = tmpfs ] || fail
	if [ -d "$run_dir" ]; then
		[ "$(findmnt -n -o FSTYPE --target "$run_dir" 2>/dev/null)" = tmpfs ] || fail
	fi
}

remove_runtime_outputs() {
	rm -f "$run_dir/compose.env"
	rm -f "$run_dir/rds-ca.pem"
	rm -f "$run_dir/caddy/origin.crt" "$run_dir/caddy/origin.key" "$run_dir/caddy/proxy-header"
	for name in DATABASE_URL SESSION_SECRET JOBCRON_CREDENTIAL_ENCRYPTION_KEY JOBCRON_SIGNUP_ACCESS_CODE JOBCRON_PROXY_SECRET; do
		rm -f "$run_dir/secrets/$name"
	done
	rmdir "$run_dir/secrets" 2>/dev/null || true
	rmdir "$run_dir/caddy" 2>/dev/null || true
}

cleanup() {
	remove_runtime_outputs
	rm -rf -- "$run_dir/docker" "$run_dir/archive"
	rmdir "$run_dir" 2>/dev/null || true
}

prepare() {
	check_runtime_custody
	remove_runtime_outputs
	[ ! -e "$run_dir/secrets" ] || fail
	if [ "$etc_dir" = /etc/jobcron ]; then [ "$(id -u)" = 0 ] || fail; fi
	[ ! -L "$etc_dir" ] && [ -d "$etc_dir" ] || fail
	[ "$(mode "$etc_dir")" = 700 ] && [ "$(owner "$etc_dir")" = "$(id -u)" ] || fail
	ca_file=$etc_dir/rds-ca.pem
	[ ! -L "$ca_file" ] && [ -f "$ca_file" ] && [ -s "$ca_file" ] || fail
	[ "$(mode "$ca_file")" = 600 ] && [ "$(owner "$ca_file")" = "$(id -u)" ] || fail
	openssl x509 -in "$ca_file" -noout >/dev/null 2>&1 || fail
	[ ! -L "$secret_id_file" ] || fail
	[ -f "$secret_id_file" ] || fail
	[ "$(mode "$secret_id_file")" = 600 ] || fail
	[ "$(owner "$secret_id_file")" = "$(id -u)" ] || fail
	[ "$(awk 'END { print NR }' "$secret_id_file")" = 1 ] || fail
	secret_id=$(sed -n '1p' "$secret_id_file")
	[ -n "$secret_id" ] || fail

	mkdir -p "$run_dir"
	chmod 700 "$run_dir"
	tmp_dir=$(mktemp -d "$run_dir/.prepare.XXXXXX")
	trap 'rm -rf -- "$tmp_dir"' EXIT HUP INT TERM
	cp "$ca_file" "$tmp_dir/rds-ca.pem"
	chmod 600 "$tmp_dir/rds-ca.pem"

	if ! aws secretsmanager get-secret-value \
		--secret-id "$secret_id" \
		--query SecretString \
		--output text >"$tmp_dir/secret.json" 2>/dev/null; then
		fail
	fi

	if ! jq -e '
		keys == [
			"DATABASE_URL",
			"JOBCRON_CREDENTIAL_ENCRYPTION_KEY",
			"JOBCRON_IMAGE",
			"JOBCRON_PROXY_SECRET",
			"JOBCRON_SIGNUP_ACCESS_CODE",
			"JOBCRON_STAGE1_SPONSOR_USER_ID",
			"ORIGIN_CA_CERT",
			"ORIGIN_CA_KEY",
			"SESSION_SECRET"
		]
		and all(.[]; type == "string" and length > 0)
		and all(to_entries[];
			(.key | startswith("ORIGIN_CA_")) or
			((.value | contains("\n") or contains("\r")) | not)
		)
		and (.JOBCRON_IMAGE | test("^ghcr\\.io/[a-z0-9]([a-z0-9-]{0,37}[a-z0-9])?/jobcron@sha256:[0-9a-f]{64}$"))
		and (.JOBCRON_STAGE1_SPONSOR_USER_ID | test("^[1-9][0-9]*$"))
		and (.JOBCRON_PROXY_SECRET | test("^[A-Za-z0-9_-]{16,128}$"))
		and all(.[]; (contains("\u0000") | not))
		and (.ORIGIN_CA_CERT | startswith("-----BEGIN CERTIFICATE-----"))
		and (.ORIGIN_CA_KEY | test("^-----BEGIN ([A-Z ]+ )?PRIVATE KEY-----"))
	' "$tmp_dir/secret.json" >/dev/null 2>&1; then
		fail
	fi
	database_url=$(jq -j '.DATABASE_URL' "$tmp_dir/secret.json")
	validate_database_url "$database_url"
	unset database_url

	jq -r '
		to_entries[]
		| select(.key == "JOBCRON_IMAGE" or .key == "JOBCRON_STAGE1_SPONSOR_USER_ID")
		| "\(.key)=\(.value)"
	' "$tmp_dir/secret.json" >"$tmp_dir/compose.env"
	jq -r '.ORIGIN_CA_CERT' "$tmp_dir/secret.json" >"$tmp_dir/origin.crt"
	jq -r '.ORIGIN_CA_KEY' "$tmp_dir/secret.json" >"$tmp_dir/origin.key"
	chmod 600 "$tmp_dir/compose.env" "$tmp_dir/origin.crt" "$tmp_dir/origin.key"
	mkdir "$tmp_dir/secrets"
	for name in DATABASE_URL SESSION_SECRET JOBCRON_CREDENTIAL_ENCRYPTION_KEY JOBCRON_SIGNUP_ACCESS_CODE JOBCRON_PROXY_SECRET; do
		jq -j --arg name "$name" '.[$name]' "$tmp_dir/secret.json" >"$tmp_dir/secrets/$name"
		chmod 600 "$tmp_dir/secrets/$name"
	done
	jq -j '"header_up X-Jobcron-Proxy " + .JOBCRON_PROXY_SECRET + "\n"' "$tmp_dir/secret.json" >"$tmp_dir/proxy-header"

	mkdir -p "$run_dir/caddy"
	chmod 700 "$run_dir/caddy"
	mv "$tmp_dir/compose.env" "$run_dir/compose.env"
	mv "$tmp_dir/origin.crt" "$run_dir/caddy/origin.crt"
	mv "$tmp_dir/origin.key" "$run_dir/caddy/origin.key"
	mv "$tmp_dir/proxy-header" "$run_dir/caddy/proxy-header"
	mv "$tmp_dir/rds-ca.pem" "$run_dir/rds-ca.pem"
	mv "$tmp_dir/secrets" "$run_dir/secrets"
	rm -f "$tmp_dir/secret.json"
	rmdir "$tmp_dir"
	trap - EXIT HUP INT TERM
}

compose_value() {
	sed -n "s/^$1=//p" "$run_dir/compose.env"
}

pull() {
	[ -f "$run_dir/compose.env" ] || fail
	image=$(compose_value JOBCRON_IMAGE)
	printf '%s\n' "$image" |
		grep -Eq '^ghcr\.io/[a-z0-9]([a-z0-9-]{0,37}[a-z0-9])?/jobcron@sha256:[0-9a-f]{64}$' || fail
	registry_owner=${image#ghcr.io/}
	registry_owner=${registry_owner%%/jobcron@sha256:*}
	token_file=$run_dir/registry-token
	if docker image inspect "$image" >/dev/null 2>&1; then
		rm -f "$token_file"
		return
	fi

	[ -f "$token_file" ] || fail
	[ "$(mode "$token_file")" = 600 ] || fail
	[ -s "$token_file" ] || fail
	rm -f "$docker_config/config.json"
	rmdir "$docker_config" 2>/dev/null || true
	mkdir "$docker_config"
	chmod 700 "$docker_config"
	cleanup_pull() {
		DOCKER_CONFIG=$docker_config docker logout ghcr.io >/dev/null 2>&1 || true
		rm -f "$docker_config/config.json" "$token_file"
		rmdir "$docker_config" 2>/dev/null || true
	}
	trap 'cleanup_pull' EXIT HUP INT TERM
	DOCKER_CONFIG=$docker_config docker login ghcr.io \
		--username "$registry_owner" --password-stdin <"$token_file" >/dev/null 2>&1 || fail
	DOCKER_CONFIG=$docker_config docker pull "$image" >/dev/null 2>&1 || fail
	cleanup_pull
	trap - EXIT HUP INT TERM
}

sanitize_logs() {
	sed -E \
		-e 's/([Aa]uthorization:[[:space:]]*[Bb]earer[[:space:]]+)[^[:space:]",}]+/\1[redacted]/g' \
		-e 's/([Cc]ookie:[[:space:]]*).*/\1[redacted]/g' \
		-e 's/(([Pp]assword|[Ss]ecret|[Tt]oken):[[:space:]]*)[^[:space:]",}]+/\1[redacted]/g' \
		-e 's/("([Pp]assword|[Ss]ecret|[Tt]oken)"[[:space:]]*:[[:space:]]*)"[^"]*"/\1"[redacted]"/g' \
		-e 's#(postgres(ql)?://)[^[:space:]@]+@#\1[redacted]@#g' \
		-e 's/(([Aa]uthorization|[Cc]ookie|[Pp]assword|[Ss]ecret|[Tt]oken)=[[:space:]]*)[^[:space:]",}]+/\1[redacted]/g'
}

percent_decode() {
	encoded=$1
	while [ -n "$encoded" ]; do
		case $encoded in
		%??*)
			hex=${encoded#%}
			hex=${hex%"${hex#??}"}
			case $hex in *[!0-9A-Fa-f]*) return 1 ;; esac
			value=$((0x$hex))
			[ "$value" -ne 0 ] || return 1
			octal=$(printf '%03o' "$value")
			printf '%b' "\0$octal"
			encoded=${encoded#???}
			;;
		*)
			printf '%s' "${encoded%"${encoded#?}"}"
			encoded=${encoded#?}
			;;
		esac
	done
}

archive() {
	check_runtime_custody
	if env | grep -q '^PG'; then fail; fi
	[ -f "$run_dir/compose.env" ] || fail
	[ -n "${JOBCRON_RECOVERY_BUCKET:-}" ] || fail
	database_url=$(cat "$run_dir/secrets/DATABASE_URL")
	[ -n "$database_url" ] || fail
	validate_database_url "$database_url"
	ca_file=$run_dir/rds-ca.pem
	[ ! -L "$ca_file" ] && [ -f "$ca_file" ] && [ -s "$ca_file" ] || fail
	[ "$(mode "$ca_file")" = 600 ] && [ "$(owner "$ca_file")" = "$(id -u)" ] || fail
	[ "$(findmnt -n -o FSTYPE --target "$ca_file" 2>/dev/null)" = tmpfs ] || fail
	openssl x509 -in "$ca_file" -noout >/dev/null 2>&1 || fail
	# Single explicit installed executable, never a shell command or download.
	pg_dump_command=${JOBCRON_PG_DUMP:-pg_dump}
	case $pg_dump_command in
	pg_dump) ;;
	/*/pg_dump) [ -f "$pg_dump_command" ] && [ -x "$pg_dump_command" ] || fail ;;
	*) fail ;;
	esac
	authority=${database_url#postgres://}
	userinfo=${authority%%@*}
	connection=${authority#*@}
	database_user=${userinfo%%:*}
	encoded_password=${userinfo#*:}
	database_endpoint=${connection%%/*}
	database_port=${database_endpoint##*:}
	[ "$database_port" -ge 1 ] 2>/dev/null && [ "$database_port" -le 65535 ] 2>/dev/null || fail
	database_password=$(percent_decode "$encoded_password" && printf '%s' x) || fail
	database_password=${database_password%x}
	[ -n "$database_password" ] || fail
	password_free_url="postgres://$database_user@$connection"
	now=${JOBCRON_NOW:-$(date -u +%Y%m%dT%H%M%SZ)}
	printf '%s\n' "$now" | grep -Eq '^[0-9]{8}T[0-9]{6}Z$' || fail
	archive_dir=$run_dir/archive
	mkdir -p "$archive_dir"
	chmod 700 "$archive_dir"
	jobcron_raw=$(mktemp "$archive_dir/.jobcron.XXXXXX.raw")
	caddy_raw=$(mktemp "$archive_dir/.caddy.XXXXXX.raw")
	chmod 600 "$jobcron_raw" "$caddy_raw"
	cleanup_archive_raw() {
		rm -f "$jobcron_raw" "$caddy_raw"
	}
	trap 'cleanup_archive_raw' EXIT HUP INT TERM

	# libpq expands a URI only when it is the dbname argument; keep its password off argv.
	PGPASSWORD=$database_password "$pg_dump_command" --dbname="$password_free_url" -Fc \
		-f "$archive_dir/database.dump" >/dev/null 2>&1 || fail
	unset database_password encoded_password
	(cd "$deploy_dir" && docker compose --env-file "$run_dir/compose.env" logs --no-color app >"$jobcron_raw") || fail
	(cd "$deploy_dir" && docker compose --env-file "$run_dir/compose.env" logs --no-color caddy >"$caddy_raw") || fail
	sanitize_logs <"$jobcron_raw" >"$archive_dir/jobcron.log"
	sanitize_logs <"$caddy_raw" >"$archive_dir/caddy.log"
	cleanup_archive_raw
	trap - EXIT HUP INT TERM
	chmod 600 "$archive_dir/database.dump" "$archive_dir/jobcron.log" "$archive_dir/caddy.log"

	for name in database.dump jobcron.log caddy.log; do
		(cd "$archive_dir" && sha256sum "$name" >"$name.sha256")
		chmod 600 "$archive_dir/$name.sha256"
	done
	key="s3://$JOBCRON_RECOVERY_BUCKET/jobcron/$now"
	for name in database.dump jobcron.log caddy.log; do
		aws s3 cp "$archive_dir/$name" "$key/$name" >/dev/null 2>&1 || fail
	done
	for name in database.dump.sha256 jobcron.log.sha256 caddy.log.sha256; do
		aws s3 cp "$archive_dir/$name" "$key/$name" >/dev/null 2>&1 || fail
	done
}

bool_mode() {
	if [ -e "$1" ] && [ "$(mode "$1")" = "$2" ]; then
		printf true
	else
		printf false
	fi
}

verify_local_state() {
	compose_env=$run_dir/compose.env
	image=
	[ ! -f "$compose_env" ] || image=$(compose_value JOBCRON_IMAGE)
	current_digest_count=0
	previous_digest_count=0
	if [ -n "$image" ]; then
		images=$(docker image ls --digests --format '{{.Repository}}@{{.Digest}}' 2>/dev/null || true)
		repository=${image%@sha256:*}
		current_digest_count=$(printf '%s\n' "$images" |
			awk -v current="$image" '$0 == current { count++ } END { print count + 0 }')
		previous_digest_count=$(printf '%s\n' "$images" |
			awk -v prefix="$repository@" -v current="$image" \
				'index($0, prefix) == 1 && $0 != current { count++ } END { print count + 0 }')
	fi
	docker_config=$(mktemp "$run_dir/.docker-config.XXXXXX")
	trap 'rm -f "$docker_config"' EXIT HUP INT TERM
	docker_json_file_logging=false
	docker_log_rotation=false
	if (cd "$deploy_dir" && docker compose config --no-interpolate --format json >"$docker_config" 2>/dev/null); then
		if jq -e '[.services.app.logging.driver, .services.caddy.logging.driver] | all(. == "json-file")' \
			"$docker_config" >/dev/null 2>&1; then
			docker_json_file_logging=true
		fi
		if jq -e '
			[.services.app.logging.options, .services.caddy.logging.options]
			| all(."max-size" == "10m" and ."max-file" == "3")
		' "$docker_config" >/dev/null 2>&1; then
			docker_log_rotation=true
		fi
	fi
	rm -f "$docker_config"
	trap - EXIT HUP INT TERM
	disk_free_kib=$(df -Pk "$run_dir" | awk 'NR == 2 { print $4 }')
	disk_free_bytes=$((disk_free_kib * 1024))
	printf 'run_dir_private=%s\n' "$(bool_mode "$run_dir" 700)"
	printf 'compose_env_private=%s\n' "$(bool_mode "$compose_env" 600)"
	printf 'origin_cert_private=%s\n' "$(bool_mode "$run_dir/caddy/origin.crt" 600)"
	printf 'origin_key_private=%s\n' "$(bool_mode "$run_dir/caddy/origin.key" 600)"
	printf 'persistent_docker_credentials=%s\n' "$([ ! -e "${HOME:?}/.docker/config.json" ] && printf false || printf true)"
	printf 'docker_json_file_logging=%s\n' "$docker_json_file_logging"
	printf 'docker_log_rotation=%s\n' "$docker_log_rotation"
	printf 'current_digest_count=%s\n' "$current_digest_count"
	printf 'previous_digest_count=%s\n' "$previous_digest_count"
	printf 'disk_free_bytes=%s\n' "$disk_free_bytes"
}

verify_secrets() {
	check_runtime_custody
	inspection=$(mktemp -d "$run_dir/.inspect.XXXXXX")
	trap 'rm -rf -- "$inspection"' EXIT HUP INT TERM
	proxy_secret=$run_dir/secrets/JOBCRON_PROXY_SECRET
	proxy_header=$run_dir/caddy/proxy-header
	for file in "$proxy_secret" "$proxy_header"; do
		[ ! -L "$file" ] && [ -f "$file" ] && [ -s "$file" ] || fail
		[ "$(mode "$file")" = 600 ] && [ "$(owner "$file")" = "$(id -u)" ] || fail
		[ "$(findmnt -n -o FSTYPE --target "$file" 2>/dev/null)" = tmpfs ] || fail
	done
	jq -n -e --rawfile secret "$proxy_secret" '
		($secret | length) >= 16 and ($secret | length) <= 128 and
		($secret | (contains("\n") or contains("\r") or contains("\u0000")) | not) and
		($secret | test("^[A-Za-z0-9_-]+$"))
	' >/dev/null 2>&1 || fail
	printf 'header_up X-Jobcron-Proxy %s\n' "$(cat "$proxy_secret")" >"$inspection/expected-proxy-header"
	chmod 600 "$inspection/expected-proxy-header"
	cmp -s "$inspection/expected-proxy-header" "$proxy_header" || fail
	for service in app caddy; do
		container=$(cd "$deploy_dir" && docker compose --env-file "$run_dir/compose.env" ps -q "$service" 2>/dev/null) || fail
		[ -n "$container" ] || fail
		docker inspect "$container" >"$inspection/$service.json" 2>/dev/null || fail
		jq -e 'type == "array" and length == 1 and (.[0].Config.Env | type == "array")' "$inspection/$service.json" >/dev/null 2>&1 || fail
		jq -e '.[0].HostConfig.Ulimits |
			map(select(.Name == "core")) as $core |
			($core | length) == 1 and $core[0].Hard == 0 and $core[0].Soft == 0
		' "$inspection/$service.json" >/dev/null 2>&1 || fail
		for name in DATABASE_URL SESSION_SECRET JOBCRON_CREDENTIAL_ENCRYPTION_KEY JOBCRON_SIGNUP_ACCESS_CODE JOBCRON_PROXY_SECRET; do
			file=$run_dir/secrets/$name
			[ ! -L "$file" ] && [ -f "$file" ] && [ -s "$file" ] || fail
			[ "$(mode "$file")" = 600 ] && [ "$(owner "$file")" = "$(id -u)" ] || fail
			[ "$(findmnt -n -o FSTYPE --target "$file" 2>/dev/null)" = tmpfs ] || fail
			jq -e --rawfile secret "$file" --arg name "$name" '
				all(.. | strings; contains($secret) | not) and
				all(.[0].Config.Env[]; startswith($name + "=") | not)
			' "$inspection/$service.json" >/dev/null 2>&1 || fail
			if [ "$service" = app ]; then
				jq -e --arg name "$name" '.[0].Config.Env | map(select(. == ($name + "_FILE=/run/jobcron/secrets/" + $name))) | length == 1' "$inspection/app.json" >/dev/null 2>&1 || fail
			fi
		done
	done
	jq -e '.[0].Mounts | map(select(.Type == "bind" and .Source == "/run/jobcron/secrets" and .Destination == "/run/jobcron/secrets" and .RW == false)) | length == 1' "$inspection/app.json" >/dev/null 2>&1 || fail
	jq -e '.[0].Mounts | map(select(.Type == "bind" and .Source == "/run/jobcron/rds-ca.pem" and .Destination == "/run/jobcron/rds-ca.pem" and .RW == false)) | length == 1' "$inspection/app.json" >/dev/null 2>&1 || fail
	jq -e '.[0] |
		.HostConfig.ReadonlyRootfs == true and
		(.HostConfig.Tmpfs | keys | sort) == ["/config", "/data", "/tmp"] and
		all(.HostConfig.Tmpfs[]; . == "mode=0700") and
		all(.Mounts[]; .Type != "volume" and (.Type != "bind" or .RW == false)) and
		([.Mounts[] | select(.Type == "bind" and .Source == "/run/jobcron/caddy" and .Destination == "/run/jobcron/caddy" and .RW == false)] | length == 1)
	' "$inspection/caddy.json" >/dev/null 2>&1 || fail
	rm -rf -- "$inspection"
	trap - EXIT HUP INT TERM
	printf '%s\n' 'runtime_secret_metadata_safe=true'
}

case ${1:-} in
prepare) prepare ;;
pull) pull ;;
archive) archive ;;
cleanup) cleanup ;;
verify-local-state) verify_local_state ;;
verify-secrets) verify_secrets ;;
*) fail ;;
esac
