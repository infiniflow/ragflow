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
from common import bulk_upload_documents, delete_document, list_documents


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func
        return decorator


@pytest.fixture(scope="function")
def add_document_func(request, WebApiAuth, add_dataset, ragflow_tmp_dir):
    def cleanup():
        res = list_documents(WebApiAuth, {"kb_id": dataset_id})
        for doc in res["data"]["docs"]:
            delete_document(WebApiAuth, {"doc_id": doc["id"]})

    request.addfinalizer(cleanup)

    dataset_id = add_dataset
    return dataset_id, bulk_upload_documents(WebApiAuth, dataset_id, 1, ragflow_tmp_dir)[0]


@pytest.fixture(scope="class")
def add_documents(request, WebApiAuth, add_dataset, ragflow_tmp_dir):
    def cleanup():
        res = list_documents(WebApiAuth, {"kb_id": dataset_id})
        for doc in res["data"]["docs"]:
            delete_document(WebApiAuth, {"doc_id": doc["id"]})

    request.addfinalizer(cleanup)

    dataset_id = add_dataset
    return dataset_id, bulk_upload_documents(WebApiAuth, dataset_id, 5, ragflow_tmp_dir)


@pytest.fixture(scope="function")
def add_documents_func(request, WebApiAuth, add_dataset_func, ragflow_tmp_dir):
    def cleanup():
        res = list_documents(WebApiAuth, {"kb_id": dataset_id})
        for doc in res["data"]["docs"]:
            delete_document(WebApiAuth, {"doc_id": doc["id"]})

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
