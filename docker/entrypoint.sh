#!/bin/bash

/usr/sbin/nginx

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

PY=/root/miniconda3/envs/py11/bin/python



function task_exe(){
  sleep 60;
  while [ 1 -eq 1 ];do mpirun -n 2 --allow-run-as-root $PY rag/svr/task_executor.py ; done
}

function watch_broker(){
  while [ 1 -eq 1 ];do
    C=`ps aux|grep "task_broker.py"|grep -v grep|wc -l`;
    if [ $C -lt 1 ];then
       $PY rag/svr/task_broker.py &
    fi
    sleep 5;
  done
}

function task_bro(){
    sleep 60;
    watch_broker;
}

task_bro &
task_exe &

$PY api/ragflow_server.py