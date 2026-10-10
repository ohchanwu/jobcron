#!/bin/sh
# Owned fixture only: enable real PostgreSQL TLS without host DB publication.
set -eu
umask 077
mkdir -p /tmp/jobcron-fixture-tls
cp /fixture-tls/origin.crt /tmp/jobcron-fixture-tls/server.crt
cp /fixture-tls/origin.key /tmp/jobcron-fixture-tls/server.key
chown postgres:postgres /tmp/jobcron-fixture-tls /tmp/jobcron-fixture-tls/server.crt /tmp/jobcron-fixture-tls/server.key
chmod 600 /tmp/jobcron-fixture-tls/server.crt /tmp/jobcron-fixture-tls/server.key
# Upstream creates versioned parents as root, then drops to postgres.
umask 022
mkdir -p /var/lib/postgresql/18
chown postgres:postgres /var/lib/postgresql/18
chmod 755 /var/lib/postgresql/18
exec /usr/local/bin/docker-entrypoint.sh postgres \
  -c ssl=on \
  -c ssl_cert_file=/tmp/jobcron-fixture-tls/server.crt \
  -c ssl_key_file=/tmp/jobcron-fixture-tls/server.key
