#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

"""Dataset-only fixtures; never provision models or delete tenant-wide resources."""

import os
from uuid import uuid4

import pytest
import requests
from ragflow_sdk import RAGFlow


@pytest.fixture(autouse=True)
def block_contract_network(request, monkeypatch):
    if request.node.get_closest_marker("integration") is None:

        def reject_request(*args, **kwargs):
            pytest.fail("Service-free dataset tests must not open a network connection")

        monkeypatch.setattr(requests.sessions.Session, "request", reject_request)


@pytest.fixture(scope="session")
def integration_settings():
    required = ("RAGFLOW_SDK_BASE_URL", "RAGFLOW_SDK_API_KEY")
    if os.getenv("RAGFLOW_SDK_ISOLATED") != "1" or any(not os.getenv(key) for key in required):
        pytest.fail(
            "Integration environment required: set RAGFLOW_SDK_ISOLATED=1, "
            "RAGFLOW_SDK_BASE_URL and RAGFLOW_SDK_API_KEY for a dedicated, empty Go test tenant. "
            "Run service-free tests with -m 'not integration'."
        )
    settings = {
        "base_url": os.environ["RAGFLOW_SDK_BASE_URL"],
        "api_key": os.environ["RAGFLOW_SDK_API_KEY"],
        "embedding_model": os.getenv("RAGFLOW_SDK_EMBEDDING_MODEL"),
        "pipeline_id": os.getenv("RAGFLOW_SDK_PIPELINE_ID"),
        "pipeline_version_id": os.getenv("RAGFLOW_SDK_PIPELINE_VERSION_ID"),
        "doc_engine": os.getenv("RAGFLOW_SDK_DOC_ENGINE"),
        "other_api_key": os.getenv("RAGFLOW_SDK_OTHER_API_KEY"),
    }
    probe = RAGFlow(settings["api_key"], settings["base_url"])
    assert probe.list_datasets(page_size=1) == [], "Refusing integrations: test tenant is not empty"
    return settings


@pytest.fixture
def client(integration_settings, monkeypatch):
    rag = RAGFlow(integration_settings["api_key"], integration_settings["base_url"])
    created = []
    create = rag.create_dataset

    def tracked_create(*args, **kwargs):
        dataset = create(*args, **kwargs)
        created.append(dataset.id)
        return dataset

    monkeypatch.setattr(rag, "create_dataset", tracked_create)
    yield rag
    # Tests may already have deleted some IDs; never fall back to delete_all.
    for offset in range(0, len(created), 100):
        ids = created[offset : offset + 100]
        remaining = rag.list_datasets(ids=ids, page_size=100)
        if remaining:
            rag.delete_datasets(ids=[dataset.id for dataset in remaining])
            assert rag.list_datasets(ids=ids, page_size=100) == [], "Test dataset cleanup failed"


@pytest.fixture
def dataset_factory(client):
    def create(name=None, **kwargs):
        return client.create_dataset(name=name if name is not None else f"sdk-dataset-{uuid4().hex}", **kwargs)

    return create


@pytest.fixture
def add_dataset_func(dataset_factory):
    return dataset_factory()


@pytest.fixture
def add_datasets_func(dataset_factory):
    return [dataset_factory() for _ in range(3)]


@pytest.fixture
def add_datasets(dataset_factory):
    return [dataset_factory() for _ in range(5)]
