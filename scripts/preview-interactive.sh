#!/bin/sh
set -eu

port=${1:-17778}
case "$port" in
	*[!0-9]*|'')
		echo "usage: $0 [port]" >&2
		exit 2
		;;
esac

state_dir=$(mktemp -d "${TMPDIR:-/tmp}/job-scraper-preview.XXXXXX")
binary="$state_dir/job-scraper"
db="$state_dir/jobs.db"

cleanup() {
	if [ "${JOBCRON_PREVIEW_KEEP:-}" != "1" ]; then
		rm -rf "$state_dir"
	fi
}
trap cleanup EXIT HUP INT TERM

echo "Preview state: $state_dir"
echo "Preview URL: http://127.0.0.1:$port"

go build -o "$binary" ./cmd/jobcron

export HOME="$state_dir"
export JOBSCRAPER_DB="$db"

if [ "${JOBCRON_PREVIEW_KEEP:-}" = "1" ]; then
	trap - EXIT HUP INT TERM
	exec "$binary" --host 127.0.0.1 --port "$port" --db "$db" --no-open
fi

"$binary" --host 127.0.0.1 --port "$port" --db "$db" --no-open &
child=$!
trap 'kill "$child" 2>/dev/null || true; cleanup; exit 0' HUP INT TERM
wait "$child"
