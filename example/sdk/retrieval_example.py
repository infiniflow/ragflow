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
This example demonstrates how to use the RAGFlow SDK to perform
cross-dataset retrieval (semantic search) over parsed documents.

Prerequisites:
  - A running RAGFlow instance
  - A valid API key
  - At least one dataset with parsed documents
"""

from ragflow_sdk import RAGFlow
import sys
import tempfile
import os
import time

HOST_ADDRESS = "http://127.0.0.1"
API_KEY = "ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

try:
    ragflow = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)

    # Create a dataset and upload a document
    dataset = ragflow.create_dataset(name="retrieval_example_dataset")

    tmp = tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False)
    tmp.write(
        "RAGFlow is an open-source RAG engine based on deep document understanding. "
        "It provides chunking, retrieval, and generation capabilities for building "
        "retrieval-augmented generation applications."
    )
    tmp.close()

    dataset.upload_documents([{"display_name": "ragflow_intro.txt", "blob": open(tmp.name, "rb")}])
    os.unlink(tmp.name)

    docs = dataset.list_documents()
    doc = docs[0]

    # Parse the document
    dataset.async_parse_documents(document_ids=[doc.id])
    for _ in range(30):
        time.sleep(2)
        docs = dataset.list_documents()
        if docs[0].progress == 1.0 or docs[0].progress == -1:
            break
    print(f"Document parsed: {doc.name}")

    # --- Retrieval ---

    results = ragflow.retrieve(
        dataset_ids=[dataset.id],
        question="What is RAGFlow?",
        page=1,
        page_size=10,
        similarity_threshold=0.2,
        vector_similarity_weight=0.3,
        top_k=1024,
    )
    print(f"Retrieved {len(results)} chunks:")
    for i, chunk in enumerate(results):
        print(f"  [{i+1}] score={chunk.get('similarity', 'N/A'):.4f} content={chunk.get('content', '')[:80]}...")

    # --- Cleanup ---

    ragflow.delete_datasets(ids=[dataset.id])

    print("test done")
    sys.exit(0)

except Exception as e:
    print(str(e))
    sys.exit(-1)
