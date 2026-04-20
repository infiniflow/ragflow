#!/bin/bash
#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

# Variables
HOST_ADDRESS="${RAGFLOW_HOST_ADDRESS:-http://localhost:9380}"
API_KEY="${RAGFLOW_API_KEY:-ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm}"

# Check for jq
if ! command -v jq &> /dev/null; then
    echo "jq could not be found, please install it to run this example."
    exit 1
fi

# 0. Setup: Create a dataset to retrieve from
echo -e "\n-- Creating a dataset"
DATASET_ID=$(curl -s --request POST \
     --url "${HOST_ADDRESS}/api/v1/datasets" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data '{"name": "retrieval_shell_example"}' | jq -r '.data.id')
echo "Dataset ID: ${DATASET_ID}"

# 1. Perform semantic retrieval from a dataset
echo -e "\n-- Perform semantic retrieval"
curl -s --request POST \
     --url "${HOST_ADDRESS}/api/v1/retrieval" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data "{
      \"dataset_ids\": [\"${DATASET_ID}\"],
      \"question\": \"What is RAGFlow?\",
      \"page\": 1,
      \"page_size\": 5,
      \"similarity_threshold\": 0.2,
      \"vector_similarity_weight\": 0.3,
      \"top_k\": 1024
      }" | jq .

# 2. Perform retrieval with keyword search enabled
echo -e "\n-- Perform retrieval with keyword search"
curl -s --request POST \
     --url "${HOST_ADDRESS}/api/v1/retrieval" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data "{
      \"dataset_ids\": [\"${DATASET_ID}\"],
      \"question\": \"workflow features\",
      \"keyword\": true,
      \"top_k\": 10
      }" | jq .

# Cleanup
echo -e "\n-- Cleaning up dataset"
curl -s --request DELETE \
     --url "${HOST_ADDRESS}/api/v1/datasets" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data "{\"ids\": [\"${DATASET_ID}\"]}" | jq .
