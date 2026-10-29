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


import os
from time import monotonic, sleep
from uuid import uuid4

import pytest
import requests
from ragflow_sdk import RAGFlow


def wait_until(probe, timeout=None):
    if timeout is None:
        timeout = float(os.getenv("RAGFLOW_SDK_INDEX_TIMEOUT", "60"))
    assert timeout > 0, "Index timeout must be positive"
    deadline = monotonic() + timeout
    while True:
        value = probe()
        if value:
            return value
        if monotonic() >= deadline:
            pytest.fail("Chunk indexing deadline exceeded")
        sleep(0.25)


def raw_chunks(document, page=1, page_size=100, **params):
    response = document.get(
        f"/datasets/{document.dataset_id}/documents/{document.id}/chunks",
        {"page": page, "page_size": page_size, **params},
    ).json()
    assert response["code"] == 0, response
    return response["data"]["chunks"]


def raw_ids(document):
    ids = set()
    page = 1
    while True:
        chunks = raw_chunks(document, page=page)
        page_ids = {chunk["chunk_id"] for chunk in chunks}
        assert len(page_ids) == len(chunks), "Chunk page contains duplicate IDs"
        assert not ids.intersection(page_ids), "Chunk pagination repeated IDs"
        ids.update(page_ids)
        if len(chunks) < 100:
            return ids
        page += 1


def wait_ids(document, expected):
    expected = set(expected)
    wait_until(lambda: raw_ids(document) == expected)


@pytest.fixture(autouse=True)
def network_boundary(request, monkeypatch):
    original = requests.sessions.Session.request

    def bounded(session, *args, **kwargs):
        if request.node.get_closest_marker("integration") is None:
            pytest.fail("Service-free chunk tests must not open network connections")
        kwargs.setdefault("timeout", (5, 60))
        return original(session, *args, **kwargs)

    monkeypatch.setattr(requests.sessions.Session, "request", bounded)


@pytest.fixture
def client():
    if os.getenv("RAGFLOW_SDK_ISOLATED") != "1" or not all(os.getenv(key) for key in ("RAGFLOW_SDK_BASE_URL", "RAGFLOW_SDK_API_KEY")):
        pytest.fail("Integration blocked: set RAGFLOW_SDK_ISOLATED=1, RAGFLOW_SDK_BASE_URL and RAGFLOW_SDK_API_KEY for an isolated Go backend")
    return RAGFlow(os.environ["RAGFLOW_SDK_API_KEY"], os.environ["RAGFLOW_SDK_BASE_URL"])


@pytest.fixture
def add_document(client):
    dataset = client.create_dataset(name=f"sdk-chunks-{uuid4().hex}")
    try:
        documents = dataset.upload_documents([{"display_name": "chunks.txt", "blob": b"Owned chunk SDK fixture."}])
        assert len(documents) == 1
        yield dataset, documents[0]
    finally:
        client.delete_datasets(ids=[dataset.id])
        assert client.list_datasets(id=dataset.id) == [], "Owned dataset cleanup failed"


@pytest.fixture
def add_chunks(add_document):
    dataset, document = add_document
    # Manual chunk operations do not need ingestion or a parser status wait.
    chunks = [
        document.add_chunk(content=f"Coriander botanical archive {label} entry {i}.", important_keywords=["coriander"], questions=["What is coriander?"])
        for i, label in enumerate(("saffron", "cumin", "basil", "thyme"))
    ]
    wait_ids(document, [chunk.id for chunk in chunks])
    return dataset, document, chunks
