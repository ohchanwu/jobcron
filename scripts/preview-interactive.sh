#!/bin/sh
set -eu

umask 077

if [ "$#" -gt 1 ]; then
	echo "usage: $0 [port]" >&2
	exit 2
fi

port=${1:-17778}
case "$port" in
	*[!0-9]*|'')
		echo "usage: $0 [port]" >&2
		exit 2
		;;
esac
if [ ${#port} -gt 5 ] || ! [ "$port" -ge 1 ] 2>/dev/null || [ "$port" -gt 65535 ]; then
	echo "usage: $0 [port]" >&2
	exit 2
fi

if [ -n "${DATABASE_URL:-}" ]; then
	echo "preview: refusing inherited DATABASE_URL; the preview generates its own loopback database" >&2
	exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
compose_file="$repo_root/deploy/local/compose.yaml"
compose_project=jobcron-local
DOCKER_CONFIG=${DOCKER_CONFIG:-"$HOME/.docker"}
export DOCKER_CONFIG

compose() {
	docker compose -p "$compose_project" -f "$compose_file" "$@"
}

shell_quote() {
	printf "'"
	printf '%s' "$1" | sed "s/'/'\\\\''/g"
	printf "'"
}

state_dir=
database=
database_created=0
key_file=
child=
cleanup_started=0
lock_root=
port_lock=
port_lock_owner=
port_lock_acquired=0
ownership_critical=0
pending_signal=
pending_signal_status=0

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ "$cleanup_started" -eq 1 ]; then
		exit "$status"
	fi
	cleanup_started=1

	if [ -n "$child" ] && kill -0 "$child" 2>/dev/null; then
		kill -TERM "$child" 2>/dev/null || true
		wait "$child" 2>/dev/null || true
	fi
	child=

	if [ "${JOBCRON_PREVIEW_KEEP:-}" = "1" ]; then
		if [ "$database_created" -eq 1 ] && [ -n "$state_dir" ]; then
			echo "Preview retained:"
			echo "  Database: $database"
			echo "  Key: $key_file"
			echo "Manual cleanup commands:"
			printf '  docker compose -p %s -f ' "$compose_project"
			shell_quote "$compose_file"
			printf ' exec -T postgres dropdb --if-exists --force %s -U postgres\n' "$database"
			printf '  rm -rf '
			shell_quote "$state_dir"
			printf '\n'
		fi
	else
		if [ "$database_created" -eq 1 ]; then
			if ! compose exec -T postgres dropdb --if-exists --force "$database" -U postgres >/dev/null 2>&1; then
				echo "preview: failed to drop disposable database $database" >&2
				status=1
			fi
		fi
		if [ -n "$state_dir" ]; then
			rm -rf "$state_dir"
		fi
	fi
	if [ "$port_lock_acquired" -eq 1 ]; then
		rm -f "$port_lock_owner"
		if rmdir "$port_lock" 2>/dev/null; then
			port_lock_acquired=0
		else
			if [ "$status" -eq 0 ]; then
				status=1
			fi
			echo "preview: failed to release port lock: $port_lock" >&2
			printf 'preview: remove it manually: rmdir ' >&2
			shell_quote "$port_lock" >&2
			printf '\n' >&2
		fi
	fi
	exit "$status"
}

stop() {
	status=${1:-0}
	trap - HUP INT TERM
	if [ -n "$child" ] && kill -0 "$child" 2>/dev/null; then
		kill -TERM "$child" 2>/dev/null || true
		wait "$child" 2>/dev/null || true
	fi
	child=
	exit "$status"
}

handle_signal() {
	signal=$1
	interrupted_status=$2
	if [ "$ownership_critical" -eq 1 ]; then
		pending_signal=$signal
		pending_signal_status=$interrupted_status
		return 0
	fi
	stop 0
}

begin_ownership_critical() {
	pending_signal=
	pending_signal_status=0
	ownership_critical=1
}

end_ownership_critical() {
	ownership_critical=0
	if [ -n "$pending_signal" ]; then
		status=$pending_signal_status
		pending_signal=
		pending_signal_status=0
		stop "$status"
	fi
	return 0
}

trap cleanup EXIT
trap 'handle_signal HUP $?' HUP
trap 'handle_signal INT $?' INT
trap 'handle_signal TERM $?' TERM

uid=$(id -u)
case "$uid" in
	*[!0-9]*|'')
		echo "preview: could not determine a safe numeric user ID for the port lock" >&2
		exit 1
		;;
esac
lock_root="/tmp/jobcron-preview-locks-$uid"
if ! mkdir -m 700 "$lock_root" 2>/dev/null; then
	if [ ! -d "$lock_root" ] || [ -L "$lock_root" ]; then
		echo "preview: refusing unsafe port lock root: $lock_root" >&2
		exit 1
	fi
	lock_root_owner=$(LC_ALL=C ls -dn "$lock_root" | awk '{print $3}')
	if [ "$lock_root_owner" != "$uid" ]; then
		echo "preview: refusing port lock root owned by another user: $lock_root" >&2
		exit 1
	fi
	chmod 700 "$lock_root"
fi
port_lock="$lock_root/port-$port.lock"
port_lock_owner="$port_lock.owner.pid"
begin_ownership_critical
if mkdir -m 700 "$port_lock" 2>/dev/null; then
	port_lock_acquired=1
	printf '%s\n' "$$" >"$port_lock_owner"
	end_ownership_critical
else
	lock_status=$?
	end_ownership_critical
	echo "preview: requested loopback port is already in use: 127.0.0.1:$port" >&2
	if [ -f "$port_lock_owner" ]; then
		lock_owner_pid=$(sed -n '1p' "$port_lock_owner")
		case "$lock_owner_pid" in
			*[!0-9]*|'') ;;
			*)
				if ! kill -0 "$lock_owner_pid" 2>/dev/null; then
					printf 'preview: stale port lock; remove it manually: rmdir ' >&2
					shell_quote "$port_lock" >&2
					printf '\n' >&2
				fi
				;;
		esac
	fi
	exit "$lock_status"
fi

if ! command -v nc >/dev/null 2>&1; then
	echo "preview: nc is required to verify loopback port availability" >&2
	exit 1
fi
if nc -z 127.0.0.1 "$port" >/dev/null 2>&1; then
	echo "preview: requested loopback port is already in use: 127.0.0.1:$port" >&2
	exit 1
fi

compose up -d --wait --wait-timeout 60 postgres >/dev/null
if ! port_owner=$(docker ps --filter publish=55432 \
	--format '{{.Label "com.docker.compose.project"}}|{{.Label "com.docker.compose.service"}}|{{.Names}}'); then
	echo "preview: could not verify the owner of host port 55432" >&2
	exit 1
fi
if [ "$port_owner" != "jobcron-local|postgres|jobcron-local-postgres-1" ]; then
	echo "preview: refusing foreign or ambiguous owner of host port 55432" >&2
	exit 1
fi
if ! nc -z 127.0.0.1 55432 >/dev/null 2>&1; then
	echo "preview: PostgreSQL is healthy in Compose but unreachable at 127.0.0.1:55432" >&2
	exit 1
fi

suffix=$(LC_ALL=C od -An -N 8 -tx1 /dev/urandom | tr -d ' \n')
case "$suffix" in
	????????????????) ;;
	*)
		echo "preview: invalid generated database suffix" >&2
		exit 1
		;;
esac
case "$suffix" in
	*[!0-9a-f]*)
		echo "preview: invalid generated database suffix" >&2
		exit 1
		;;
esac
database="jobcron_preview_$suffix"
state_dir=$(mktemp -d "${TMPDIR:-/tmp}/jobcron-preview.XXXXXX")
key_file="$state_dir/credential-encryption.key"
binary="$state_dir/jobcron"
bootstrap_log="$state_dir/bootstrap.log"

dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 >"$key_file"
begin_ownership_critical
if compose exec -T postgres createdb -U postgres "$database"; then
	database_created=1
	end_ownership_critical
else
	createdb_status=$?
	end_ownership_critical
	exit "$createdb_status"
fi

cd "$repo_root"
go build -o "$binary" ./cmd/jobcron

export HOME="$state_dir"
export XDG_CONFIG_HOME="$state_dir/config"
export DATABASE_URL="postgres://postgres@127.0.0.1:55432/$database?sslmode=disable"
export JOBCRON_CREDENTIAL_ENCRYPTION_KEY
JOBCRON_CREDENTIAL_ENCRYPTION_KEY=$(tr -d '\r\n' <"$key_file")
export JOBCRON_NO_OPEN=1
export JOBCRON_HOST=127.0.0.1
export JOBCRON_PORT="$port"
export JOBCRON_STRICT_PORT=1
unset JOBCRON_ENV JOBCRON_DAILY_SCRAPE_TIME
export JOBCRON_SCHEDULER_ENABLED=0

# Opening the empty database once applies the app's embedded migrations. The
# explicit PostgreSQL owner contract then rejects the still-empty users table;
# seed only the fixed no-login local owner before the real preview starts.
if "$binary" >"$bootstrap_log" 2>&1; then
	echo "preview: empty database bootstrap unexpectedly started the server" >&2
	exit 1
fi
if ! grep -Fq "explicit DATABASE_URL requires exactly one existing user" "$bootstrap_log"; then
	echo "preview: database bootstrap failed before the expected missing-owner gate" >&2
	exit 1
fi
compose exec -T postgres psql -U postgres -d "$database" -v ON_ERROR_STOP=1 \
	-c "INSERT INTO users (email, password_hash, created_at, updated_at) VALUES ('local-owner@jobcron.example.invalid', '\$jobcron\$local-login-disabled', now(), now())" \
	>/dev/null

echo "Preview state: $state_dir"
echo "Preview database: $database"
echo "Preview key: $key_file"
echo "Preview URL: http://127.0.0.1:$port"

"$binary" &
child=$!
if wait "$child"; then
	status=0
else
	status=$?
fi
child=
exit "$status"
