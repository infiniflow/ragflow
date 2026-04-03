#!/bin/bash
# Re-deploy all modified files to the running container after a change.
# Usage: bash docker/deploy-local.sh
set -e
CONTAINER="docker-ragflow-cpu-1"
BASE="/home/oussama_raji/ragflow"

echo "Restarting $CONTAINER..."
docker restart $CONTAINER
sleep 45
# Verify container is using correct image
IMAGE=$(docker inspect $CONTAINER --format "{{.Config.Image}}")
echo "Running image: $IMAGE"

echo "Copying files..."
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
echo "All files deployed. Container is ready."
