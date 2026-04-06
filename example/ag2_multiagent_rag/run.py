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
AG2 Multi-Agent RAG with RAGFlow
=================================

This example demonstrates AG2 (formerly AutoGen) multi-agent conversations
using RAGFlow as the knowledge retrieval backend. Two AG2 agents collaborate:

- Research Assistant: Retrieves relevant documents via RAGFlow SDK
- Analyst: Synthesizes retrieved information into comprehensive answers

Requirements:
    pip install "ag2[openai]>=0.11.4,<1.0" ragflow-sdk

Environment variables:
    OPENAI_API_KEY      - OpenAI API key for AG2 agents
    RAGFLOW_API_KEY     - RAGFlow API key
    RAGFLOW_BASE_URL    - RAGFlow server URL (default: http://localhost:9380)

Usage:
    python run.py
"""

import os
import sys

from autogen import AssistantAgent, GroupChat, GroupChatManager, LLMConfig, UserProxyAgent
from ragflow_sdk import RAGFlow


def create_ragflow_client() -> RAGFlow:
    """Initialize the RAGFlow SDK client."""
    api_key = os.environ.get("RAGFLOW_API_KEY")
    base_url = os.environ.get("RAGFLOW_BASE_URL", "http://localhost:9380")

    if not api_key:
        print("Error: RAGFLOW_API_KEY environment variable is not set.")
        print("To get your API key: RAGFlow UI -> Settings -> API -> API KEY -> Create new key")
        sys.exit(1)

    return RAGFlow(api_key=api_key, base_url=base_url)


def main():
    """Run AG2 multi-agent RAG conversation with RAGFlow."""

    if not os.environ.get("OPENAI_API_KEY"):
        print("Error: OPENAI_API_KEY environment variable is not set.")
        sys.exit(1)

    # Initialize RAGFlow client
    rag_client = create_ragflow_client()

    # List available datasets
    datasets = rag_client.list_datasets()
    if not datasets:
        print("Error: No datasets found in RAGFlow.")
        print("Please create a dataset and upload documents first.")
        print("See: https://ragflow.io/docs/guides/knowledge_base")
        sys.exit(1)

    print(f"Available RAGFlow datasets: {[ds.name for ds in datasets]}")
    dataset_ids = [ds.id for ds in datasets]

    # Build document name lookup for source attribution
    doc_name_map = {}
    for ds in datasets:
        try:
            for doc in ds.list_documents(page_size=100):
                doc_name_map[doc.id] = doc.name
        except Exception:
            pass

    # AG2 LLM configuration
    llm_config = LLMConfig(
        {
            "model": "gpt-4o-mini",
            "api_key": os.environ["OPENAI_API_KEY"],
            "api_type": "openai",
        }
    )

    # Create AG2 agents
    research_assistant = AssistantAgent(
        name="research_assistant",
        system_message=(
            "You are a research assistant. When asked a question, use the "
            "search_ragflow tool to retrieve relevant documents from the "
            "knowledge base. Always include source citations in your findings. "
            "Present your retrieved information clearly for the analyst."
        ),
        llm_config=llm_config,
    )

    analyst = AssistantAgent(
        name="analyst",
        system_message=(
            "You are an analyst. Based on the research assistant's findings, "
            "synthesize the information into a comprehensive, well-structured "
            "answer. Always reference the source documents. If the retrieved "
            "information is insufficient, ask the research assistant to search "
            "with different terms. End with TERMINATE when the answer is complete."
        ),
        llm_config=llm_config,
    )

    user_proxy = UserProxyAgent(
        name="user_proxy",
        human_input_mode="NEVER",
        max_consecutive_auto_reply=10,
        code_execution_config=False,
        is_termination_msg=lambda x: x.get("content", "") and "TERMINATE" in x.get("content", ""),
    )

    # Register RAGFlow search tool using the retrieve() API
    @user_proxy.register_for_execution()
    @research_assistant.register_for_llm(
        description=(
            "Search RAGFlow knowledge base for relevant document chunks. Returns document chunks with similarity scores and source citations. Use specific, targeted search queries for best results."
        )
    )
    def search_ragflow(query: str, top_k: int = 5) -> str:
        """
        Search RAGFlow datasets for relevant document chunks.

        Args:
            query: The search query string.
            top_k: Number of top results to return (default: 5).

        Returns:
            Formatted string with retrieved document chunks and sources.
        """
        try:
            chunks = rag_client.retrieve(
                dataset_ids=dataset_ids,
                question=query,
                top_k=top_k,
                similarity_threshold=0.2,
                vector_similarity_weight=0.3,
            )

            if not chunks:
                return "No relevant documents found for this query."

            results = []
            for i, chunk in enumerate(chunks[:top_k], 1):
                source = chunk.document_name or doc_name_map.get(chunk.document_id, "Unknown source")
                similarity = f"{chunk.similarity:.3f}" if chunk.similarity else "N/A"
                results.append(f"[{i}] Source: {source} (similarity: {similarity})\n{chunk.content}\n")

            return "\n---\n".join(results)

        except Exception as e:
            return f"RAGFlow search error: {e}"

    @user_proxy.register_for_execution()
    @research_assistant.register_for_llm(description="List all available datasets in RAGFlow knowledge base.")
    def list_datasets() -> str:
        """List available RAGFlow datasets with their document counts."""
        try:
            ds_list = rag_client.list_datasets()
            if not ds_list:
                return "No datasets found."

            info = []
            for ds in ds_list:
                doc_count = getattr(ds, "document_count", "N/A")
                info.append(f"- {ds.name} ({doc_count} documents)")

            return "Available datasets:\n" + "\n".join(info)

        except Exception as e:
            return f"Error listing datasets: {e}"

    # Set up group chat
    group_chat = GroupChat(
        agents=[user_proxy, research_assistant, analyst],
        messages=[],
        max_round=12,
    )

    manager = GroupChatManager(
        groupchat=group_chat,
        llm_config=llm_config,
    )

    # Run the conversation
    query = "What are the key concepts and best practices described in the knowledge base? Provide a comprehensive summary with citations."

    print(f"\n{'=' * 60}")
    print("AG2 Multi-Agent RAG with RAGFlow")
    print(f"{'=' * 60}")
    print(f"Query: {query}\n")

    user_proxy.run(manager, message=query).process()

    print(f"\n{'=' * 60}")
    print("Conversation complete.")
    print(f"{'=' * 60}")


if __name__ == "__main__":
    main()
