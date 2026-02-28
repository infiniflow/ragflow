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
This example demonstrates how to create a chat assistant, manage sessions,
and perform chat completions using the RAGFlow SDK.

Prerequisites:
  - A running RAGFlow instance
  - A valid API key
  - At least one dataset with parsed documents
"""

from ragflow_sdk import RAGFlow
import sys

HOST_ADDRESS = "http://127.0.0.1"
API_KEY = "ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

try:
    ragflow = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)

    # --- Chat CRUD ---

    # Create a dataset and use it for the chat assistant
    dataset = ragflow.create_dataset(name="chat_example_dataset")

    # Create a chat assistant linked to the dataset
    chat = ragflow.create_chat(
        name="my_assistant",
        dataset_ids=[dataset.id],
    )
    print(f"Created chat: {chat.name} (id={chat.id})")

    # Update the chat assistant
    chat.update({"name": "updated_assistant"})
    print(f"Updated chat name: {chat.name}")

    # List all chat assistants
    chats = ragflow.list_chats()
    print(f"Total chats: {len(chats)}")

    # --- Session management ---

    # Create a session within the chat
    session = chat.create_session(name="test_session")
    print(f"Created session: {session.name} (id={session.id})")

    # List sessions
    sessions = chat.list_sessions()
    print(f"Total sessions: {len(sessions)}")

    # --- Chat completion (non-streaming) ---

    answer = session.ask("What is RAGFlow?", stream=False)
    print(f"Answer: {answer}")

    # --- Chat completion (streaming) ---

    print("Streaming answer:")
    for chunk in session.ask("How does RAGFlow work?", stream=True):
        print(chunk, end="", flush=True)
    print()

    # --- Cleanup ---

    # Delete sessions
    chat.delete_sessions(ids=[session.id])

    # Delete chat
    ragflow.delete_chats(ids=[chat.id])

    # Delete dataset
    ragflow.delete_datasets(ids=[dataset.id])

    print("test done")
    sys.exit(0)

except Exception as e:
    print(str(e))
    sys.exit(-1)
