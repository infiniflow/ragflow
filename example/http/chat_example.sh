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

# Replace the dataset_id, chat_id, and session_id with actual values from your instance.

HOST="http://localhost:9380"
API_KEY="ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

# Create a chat assistant
echo -e "\n-- Create a chat assistant"
curl --request POST \
     --url $HOST/api/v1/chats \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "name": "my_assistant",
       "dataset_ids": ["YOUR_DATASET_ID"]
     }'

# Update the chat assistant
echo -e "\n-- Update the chat assistant"
curl --request PUT \
     --url $HOST/api/v1/chats/YOUR_CHAT_ID \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "name": "updated_assistant"
     }'

# List chat assistants
echo -e "\n-- List chat assistants"
curl --request GET \
     --url $HOST/api/v1/chats \
     --header "Authorization: Bearer $API_KEY"

# Delete chat assistants
echo -e "\n-- Delete chat assistants"
curl --request DELETE \
     --url $HOST/api/v1/chats \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "ids": ["YOUR_CHAT_ID"]
     }'

# Create a session within the chat
echo -e "\n-- Create a session"
curl --request POST \
     --url $HOST/api/v1/chats/YOUR_CHAT_ID/sessions \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "name": "test_session"
     }'

# List sessions
echo -e "\n-- List sessions"
curl --request GET \
     --url $HOST/api/v1/chats/YOUR_CHAT_ID/sessions \
     --header "Authorization: Bearer $API_KEY"

# Chat completion (non-streaming)
echo -e "\n-- Chat completion (non-streaming)"
curl --request POST \
     --url $HOST/api/v1/chats/YOUR_CHAT_ID/completions \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "question": "What is RAGFlow?",
       "stream": false,
       "session_id": "YOUR_SESSION_ID"
     }'

# Chat completion (streaming)
echo -e "\n-- Chat completion (streaming)"
curl --request POST \
     --url $HOST/api/v1/chats/YOUR_CHAT_ID/completions \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "question": "How does RAGFlow work?",
       "stream": true,
       "session_id": "YOUR_SESSION_ID"
     }'

# Delete sessions
echo -e "\n-- Delete sessions"
curl --request DELETE \
     --url $HOST/api/v1/chats/YOUR_CHAT_ID/sessions \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer $API_KEY" \
     --data '{
       "ids": ["YOUR_SESSION_ID"]
     }'
