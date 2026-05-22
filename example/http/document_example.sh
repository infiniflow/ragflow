#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

# Replace the dataset_id and document_id with actual values from your instance.

HOST="http://localhost:9380"
API_KEY="ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

# Upload a document to a dataset
echo -e "\n-- Upload a document"
curl --request POST \
     --url $HOST/api/v1/datasets/YOUR_DATASET_ID/documents \
     --header "Authorization: Bearer $API_KEY" \
     --form 'file=@/path/to/your/test.txt'

# List documents in a dataset
echo -e "\n-- List documents"
curl --request GET \
     --url $HOST/api/v1/datasets/YOUR_DATASET_ID/documents \
     --header "Authorization: Bearer $API_KEY"

# Update a document
echo -e "\n-- Update a document"
curl --request PUT \
     --url $HOST/api/v1/datasets/YOUR_DATASET_ID/documents/YOUR_DOCUMENT_ID \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "name": "renamed_document.txt"
     }'

# Start parsing a document
echo -e "\n-- Parse a document"
curl --request POST \
     --url $HOST/api/v1/datasets/YOUR_DATASET_ID/chunks \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "document_ids": ["YOUR_DOCUMENT_ID"]
     }'

# Stop parsing a document
echo -e "\n-- Stop parsing a document"
curl --request DELETE \
     --url $HOST/api/v1/datasets/YOUR_DATASET_ID/chunks \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "document_ids": ["YOUR_DOCUMENT_ID"]
     }'

# List chunks of a document
echo -e "\n-- List chunks"
curl --request GET \
     --url "$HOST/api/v1/datasets/YOUR_DATASET_ID/documents/YOUR_DOCUMENT_ID/chunks" \
     --header "Authorization: Bearer $API_KEY"

# Delete documents
echo -e "\n-- Delete documents"
curl --request DELETE \
     --url $HOST/api/v1/datasets/YOUR_DATASET_ID/documents \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "ids": ["YOUR_DOCUMENT_ID"]
     }'
