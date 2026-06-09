#!/bin/bash
set -euo pipefail

/usr/sbin/sshd
/root/boot/start.sh &
start_pid=$!

/root/boot/ragflow-ob-reconcile.sh || true &

wait "${start_pid}"
