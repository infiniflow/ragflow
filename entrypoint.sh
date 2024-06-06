# Adjust configurations according to your actual situation (the following two export commands are newly added):
# - Assign the result of `which python` to `PY`.
# - Assign the result of `pwd` to `PYTHONPATH`.
# - Comment out `LD_LIBRARY_PATH`, if it is configured.
# - Optional: Add Hugging Face mirror.
PY=/home/infinity/miniconda/envs/ragflow/bin/python
export PYTHONPATH=/home/infinity/PycharmProjects/pythonProject/ragflow
export HF_ENDPOINT=https://hf-mirror.com

#!/bin/bash

/usr/sbin/nginx

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

PY=python3
$PY rag/svr/task_executor.py &


$PY api/ragflow_server.py

wait;
