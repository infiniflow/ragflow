---
name: ragflow-python-sdk
description: >-
  Script RAGFlow with the ragflow-sdk Python package: create datasets,
  upload and parse documents, retrieve chunks, create chat assistants and
  sessions and ask questions, plus the OpenAI-compatible chat endpoint. Use
  when building or automating a RAG pipeline on a RAGFlow server from code.
---

# RAGFlow Python SDK

References: `docs/references/python_api_reference.md`,
`docs/references/http_api_reference.md`, `sdk/python/ragflow_sdk/`.

```bash
pip install ragflow-sdk
```

```python
import os
from ragflow_sdk import RAGFlow

rag = RAGFlow(api_key=os.environ["RAGFLOW_API_KEY"], base_url="http://localhost:9380")
```

## Ingest

```python
dataset = rag.create_dataset(name="handbook")          # parser_id="table" etc. for other chunk methods
with open("policy.pdf", "rb") as f:
    docs = dataset.upload_documents([{"display_name": "policy.pdf", "blob": f.read()}])

ids = [d.id for d in docs]
for doc_id, status, chunks, tokens in dataset.parse_documents(ids):   # blocks until done
    print(doc_id, status, chunks, tokens)
```

Use `dataset.async_parse_documents(ids)` to start parsing without waiting.
Uploading alone does not make documents searchable; they must be parsed.
`avatar`/`description` are set afterwards with `dataset.update(...)`.

## Retrieve

```python
chunks = rag.retrieve(
    dataset_ids=[dataset.id],
    question="How many vacation days do new hires get?",
    page_size=10,
    similarity_threshold=0.2,
    vector_similarity_weight=0.3,
)
for c in chunks:
    print(c.content)
```

All `document_ids` passed together must share an embedding model.

## Chat

```python
assistant = rag.create_chat("Handbook bot", dataset_ids=[dataset.id])
session = assistant.create_session()
answer = session.ask("How many vacation days do new hires get?", stream=False)
print(answer.content)

for part in session.ask("And after five years?", stream=True):
    print(part.content)
```

Agents work the same way via `rag.list_agents()` and agent sessions.

## OpenAI-compatible endpoint

```python
from openai import OpenAI
client = OpenAI(api_key=RAGFLOW_API_KEY, base_url=f"http://<host>/api/v1/openai/{chat_id}/chat")
client.chat.completions.create(model="model", messages=[...], extra_body={"reference": True})
```

## Errors

SDK methods raise `Exception` with the server message on failure. A
missing Chat/Embedding default model in the account is the most common
cause of parse or chat failures.
