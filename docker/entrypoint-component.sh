#!/bin/bash

set -e

# unset http proxy which maybe set by docker daemon
export http_proxy=""; export https_proxy=""; export no_proxy=""; export HTTP_PROXY=""; export HTTPS_PROXY=""; export NO_PROXY=""

#/usr/sbin/nginx

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

if [[ "${MODE}" == "worker" ]]; then
  if [[ -z "$WS" || $WS -lt 1 ]]; then
    WS=1
  fi

  function task_exe(){
      while [ 1 -eq 1 ];do
        exec python3 rag/svr/task_executor.py $1;
      done
  }

  timestamp=$(date +%s)
  unique_id="timestamp_$timestamp"
  task_exe $unique_id &

else
  while [ 1 -eq 1 ];do
      exec python3 api/ragflow_server.py
  done
fi

wait;
