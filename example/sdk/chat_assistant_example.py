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

"""
The example demonstrates how to create a chat assistant, manage sessions, 
and perform both standard and streaming chat.
"""

from ragflow_sdk import RAGFlow
import sys

HOST_ADDRESS = "http://127.0.0.1"
API_KEY = "ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

try:
    rag = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)

    # 1. Create a dataset to be used by the assistant
    print("Creating dataset...")
    dataset = rag.create_dataset(name="assistant_example_dataset")

    # 2. Create a chat assistant
    print("Creating chat assistant...")
    assistant = rag.create_chat(
        name="Test Assistant",
        dataset_ids=[dataset.id],
        llm_id="deepseek-chat",  # Example LLM ID, replace with your actual model ID
        prompt_config={"system": "You are a helpful assistant."}
    )
    print(f"Assistant created: {assistant.name} (ID: {assistant.id})")

    # 3. Create a session
    print("Creating a new session...")
    session = assistant.create_session(name="Example Session")
    print(f"Session created: {session.name} (ID: {session.id})")

    # 4. Standard chat (non-streaming)
    print("\n--- Standard Chat ---")
    question = "What is RAGFlow?"
    print(f"User: {question}")
    
    # ask returns a generator of Message objects
    # for stream=False, it yields once with the full answer
    for message in session.ask(question=question, stream=False):
        print(f"Assistant: {message.content}")
        if hasattr(message, 'reference') and message.reference:
            print(f"References used: {len(message.reference)} chunks")

    # 5. Streaming chat
    print("\n--- Streaming Chat ---")
    question = "Tell me more about its features."
    print(f"User: {question}")
    print("Assistant: ", end="", flush=True)
    
    for message in session.ask(question=question, stream=True):
        # In streaming mode, each message.content usually contains the incremental part
        # or the full content so far depending on the SDK implementation. 
        # Based on RAGFlow SDK, it typically yields incremental parts.
        print(message.content, end="", flush=True)
    print("\n")

    # 6. List sessions
    print("Listing sessions for this assistant...")
    sessions = assistant.list_sessions(page=1, page_size=10)
    for s in sessions:
        print(f"- {s.name} (ID: {s.id})")

    # Cleanup
    print("\nCleaning up...")
    assistant.delete_sessions(ids=[session.id])
    rag.delete_chats(ids=[assistant.id])
    rag.delete_datasets(ids=[dataset.id])

    print("Chat assistant example done.")
    sys.exit(0)

except Exception as e:
    print(f"An error occurred: {e}")
    sys.exit(-1)
