#!/bin/bash

python rag/svr/task_broker.py &

function task_exe(){
  while [ 1 -eq 1 ];do mpirun -n 2 python rag/svr/task_executor.py ; done
}

function watch_broker(){
  while [ 1 -eq 1];do
    C=`ps aux|grep "task_broker.py"|grep -v grep|wc -l`;
    if [ $C -lt 1 ];then
      python rag/svr/task_broker.py &
    fi
    sleep 5;
  done
}


task_exe &
sleep 10;
watch_broker &

python api/ragflow_server.py