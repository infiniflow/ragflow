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
"""Regression tests for the list filters of `GET /api/v1/memories`
(api/apps/restful_apis/memory_api.py).

`tenant_id` and `memory_type` are documented as accepting several values;
`owner_ids` (what the web client sends) and `ids` are undocumented filters the
route also reads. Two wire forms carry them: repeated query keys, which is what
`requests` builds when the Python SDK is handed a list, and one comma-joined
value, which is what the web client and the HTTP reference use. Both forms have
to arrive at the service intact, so these tests assert the values the service
receives and never the encoding they were sent in.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from urllib.parse import parse_qsl

import pytest
from werkzeug.datastructures import ImmutableMultiDict

from api.utils.pagination_utils import REST_API_MAX_IDS

TENANT_ID = "tenant-key-owner"


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _query_args(query_string):
    """Build `request.args` the way werkzeug does for a real request."""
    return ImmutableMultiDict(parse_qsl(query_string, keep_blank_values=True))


def _load_memory_api(monkeypatch, query_string, calls):
    """Load memory_api.py with the minimum stubs `list_memory` needs."""

    async def _list_memory(filter_params, keywords, page, page_size):
        calls.append({"filter_params": filter_params, "keywords": keywords, "page": page, "page_size": page_size})
        return {"memory_list": [], "total_count": 0}

    _stub(
        monkeypatch,
        "quart",
        g=SimpleNamespace(auth_type="API"),
        request=SimpleNamespace(args=_query_args(query_string)),
    )
    _stub(
        monkeypatch,
        "api.apps",
        AUTH_API="API",
        AUTH_JWT="JWT",
        current_user=SimpleNamespace(id=TENANT_ID),
        login_required=lambda func=None, **_kwargs: (lambda f: f) if func is None else func,
    )
    _stub(monkeypatch, "api.apps.services", memory_api_service=SimpleNamespace(list_memory=_list_memory))
    _stub(monkeypatch, "api.db.joint_services.tenant_model_service", ensure_tenant_model_ids_for_params=lambda *_a, **_k: {})
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        get_error_argument_result=lambda message="Sorry": {"code": 101, "message": message, "data": None},
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
        get_request_json=lambda: None,
        validate_request=lambda *_a, **_k: lambda func: func,
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "memory_api.py"
    spec = importlib.util.spec_from_file_location("test_list_memory_filters_memory_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_list_memory_filters_memory_api", module)
    spec.loader.exec_module(module)
    return module


async def _list(monkeypatch, query_string):
    calls = []
    module = _load_memory_api(monkeypatch, query_string, calls)
    response = await module.list_memory()
    return response, calls


@pytest.mark.p1
@pytest.mark.asyncio
async def test_repeated_memory_type_keys_all_reach_the_service(monkeypatch):
    """`rag.list_memory(memory_type=["episodic", "procedural"])` puts the list on
    the wire as `?memory_type=episodic&memory_type=procedural`. Every value of
    the repeated key reaches the service, in the order sent."""
    response, calls = await _list(monkeypatch, "memory_type=episodic&memory_type=procedural")

    assert response["code"] == 0
    assert calls[0]["filter_params"]["memory_type"] == ["episodic", "procedural"]


@pytest.mark.p1
@pytest.mark.asyncio
async def test_comma_joined_memory_type_reaches_the_service(monkeypatch):
    """One comma-joined value is split into its parts before reaching the service."""
    _, calls = await _list(monkeypatch, "memory_type=episodic%2Cprocedural")

    assert calls[0]["filter_params"]["memory_type"] == ["episodic", "procedural"]


@pytest.mark.p1
@pytest.mark.asyncio
async def test_repeated_tenant_id_keys_all_reach_the_service(monkeypatch):
    _, calls = await _list(monkeypatch, "tenant_id=t1&tenant_id=t2")

    assert calls[0]["filter_params"]["tenant_id"] == ["t1", "t2"]


@pytest.mark.p1
@pytest.mark.asyncio
async def test_comma_joined_tenant_id_reaches_the_service(monkeypatch):
    _, calls = await _list(monkeypatch, "tenant_id=t1%2Ct2")

    assert calls[0]["filter_params"]["tenant_id"] == ["t1", "t2"]


@pytest.mark.p1
@pytest.mark.asyncio
async def test_owner_ids_and_ids_reach_the_service_in_both_forms(monkeypatch):
    """`_split_filter_values` in the service reads these as lists, and
    `owner_ids` is the fallback the service uses when `tenant_id` is absent."""
    _, calls = await _list(monkeypatch, "owner_ids=o1&owner_ids=o2&ids=m1%2Cm2")

    assert calls[0]["filter_params"]["owner_ids"] == ["o1", "o2"]
    assert calls[0]["filter_params"]["ids"] == ["m1", "m2"]


@pytest.mark.p1
@pytest.mark.asyncio
async def test_id_filters_are_counted_against_the_public_maximum(monkeypatch):
    """`validate_rest_api_ids` caps both fields at REST_API_MAX_IDS. The count
    has to be taken over every value sent, in either form."""
    for field_name in ("owner_ids", "ids"):
        repeated = "&".join(f"{field_name}=x{i}" for i in range(REST_API_MAX_IDS + 1))
        response, calls = await _list(monkeypatch, repeated)
        assert response["code"] == 101, f"{field_name} repeated keys over the maximum"
        assert calls == []

        joined = f"{field_name}=" + "%2C".join(f"x{i}" for i in range(REST_API_MAX_IDS + 1))
        response, calls = await _list(monkeypatch, joined)
        assert response["code"] == 101, f"{field_name} comma-joined over the maximum"
        assert calls == []

        allowed = "&".join(f"{field_name}=x{i}" for i in range(REST_API_MAX_IDS))
        response, calls = await _list(monkeypatch, allowed)
        assert response["code"] == 0
        assert len(calls[0]["filter_params"][field_name]) == REST_API_MAX_IDS


@pytest.mark.p1
@pytest.mark.asyncio
async def test_storage_type_stays_a_single_value(monkeypatch):
    """The service assigns `storage_type` straight into the filter dict, so it
    must not become a list."""
    _, calls = await _list(monkeypatch, "storage_type=elasticsearch&memory_type=episodic")

    assert calls[0]["filter_params"]["storage_type"] == "elasticsearch"


@pytest.mark.p1
@pytest.mark.asyncio
async def test_absent_filters_are_not_invented(monkeypatch):
    """A key that was not sent stays out of the filter dict. The service treats
    a missing key and an empty list alike, so this pins the route's contract,
    not a query outcome."""
    _, calls = await _list(monkeypatch, "page=2&page_size=10")

    assert calls[0]["filter_params"] == {}
    assert calls[0]["page"] == 2
    assert calls[0]["page_size"] == 10
