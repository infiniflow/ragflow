#!/bin/sh
# Kvrocks entrypoint for the RAGFlow cache service.
#
# Kvrocks 2.16.0 does NOT support ${ENV} expansion inside its config file and
# rejects `--requirepass` as a CLI flag (gflags intercepts the config key before
# Kvrocks' own handler runs). To preserve the password-based auth from the
# previous Valkey deployment without committing the secret, we render the config
# from the REDIS_PASSWORD environment variable (injected via the compose
# env_file) at container start.
#
# This service is configured with `user: "0"` so the entrypoint can write the
# generated config and the RocksDB data directory regardless of the mounted
# volume's ownership; kvrocks then runs as root. (The upstream image normally
# drops to an unprivileged user via its own entrypoint, which we override here.)
#
# Config keys verified against apache/kvrocks:2.16.0:
#   bind, port, dir, requirepass
# Intentionally omitted (invalid in kvrocks 2.16.0): maxmemory, maxmemory-policy.
set -eu

mkdir -p /var/lib/kvrocks

{
  echo "bind 0.0.0.0"
  echo "port 6379"
  echo "dir /var/lib/kvrocks"
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    printf 'requirepass %s\n' "$REDIS_PASSWORD"
  fi
} > /var/lib/kvrocks/kvrocks.conf

exec kvrocks -c /var/lib/kvrocks/kvrocks.conf --dir /var/lib/kvrocks
