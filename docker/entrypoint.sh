#!/bin/bash

/usr/sbin/nginx

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

PY=python3

function task_exe(){
    while [ 1 -eq 1 ];do
      $PY rag/svr/task_executor.py ;
    done
}

WS=1
for ((i=0;i<WS;i++))
do
  task_exe  &
done

while [ 1 -eq 1 ];do
    $PY api/ragflow_server.py
done

wait;
