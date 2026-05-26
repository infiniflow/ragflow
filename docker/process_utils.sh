#!/usr/bin/env bash

wait_for_server() {
    local url="$1"
    local server_name="$2"
    local timeout=90
    local interval=2
    local start_time
    start_time=$(date +%s)

    echo "Waiting for ${server_name} to be ready at ${url}..."
    while ! curl -f -s -o /dev/null "$url"; do
        if [ $(($(date +%s) - start_time)) -gt $timeout ]; then
            echo "Timeout waiting for ${server_name} after ${timeout} seconds"
            return 1
        fi
        sleep "${interval}"
    done
    echo "${server_name} is ready."
}

run_forever() {
    local name="$1"
    local max_restarts="${RUN_FOREVER_MAX_RESTARTS:-0}"
    local restarts=0
    shift

    while true; do
        echo "Starting ${name}..."
        if "$@"; then
            exit_code=0
            echo "${name} exited with code 0"
        else
            exit_code=$?
            echo "${name} exited with code ${exit_code}"
        fi

        restarts=$((restarts + 1))
        if [[ "${max_restarts}" -gt 0 && "${restarts}" -ge "${max_restarts}" ]]; then
            return 0
        fi

        sleep 1
    done
}
