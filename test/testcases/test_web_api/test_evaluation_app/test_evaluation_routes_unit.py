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

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _Args(dict):
    def get(self, key, default=None):
        return super().get(key, default)


class _DummyRetCode:
    SUCCESS = 0
    EXCEPTION_ERROR = 100
    ARGUMENT_ERROR = 101
    DATA_ERROR = 102
    OPERATING_ERROR = 103
    AUTHENTICATION_ERROR = 109


def _run(coro):
    return asyncio.run(coro)


def _set_request_json(monkeypatch, module, payload):
    async def _request_json():
        return payload

    monkeypatch.setattr(module, "get_request_json", _request_json)


def _set_request_args(monkeypatch, module, args=None):
    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args(args or {})))


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_evaluation_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args=_Args())
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    constants_mod = ModuleType("common.constants")
    constants_mod.RetCode = _DummyRetCode
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)
    common_pkg.constants = constants_mod

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="tenant-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    evaluation_service_mod = ModuleType("api.db.services.evaluation_service")

    class _EvaluationService:
        @staticmethod
        def create_dataset(**_kwargs):
            return True, "dataset-1"

        @staticmethod
        def list_datasets(**_kwargs):
            return {"datasets": [], "total": 0}

        @staticmethod
        def get_dataset(_dataset_id):
            return {"id": _dataset_id}

        @staticmethod
        def update_dataset(_dataset_id, **_kwargs):
            return True

        @staticmethod
        def delete_dataset(_dataset_id):
            return True

        @staticmethod
        def add_test_case(**_kwargs):
            return True, "case-1"

        @staticmethod
        def import_test_cases(**_kwargs):
            return 0, 0

        @staticmethod
        def get_test_cases(_dataset_id):
            return []

        @staticmethod
        def delete_test_case(_case_id):
            return True

        @staticmethod
        def start_evaluation(**_kwargs):
            return True, "run-1"

        @staticmethod
        def get_run_results(_run_id):
            return {"id": _run_id}

        @staticmethod
        def get_recommendations(_run_id):
            return []

    evaluation_service_mod.EvaluationService = _EvaluationService
    monkeypatch.setitem(sys.modules, "api.db.services.evaluation_service", evaluation_service_mod)

    utils_pkg = ModuleType("api.utils")
    utils_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.utils", utils_pkg)

    api_utils_mod = ModuleType("api.utils.api_utils")

    async def _default_request_json():
        return {}

    def _get_data_error_result(code=_DummyRetCode.DATA_ERROR, message="Sorry! Data missing!"):
        return {"code": code, "message": message}

    def _get_json_result(code=_DummyRetCode.SUCCESS, message="success", data=None):
        return {"code": code, "message": message, "data": data}

    def _server_error_response(error):
        return {"code": _DummyRetCode.EXCEPTION_ERROR, "message": repr(error)}

    def _validate_request(*_args, **_kwargs):
        def _decorator(func):
            return func

        return _decorator

    api_utils_mod.get_data_error_result = _get_data_error_result
    api_utils_mod.get_json_result = _get_json_result
    api_utils_mod.get_request_json = _default_request_json
    api_utils_mod.server_error_response = _server_error_response
    api_utils_mod.validate_request = _validate_request
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)
    utils_pkg.api_utils = api_utils_mod

    module_name = "test_evaluation_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "evaluation_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_dataset_routes_matrix_unit(monkeypatch):
    module = _load_evaluation_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"name": "  data-1  ", "description": "desc", "kb_ids": ["kb-1"]})
    monkeypatch.setattr(module.EvaluationService, "create_dataset", lambda **_kwargs: (True, "dataset-ok"))
    res = _run(module.create_dataset())
    assert res["code"] == 0
    assert res["data"]["dataset_id"] == "dataset-ok"

    _set_request_json(monkeypatch, module, {"name": "   ", "kb_ids": ["kb-1"]})
    res = _run(module.create_dataset())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "empty" in res["message"].lower()

    _set_request_json(monkeypatch, module, {"name": "data-2", "kb_ids": "kb-1"})
    res = _run(module.create_dataset())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "kb_ids" in res["message"]

    _set_request_json(monkeypatch, module, {"name": "data-3", "kb_ids": ["kb-1"]})
    monkeypatch.setattr(module.EvaluationService, "create_dataset", lambda **_kwargs: (False, "create failed"))
    res = _run(module.create_dataset())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert res["message"] == "create failed"

    def _raise_create(**_kwargs):
        raise RuntimeError("create boom")

    monkeypatch.setattr(module.EvaluationService, "create_dataset", _raise_create)
    res = _run(module.create_dataset())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "create boom" in res["message"]

    _set_request_args(monkeypatch, module, {"page": "2", "page_size": "3"})
    monkeypatch.setattr(module.EvaluationService, "list_datasets", lambda **_kwargs: {"datasets": [{"id": "a"}], "total": 1})
    res = _run(module.list_datasets())
    assert res["code"] == 0
    assert res["data"]["total"] == 1

    _set_request_args(monkeypatch, module, {"page": "x"})
    res = _run(module.list_datasets())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR

    monkeypatch.setattr(module.EvaluationService, "get_dataset", lambda _dataset_id: None)
    res = _run(module.get_dataset("dataset-1"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "not found" in res["message"].lower()

    monkeypatch.setattr(module.EvaluationService, "get_dataset", lambda _dataset_id: {"id": _dataset_id})
    res = _run(module.get_dataset("dataset-2"))
    assert res["code"] == 0
    assert res["data"]["id"] == "dataset-2"

    def _raise_get(_dataset_id):
        raise RuntimeError("get dataset boom")

    monkeypatch.setattr(module.EvaluationService, "get_dataset", _raise_get)
    res = _run(module.get_dataset("dataset-3"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "get dataset boom" in res["message"]

    captured = {}

    def _update(dataset_id, **kwargs):
        captured["dataset_id"] = dataset_id
        captured["kwargs"] = kwargs
        return True

    _set_request_json(
        monkeypatch,
        module,
        {
            "id": "forbidden",
            "tenant_id": "forbidden",
            "created_by": "forbidden",
            "create_time": 123,
            "name": "new-name",
        },
    )
    monkeypatch.setattr(module.EvaluationService, "update_dataset", _update)
    res = _run(module.update_dataset("dataset-4"))
    assert res["code"] == 0
    assert res["data"]["dataset_id"] == "dataset-4"
    assert captured["dataset_id"] == "dataset-4"
    assert "id" not in captured["kwargs"]
    assert "tenant_id" not in captured["kwargs"]
    assert "created_by" not in captured["kwargs"]
    assert "create_time" not in captured["kwargs"]

    _set_request_json(monkeypatch, module, {"name": "new-name"})
    monkeypatch.setattr(module.EvaluationService, "update_dataset", lambda _dataset_id, **_kwargs: False)
    res = _run(module.update_dataset("dataset-5"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "failed" in res["message"].lower()

    def _raise_update(_dataset_id, **_kwargs):
        raise RuntimeError("update boom")

    monkeypatch.setattr(module.EvaluationService, "update_dataset", _raise_update)
    res = _run(module.update_dataset("dataset-6"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "update boom" in res["message"]

    monkeypatch.setattr(module.EvaluationService, "delete_dataset", lambda _dataset_id: False)
    res = _run(module.delete_dataset("dataset-7"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "failed" in res["message"].lower()

    monkeypatch.setattr(module.EvaluationService, "delete_dataset", lambda _dataset_id: True)
    res = _run(module.delete_dataset("dataset-8"))
    assert res["code"] == 0
    assert res["data"]["dataset_id"] == "dataset-8"

    def _raise_delete(_dataset_id):
        raise RuntimeError("delete dataset boom")

    monkeypatch.setattr(module.EvaluationService, "delete_dataset", _raise_delete)
    res = _run(module.delete_dataset("dataset-9"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "delete dataset boom" in res["message"]


@pytest.mark.p2
def test_test_case_routes_matrix_unit(monkeypatch):
    module = _load_evaluation_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"question": "   "})
    res = _run(module.add_test_case("dataset-1"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "question" in res["message"].lower()

    _set_request_json(monkeypatch, module, {"question": "q1"})
    monkeypatch.setattr(module.EvaluationService, "add_test_case", lambda **_kwargs: (False, "add failed"))
    res = _run(module.add_test_case("dataset-2"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "add failed" in res["message"]

    _set_request_json(
        monkeypatch,
        module,
        {
            "question": "q2",
            "reference_answer": "a2",
            "relevant_doc_ids": ["doc-1"],
            "relevant_chunk_ids": ["chunk-1"],
            "metadata": {"k": "v"},
        },
    )
    monkeypatch.setattr(module.EvaluationService, "add_test_case", lambda **_kwargs: (True, "case-ok"))
    res = _run(module.add_test_case("dataset-3"))
    assert res["code"] == 0
    assert res["data"]["case_id"] == "case-ok"

    def _raise_add(**_kwargs):
        raise RuntimeError("add case boom")

    monkeypatch.setattr(module.EvaluationService, "add_test_case", _raise_add)
    res = _run(module.add_test_case("dataset-4"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "add case boom" in res["message"]

    _set_request_json(monkeypatch, module, {"cases": {}})
    res = _run(module.import_test_cases("dataset-5"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "cases" in res["message"]

    _set_request_json(monkeypatch, module, {"cases": [{"question": "q1"}, {"question": "q2"}]})
    monkeypatch.setattr(module.EvaluationService, "import_test_cases", lambda **_kwargs: (2, 0))
    res = _run(module.import_test_cases("dataset-6"))
    assert res["code"] == 0
    assert res["data"]["success_count"] == 2
    assert res["data"]["failure_count"] == 0
    assert res["data"]["total"] == 2

    def _raise_import(**_kwargs):
        raise RuntimeError("import boom")

    monkeypatch.setattr(module.EvaluationService, "import_test_cases", _raise_import)
    res = _run(module.import_test_cases("dataset-7"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "import boom" in res["message"]

    monkeypatch.setattr(module.EvaluationService, "get_test_cases", lambda _dataset_id: [{"id": "case-1"}])
    res = _run(module.get_test_cases("dataset-8"))
    assert res["code"] == 0
    assert res["data"]["total"] == 1
    assert res["data"]["cases"][0]["id"] == "case-1"

    def _raise_get_cases(_dataset_id):
        raise RuntimeError("get cases boom")

    monkeypatch.setattr(module.EvaluationService, "get_test_cases", _raise_get_cases)
    res = _run(module.get_test_cases("dataset-9"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "get cases boom" in res["message"]

    monkeypatch.setattr(module.EvaluationService, "delete_test_case", lambda _case_id: False)
    res = _run(module.delete_test_case("case-1"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "failed" in res["message"].lower()

    monkeypatch.setattr(module.EvaluationService, "delete_test_case", lambda _case_id: True)
    res = _run(module.delete_test_case("case-2"))
    assert res["code"] == 0
    assert res["data"]["case_id"] == "case-2"

    def _raise_delete_case(_case_id):
        raise RuntimeError("delete case boom")

    monkeypatch.setattr(module.EvaluationService, "delete_test_case", _raise_delete_case)
    res = _run(module.delete_test_case("case-3"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "delete case boom" in res["message"]


@pytest.mark.p2
def test_run_and_recommendation_routes_matrix_unit(monkeypatch):
    module = _load_evaluation_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"dataset_id": "d1", "dialog_id": "dialog-1", "name": "run 1"})
    monkeypatch.setattr(module.EvaluationService, "start_evaluation", lambda **_kwargs: (False, "start failed"))
    res = _run(module.start_evaluation())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "start failed" in res["message"]

    monkeypatch.setattr(module.EvaluationService, "start_evaluation", lambda **_kwargs: (True, "run-ok"))
    res = _run(module.start_evaluation())
    assert res["code"] == 0
    assert res["data"]["run_id"] == "run-ok"

    def _raise_start(**_kwargs):
        raise RuntimeError("start boom")

    monkeypatch.setattr(module.EvaluationService, "start_evaluation", _raise_start)
    res = _run(module.start_evaluation())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "start boom" in res["message"]

    monkeypatch.setattr(module.EvaluationService, "get_run_results", lambda _run_id: None)
    res = _run(module.get_evaluation_run("run-1"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "not found" in res["message"].lower()

    monkeypatch.setattr(module.EvaluationService, "get_run_results", lambda _run_id: {"id": _run_id})
    res = _run(module.get_evaluation_run("run-2"))
    assert res["code"] == 0
    assert res["data"]["id"] == "run-2"

    def _raise_get_run(_run_id):
        raise RuntimeError("get run boom")

    monkeypatch.setattr(module.EvaluationService, "get_run_results", _raise_get_run)
    res = _run(module.get_evaluation_run("run-3"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "get run boom" in res["message"]

    monkeypatch.setattr(module.EvaluationService, "get_run_results", lambda _run_id: None)
    res = _run(module.get_run_results("run-4"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "not found" in res["message"].lower()

    monkeypatch.setattr(module.EvaluationService, "get_run_results", lambda _run_id: {"id": _run_id, "score": 0.9})
    res = _run(module.get_run_results("run-5"))
    assert res["code"] == 0
    assert res["data"]["id"] == "run-5"

    def _raise_results(_run_id):
        raise RuntimeError("get results boom")

    monkeypatch.setattr(module.EvaluationService, "get_run_results", _raise_results)
    res = _run(module.get_run_results("run-6"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "get results boom" in res["message"]

    res = _run(module.list_evaluation_runs())
    assert res["code"] == 0
    assert res["data"]["total"] == 0

    def _raise_json_list(*_args, **_kwargs):
        raise RuntimeError("list runs boom")

    monkeypatch.setattr(module, "get_json_result", _raise_json_list)
    res = _run(module.list_evaluation_runs())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "list runs boom" in res["message"]

    monkeypatch.setattr(module, "get_json_result", lambda code=0, message="success", data=None: {"code": code, "message": message, "data": data})
    res = _run(module.delete_evaluation_run("run-7"))
    assert res["code"] == 0
    assert res["data"]["run_id"] == "run-7"

    def _raise_json_delete(*_args, **_kwargs):
        raise RuntimeError("delete run boom")

    monkeypatch.setattr(module, "get_json_result", _raise_json_delete)
    res = _run(module.delete_evaluation_run("run-8"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "delete run boom" in res["message"]

    monkeypatch.setattr(module, "get_json_result", lambda code=0, message="success", data=None: {"code": code, "message": message, "data": data})
    monkeypatch.setattr(module.EvaluationService, "get_recommendations", lambda _run_id: [{"name": "cfg-1"}])
    res = _run(module.get_recommendations("run-9"))
    assert res["code"] == 0
    assert res["data"]["recommendations"][0]["name"] == "cfg-1"

    def _raise_recommend(_run_id):
        raise RuntimeError("recommend boom")

    monkeypatch.setattr(module.EvaluationService, "get_recommendations", _raise_recommend)
    res = _run(module.get_recommendations("run-10"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "recommend boom" in res["message"]


@pytest.mark.p2
def test_compare_export_and_evaluate_single_matrix_unit(monkeypatch):
    module = _load_evaluation_app(monkeypatch)

    _set_request_json(monkeypatch, module, {"run_ids": ["run-1"]})
    res = _run(module.compare_runs())
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "at least 2" in res["message"]

    _set_request_json(monkeypatch, module, {"run_ids": ["run-1", "run-2"]})
    res = _run(module.compare_runs())
    assert res["code"] == 0
    assert res["data"]["comparison"] == {}

    def _raise_json_compare(*_args, **_kwargs):
        raise RuntimeError("compare boom")

    monkeypatch.setattr(module, "get_json_result", _raise_json_compare)
    _set_request_json(monkeypatch, module, {"run_ids": ["run-1", "run-2", "run-3"]})
    res = _run(module.compare_runs())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "compare boom" in res["message"]

    monkeypatch.setattr(module, "get_json_result", lambda code=0, message="success", data=None: {"code": code, "message": message, "data": data})
    monkeypatch.setattr(module.EvaluationService, "get_run_results", lambda _run_id: None)
    res = _run(module.export_results("run-11"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "not found" in res["message"].lower()

    monkeypatch.setattr(module.EvaluationService, "get_run_results", lambda _run_id: {"id": _run_id, "rows": []})
    res = _run(module.export_results("run-12"))
    assert res["code"] == 0
    assert res["data"]["id"] == "run-12"

    def _raise_export(_run_id):
        raise RuntimeError("export boom")

    monkeypatch.setattr(module.EvaluationService, "get_run_results", _raise_export)
    res = _run(module.export_results("run-13"))
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "export boom" in res["message"]

    monkeypatch.setattr(module, "get_json_result", lambda code=0, message="success", data=None: {"code": code, "message": message, "data": data})
    res = _run(module.evaluate_single())
    assert res["code"] == 0
    assert res["data"]["answer"] == ""
    assert res["data"]["metrics"] == {}
    assert res["data"]["retrieved_chunks"] == []

    def _raise_json_single(*_args, **_kwargs):
        raise RuntimeError("single boom")

    monkeypatch.setattr(module, "get_json_result", _raise_json_single)
    res = _run(module.evaluate_single())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "single boom" in res["message"]
