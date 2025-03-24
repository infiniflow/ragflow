#!/bin/bash

# replace env variables in the service_conf.yaml file
rm -rf /ragflow/conf/service_conf.yaml
while IFS= read -r line || [[ -n "$line" ]]; do
    # Use eval to interpret the variable with default values
    eval "echo \"$line\"" >> /ragflow/conf/service_conf.yaml
done < /ragflow/conf/service_conf.yaml.template

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

PY=python3

CONSUMER_NO_BEG=$1
CONSUMER_NO_END=$2

function task_exe(){
    JEMALLOC_PATH=$(pkg-config --variable=libdir jemalloc)/libjemalloc.so
    while [ 1 -eq 1 ]; do
      LD_PRELOAD=$JEMALLOC_PATH $PY rag/svr/task_executor.py $1;
    done
}

for ((i=CONSUMER_NO_BEG; i<CONSUMER_NO_END; i++))
do
  task_exe $i &
done

wait;
