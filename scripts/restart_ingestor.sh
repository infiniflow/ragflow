#!/usr/bin/env bash
#
# Restart the ingestor so it runs the freshly built bin/ragflow_server.
#
# Why this is a script and not an ad-hoc command: the re-parse deploy requires stopping the running
# ingestor, and stopping it is the one step that can cost in-flight work. The earlier incidents were
# exactly this - tasks delivered to an ingestor that died before it settled them, which JetStream
# then stops redelivering after MaxDeliver (16). So this script checks that nothing is in flight
# first, refuses to proceed if something is, and prints the queue state before and after.
#
# Usage:
#   bash scripts/restart_ingestor.sh
#
set -euo pipefail
cd "$(dirname "$0")/.."

NATS_BOX="natsio/nats-box"
NATS_NET="docker_ragflow"

queue_state() {
	docker run --rm --network "$NATS_NET" "$NATS_BOX" \
		nats --server nats:4222 consumer ls RAGFLOW_TASKS 2>/dev/null | grep RAGFLOW_CONSUMER || true
	docker exec docker-mysql-1 sh -c \
		'MYSQL_PWD=infini_rag_flow mysql -N -uroot rag_flow -e "SELECT CONCAT(\"documents run=1: \", COUNT(*)) FROM document WHERE run=1;"' 2>/dev/null || true
}

echo "=== before ==="
queue_state

inflight=$(docker run --rm --network "$NATS_NET" "$NATS_BOX" \
	nats --server nats:4222 consumer ls RAGFLOW_TASKS 2>/dev/null | grep RAGFLOW_CONSUMER | awk '{print $(NF-2)}' | tr -d '│ ')
if [ "${inflight:-0}" != "0" ]; then
	echo "refusing to restart: $inflight message(s) in flight; wait for them to settle" >&2
	exit 1
fi

echo "=== restarting ingestor ==="
pkill -f "ragflow_server --ingestor" || true
sleep 3
setsid nohup ./bin/ragflow_server --ingestor >>logs/ingestor_server.log 2>&1 &
sleep 30
pgrep -af "ragflow_server --ingestor" | cut -c1-60

echo "=== after ==="
queue_state
tail -3 logs/ingestor_server.log | cut -c1-120
