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
The example demonstrates the RAG retrieval flow using the Python SDK.
It shows how to perform semantic search across one or more datasets.
"""

from ragflow_sdk import RAGFlow
import sys
import time
import os

HOST_ADDRESS = os.environ.get("RAGFLOW_HOST_ADDRESS", "http://127.0.0.1")
API_KEY = os.environ.get("RAGFLOW_API_KEY", "ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm")

try:
    rag = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)

    # 1. Create a dataset
    print("Creating dataset...")
    dataset = rag.create_dataset(name="retrieval_example_dataset")

    # 2. Upload and parse a document to have content for retrieval
    print("Uploading and parsing document...")
    content = "RAGFlow is an open-source RAG engine based on deep document understanding. It features a streamlined RAG workflow for businesses of any size."
    docs = dataset.upload_documents([{"display_name": "ragflow_info.txt", "blob": content.encode('utf-8')}])
    doc = docs[0]
    
    # Wait for parsing to complete with timeout
    print("Parsing document...")
    dataset.async_parse_documents([doc.id])
    MAX_WAIT = 120  # seconds
    elapsed = 0
    while elapsed < MAX_WAIT:
        doc_status = dataset.list_documents(id=doc.id)[0]
        if doc_status.run == "1" and doc_status.progress >= 1.0:
             break
        print(f"Parsing progress: {doc_status.progress:.2f}")
        time.sleep(2)
        elapsed += 2
    else:
        print("Parsing timed out.")
        sys.exit(-1)
    print("Document parsed and ready for retrieval.")

    # 3. Perform retrieval (Semantic Search)
    print("\n--- Performing Retrieval ---")
    question = "What is RAGFlow?"
    print(f"Question: {question}")
    
    # Retrieve relevant chunks from one or more datasets
    chunks = rag.retrieve(
        dataset_ids=[dataset.id],
        question=question,
        top_k=5,
        similarity_threshold=0.1
    )

    print(f"Found {len(chunks)} relevant chunks:")
    for i, chunk in enumerate(chunks):
        print(f"\nChunk {i+1}:")
        print(f"Content: {chunk.content[:200]}...")
        print(f"Similarity Score: {chunk.similarity:.4f}")
        print(f"Source Document: {chunk.document_name}")

    # 4. Perform retrieval with additional parameters
    print("\n--- Performing Retrieval with Keyword Search ---")
    chunks = rag.retrieve(
        dataset_ids=[dataset.id],
        question="workflow for businesses",
        top_k=3,
        keyword=True  # Enable keyword search in addition to semantic search
    )
    for i, chunk in enumerate(chunks):
        print(f"Chunk {i+1}: {chunk.content[:100]}... (Score: {chunk.similarity:.4f})")

    # Cleanup
    print("\nCleaning up...")
    rag.delete_datasets(ids=[dataset.id])

    print("Retrieval example done.")
    sys.exit(0)

except Exception as e:
    print(f"An error occurred: {e}")
    sys.exit(-1)
