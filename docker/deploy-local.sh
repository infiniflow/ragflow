#!/bin/bash
# Re-deploy all modified files to the running container after a change.
# Usage: bash docker/deploy-local.sh
set -e
CONTAINER="docker-ragflow-cpu-1"
BASE="/home/oussama_raji/ragflow"

# Verify container is running
IMAGE=$(docker inspect $CONTAINER --format "{{.Config.Image}}" 2>/dev/null || echo "not found")
if [ "$IMAGE" = "not found" ]; then
    echo "ERROR: Container $CONTAINER is not running. Start it with: cd docker && docker compose up -d"
    exit 1
fi
echo "Running image: $IMAGE"

echo "Copying files..."
docker cp $BASE/docker/service_conf.yaml.template $CONTAINER:/ragflow/conf/service_conf.yaml.template
docker exec $CONTAINER mkdir -p /ragflow/tests
docker cp $BASE/common/constants.py            $CONTAINER:/ragflow/common/constants.py
docker cp $BASE/rag/app/video.py               $CONTAINER:/ragflow/rag/app/video.py
docker cp $BASE/rag/svr/task_executor.py       $CONTAINER:/ragflow/rag/svr/task_executor.py
docker cp $BASE/rag/nlp/search.py              $CONTAINER:/ragflow/rag/nlp/search.py
docker cp $BASE/api/apps/sdk/dataset.py        $CONTAINER:/ragflow/api/apps/sdk/dataset.py
docker cp $BASE/api/apps/sdk/doc.py            $CONTAINER:/ragflow/api/apps/sdk/doc.py
docker cp $BASE/api/utils/validation_utils.py  $CONTAINER:/ragflow/api/utils/validation_utils.py
docker cp $BASE/api/utils/api_utils.py         $CONTAINER:/ragflow/api/utils/api_utils.py
docker cp $BASE/api/db/init_data.py            $CONTAINER:/ragflow/api/db/init_data.py
docker cp $BASE/conf/infinity_mapping.json     $CONTAINER:/ragflow/conf/infinity_mapping.json
docker cp $BASE/tests/test_ragflow_pipeline.py  $CONTAINER:/ragflow/tests/test_ragflow_pipeline.py
# Copy nginx conf files (not bind-mounted — entrypoint.sh needs to mv them freely)
docker cp $BASE/docker/nginx/ragflow.conf  $CONTAINER:/etc/nginx/conf.d/ragflow.conf
docker cp $BASE/docker/nginx/proxy.conf    $CONTAINER:/etc/nginx/proxy.conf
docker cp $BASE/docker/nginx/nginx.conf    $CONTAINER:/etc/nginx/nginx.conf

# Copy test credentials
docker cp $BASE/tests/.env.test            $CONTAINER:/ragflow/tests/.env.test

# Remove default nginx conf that conflicts with ragflow.conf
docker exec $CONTAINER rm -f /etc/nginx/conf.d/default.conf
docker exec $CONTAINER nginx -s reload

echo "All files deployed. Container is ready."
