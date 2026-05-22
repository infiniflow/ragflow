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
This example demonstrates document upload, listing, parsing, and chunk
management within a dataset using the RAGFlow SDK.

Prerequisites:
  - A running RAGFlow instance
  - A valid API key
  - A test file (e.g., a .txt or .pdf file) for uploading
"""

from ragflow_sdk import RAGFlow
import sys
import os
import tempfile

HOST_ADDRESS = "http://127.0.0.1"
API_KEY = "ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

try:
    ragflow = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)

    # Create a dataset
    dataset = ragflow.create_dataset(name="doc_example_dataset")
    print(f"Created dataset: {dataset.name} (id={dataset.id})")

    # --- Upload documents ---

    # Create a temporary test file
    tmp = tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False)
    tmp.write("RAGFlow is an open-source RAG engine based on deep document understanding.")
    tmp.close()

    dataset.upload_documents([{"display_name": "test.txt", "blob": open(tmp.name, "rb")}])
    print("Uploaded document: test.txt")
    os.unlink(tmp.name)

    # --- List documents ---

    docs = dataset.list_documents()
    print(f"Total documents: {len(docs)}")
    doc = docs[0]
    print(f"Document: {doc.name} (id={doc.id})")

    # --- Parse documents (start chunking) ---

    dataset.async_parse_documents(document_ids=[doc.id])
    print(f"Started parsing document: {doc.name}")

    # Wait for parsing to complete
    import time
    for _ in range(30):
        time.sleep(2)
        docs = dataset.list_documents()
        progress = docs[0].progress
        print(f"Parsing progress: {progress}")
        if progress == 1.0 or progress == -1:
            break

    # --- List chunks ---

    chunks = doc.list_chunks()
    print(f"Total chunks: {len(chunks)}")
    if chunks:
        chunk = chunks[0]
        print(f"First chunk: {chunk.content[:100]}...")

    # --- Add a chunk ---

    new_chunk = doc.add_chunk(content="This is a manually added chunk for testing.")
    print(f"Added chunk: {new_chunk.id}")

    # --- Update a chunk ---

    new_chunk.update({"content": "This is an updated chunk."})
    print("Updated chunk content")

    # --- Delete chunks ---

    doc.delete_chunks(chunk_ids=[new_chunk.id])
    print("Deleted the added chunk")

    # --- Delete documents ---

    dataset.delete_documents(ids=[doc.id])
    print("Deleted the uploaded document")

    # --- Cleanup ---

    ragflow.delete_datasets(ids=[dataset.id])
    print("Deleted dataset")

    print("test done")
    sys.exit(0)

except Exception as e:
    print(str(e))
    sys.exit(-1)
