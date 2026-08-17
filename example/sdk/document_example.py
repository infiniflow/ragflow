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
This example demonstrates how to manage documents within a dataset,
including uploading, listing, parsing, and deleting documents.
"""

from ragflow_sdk import RAGFlow
import sys
import os
import tempfile

HOST_ADDRESS = os.environ.get("RAGFLOW_HOST_ADDRESS", "http://127.0.0.1")
API_KEY = os.environ.get("RAGFLOW_API_KEY", "ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm")

try:
    rag = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)

    # 1. Create a dataset
    print("=== Creating dataset ===")
    dataset = rag.create_dataset(name="document_example_dataset")
    print(f"Dataset created: {dataset.name} (ID: {dataset.id})")

    # 2. Upload documents
    print("\n=== Uploading documents ===")

    # Create a temporary text file for demonstration
    with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
        f.write("This is a sample document for testing RAGFlow document management.\n")
        f.write("It contains some text that can be indexed and searched.\n")
        temp_file = f.name

    with open(temp_file, "rb") as f:
        content = f.read()

    docs = dataset.upload_documents([
        {"display_name": "sample.txt", "blob": content},
    ])
    print(f"Uploaded {len(docs)} document(s)")
    for doc in docs:
        print(f"  - {doc.name} (ID: {doc.id})")

    # Clean up temp file
    os.unlink(temp_file)

    # 3. List documents
    print("\n=== Listing documents ===")
    documents = dataset.list_documents()
    for doc in documents:
        print(f"  - {doc.name} (Status: {doc.run})")

    # 4. Parse documents
    print("\n=== Parsing documents ===")
    doc_ids = [doc.id for doc in documents]
    dataset.async_parse_documents(doc_ids)
    print(f"Async parsing initiated for {len(doc_ids)} document(s)")

    # 5. Update document
    print("\n=== Updating document ===")
    if documents:
        doc = documents[0]
        doc.update({"name": "renamed_sample.txt"})
        print(f"Document renamed to: renamed_sample.txt")

    # 6. Delete documents
    print("\n=== Deleting documents ===")
    dataset.delete_documents(ids=[doc.id for doc in documents])
    print("Documents deleted")

    # 7. Clean up dataset
    print("\n=== Cleaning up ===")
    rag.delete_datasets(ids=[dataset.id])
    print("Dataset deleted")

    print("\nAll operations completed successfully!")
    sys.exit(0)

except Exception as e:
    print(f"Error: {e}")
    sys.exit(-1)
