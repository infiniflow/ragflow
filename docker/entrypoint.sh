#!/bin/bash

/usr/sbin/nginx

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

PY=/root/miniconda3/envs/py11/bin/python

function task_exe(){
    while [ 1 -eq 1 ];do
      $PY rag/svr/task_executor.py $1 $2;
    done
}

WS=1
for ((i=0;i<WS;i++))
do
  task_exe $i $WS &
done

while [ 1 -eq 1 ];do
    $PY api/ragflow_server.py
done

wait;
