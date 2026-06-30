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

# Agent CRUD Operations and Session Management Examples
# Replace AGENT_ID and SESSION_ID with actual IDs from responses

# Create an agent
echo -e "\n-- Create an agent"
curl --request POST \
     --url http://localhost:9380/api/v1/agents \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm' \
     --data '{
      "title": "Example Agent",
      "description": "A simple example agent for testing",
      "dsl": {
        "components": {
          "begin": {
            "obj": {
              "component_name": "Begin",
              "params": {
                "mode": "Chat",
                "prologue": "Hello! How can I help you today?"
              }
            },
            "upstream": [],
            "downstream": ["llm_0"]
          },
          "llm_0": {
            "obj": {
              "component_name": "LLM",
              "params": {
                "prompt": "You are a helpful assistant. Please answer: {begin.query}",
                "temperature": 0.7
              }
            },
            "upstream": ["begin"],
            "downstream": []
          }
        },
        "graph": {
          "nodes": [
            {"id": "begin", "data": {"label": "Begin"}, "position": {"x": 50, "y": 200}},
            {"id": "llm_0", "data": {"label": "LLM"}, "position": {"x": 200, "y": 200}}
          ],
          "edges": [
            {"source": "begin", "target": "llm_0"}
          ]
        },
        "history": [],
        "path": ["begin"]
      }
     }'

# List agents
echo -e "\n-- List agents"
curl --request GET \
     --url 'http://localhost:9380/api/v1/agents?page=1&page_size=10&orderby=create_time&desc=true' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm'

# Update an agent (replace AGENT_ID)
echo -e "\n-- Update an agent"
curl --request PUT \
     --url http://localhost:9380/api/v1/agents/AGENT_ID \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm' \
     --data '{
      "title": "Updated Example Agent",
      "description": "An updated example agent"
     }'

# Create a session for the agent (replace AGENT_ID)
echo -e "\n-- Create a session"
curl --request POST \
     --url http://localhost:9380/api/v1/agents/AGENT_ID/sessions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm' \
     --data '{
      "user_id": "example_user"
     }'

# List sessions for an agent (replace AGENT_ID)
echo -e "\n-- List sessions"
curl --request GET \
     --url 'http://localhost:9380/api/v1/agents/AGENT_ID/sessions?page=1&page_size=10' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm'

# Execute agent with streaming (replace AGENT_ID and SESSION_ID)
echo -e "\n-- Execute agent (streaming)"
curl --request POST \
     --url http://localhost:9380/api/v1/agents/AGENT_ID/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm' \
     --data '{
      "question": "What is RAGFlow?",
      "session_id": "SESSION_ID",
      "stream": true
     }'

# Execute agent without streaming (replace AGENT_ID and SESSION_ID)
echo -e "\n-- Execute agent (non-streaming)"
curl --request POST \
     --url http://localhost:9380/api/v1/agents/AGENT_ID/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm' \
     --data '{
      "question": "Explain agents in simple terms",
      "session_id": "SESSION_ID",
      "stream": false,
      "return_trace": true
     }'

# Delete session(s) (replace AGENT_ID and SESSION_ID)
echo -e "\n-- Delete sessions"
curl --request DELETE \
     --url http://localhost:9380/api/v1/agents/AGENT_ID/sessions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm' \
     --data '{
      "ids": ["SESSION_ID"]
     }'

# Delete an agent (replace AGENT_ID)
echo -e "\n-- Delete agent"
curl --request DELETE \
     --url http://localhost:9380/api/v1/agents/AGENT_ID \
     --header 'Authorization: Bearer ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm'
