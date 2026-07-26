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

from pathlib import Path
from time import sleep

import pytest
from common import (
    batch_add_chunks,
    batch_create_chat_assistants,
    batch_create_datasets,
    bulk_upload_documents,
    delete_all_chats,
    delete_all_chunks,
    delete_all_datasets,
    delete_all_sessions,
)
from configs import HOST_ADDRESS, IS_GO_PROXY, VERSION
from pytest import FixtureRequest
from ragflow_sdk import Chat, Chunk, DataSet, Document, RAGFlow
from utils import wait_for
from utils.file_utils import (
    create_docx_file,
    create_eml_file,
    create_excel_file,
    create_html_file,
    create_image_file,
    create_json_file,
    create_md_file,
    create_pdf_file,
    create_ppt_file,
    create_txt_file,
)


GO_ONLY_PATH_SKIPS = {
    "test_file_management_within_dataset/test_download_document.py::test_file_type_validation": "Go deployment database schema is missing document.meta_fields",
    "test_file_management_within_dataset/test_upload_documents.py": "Go deployment database schema is missing document.meta_fields",
    "test_dataset_mangement/test_create_dataset.py::TestDatasetCreate::test_parser_config_invalid": "Go dataset parser_config only validates serialized size, not individual fields",
    "test_dataset_mangement/test_create_dataset.py::TestDatasetCreate::test_parser_config_empty": "Go dataset creation does not preserve an explicit empty parser_config",
    "test_dataset_mangement/test_create_dataset.py::TestDatasetCreate::test_parser_config_unset": "Go dataset creation does not return the SDK parser_config contract",
    "test_dataset_mangement/test_create_dataset.py::TestParserConfigBugFix": "Go dataset parser_config defaults do not match the SDK contract",
    "test_dataset_mangement/test_update_dataset.py::TestDatasetUpdate::test_parser_config": "Go dataset updates do not return the SDK parser_config contract",
    "test_dataset_mangement/test_update_dataset.py::TestDatasetUpdate::test_parser_config_invalid": "Go dataset parser_config only validates serialized size, not individual fields",
    "test_dataset_mangement/test_update_dataset.py::TestDatasetUpdate::test_parser_config_empty": "Go dataset update ignores an explicit empty parser_config",
    "test_dataset_mangement/test_update_dataset.py::TestDatasetUpdate::test_field_unsupported": "Go dataset update ignores unsupported fields",
    "test_dataset_mangement/test_update_dataset.py::TestDatasetUpdate::test_pagerank": "Go dataset update does not persist pagerank",
    "test_dataset_mangement/test_update_dataset.py::TestDatasetUpdate::test_pagerank_set_to_0": "Go dataset update does not persist pagerank",
    "test_dataset_mangement/test_list_datasets.py::TestDatasetsList::test_page_invalid": "Go dataset list normalizes invalid page values instead of rejecting them",
    "test_dataset_mangement/test_list_datasets.py::TestDatasetsList::test_page_size_invalid": "Go dataset list normalizes invalid page_size values instead of rejecting them",
    "test_dataset_mangement/test_list_datasets.py::TestDatasetsList::test_name": "Go dataset list does not apply the name filter",
    "test_dataset_mangement/test_list_datasets.py::TestDatasetsList::test_id": "Go dataset list does not apply the id filter",
    "test_dataset_mangement/test_list_datasets.py::TestDatasetsList::test_id_empty": "Go dataset list accepts an empty id instead of rejecting it",
    "test_dataset_mangement/test_list_datasets.py::TestDatasetsList::test_name_and_id": "Go dataset list does not combine name and id filters",
    "test_memory_management/test_list_memory.py::TestMemoryList::test_get_config_invalid_memory_id_raises": "Go memory config lookup does not reject an unknown memory id",
    "test_message_management/test_add_message.py::TestAddRawMessage": "Go built-in embedding uses an unavailable localhost:6380 endpoint",
    "test_message_management/test_add_message.py::TestAddMultipleTypeMessage": "Go built-in embedding uses an unavailable localhost:6380 endpoint",
    "test_message_management/test_add_message.py::TestAddToMultipleMemory": "Go built-in embedding uses an unavailable localhost:6380 endpoint",
    "test_message_management/test_forget_message.py::TestForgetMessage": "Go built-in embedding uses an unavailable localhost:6380 endpoint",
    "test_message_management/test_get_message_content.py::TestGetMessageContent": "Go built-in embedding uses an unavailable localhost:6380 endpoint",
    "test_message_management/test_get_recent_message.py::TestGetRecentMessage": "Go built-in embedding uses an unavailable localhost:6380 endpoint",
    "test_message_management/test_list_message.py::TestMessageList": "Go built-in embedding uses an unavailable localhost:6380 endpoint",
    "test_message_management/test_search_message.py::TestSearchMessage": "Go built-in embedding uses an unavailable localhost:6380 endpoint",
    "test_message_management/test_update_message_status.py::TestUpdateMessageStatus": "Go built-in embedding uses an unavailable localhost:6380 endpoint",
}


def pytest_collection_modifyitems(items):
    if not IS_GO_PROXY:
        return
    for item in items:
        for test_path, reason in GO_ONLY_PATH_SKIPS.items():
            matched_at = item.nodeid.find(test_path)
            matched_suffix = item.nodeid[matched_at + len(test_path) :] if matched_at >= 0 else ""
            if matched_at >= 0 and (not matched_suffix or matched_suffix.startswith("[") or matched_suffix.startswith("::")):
                item.add_marker(pytest.mark.skip(reason=reason))
                break


@wait_for(200, 1, "Document parsing timeout")
def condition(_dataset: DataSet):
    documents = _dataset.list_documents(page_size=100)
    for document in documents:
        if document.run != "DONE":
            return False
    return True


@pytest.fixture
def generate_test_files(request: FixtureRequest, tmp_path: Path):
    file_creators = {
        "docx": (tmp_path / "ragflow_test.docx", create_docx_file),
        "excel": (tmp_path / "ragflow_test.xlsx", create_excel_file),
        "ppt": (tmp_path / "ragflow_test.pptx", create_ppt_file),
        "image": (tmp_path / "ragflow_test.png", create_image_file),
        "pdf": (tmp_path / "ragflow_test.pdf", create_pdf_file),
        "txt": (tmp_path / "ragflow_test.txt", create_txt_file),
        "md": (tmp_path / "ragflow_test.md", create_md_file),
        "json": (tmp_path / "ragflow_test.json", create_json_file),
        "eml": (tmp_path / "ragflow_test.eml", create_eml_file),
        "html": (tmp_path / "ragflow_test.html", create_html_file),
    }

    files = {}
    for file_type, (file_path, creator_func) in file_creators.items():
        if request.param in ["", file_type]:
            creator_func(file_path)
            files[file_type] = file_path
    return files


@pytest.fixture(scope="class")
def ragflow_tmp_dir(request: FixtureRequest, tmp_path_factory: Path) -> Path:
    class_name = request.cls.__name__
    return tmp_path_factory.mktemp(class_name)


@pytest.fixture(scope="session")
def client(token: str) -> RAGFlow:
    return RAGFlow(api_key=token, base_url=HOST_ADDRESS, version=VERSION)


@pytest.fixture(scope="function")
def clear_datasets(request: FixtureRequest, client: RAGFlow):
    def cleanup():
        delete_all_datasets(client)

    request.addfinalizer(cleanup)


@pytest.fixture(scope="function")
def clear_chat_assistants(request: FixtureRequest, client: RAGFlow):
    def cleanup():
        delete_all_chats(client)

    request.addfinalizer(cleanup)


@pytest.fixture(scope="function")
def clear_session_with_chat_assistants(request, add_chat_assistants):
    def cleanup():
        for chat_assistant in chat_assistants:
            try:
                delete_all_sessions(chat_assistant)
            except Exception:
                pass

    request.addfinalizer(cleanup)

    _, _, chat_assistants = add_chat_assistants


@pytest.fixture(scope="class")
def add_dataset(request: FixtureRequest, client: RAGFlow) -> DataSet:
    def cleanup():
        delete_all_datasets(client)

    request.addfinalizer(cleanup)
    return batch_create_datasets(client, 1)[0]


@pytest.fixture(scope="function")
def add_dataset_func(request: FixtureRequest, client: RAGFlow) -> DataSet:
    def cleanup():
        delete_all_datasets(client)

    request.addfinalizer(cleanup)
    return batch_create_datasets(client, 1)[0]


@pytest.fixture(scope="class")
def add_document(add_dataset: DataSet, ragflow_tmp_dir: Path) -> tuple[DataSet, Document]:
    return add_dataset, bulk_upload_documents(add_dataset, 1, ragflow_tmp_dir)[0]


@pytest.fixture(scope="class")
def add_chunks(request: FixtureRequest, add_document: tuple[DataSet, Document]) -> tuple[DataSet, Document, list[Chunk]]:
    def cleanup():
        try:
            delete_all_chunks(document)
        except Exception:
            pass

    request.addfinalizer(cleanup)

    dataset, document = add_document
    dataset.async_parse_documents([document.id])
    condition(dataset)
    chunks = batch_add_chunks(document, 4)
    # issues/6487
    sleep(1)
    return dataset, document, chunks


@pytest.fixture(scope="class")
def add_chat_assistants(request, client, add_document) -> tuple[DataSet, Document, list[Chat]]:
    def cleanup():
        try:
            delete_all_chats(client)
        except Exception:
            pass

    request.addfinalizer(cleanup)

    dataset, document = add_document
    dataset.async_parse_documents([document.id])
    condition(dataset)
    return dataset, document, batch_create_chat_assistants(client, 5)
