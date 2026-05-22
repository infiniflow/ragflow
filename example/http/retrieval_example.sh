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

# Replace the dataset_id with actual values from your instance.

HOST="http://localhost:9380"
API_KEY="ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

# Retrieval (semantic search across datasets)
echo -e "\n-- Retrieval"
curl --request POST \
     --url $HOST/api/v1/retrieval \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "dataset_ids": ["YOUR_DATASET_ID"],
       "question": "What is RAGFlow?",
       "page": 1,
       "page_size": 10,
       "similarity_threshold": 0.2,
       "vector_similarity_weight": 0.3,
       "top_k": 1024
     }'
