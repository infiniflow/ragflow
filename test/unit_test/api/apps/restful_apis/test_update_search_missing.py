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
"""Regression tests for `update` (api/apps/restful_apis/search_api.py).

The handler indexed ``SearchService.query(...)[0]`` BEFORE the empty-result
check, so a search that passes ``accessible4deletion`` (matches ``created_by``)
but is invisible to the ``tenant_id``-scoped ``query`` — e.g. one the caller
created inside another tenant's team workspace — raised
``IndexError: list index out of range`` and the blanket ``except Exception``
converted it to HTTP 500. The intended response is the guarded DATA_ERROR
"Cannot find search <id>", mirroring the detail/completion handlers.
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_search_api(monkeypatch, query_result, accessible=True, request_json=None):
    """Load search_api.py with the minimum stubs the import chain needs.

    `query_result` is what stub `SearchService.query` returns for the
    (tenant_id, id) lookup the `update` handler performs.
    """
    if request_json is None:
        request_json = {"name": "renamed", "search_config": {}}

    async def _get_request_json():
        return request_json

    _stub(monkeypatch, "api.apps", current_user=SimpleNamespace(id="tenant-1"), login_required=lambda func: func)
    _stub(monkeypatch, "api.constants", DATASET_NAME_LIMIT=255)
    _stub(monkeypatch, "api.db", DB=SimpleNamespace())
    _stub(monkeypatch, "api.db.db_models", DB=SimpleNamespace())
    _stub(monkeypatch, "api.db.services", duplicate_name=lambda *_a, **_k: "renamed")
    _stub(monkeypatch, "api.db.services.dialog_service", async_ask=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=SimpleNamespace)
    _stub(
        monkeypatch,
        "api.db.services.search_service",
        SearchService=SimpleNamespace(
            accessible4deletion=lambda *_a, **_k: accessible,
            query=lambda **_kwargs: query_result,
            update_by_id=lambda *_a, **_k: True,
            get_by_id=lambda *_a, **_k: (True, SimpleNamespace(to_dict=lambda: {"id": "sess-1", "name": "renamed"})),
        ),
    )

    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(get_by_id=lambda *_a, **_k: (True, SimpleNamespace())),
        UserTenantService=SimpleNamespace(query=lambda **_kwargs: []),
    )
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid")
    _stub(
        monkeypatch,
        "common.constants",
        RetCode=SimpleNamespace(DATA_ERROR=102, AUTHENTICATION_ERROR=401),
        StatusEnum=SimpleNamespace(VALID=SimpleNamespace(value="1")),
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        get_data_error_result=lambda message="Sorry": {"code": 102, "message": message, "data": None},
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_request_json=_get_request_json,
        server_error_response=lambda exc: {"code": 500, "message": str(exc)},
        validate_request=lambda *_a, **_k: lambda func: func,
    )
    _stub(
        monkeypatch,
        "api.utils.pagination_utils",
        DEFAULT_PAGE=1,
        DEFAULT_PAGE_SIZE=30,
        validate_rest_api_ids=lambda *_a, **_k: None,
        validate_rest_api_page=lambda *_a, **_k: None,
        validate_rest_api_page_size=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "quart", Response=SimpleNamespace, request=SimpleNamespace())

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "search_api.py"
    spec = importlib.util.spec_from_file_location("test_update_search_search_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_update_search_search_api", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p1
class TestUpdateSearchMissing:
    """PUT /api/v1/searches/<id> must answer a graceful "search not found"
    instead of HTTP 500 when the search is reachable via `created_by` but
    missing under the caller's `tenant_id` (#16011)."""

    def test_returns_data_error_when_search_missing(self, monkeypatch):
        """An empty `query` result yields the guarded DATA_ERROR, not IndexError→500."""
        module = _load_search_api(monkeypatch, query_result=[])

        result = asyncio.run(module.update(search_id="does-not-exist"))

        assert result == {
            "code": 102,
            "message": "Cannot find search does-not-exist",
            "data": False,
        }

    def test_updates_when_search_found(self, monkeypatch):
        """A found search still flows past the guard to a successful update."""
        search_app = SimpleNamespace(name="renamed", search_config={})
        module = _load_search_api(monkeypatch, query_result=[search_app])

        result = asyncio.run(module.update(search_id="sess-1"))

        assert result["code"] == 0
