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

"""Document-only fixtures; no legacy backend imports or tenant-wide cleanup."""

import os
import re
from uuid import uuid4

import pytest
import requests
from ragflow_sdk import RAGFlow


@pytest.fixture(autouse=True)
def network_boundary(request, monkeypatch):
    original = requests.sessions.Session.request

    def bounded_request(session, *args, **kwargs):
        if request.node.get_closest_marker("integration") is None:
            pytest.fail("Service-free document tests must not open a network connection")
        kwargs.setdefault("timeout", (5, 60))
        return original(session, *args, **kwargs)

    monkeypatch.setattr(requests.sessions.Session, "request", bounded_request)


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
        "doc_engine": os.getenv("RAGFLOW_SDK_DOC_ENGINE"),
        "parse_timeout": float(os.getenv("RAGFLOW_SDK_PARSE_TIMEOUT", "180")),
    }
    probe = RAGFlow(settings["api_key"], settings["base_url"])
    # This read-only probe also has a timeout before any test may create resources.
    original = requests.sessions.Session.request

    def bounded_probe(session, *args, **kwargs):
        kwargs.setdefault("timeout", (5, 60))
        return original(session, *args, **kwargs)

    with pytest.MonkeyPatch.context() as patch:
        patch.setattr(requests.sessions.Session, "request", bounded_probe)
        assert probe.list_datasets(page_size=1) == [], "Refusing integrations: test tenant is not empty"
    return settings


@pytest.fixture
def client(integration_settings, monkeypatch):
    rag = RAGFlow(integration_settings["api_key"], integration_settings["base_url"])
    created = []
    unresolved = set()
    create = rag.create_dataset

    def tracked_create(*args, **kwargs):
        name = kwargs.get("name", args[0] if args else None)
        # Recover only our UUID-named creations if persistence succeeds before the response fails.
        if isinstance(name, str) and re.fullmatch(r"sdk-document-[0-9a-f]{32}", name):
            unresolved.add(name)
        dataset = create(*args, **kwargs)
        created.append(dataset.id)
        unresolved.discard(name)
        return dataset

    monkeypatch.setattr(rag, "create_dataset", tracked_create)
    yield rag
    for name in unresolved:
        created.extend(dataset.id for dataset in rag.list_datasets(name=name, page_size=100) if dataset.name == name)
    # A test can delete its own dataset early; never broaden cleanup to delete_all.
    for offset in range(0, len(created), 100):
        ids = created[offset : offset + 100]
        remaining = rag.list_datasets(ids=ids, page_size=100)
        if remaining:
            rag.delete_datasets(ids=[dataset.id for dataset in remaining])
            assert rag.list_datasets(ids=ids, page_size=100) == [], "Test dataset cleanup failed"


@pytest.fixture
def dataset_factory(client):
    def create(name=None, **kwargs):
        return client.create_dataset(name=name if name is not None else f"sdk-document-{uuid4().hex}", **kwargs)

    return create


@pytest.fixture
def add_dataset_func(dataset_factory):
    return dataset_factory()


@pytest.fixture
def add_dataset(add_dataset_func):
    return add_dataset_func


@pytest.fixture
def document_factory():
    def create(dataset, name=None, blob=b"RAGFlow document SDK integration test.\n"):
        documents = dataset.upload_documents([{"display_name": name or f"sdk-{uuid4().hex}.txt", "blob": blob}])
        assert len(documents) == 1
        return documents[0]

    return create


@pytest.fixture
def upload_documents():
    def upload(dataset, num):
        payload = [{"display_name": f"ragflow_test_upload_{i}.txt", "blob": f"Document {i}: SDK integration test.\n".encode()} for i in range(num)]
        documents = dataset.upload_documents(payload)
        assert len(documents) == num
        return documents

    return upload


@pytest.fixture
def add_document_func(add_dataset_func, document_factory):
    return add_dataset_func, document_factory(add_dataset_func, name="ragflow_test_upload_0.txt")


@pytest.fixture
def add_documents_func(add_dataset_func, upload_documents):
    return add_dataset_func, upload_documents(add_dataset_func, 3)


@pytest.fixture
def add_documents(add_dataset_func, upload_documents):
    return add_dataset_func, upload_documents(add_dataset_func, 5)


@pytest.fixture
def generate_test_files(request, tmp_path):
    # Import existing format generators only for live upload/download fixtures.
    from utils import file_utils

    formats = {
        "docx": ("docx", file_utils.create_docx_file),
        "excel": ("xlsx", file_utils.create_excel_file),
        "ppt": ("pptx", file_utils.create_ppt_file),
        "image": ("png", file_utils.create_image_file),
        "pdf": ("pdf", file_utils.create_pdf_file),
        "txt": ("txt", file_utils.create_txt_file),
        "md": ("md", file_utils.create_md_file),
        "json": ("json", file_utils.create_json_file),
        "eml": ("eml", file_utils.create_eml_file),
        "html": ("html", file_utils.create_html_file),
    }
    extension, create = formats[request.param]
    path = tmp_path / f"ragflow_test.{extension}"
    create(path)
    return {request.param: path}
