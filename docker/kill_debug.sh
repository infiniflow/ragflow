#!/bin/bash

# 查找并终止所有与 debugpy 相关的进程
pids=$(ps -ef | grep "debugpy" | grep -v grep | awk '{print $2}')

if [ -n "$pids" ]; then
    echo "Killing PIDs: $pids"
    kill $pids
else
    echo "No debugpy processes found."
fi