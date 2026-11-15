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

import os
from pathlib import Path
from uuid import uuid4
from time import sleep

import pytest
import requests
from test.testcases.test_sdk_api.common import (
    batch_add_chunks,
    batch_create_chat_assistants,
    batch_create_datasets,
    bulk_upload_documents,
    delete_all_chats,
    delete_all_chunks,
    delete_all_datasets,
    delete_all_sessions,
)
from test.testcases.configs import HOST_ADDRESS, PASSWORD, VERSION
from pytest import FixtureRequest
from ragflow_sdk import Chat, Chunk, DataSet, Document, RAGFlow
from test.testcases.utils import wait_for
from test.testcases.utils.file_utils import (
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


def pytest_configure(config):
    config.addinivalue_line("markers", "integration: requires a disposable Go backend and dedicated test tenant")
    # SDK folders contain duplicate module names and must keep distinct imports.
    config.option.importmode = "importlib"
    config._sdk_ci_environments = {}
    config._sdk_ci_environment = pytest.MonkeyPatch()


EMBEDDING = "BAAI/bge-small-en-v1.5@Local@Builtin"
CHAT = "glm-4-flash@CI@ZHIPU-AI"
UPDATE_CHAT = "glm-4-flashx@CI@ZHIPU-AI"
RERANK = "BAAI/bge-reranker-v2-m3@CI@SILICONFLOW"


class _API:
    def __init__(self, base_url):
        self.base_url = base_url.rstrip("/") + "/api/v1"
        self.session = requests.Session()

    def call(self, method, path, body=None):
        # Never print response bodies: provider errors may contain credentials.
        try:
            response = self.session.request(method, self.base_url + path, json=body, timeout=120)
            payload = response.json()
            if response.status_code != 200 or payload.get("code") != 0:
                raise RuntimeError(f"SDK environment setup failed: {method} {path} (HTTP {response.status_code}, API {payload.get('code')})")
        except (requests.RequestException, ValueError):
            raise RuntimeError(f"SDK environment setup failed: {method} {path}") from None
        return payload.get("data"), response.headers

    def close(self):
        self.session.close()


def _tenant(base_url):
    api = _API(base_url)
    try:
        password = PASSWORD
        account = {"email": f"sdk-{uuid4().hex}@example.org", "nickname": "SDK CI", "password": password}
        api.call("POST", "/users", account)
        profile, headers = api.call("POST", "/auth/login", {"email": account["email"], "password": password})
        token = headers.get("Authorization")
        if not token or not isinstance(profile, dict) or not profile.get("id"):
            raise RuntimeError("SDK environment login did not return an identity and Authorization header")
        api.session.headers["Authorization"] = token
        key, _ = api.call("POST", "/system/tokens")
        if not isinstance(key, dict) or not key.get("token"):
            raise RuntimeError("SDK environment token creation did not return an API key")
        return api, profile["id"], key["token"]
    except Exception:
        api.close()
        raise


def _models(api, credentials):
    providers = (
        ("ZHIPU-AI", credentials["ZHIPU_AI_API_KEY"], "chat", ("glm-4-flash", "glm-4-flashx")),
        ("SILICONFLOW", credentials["SILICONFLOW_API_KEY"], "rerank", ("BAAI/bge-reranker-v2-m3",)),
    )
    for provider, key, kind, names in providers:
        api.call("PUT", "/providers", {"provider_name": provider})
        api.call(
            "POST",
            f"/providers/{provider}/instances",
            {
                "instance_name": "CI",
                "api_key": key,
                "base_url": "",
                "region": "default",
                "model_info": [{"model_type": [kind], "model_name": name, "max_tokens": 8192} for name in names],
            },
        )
    for kind, identifier in (("chat", CHAT), ("embedding", EMBEDDING), ("rerank", RERANK)):
        name, instance, provider = identifier.rsplit("@", 2)
        api.call(
            "PATCH",
            "/models/default",
            {
                "model_provider": provider,
                "model_instance": instance,
                "model_type": kind,
                "model_name": name,
            },
        )


def _prepare_ci_environment(folder_name):
    if os.getenv("RAGFLOW_SDK_CI") != "1" or os.getenv("CI") != "true" or not os.getenv("HOST_ADDRESS") or os.getenv("DOC_ENGINE") not in {"infinity", "elasticsearch"}:
        raise RuntimeError("SDK provisioning requires explicit SDK CI opt-in, CI=true, HOST_ADDRESS and a supported DOC_ENGINE in a disposable stack")
    credentials = {name: os.getenv(name) for name in ("ZHIPU_AI_API_KEY", "SILICONFLOW_API_KEY")}
    missing = [name for name, value in credentials.items() if not value]
    if missing:
        raise RuntimeError("Full SDK integration tests require credentials: " + ", ".join(missing))
    base_url = os.environ["HOST_ADDRESS"].rstrip("/")
    engine = os.environ["DOC_ENGINE"]
    api, tenant_id, api_key = _tenant(base_url)
    try:
        _models(api, credentials)
        other, other_id, other_key = _tenant(base_url)
        other.close()
        env = {
            "RAGFLOW_SDK_ISOLATED": "1",
            "RAGFLOW_SDK_BASE_URL": base_url,
            "RAGFLOW_SDK_API_KEY": api_key,
            "RAGFLOW_SDK_TENANT_ID": tenant_id,
            "RAGFLOW_SDK_OTHER_API_KEY": other_key,
            "RAGFLOW_SDK_OTHER_TENANT_ID": other_id,
            "RAGFLOW_SDK_DOC_ENGINE": engine,
            "RAGFLOW_SDK_EMBEDDING_MODEL": EMBEDDING,
            "RAGFLOW_SDK_MEMORY_EMBD_ID": EMBEDDING,
            "RAGFLOW_SDK_MEMORY_LLM_ID": CHAT,
            "RAGFLOW_SDK_MEMORY_UPDATE_LLM_ID": UPDATE_CHAT,
            "RAGFLOW_SDK_RERANK_ID": RERANK,
            "RAGFLOW_SDK_CHAT_CONFIGURED": "1",
        }
        if folder_name == "test_dataset_mangement":
            template, _ = api.call("GET", "/pipelines/general")
            pipeline, _ = api.call(
                "POST",
                "/agents",
                {
                    "title": "SDK CI ingestion",
                    "canvas_category": "dataflow_canvas",
                    "dsl": template["dsl"],
                },
            )
            version, _ = api.call("POST", f"/agents/{pipeline['id']}/publish", {})
            env.update(RAGFLOW_SDK_PIPELINE_ID=pipeline["id"], RAGFLOW_SDK_PIPELINE_VERSION_ID=version["id"])
        return env
    finally:
        api.close()


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    # Audited SDK fixtures own their tenant setup; never configure the shared QA account.
    return None


@pytest.hookimpl(tryfirst=True)
def pytest_runtest_setup(item):
    if os.getenv("RAGFLOW_SDK_CI") != "1" or item.get_closest_marker("integration") is None or item.get_closest_marker("skip") is not None:
        return
    # Run before folder-scoped session fixtures read their environment settings.
    folder = item.path.parent.name
    environments = item.config._sdk_ci_environments
    if folder not in environments:
        try:
            environments[folder] = _prepare_ci_environment(folder)
        except Exception as error:
            detail = str(error) if isinstance(error, RuntimeError) else type(error).__name__
            pytest.exit(f"SDK CI provisioning failed: {detail}", returncode=1)
    for key, value in environments[folder].items():
        item.config._sdk_ci_environment.setenv(key, value)


def pytest_unconfigure(config):
    config._sdk_ci_environment.undo()


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
