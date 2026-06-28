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
The example demonstrates chunk management (Add, List, Update, Delete, Retrieve)
within a RAGFlow dataset using the Python SDK.
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
    dataset = rag.create_dataset(name="chunk_example_dataset")

    # 2. Upload a document
    print("Uploading document...")
    # Using a simple text content for example
    content = "RAGFlow is an open-source RAG (Retrieval-Augmented Generation) engine based on deep document understanding."
    docs = dataset.upload_documents([{"display_name": "sample.txt", "blob": content.encode('utf-8')}])
    doc = docs[0]

    # 3. Parse the document (required before manual chunk operations if you want it to be processed)
    print("Parsing document...")
    dataset.async_parse_documents([doc.id])
    
    # Wait for parsing to complete with timeout
    MAX_WAIT = 120  # seconds
    elapsed = 0
    while elapsed < MAX_WAIT:
        doc_status = dataset.list_documents(id=doc.id)[0]
        if doc_status.run == "1" and doc_status.progress >= 1.0:
             print("Parsing completed.")
             break
        print(f"Parsing progress: {doc_status.progress:.2f}")
        time.sleep(2)
        elapsed += 2
    else:
        print("Parsing timed out.")
        sys.exit(-1)

    # 4. Add a manual chunk
    print("Adding a manual chunk...")
    chunk = doc.add_chunk(content="RAGFlow features a streamlined RAG workflow.")
    print(f"Added chunk ID: {chunk.id}")

    # 5. List chunks
    print("Listing chunks...")
    chunks = doc.list_chunks(page=1, page_size=10)
    print(f"Total chunks found: {len(chunks)}")
    for i, c in enumerate(chunks):
        print(f"Chunk {i}: {c.content[:50]}...")

    # 6. Update a chunk
    print("Updating chunk...")
    chunk.update({"content": "RAGFlow features a streamlined and powerful RAG workflow."})
    
    # 7. Delete the chunk
    print("Deleting chunk...")
    doc.delete_chunks([chunk.id])

    # Cleanup
    print("Cleaning up dataset...")
    rag.delete_datasets(ids=[dataset.id])
    
    print("Chunk example done.")
    sys.exit(0)

except Exception as e:
    print(f"An error occurred: {e}")
    sys.exit(-1)
