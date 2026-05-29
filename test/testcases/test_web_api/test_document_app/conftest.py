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


import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest
from test_common import bulk_upload_documents, delete_document, list_documents


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func
        return decorator


class _StubKBRecord(dict):
    def __getattr__(self, item):
        try:
            return self[item]
        except KeyError as exc:
            raise AttributeError(item) from exc


@pytest.fixture(scope="function")
def add_document_func(request, WebApiAuth, add_dataset, ragflow_tmp_dir):
    def cleanup():
        res = list_documents(WebApiAuth, {"kb_id": dataset_id})
        for doc in res["data"]["docs"]:
            delete_document(WebApiAuth, dataset_id, {"ids": [doc["id"]]})

    request.addfinalizer(cleanup)

    dataset_id = add_dataset
    return dataset_id, bulk_upload_documents(WebApiAuth, dataset_id, 1, ragflow_tmp_dir)[0]


@pytest.fixture(scope="class")
def add_documents(request, WebApiAuth, add_dataset, ragflow_tmp_dir):
    def cleanup():
        res = list_documents(WebApiAuth, {"kb_id": dataset_id})
        for doc in res["data"]["docs"]:
            delete_document(WebApiAuth, dataset_id, {"ids": [doc["id"]]})

    request.addfinalizer(cleanup)

    dataset_id = add_dataset
    return dataset_id, bulk_upload_documents(WebApiAuth, dataset_id, 5, ragflow_tmp_dir)


@pytest.fixture(scope="function")
def add_documents_func(request, WebApiAuth, add_dataset_func, ragflow_tmp_dir):
    def cleanup():
        res = list_documents(WebApiAuth, {"kb_id": dataset_id})
        for doc in res["data"]["docs"]:
            delete_document(WebApiAuth, dataset_id, {"ids": [doc["id"]]})

    request.addfinalizer(cleanup)

    dataset_id = add_dataset_func
    return dataset_id, bulk_upload_documents(WebApiAuth, dataset_id, 3, ragflow_tmp_dir)


def _check_duplicate_ids(ids, *_args, **_kwargs):
    return list(dict.fromkeys(ids)), []


def _stub_document_api_dependencies(monkeypatch, repo_root):
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    common_settings_mod = ModuleType("common.settings")
    common_settings_mod.STORAGE_IMPL = SimpleNamespace(get=lambda *_args, **_kwargs: b"", obj_exist=lambda *_args, **_kwargs: False)
    common_settings_mod.docStoreConn = SimpleNamespace(
        index_exist=lambda *_args, **_kwargs: False,
        search=lambda *_args, **_kwargs: {},
        get_fields=lambda *_args, **_kwargs: {},
    )
    monkeypatch.setitem(sys.modules, "common.settings", common_settings_mod)

    metadata_utils_mod = ModuleType("common.metadata_utils")
    metadata_utils_mod.convert_conditions = lambda *_args, **_kwargs: {}
    metadata_utils_mod.meta_filter = lambda *_args, **_kwargs: True
    metadata_utils_mod.turn2jsonschema = lambda value: value
    monkeypatch.setitem(sys.modules, "common.metadata_utils", metadata_utils_mod)

    rag_nlp_mod = ModuleType("rag.nlp")
    rag_nlp_mod.search = SimpleNamespace(index_name=lambda tenant_id: f"ragflow_{tenant_id}")
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_mod)

    deepdoc_pkg = ModuleType("deepdoc")
    deepdoc_parser_pkg = ModuleType("deepdoc.parser")
    deepdoc_parser_pkg.__path__ = []

    class _StubPdfParser:
        pass

    class _StubExcelParser:
        pass

    class _StubHtmlParser:
        pass

    class _StubMinerUParser:
        pass

    class _StubPaddleOCRParser:
        pass

    deepdoc_parser_pkg.PdfParser = _StubPdfParser
    deepdoc_pkg.parser = deepdoc_parser_pkg
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser_pkg)

    deepdoc_excel_module = ModuleType("deepdoc.parser.excel_parser")
    deepdoc_excel_module.RAGFlowExcelParser = _StubExcelParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.excel_parser", deepdoc_excel_module)

    deepdoc_html_module = ModuleType("deepdoc.parser.html_parser")
    deepdoc_html_module.RAGFlowHtmlParser = _StubHtmlParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.html_parser", deepdoc_html_module)

    deepdoc_mineru_module = ModuleType("deepdoc.parser.mineru_parser")
    deepdoc_mineru_module.MinerUParser = _StubMinerUParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.mineru_parser", deepdoc_mineru_module)

    deepdoc_paddleocr_module = ModuleType("deepdoc.parser.paddleocr_parser")
    deepdoc_paddleocr_module.PaddleOCRParser = _StubPaddleOCRParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.paddleocr_parser", deepdoc_paddleocr_module)
    monkeypatch.setitem(sys.modules, "xgboost", ModuleType("xgboost"))

    stub_apps = ModuleType("api.apps")
    stub_apps.__path__ = [str(repo_root / "api" / "apps")]
    stub_apps.current_user = SimpleNamespace(id="user-1")
    stub_apps.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", stub_apps)

    stub_apps_services = ModuleType("api.apps.services")
    stub_apps_services.__path__ = [str(repo_root / "api" / "apps" / "services")]
    monkeypatch.setitem(sys.modules, "api.apps.services", stub_apps_services)

    document_api_service_mod = ModuleType("api.apps.services.document_api_service")
    document_api_service_mod.validate_document_update_fields = lambda *_args, **_kwargs: (None, None)
    document_api_service_mod.map_doc_keys = lambda doc: doc.to_dict() if hasattr(doc, "to_dict") else doc

    def _map_doc_keys_with_run_status(doc, run_status="0"):
        payload = doc if isinstance(doc, dict) else doc.to_dict()
        return {**payload, "run": run_status}

    document_api_service_mod.map_doc_keys_with_run_status = _map_doc_keys_with_run_status
    document_api_service_mod.update_document_name_only = lambda *_args, **_kwargs: None
    document_api_service_mod.update_chunk_method = lambda *_args, **_kwargs: None
    document_api_service_mod.update_document_status_only = lambda *_args, **_kwargs: None
    document_api_service_mod.reset_document_for_reparse = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.apps.services.document_api_service", document_api_service_mod)

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.Task = type("Task", (), {})
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    doc_metadata_service_mod = ModuleType("api.db.services.doc_metadata_service")
    doc_metadata_service_mod.DocMetadataService = SimpleNamespace(get_metadata_for_documents=lambda *_args, **_kwargs: {})
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", doc_metadata_service_mod)

    document_service_mod = ModuleType("api.db.services.document_service")
    document_service_mod.DocumentService = SimpleNamespace(
        query=lambda **_kwargs: [],
        get_by_id=lambda _doc_id: (False, None),
        accessible=lambda *_args, **_kwargs: False,
        get_by_kb_id=lambda *_args, **_kwargs: ([], 0),
        get_thumbnails=lambda _doc_ids: [],
        update_parser_config=lambda *_args, **_kwargs: None,
        update_by_id=lambda *_args, **_kwargs: True,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)

    file2document_service_mod = ModuleType("api.db.services.file2document_service")
    file2document_service_mod.File2DocumentService = SimpleNamespace(get_storage_address=lambda **_kwargs: ("bucket", "name"))
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2document_service_mod)

    file_service_mod = ModuleType("api.db.services.file_service")
    file_service_mod.FileService = SimpleNamespace(get_by_id=lambda *_args, **_kwargs: (False, None))
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)

    knowledgebase_service_mod = ModuleType("api.db.services.knowledgebase_service")
    knowledgebase_service_mod.KnowledgebaseService = SimpleNamespace(
        query=lambda **_kwargs: [],
        get_by_tenant_ids=lambda *_args, **_kwargs: ([], 0),
        get_by_id=lambda _dataset_id: (False, None),
        accessible=lambda *_args, **_kwargs: False,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", knowledgebase_service_mod)

    task_service_mod = ModuleType("api.db.services.task_service")
    task_service_mod.TaskService = SimpleNamespace(query=lambda **_kwargs: [])
    task_service_mod.cancel_all_task_of = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_service_mod)

    check_team_permission_mod = ModuleType("api.common.check_team_permission")
    check_team_permission_mod.check_kb_team_permission = lambda *_args, **_kwargs: True
    monkeypatch.setitem(sys.modules, "api.common.check_team_permission", check_team_permission_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    async def _default_request_json():
        return {}

    def _ok_result(*, data=None, message="success", code=0):
        return {"code": code, "message": message, "data": data}

    def _error_result(*, message="Sorry! Data missing!", code=102):
        return {"code": code, "message": message}

    def _server_error_response(error):
        return {"code": 500, "message": str(error)}

    api_utils_mod.get_request_json = _default_request_json
    api_utils_mod.get_data_error_result = _error_result
    api_utils_mod.get_error_data_result = _error_result
    api_utils_mod.get_result = _ok_result
    api_utils_mod.get_json_result = _ok_result
    api_utils_mod.server_error_response = _server_error_response
    api_utils_mod.add_tenant_id_to_kwargs = lambda func: func
    api_utils_mod.get_error_argument_result = _error_result
    api_utils_mod.check_duplicate_ids = _check_duplicate_ids
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    file_utils_mod = ModuleType("api.utils.file_utils")
    file_utils_mod.filename_type = lambda *_args, **_kwargs: "txt"
    file_utils_mod.thumbnail = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "api.utils.file_utils", file_utils_mod)

    web_utils_mod = ModuleType("api.utils.web_utils")
    web_utils_mod.CONTENT_TYPE_MAP = {"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg", "txt": "text/plain"}
    web_utils_mod.html2pdf = lambda *_args, **_kwargs: b""
    web_utils_mod.is_valid_url = lambda *_args, **_kwargs: True
    web_utils_mod.apply_safe_file_response_headers = lambda response, content_type, extension=None: response.headers.update({"content_type": content_type, "extension": extension})
    monkeypatch.setitem(sys.modules, "api.utils.web_utils", web_utils_mod)

    user_service_mod = ModuleType("api.db.services.user_service")
    user_service_mod.UserService = SimpleNamespace(query=lambda **_kwargs: [])
    user_service_mod.UserTenantService = SimpleNamespace(query=lambda **_kwargs: [])
    user_service_mod.TenantService = SimpleNamespace(query=lambda **_kwargs: [])
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)


def _load_document_api_module(repo_root, module_name):
    module_path = repo_root / "api" / "apps" / "restful_apis" / "document_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


@pytest.fixture()
def document_app_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    _stub_document_api_dependencies(monkeypatch, repo_root)
    return _load_document_api_module(repo_root, "test_document_app_unit")


@pytest.fixture()
def document_rest_api_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    _stub_document_api_dependencies(monkeypatch, repo_root)
    module = _load_document_api_module(repo_root, "test_document_api_unit")
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_by_id",
        lambda dataset_id: (
            True,
            _StubKBRecord(
                id=dataset_id,
                tenant_id="tenant1",
                name="kb",
                parser_id="parser",
                pipeline_id="pipe",
                parser_config={},
            ),
        ),
    )
    monkeypatch.setattr(module, "check_kb_team_permission", lambda *_args, **_kwargs: True)
    return module
