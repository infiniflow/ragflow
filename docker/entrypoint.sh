#!/bin/bash

/usr/sbin/nginx

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

PY=/root/miniconda3/envs/py11/bin/python

function watch_broker(){
  # shellcheck disable=SC2050
  while [ 1 -eq 1 ];do
    # shellcheck disable=SC2006
    # shellcheck disable=SC2126
    # shellcheck disable=SC2009
    C=`ps aux|grep "task_broker.py"|grep -v grep|wc -l`;
    if [ "$C" -lt 1 ];then
       $PY rag/svr/task_broker.py &
    fi
    sleep 5;
  done
}

function task_bro(){
    watch_broker;
}

task_bro &

$PY api/ragflow_server.py

wait;
