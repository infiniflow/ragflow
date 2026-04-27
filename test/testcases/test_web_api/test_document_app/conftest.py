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


@pytest.fixture()
def document_app_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    deepdoc_pkg = ModuleType("deepdoc")
    deepdoc_parser_pkg = ModuleType("deepdoc.parser")
    deepdoc_parser_pkg.__path__ = []

    class _StubPdfParser:
        pass

    class _StubExcelParser:
        pass

    deepdoc_parser_pkg.PdfParser = _StubPdfParser
    deepdoc_pkg.parser = deepdoc_parser_pkg
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser_pkg)
    deepdoc_excel_module = ModuleType("deepdoc.parser.excel_parser")
    deepdoc_excel_module.RAGFlowExcelParser = _StubExcelParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.excel_parser", deepdoc_excel_module)
    deepdoc_html_module = ModuleType("deepdoc.parser.html_parser")

    class _StubHtmlParser:
        pass

    deepdoc_html_module.RAGFlowHtmlParser = _StubHtmlParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.html_parser", deepdoc_html_module)
    deepdoc_mineru_module = ModuleType("deepdoc.parser.mineru_parser")

    class _StubMinerUParser:
        pass

    deepdoc_mineru_module.MinerUParser = _StubMinerUParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.mineru_parser", deepdoc_mineru_module)
    deepdoc_paddleocr_module = ModuleType("deepdoc.parser.paddleocr_parser")

    class _StubPaddleOCRParser:
        pass

    deepdoc_paddleocr_module.PaddleOCRParser = _StubPaddleOCRParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.paddleocr_parser", deepdoc_paddleocr_module)
    monkeypatch.setitem(sys.modules, "xgboost", ModuleType("xgboost"))

    stub_apps = ModuleType("api.apps")
    stub_apps.current_user = SimpleNamespace(id="user-1")
    stub_apps.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", stub_apps)

    module_path = repo_root / "api" / "apps" / "document_app.py"
    spec = importlib.util.spec_from_file_location("test_document_app_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


@pytest.fixture()
def document_rest_api_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    deepdoc_pkg = ModuleType("deepdoc")
    deepdoc_parser_pkg = ModuleType("deepdoc.parser")
    deepdoc_parser_pkg.__path__ = []

    class _StubPdfParser:
        pass

    class _StubExcelParser:
        pass

    deepdoc_parser_pkg.PdfParser = _StubPdfParser
    deepdoc_pkg.parser = deepdoc_parser_pkg
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser_pkg)
    deepdoc_excel_module = ModuleType("deepdoc.parser.excel_parser")
    deepdoc_excel_module.RAGFlowExcelParser = _StubExcelParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.excel_parser", deepdoc_excel_module)
    deepdoc_html_module = ModuleType("deepdoc.parser.html_parser")

    class _StubHtmlParser:
        pass

    deepdoc_html_module.RAGFlowHtmlParser = _StubHtmlParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.html_parser", deepdoc_html_module)
    deepdoc_mineru_module = ModuleType("deepdoc.parser.mineru_parser")

    class _StubMinerUParser:
        pass

    deepdoc_mineru_module.MinerUParser = _StubMinerUParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.mineru_parser", deepdoc_mineru_module)
    deepdoc_paddleocr_module = ModuleType("deepdoc.parser.paddleocr_parser")

    class _StubPaddleOCRParser:
        pass

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
    document_api_service_mod.update_chunk_method_only = lambda *_args, **_kwargs: None
    document_api_service_mod.update_document_status_only = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.apps.services.document_api_service", document_api_service_mod)

    module_path = repo_root / "api" / "apps" / "restful_apis" / "document_api.py"
    spec = importlib.util.spec_from_file_location("test_document_api_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
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
