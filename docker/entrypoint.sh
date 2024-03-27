#!/bin/bash

/usr/sbin/nginx

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

PY=/root/miniconda3/envs/py11/bin/python

function task_exe(){
    while [ 1 -eq 1 ];do
      $PY rag/svr/task_executor.py $1 $2;
    done
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

WS=2
for ((i=0;i<WS;i++))
do
  task_exe $i $WS &
done

$PY api/ragflow_server.py

wait;