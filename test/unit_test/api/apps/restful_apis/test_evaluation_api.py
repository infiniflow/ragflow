#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

REPO_ROOT = Path(__file__).resolve().parents[5]


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


def _load_evaluation_api(monkeypatch):
    for name in list(sys.modules):
        if name.startswith("api.") or name.startswith("quart"):
            monkeypatch.delitem(sys.modules, name, raising=False)

    monkeypatch.setitem(sys.modules, "quart", ModuleType("quart"))
    sys.modules["quart"].request = SimpleNamespace(args={})
    sys.modules["quart"].g = SimpleNamespace()

    api_pkg = ModuleType("api")
    api_apps = ModuleType("api.apps")
    api_apps.login_required = lambda f: f
    api_apps.current_user = SimpleNamespace(id="tenant-1")
    api_pkg.apps = api_apps
    monkeypatch.setitem(sys.modules, "api", api_pkg)
    monkeypatch.setitem(sys.modules, "api.apps", api_apps)

    utils_pkg = ModuleType("api.utils")
    api_utils = ModuleType("api.utils.api_utils")

    async def get_request_json():
        return getattr(get_request_json, "payload", {})

    def get_json_result(data=None, code=0, message=True):
        return {"code": code, "message": message, "data": data}

    def get_error_argument_result(msg):
        return {"code": 101, "message": msg}

    def validate_request(*_required):
        def decorator(func):
            return func

        return decorator

    api_utils.get_request_json = get_request_json
    api_utils.get_json_result = get_json_result
    api_utils.get_error_argument_result = get_error_argument_result
    api_utils.validate_request = validate_request
    utils_pkg.api_utils = api_utils
    monkeypatch.setitem(sys.modules, "api.utils", utils_pkg)
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils)

    dialog_svc = ModuleType("api.db.services.dialog_service")
    dialog_svc.DialogService = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "api.db.services.dialog_service", dialog_svc)

    kb_svc = ModuleType("api.db.services.knowledgebase_service")
    kb_svc.KnowledgebaseService = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_svc)

    eval_svc = ModuleType("api.db.services.evaluation_service")
    eval_svc.EvaluationService = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "api.db.services.evaluation_service", eval_svc)

    constants = ModuleType("common.constants")
    constants.RetCode = SimpleNamespace(NOT_FOUND=404, SERVER_ERROR=500)
    constants.StatusEnum = SimpleNamespace(VALID=SimpleNamespace(value=1))
    monkeypatch.setitem(sys.modules, "common.constants", constants)

    module_path = REPO_ROOT / "api" / "apps" / "restful_apis" / "evaluation_api.py"
    spec = importlib.util.spec_from_file_location("evaluation_api_under_test", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module, eval_svc, kb_svc, get_request_json


@pytest.mark.asyncio
async def test_create_evaluation_dataset_success(monkeypatch):
    module, eval_svc, kb_svc, get_request_json = _load_evaluation_api(monkeypatch)

    kb_svc.KnowledgebaseService.get_by_id = staticmethod(
        lambda kb_id: (True, SimpleNamespace(tenant_id="tenant-1", status=1))
    )
    eval_svc.EvaluationService.create_dataset = staticmethod(
        lambda **kwargs: (True, "dataset-1")
    )

    get_request_json.payload = {
        "name": "Regression set",
        "description": "desc",
        "kb_ids": ["kb-1"],
    }

    res = await module.create_evaluation_dataset()
    assert res["code"] == 0
    assert res["data"]["id"] == "dataset-1"


@pytest.mark.asyncio
async def test_list_evaluation_datasets(monkeypatch):
    module, eval_svc, _, _ = _load_evaluation_api(monkeypatch)
    eval_svc.EvaluationService.list_datasets = staticmethod(
        lambda tenant_id, user_id, page, page_size: {
            "total": 1,
            "datasets": [{"id": "dataset-1", "name": "Test"}],
        }
    )

    from quart import request as quart_request

    quart_request.args = {"page": "1", "page_size": "20"}

    res = await module.list_evaluation_datasets()
    assert res["code"] == 0
    assert res["data"]["total"] == 1
