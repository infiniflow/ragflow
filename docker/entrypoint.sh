#!/bin/bash
set -e

# replace env variables in the service_conf.yaml file
rm -rf /ragflow/conf/service_conf.yaml
while IFS= read -r line || [[ -n "$line" ]]; do
    # Use eval to interpret the variable with default values
    eval "echo \"$line\"" >> /ragflow/conf/service_conf.yaml
done < /ragflow/conf/service_conf.yaml.template

/usr/sbin/nginx

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

PY=python3
if [[ -z "$WS" || $WS -lt 1 ]]; then
  WS=1
fi

function task_exe(){
    JEMALLOC_PATH=$(pkg-config --variable=libdir jemalloc)/libjemalloc.so
    while [ 1 -eq 1 ];do
      LD_PRELOAD=$JEMALLOC_PATH $PY rag/svr/task_executor.py $1;
    done
}

for ((i=0;i<WS;i++))
do
  task_exe  $i &
done

# Function to wait for document engine to be ready
wait_for_doc_engine() {
  # Get document engine type
  DOC_ENGINE=${DOC_ENGINE:-elasticsearch}
  echo "Using document engine: $DOC_ENGINE"

  if [ "$DOC_ENGINE" = "elasticsearch" ]; then
    # Get Elasticsearch configuration
    ES_HOST=${ES_HOST:-es01}
    ES_PORT=${ES_PORT:-1200}
    ELASTIC_PASSWORD=${ELASTIC_PASSWORD:-infini_rag_flow}
    
    echo "Waiting for Elasticsearch (${ES_HOST}:${ES_PORT}) to be ready..."
    
    # Build health check URL
    ES_URL="http://${ES_HOST}:${ES_PORT}"
    
    # Add authentication header
    AUTH_HEADER=""
    if [ -n "$ELASTIC_PASSWORD" ]; then
      AUTH_HEADER="-u elastic:${ELASTIC_PASSWORD}"
    fi
    
    until curl -s -f $AUTH_HEADER "${ES_URL}/_cluster/health?wait_for_status=yellow" > /dev/null 2>&1; do
      echo "Elasticsearch is not ready - waiting"
      sleep 5
    done
    echo "Elasticsearch is ready - starting RAGFlow"
  elif [ "$DOC_ENGINE" = "infinity" ]; then
    # Get Infinity configuration
    INFINITY_HOST=${INFINITY_HOST:-infinity}
    INFINITY_HTTP_PORT=${INFINITY_HTTP_PORT:-23820}
    
    echo "Waiting for Infinity (${INFINITY_HOST}:${INFINITY_HTTP_PORT}) to be ready..."
    
    # Build health check URL
    INFINITY_URL="http://${INFINITY_HOST}:${INFINITY_HTTP_PORT}"
    
    until curl -s -f "${INFINITY_URL}/admin/node/current" > /dev/null 2>&1; do
      echo "Infinity is not ready - waiting"
      sleep 5
    done
    echo "Infinity is ready - starting RAGFlow"
  fi
}

# Call the function
wait_for_doc_engine

while [ 1 -eq 1 ];do
    $PY api/ragflow_server.py
done

wait;

exec "$@"
