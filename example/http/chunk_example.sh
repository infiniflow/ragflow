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
HOST_ADDRESS="http://localhost:9380"
API_KEY="ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"
DATASET_ID="your_dataset_id"
DOC_ID="your_document_id"
CHUNK_ID="your_chunk_id"

# 1. Add a chunk to a document
echo -e "\n-- Add a chunk to a document"
curl --request POST \
     --url "${HOST_ADDRESS}/api/v1/datasets/${DATASET_ID}/documents/${DOC_ID}/chunks" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data '{
      "content": "RAGFlow is an open-source RAG engine.",
      "important_keywords": ["RAGFlow", "open-source"]
      }'

# 2. List chunks of a document
echo -e "\n-- List chunks of a document"
curl --request GET \
     --url "${HOST_ADDRESS}/api/v1/datasets/${DATASET_ID}/documents/${DOC_ID}/chunks?page=1&page_size=10" \
     --header "Authorization: Bearer ${API_KEY}"

# 3. Update a chunk
echo -e "\n-- Update a chunk"
curl --request PUT \
     --url "${HOST_ADDRESS}/api/v1/datasets/${DATASET_ID}/documents/${DOC_ID}/chunks/${CHUNK_ID}" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data '{
      "content": "RAGFlow is a powerful open-source RAG engine."
      }'

# 4. Delete chunks
echo -e "\n-- Delete chunks"
curl --request DELETE \
     --url "${HOST_ADDRESS}/api/v1/datasets/${DATASET_ID}/documents/${DOC_ID}/chunks" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data "{
      \"chunk_ids\": [\"${CHUNK_ID}\"]
      }"
