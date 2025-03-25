#!/bin/bash

# AWS Lambda ENTRYPOINT (if ever used manually, though CMD is preferred)
# Note: In Lambda, this script is not automatically executed.
# All logic should be handled via app.lambda_handler

# Replace env variables in the service_conf.yaml file (optional preprocessing)
rm -rf /var/task/conf/service_conf.yaml
while IFS= read -r line || [[ -n "$line" ]]; do
    eval "echo \"$line\"" >> /var/task/conf/service_conf.yaml
done < /var/task/conf/service_conf.yaml.template

# Optional: environment setup
export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/
export PYTHONPATH=/var/task

# All processing is now expected to happen through the Lambda handler.
# Uncomment for local testing or container testing outside Lambda:

# python3 api/ragflow_server.py
