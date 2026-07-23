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

# This example demonstrates how to manage documents within a dataset
# using RAGFlow's HTTP API, including uploading, listing, parsing,
# and deleting documents.

# Configuration - replace with your actual values
HOST_ADDRESS="http://127.0.0.1:9380"
API_KEY="ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

# Step 1: Create a dataset
echo "=== Creating dataset ==="
DATASET_RESPONSE=$(curl -s --request POST \
     --url "${HOST_ADDRESS}/api/v1/datasets" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data '{
      "name": "document_example_dataset"
      }')

echo "$DATASET_RESPONSE"

# Extract dataset ID from response
DATASET_ID=$(echo "$DATASET_RESPONSE" | python -c "import sys,json; print(json.load(sys.stdin).get('data',{}).get('id',''))" 2>/dev/null)
echo "Dataset ID: $DATASET_ID"

# Step 2: Upload documents
echo -e "\n=== Uploading documents ==="

# Create a temporary file for upload
TEMP_FILE=$(mktemp /tmp/ragflow_doc_XXXXXX.txt)
echo "This is a sample document for testing RAGFlow document management." > "$TEMP_FILE"
echo "It contains some text that can be indexed and searched." >> "$TEMP_FILE"

curl --request POST \
     --url "${HOST_ADDRESS}/api/v1/datasets/${DATASET_ID}/documents" \
     --header "Authorization: Bearer ${API_KEY}" \
     --form "file=@${TEMP_FILE};filename=sample.txt"

rm -f "$TEMP_FILE"

# Step 3: List documents
echo -e "\n=== Listing documents ==="
curl --request GET \
     --url "${HOST_ADDRESS}/api/v1/datasets/${DATASET_ID}/documents" \
     --header "Authorization: Bearer ${API_KEY}"

# Step 4: Parse documents
echo -e "\n=== Parsing documents ==="
curl --request POST \
     --url "${HOST_ADDRESS}/api/v1/datasets/${DATASET_ID}/chunks" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data '{
      "document_ids": ["ALL"]
      }'

# Step 5: Delete documents
echo -e "\n=== Deleting documents ==="
curl --request DELETE \
     --url "${HOST_ADDRESS}/api/v1/datasets/${DATASET_ID}/documents" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data '{
      "ids": ["ALL"]
      }'

# Step 6: Clean up dataset
echo -e "\n=== Cleaning up ==="
curl --request DELETE \
     --url "${HOST_ADDRESS}/api/v1/datasets" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data "{
      \"ids\": [\"${DATASET_ID}\"]
      }"

echo -e "\nAll operations completed!"
