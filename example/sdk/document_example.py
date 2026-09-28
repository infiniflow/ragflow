"""Manage a document through its dataset lifecycle with the Python SDK."""

import os

from ragflow_sdk import RAGFlow

HOST_ADDRESS = os.environ.get("RAGFLOW_HOST_ADDRESS", "http://127.0.0.1:9380")
API_KEY = os.environ.get("RAGFLOW_API_KEY", "ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm")


rag = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)
dataset = rag.create_dataset(name="document_example_dataset")
document = None
try:
    documents = dataset.upload_documents([{"display_name": "sample.txt", "blob": b"RAGFlow is an open-source RAG engine.\n"}])
    document = documents[0]
    print(f"Uploaded {document.name} (id={document.id})")

    listed = dataset.list_documents(id=document.id)
    document = listed[0]
    print(f"Listed {document.name} (run={document.run})")

    document.update({"name": "renamed_sample.txt"})
    print(f"Renamed document to {document.name}")

    statuses = dataset.parse_documents([document.id])
    status = next(item for item in statuses if item[0] == document.id)
    if str(status[1]).upper() != "DONE":
        raise RuntimeError(f"Parsing failed: {status[1]}")
    print(f"Parsing completed: {status[2]} chunks, {status[3]} tokens")
finally:
    if document is not None:
        dataset.delete_documents(ids=[document.id])
    rag.delete_datasets(ids=[dataset.id])
