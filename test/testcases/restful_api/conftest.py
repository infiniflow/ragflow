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

import pytest

from libs.auth import RAGFlowHttpApiAuth
from test.testcases.restful_api.helpers.client import RestClient
from utils.file_utils import create_txt_file
from utils import wait_for


@pytest.fixture(scope="session")
def RestApiAuth(token):
    return RAGFlowHttpApiAuth(token)


@pytest.fixture(scope="session")
def rest_client(token):
    return RestClient(token=token)


@pytest.fixture(scope="session")
def rest_client_noauth():
    return RestClient(token=None)


@pytest.fixture
def clear_datasets(rest_client):
    def _cleanup():
        res = rest_client.delete("/datasets", json={"ids": None, "delete_all": True})
        assert res.status_code == 200, res.text
        payload = res.json()
        assert payload["code"] in (0, 102), payload

    yield
    _cleanup()


@pytest.fixture
def clear_chats(rest_client):
    def _cleanup():
        res = rest_client.delete("/chats", json={"ids": None, "delete_all": True})
        assert res.status_code == 200, res.text
        payload = res.json()
        assert payload["code"] in (0, 102), payload

    yield
    _cleanup()


@pytest.fixture
def create_dataset(rest_client, clear_datasets):
    created_ids: list[str] = []

    def _create(name: str = "restful_dataset") -> str:
        res = rest_client.post("/datasets", json={"name": name})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 0, payload
        dataset_id = payload["data"]["id"]
        created_ids.append(dataset_id)
        return dataset_id

    yield _create

    if created_ids:
        res = rest_client.delete("/datasets", json={"ids": created_ids})
        assert res.status_code == 200
        payload = res.json()
        # Dataset may already be removed by test logic/cleanup.
        assert payload["code"] in (0, 102), payload


@pytest.fixture
def create_chat(rest_client, clear_chats):
    created_ids: list[str] = []

    def _create(name: str = "restful_chat") -> str:
        res = rest_client.post("/chats", json={"name": name, "dataset_ids": []})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 0, payload
        chat_id = payload["data"]["id"]
        created_ids.append(chat_id)
        return chat_id

    yield _create

    if created_ids:
        res = rest_client.delete("/chats", json={"ids": created_ids})
        assert res.status_code == 200, res.text
        payload = res.json()
        assert payload["code"] in (0, 102), payload


@pytest.fixture
def create_document(rest_client, create_dataset, tmp_path):
    created_docs: list[tuple[str, str]] = []

    def _create(name: str = "restful_doc.txt") -> tuple[str, str]:
        dataset_id = create_dataset("dataset_for_doc")
        fp = create_txt_file(tmp_path / name)
        with fp.open("rb") as file_obj:
            files = [("file", (fp.name, file_obj))]
            res = rest_client.post(f"/datasets/{dataset_id}/documents", files=files)
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 0, payload
        document_id = payload["data"][0]["id"]
        created_docs.append((dataset_id, document_id))
        return dataset_id, document_id

    yield _create

    for dataset_id, document_id in created_docs:
        res = rest_client.delete(f"/datasets/{dataset_id}/documents", json={"ids": [document_id]})
        assert res.status_code == 200, res.text
        payload = res.json()
        assert payload["code"] in (0, 102), payload


@wait_for(60, 1, "Document parsing timeout in RESTful batch2 tests")
def _parsed(rest_client: RestClient, dataset_id: str, document_id: str):
    res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"id": document_id})
    if res.status_code != 200:
        return False
    payload = res.json()
    if payload["code"] != 0:
        return False
    docs = payload["data"]["docs"]
    if not docs:
        return False
    return docs[0].get("run") == "DONE"


@pytest.fixture
def ensure_parsed_document(rest_client, create_document):
    def _ensure() -> tuple[str, str]:
        dataset_id, document_id = create_document()
        res = rest_client.post(
            f"/datasets/{dataset_id}/documents/parse",
            json={"document_ids": [document_id]},
        )
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 0, payload
        _parsed(rest_client, dataset_id, document_id)
        return dataset_id, document_id

    return _ensure
