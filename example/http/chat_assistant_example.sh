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

# 1. Create a chat assistant
echo -e "\n-- Create a chat assistant"
CHAT_RESPONSE=$(curl -s --request POST \
     --url "${HOST_ADDRESS}/api/v1/chats" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data '{
      "name": "My Assistant",
      "llm_id": "deepseek-chat"
      }')
CHAT_ID=$(echo $CHAT_RESPONSE | jq -r '.data.id')
echo "Chat Assistant ID: ${CHAT_ID}"

# 2. Create a session for the assistant
echo -e "\n-- Create a session"
SESSION_RESPONSE=$(curl -s --request POST \
     --url "${HOST_ADDRESS}/api/v1/chats/${CHAT_ID}/sessions" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data '{
      "name": "New Session"
      }')
SESSION_ID=$(echo $SESSION_RESPONSE | jq -r '.data.id')
echo "Session ID: ${SESSION_ID}"

# 3. Ask a question (Non-streaming)
echo -e "\n-- Ask a question (Non-streaming)"
curl -s --request POST \
     --url "${HOST_ADDRESS}/api/v1/chats/${CHAT_ID}/completions" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data "{
      \"question\": \"What is RAGFlow?\",
      \"stream\": false,
      \"session_id\": \"${SESSION_ID}\"
      }" | jq .

# 4. Ask a question (Streaming)
echo -e "\n-- Ask a question (Streaming)"
# Note: Streaming output will be raw SSE data
curl -N -s --request POST \
     --url "${HOST_ADDRESS}/api/v1/chats/${CHAT_ID}/completions" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data "{
      \"question\": \"Tell me more.\",
      \"stream\": true,
      \"session_id\": \"${SESSION_ID}\"
      }"

# 5. List sessions
echo -e "\n-- List sessions"
curl -s --request GET \
     --url "${HOST_ADDRESS}/api/v1/chats/${CHAT_ID}/sessions" \
     --header "Authorization: Bearer ${API_KEY}" | jq .

# 6. Delete sessions
echo -e "\n-- Delete sessions"
curl -s --request DELETE \
     --url "${HOST_ADDRESS}/api/v1/chats/${CHAT_ID}/sessions" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data "{
      \"ids\": [\"${SESSION_ID}\"]
      }" | jq .

# Cleanup
echo -e "\n-- Deleting chat assistant"
curl -s --request DELETE \
     --url "${HOST_ADDRESS}/api/v1/chats" \
     --header 'Content-Type: application/json' \
     --header "Authorization: Bearer ${API_KEY}" \
     --data "{\"ids\": [\"${CHAT_ID}\"]}" | jq .
