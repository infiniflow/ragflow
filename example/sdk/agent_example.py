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

"""
This example demonstrates CRUD operations (Create, Read, Update, Delete) on agents
and basic session management.

Prerequisites:
- A running RAGFlow instance
- A valid API key from RAGFlow admin panel
- (Optional) A knowledge base ID for retrieval-enabled agents
"""

from ragflow_sdk import RAGFlow
import sys

HOST_ADDRESS = "http://127.0.0.1"
API_KEY = "ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

# Simple agent DSL structure (basic chat agent)
SIMPLE_AGENT_DSL = {
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
                    "prompt": "You are a helpful assistant. Please answer the user's question: {begin.query}",
                    "temperature": 0.7
                }
            },
            "upstream": ["begin"],
            "downstream": []
        }
    },
    "graph": {
        "nodes": [
            {"id": "begin", "data": {"label": "Begin", "name": "begin"}, "position": {"x": 50, "y": 200}},
            {"id": "llm_0", "data": {"label": "LLM"}, "position": {"x": 200, "y": 200}}
        ],
        "edges": [
            {"source": "begin", "target": "llm_0"}
        ]
    },
    "history": [],
    "path": ["begin"]
}

try:
    # Create a RAGFlow instance
    ragflow_instance = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)
    
    # Create an agent
    print("Creating agent...")
    ragflow_instance.create_agent(
        title="Example Agent",
        dsl=SIMPLE_AGENT_DSL,
        description="A simple example agent for testing"
    )
    
    # List agents to get the created agent
    print("Listing agents...")
    agents = ragflow_instance.list_agents(page=1, page_size=10, orderby="create_time", desc=True)
    if not agents:
        raise Exception("No agents found after creation")
    
    agent_instance = agents[0]  # Get the most recently created agent
    print(f"Agent created: {agent_instance.id} - {agent_instance.description}")
    
    # Update the agent
    print("Updating agent...")
    ragflow_instance.update_agent(
        agent_id=agent_instance.id,
        title="Updated Example Agent",
        description="An updated example agent"
    )
    print("Agent updated successfully")
    
    # Create a session for the agent
    print("Creating session...")
    session_instance = agent_instance.create_session(user_id="example_user")
    print(f"Session created: {session_instance.id}")
    
    # List sessions
    print("Listing sessions...")
    sessions = agent_instance.list_sessions(page=1, page_size=10)
    print(f"Total sessions: {len(sessions)}")
    
    # Delete session
    print("Deleting session...")
    agent_instance.delete_sessions(ids=[session_instance.id])
    print("Session deleted successfully")
    
    # Delete the agent
    print("Deleting agent...")
    ragflow_instance.delete_agent(agent_id=agent_instance.id)
    print("Agent deleted successfully")
    
    print("\nTest completed successfully!")
    sys.exit(0)

except Exception as e:
    print(f"Error: {str(e)}")
    sys.exit(-1)
