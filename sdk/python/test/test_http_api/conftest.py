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

import pytest
from common import (
    add_chunk,
    batch_create_datasets,
    bulk_upload_documents,
    create_chat_assistant,
    delete_chat_assistants,
    delete_datasets,
    delete_session_with_chat_assistants,
    list_documnets,
    parse_documnets,
)
from libs.auth import RAGFlowHttpApiAuth
from libs.utils import wait_for
from libs.utils.file_utils import (
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

MARKER_EXPRESSIONS = {
    "p1": "p1",
    "p2": "p1 or p2",
    "p3": "p1 or p2 or p3",
}
HOST_ADDRESS = os.getenv("HOST_ADDRESS", "http://127.0.0.1:9380")


def pytest_addoption(parser: pytest.Parser) -> None:
    parser.addoption(
        "--level",
        action="store",
        default="p2",
        choices=list(MARKER_EXPRESSIONS.keys()),
        help=f"Test level ({'/'.join(MARKER_EXPRESSIONS)}): p1=smoke, p2=core, p3=full",
    )


def pytest_configure(config: pytest.Config) -> None:
    level = config.getoption("--level")
    config.option.markexpr = MARKER_EXPRESSIONS[level]
    if config.option.verbose > 0:
        print(f"\n[CONFIG] Active test level: {level}")


@wait_for(30, 1, "Document parsing timeout")
def condition(_auth, _dataset_id):
    res = list_documnets(_auth, _dataset_id)
    for doc in res["data"]["docs"]:
        if doc["run"] != "DONE":
            return False
    return True


@pytest.fixture(scope="session")
def get_http_api_auth(get_api_key_fixture):
    return RAGFlowHttpApiAuth(get_api_key_fixture)


@pytest.fixture(scope="function")
def clear_datasets(request, get_http_api_auth):
    def cleanup():
        delete_datasets(get_http_api_auth, {"ids": None})

    request.addfinalizer(cleanup)


@pytest.fixture(scope="function")
def clear_chat_assistants(request, get_http_api_auth):
    def cleanup():
        delete_chat_assistants(get_http_api_auth)

    request.addfinalizer(cleanup)


@pytest.fixture(scope="function")
def clear_session_with_chat_assistants(request, get_http_api_auth, add_chat_assistants):
    _, _, chat_assistant_ids = add_chat_assistants

    def cleanup():
        for chat_assistant_id in chat_assistant_ids:
            delete_session_with_chat_assistants(get_http_api_auth, chat_assistant_id)

    request.addfinalizer(cleanup)


@pytest.fixture
def generate_test_files(request, tmp_path):
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
def ragflow_tmp_dir(request, tmp_path_factory):
    class_name = request.cls.__name__
    return tmp_path_factory.mktemp(class_name)


@pytest.fixture(scope="class")
def add_dataset(request, get_http_api_auth):
    def cleanup():
        delete_datasets(get_http_api_auth, {"ids": None})

    request.addfinalizer(cleanup)

    dataset_ids = batch_create_datasets(get_http_api_auth, 1)
    return dataset_ids[0]


@pytest.fixture(scope="function")
def add_dataset_func(request, get_http_api_auth):
    def cleanup():
        delete_datasets(get_http_api_auth, {"ids": None})

    request.addfinalizer(cleanup)

    return batch_create_datasets(get_http_api_auth, 1)[0]


@pytest.fixture(scope="class")
def add_document(get_http_api_auth, add_dataset, ragflow_tmp_dir):
    dataset_id = add_dataset
    document_ids = bulk_upload_documents(get_http_api_auth, dataset_id, 1, ragflow_tmp_dir)
    return dataset_id, document_ids[0]


@pytest.fixture(scope="class")
def add_chunks(get_http_api_auth, add_document):
    dataset_id, document_id = add_document
    parse_documnets(get_http_api_auth, dataset_id, {"document_ids": [document_id]})
    condition(get_http_api_auth, dataset_id)

    chunk_ids = []
    for i in range(4):
        res = add_chunk(get_http_api_auth, dataset_id, document_id, {"content": f"chunk test {i}"})
        chunk_ids.append(res["data"]["chunk"]["id"])

    # issues/6487
    from time import sleep

    sleep(1)
    return dataset_id, document_id, chunk_ids


@pytest.fixture(scope="class")
def add_chat_assistants(request, get_http_api_auth, add_document):
    def cleanup():
        delete_chat_assistants(get_http_api_auth)

    request.addfinalizer(cleanup)

    dataset_id, document_id = add_document
    parse_documnets(get_http_api_auth, dataset_id, {"document_ids": [document_id]})
    condition(get_http_api_auth, dataset_id)

    chat_assistant_ids = []
    for i in range(5):
        res = create_chat_assistant(get_http_api_auth, {"name": f"test_chat_assistant_{i}", "dataset_ids": [dataset_id]})
        chat_assistant_ids.append(res["data"]["id"])

    return dataset_id, document_id, chat_assistant_ids
