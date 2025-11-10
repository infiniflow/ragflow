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
from time import sleep

import pytest
from common import (
    batch_add_chunks,
    batch_create_chat_assistants,
    batch_create_datasets,
    bulk_upload_documents,
    delete_chat_assistants,
    delete_datasets,
    delete_session_with_chat_assistants,
    list_documents,
    parse_documents,
)
from libs.auth import RAGFlowHttpApiAuth
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


@wait_for(30, 1, "Document parsing timeout")
def condition(_auth, _dataset_id):
    res = list_documents(_auth, _dataset_id)
    for doc in res["data"]["docs"]:
        if doc["run"] != "DONE":
            return False
    return True


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


@pytest.fixture(scope="session")
def HttpApiAuth(token):
    return RAGFlowHttpApiAuth(token)


@pytest.fixture(scope="function")
def clear_datasets(request, HttpApiAuth):
    def cleanup():
        delete_datasets(HttpApiAuth, {"ids": None})

    request.addfinalizer(cleanup)


@pytest.fixture(scope="function")
def clear_chat_assistants(request, HttpApiAuth):
    def cleanup():
        delete_chat_assistants(HttpApiAuth)

    request.addfinalizer(cleanup)


@pytest.fixture(scope="function")
def clear_session_with_chat_assistants(request, HttpApiAuth, add_chat_assistants):
    def cleanup():
        for chat_assistant_id in chat_assistant_ids:
            delete_session_with_chat_assistants(HttpApiAuth, chat_assistant_id)

    request.addfinalizer(cleanup)

    _, _, chat_assistant_ids = add_chat_assistants


@pytest.fixture(scope="class")
def add_dataset(request, HttpApiAuth):
    def cleanup():
        delete_datasets(HttpApiAuth, {"ids": None})

    request.addfinalizer(cleanup)

    dataset_ids = batch_create_datasets(HttpApiAuth, 1)
    return dataset_ids[0]


@pytest.fixture(scope="function")
def add_dataset_func(request, HttpApiAuth):
    def cleanup():
        delete_datasets(HttpApiAuth, {"ids": None})

    request.addfinalizer(cleanup)

    return batch_create_datasets(HttpApiAuth, 1)[0]


@pytest.fixture(scope="class")
def add_document(HttpApiAuth, add_dataset, ragflow_tmp_dir):
    dataset_id = add_dataset
    document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, 1, ragflow_tmp_dir)
    return dataset_id, document_ids[0]


@pytest.fixture(scope="class")
def add_chunks(HttpApiAuth, add_document):
    dataset_id, document_id = add_document
    parse_documents(HttpApiAuth, dataset_id, {"document_ids": [document_id]})
    condition(HttpApiAuth, dataset_id)
    chunk_ids = batch_add_chunks(HttpApiAuth, dataset_id, document_id, 4)
    sleep(1)  # issues/6487
    return dataset_id, document_id, chunk_ids


@pytest.fixture(scope="class")
def add_chat_assistants(request, HttpApiAuth, add_document):
    def cleanup():
        delete_chat_assistants(HttpApiAuth)

    request.addfinalizer(cleanup)

    dataset_id, document_id = add_document
    parse_documents(HttpApiAuth, dataset_id, {"document_ids": [document_id]})
    condition(HttpApiAuth, dataset_id)
    return dataset_id, document_id, batch_create_chat_assistants(HttpApiAuth, 5)
