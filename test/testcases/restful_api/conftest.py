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

from test.testcases.configs import IS_GO_PROXY
from libs.auth import RAGFlowHttpApiAuth
from test.testcases.restful_api.helpers.client import RestClient
from utils.file_utils import create_txt_file
from utils import wait_for


GO_ONLY_SKIPS = {
    "Go route is not implemented": {
        "test_document_download_by_id_invalid_id_contract",
        "test_llm_factories_live_auth_contract",
        "test_llm_list_live_auth_contract",
        "test_retrieval_compatibility_endpoint",
        "test_retrieval_compatibility_requires_dataset_ids",
        "test_retrieval_compatibility_requires_auth",
        "test_retrieval_requires_auth_contract",
        "test_system_oceanbase_status_auth_contract",
        "test_task_routes_require_auth",
        "test_patch_task_rejects_unsupported_action",
        "test_cancel_missing_task_sets_cancel_contract",
    },
    "Go validation or response contract does not match the established API contract": {
        "test_chat_list_default_get_and_separate_lookup_contract",
        "test_chat_list_page_and_page_size_contract",
        "test_chat_list_sorting_contract",
        "test_chat_create_prompt_contract",
        "test_chat_update_name_contract",
        "test_chat_update_mapping_and_validation_branches_p2",
        "test_dataset_update_name_and_case_insensitive_contract",
        "test_dataset_update_parser_config_valid_matrix_contract",
        "test_dataset_update_parser_config_with_chunk_method_change_contract",
        "test_dataset_update_pagerank_contract",
        "test_dataset_update_pagerank_set_to_zero_contract",
        "test_dataset_update_content_type_and_payload_contract",
        "test_dataset_update_identifier_validation_contract",
        "test_dataset_update_parser_config_defaults_contract",
        "test_dataset_update_parser_config_invalid_contract",
        "test_dataset_update_field_unset_and_unsupported_contract",
        "test_dataset_create_parser_config_missing_raptor_and_graphrag",
        "test_dataset_create_parser_config_bugfix_contract",
        "test_dataset_create_content_type_and_payload_bad_contract",
        "test_dataset_create_parser_config_invalid_contract",
        "test_dataset_create_parser_config_defaults_and_extra_fields_contract",
        "test_dataset_list_query_contract_matrix",
        "test_dataset_delete_contract_matrix",
        "test_memory_update_invalid_name",
        "test_search_create_invalid_name",
        "test_search_update_invalid_search_id",
        "test_session_create_validation_and_deleted_chat_contract",
        "test_session_delete_basic_scenarios",
        "test_session_list_filter_and_deleted_chat_contract",
        "test_session_list_page_and_sort_contract",
        "test_session_update_name_and_param_contract",
        "test_session_update_requires_auth_and_invalid_target_contract",
        "test_chat_completion_validation_errors",
        "test_chat_completion_nonstream_with_session",
        "test_search_completion_sse_shape_when_kb_ids_provided",
        "test_system_tokens_auth_and_crud",
    },
    "Go LLM setup cannot exercise the configured model": {
        "test_related_questions_contract",
    },
}


def pytest_collection_modifyitems(items):
    if not IS_GO_PROXY:
        return
    for item in items:
        test_name = item.name.split("[", 1)[0]
        for reason, skipped_tests in GO_ONLY_SKIPS.items():
            if test_name in skipped_tests:
                item.add_marker(pytest.mark.skip(reason=reason))
                break


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
