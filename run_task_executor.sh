#!/bin/bash

# Process ID as an argument
if [ -z "$1" ]; then
  echo "Usage: $0 <process_id>"
  exit 1
fi

PROCESS_ID=$1
LOG_FILE="task_executor_${PROCESS_ID}.log"

# Run the command in the background and log output
nohup python rag/svr/task_executor.py $PROCESS_ID > "$LOG_FILE" 2>&1 &

echo "Task executor started for process ID $PROCESS_ID. Logs are being written to $LOG_FILE."
