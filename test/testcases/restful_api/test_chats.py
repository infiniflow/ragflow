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
from copy import deepcopy
from concurrent.futures import ThreadPoolExecutor
from enum import Enum
from functools import wraps
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

from test.testcases.configs import CHAT_ASSISTANT_NAME_LIMIT, INVALID_API_TOKEN
from test.testcases.restful_api.helpers.client import RestClient
from test.testcases.utils import encode_avatar
from test.testcases.utils.file_utils import create_image_file


DEFAULT_CHAT_EMPTY_RESPONSE = "Sorry! No relevant content was found in the knowledge base!"
DEFAULT_CHAT_PROLOGUE = "Hi! I'm your assistant. What can I do for you?"
DEFAULT_CHAT_SYSTEM_PROMPT = (
    'You are an intelligent assistant. Please summarize the content of the dataset to answer the question. '
    'Please list the data in the dataset and answer in detail. When all dataset content is irrelevant to the '
    'question, your answer must include the sentence "The answer you are looking for is not found in the dataset!" '
    "Answers need to consider chat history.\n"
    "      Here is the knowledge base:\n"
    "      {knowledge}\n"
    "      The above is the knowledge base."
)


def _get_nested(data, path):
    current = data
    for key in path:
        current = current[key]
    return current


def _chat_names(payload):
    return [chat["name"] for chat in payload["data"]["chats"]]


def _reset_chat_batch(rest_client, prefix, count=5):
    cleanup_res = rest_client.delete("/chats", json={"ids": None, "delete_all": True})
    assert cleanup_res.status_code == 200, cleanup_res.text
    cleanup_payload = cleanup_res.json()
    assert cleanup_payload["code"] in (0, 102), cleanup_payload

    ids = []
    for index in range(count):
        res = rest_client.post("/chats", json={"name": f"{prefix}_{index}", "dataset_ids": []})
        assert res.status_code == 200, (prefix, index, res.text)
        payload = res.json()
        assert payload["code"] == 0, (prefix, index, payload)
        ids.append(payload["data"]["id"])
    return ids



@pytest.mark.p1
class TestChatsAuthorization:
    def test_create_requires_auth(self, rest_client_noauth):
        res = rest_client_noauth.post("/chats", json={"name": "chat_auth", "dataset_ids": []})
        assert res.status_code == 401


@pytest.mark.p1
def test_chat_crud_cycle(rest_client, clear_chats):
    create_res = rest_client.post("/chats", json={"name": "restful_chat_crud", "dataset_ids": []})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    chat_id = create_payload["data"]["id"]

    list_res = rest_client.get("/chats", params={"id": chat_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert len(list_payload["data"]["chats"]) == 1, list_payload
    assert list_payload["data"]["chats"][0]["id"] == chat_id, list_payload

    get_res = rest_client.get(f"/chats/{chat_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["id"] == chat_id, get_payload

    update_res = rest_client.put(f"/chats/{chat_id}", json={"name": "restful_chat_crud_updated", "dataset_ids": []})
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"]["name"] == "restful_chat_crud_updated", update_payload

    patch_res = rest_client.patch(f"/chats/{chat_id}", json={"name": "restful_chat_crud_patched"})
    assert patch_res.status_code == 200
    patch_payload = patch_res.json()
    assert patch_payload["code"] == 0, patch_payload
    assert patch_payload["data"]["name"] == "restful_chat_crud_patched", patch_payload

    delete_res = rest_client.delete("/chats", json={"ids": [chat_id]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload
    assert delete_payload["data"]["success_count"] == 1, delete_payload

    list_after_delete = rest_client.get("/chats", params={"id": chat_id})
    assert list_after_delete.status_code == 200
    list_after_delete_payload = list_after_delete.json()
    assert list_after_delete_payload["code"] == 0, list_after_delete_payload
    assert list_after_delete_payload["data"]["chats"] == [], list_after_delete_payload


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, expected_fragment",
    [
        ("", "`name` is required."),
        (" ", "`name` is required."),
    ],
)
def test_chat_create_name_validation(rest_client, clear_chats, name, expected_fragment):
    res = rest_client.post("/chats", json={"name": name, "dataset_ids": []})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert expected_fragment in payload["message"], payload


@pytest.mark.p2
def test_chat_duplicate_name_validation(rest_client, clear_chats):
    first = rest_client.post("/chats", json={"name": "duplicate_chat_name", "dataset_ids": []})
    assert first.status_code == 200
    first_payload = first.json()
    assert first_payload["code"] == 0, first_payload

    second = rest_client.post("/chats", json={"name": "duplicate_chat_name", "dataset_ids": []})
    assert second.status_code == 200
    second_payload = second.json()
    assert second_payload["code"] == 102, second_payload
    assert "Duplicated chat name" in second_payload["message"], second_payload


@pytest.mark.p2
def test_chat_list_pagination(rest_client, clear_chats):
    for i in range(3):
        res = rest_client.post("/chats", json={"name": f"chat_page_{i}", "dataset_ids": []})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 0, payload

    page_res = rest_client.get("/chats", params={"page": 1, "page_size": 2, "orderby": "create_time", "desc": "true"})
    assert page_res.status_code == 200
    page_payload = page_res.json()
    assert page_payload["code"] == 0, page_payload
    assert len(page_payload["data"]["chats"]) == 2, page_payload
    assert page_payload["data"]["total"] >= 3, page_payload


@pytest.mark.p1
def test_chat_delete_requires_auth():
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.delete("/chats", json={"ids": []})
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


@pytest.mark.p2
def test_chat_delete_basic_scenarios(rest_client, clear_chats):
    existing_ids = _reset_chat_batch(rest_client, "delete_basic")
    existing_res = rest_client.delete("/chats", json={"ids": existing_ids})
    assert existing_res.status_code == 200
    existing_payload = existing_res.json()
    assert existing_payload["code"] == 0, existing_payload
    assert existing_payload["data"]["success_count"] == len(existing_ids), existing_payload

    list_after_existing = rest_client.get("/chats").json()
    assert list_after_existing["code"] == 0, list_after_existing
    assert list_after_existing["data"]["chats"] == [], list_after_existing

    empty_res = rest_client.delete("/chats", json={"ids": []})
    assert empty_res.status_code == 200
    empty_payload = empty_res.json()
    assert empty_payload["code"] == 0, empty_payload
    assert empty_payload["message"] == "success", empty_payload

    delete_all_ids = _reset_chat_batch(rest_client, "delete_all")
    delete_all_res = rest_client.delete("/chats", json={"ids": None, "delete_all": True})
    assert delete_all_res.status_code == 200
    delete_all_payload = delete_all_res.json()
    assert delete_all_payload["code"] == 0, delete_all_payload
    assert delete_all_payload["data"]["success_count"] == len(delete_all_ids), delete_all_payload

    list_after_delete_all = rest_client.get("/chats").json()
    assert list_after_delete_all["code"] == 0, list_after_delete_all
    assert list_after_delete_all["data"]["chats"] == [], list_after_delete_all


@pytest.mark.p2
def test_chat_delete_error_and_repeat_contract(rest_client, clear_chats):
    partial_cases = [
        ("partial invalid id", lambda ids: {"ids": ids + ["invalid_id"]}),
        ("partial invalid punctuation id", lambda ids: {"ids": ids + ["!@#$%^&*()"]}),
    ]
    for scenario_name, payload in partial_cases:
        ids = _reset_chat_batch(rest_client, f"delete_partial_{scenario_name.replace(' ', '_')}")
        res = rest_client.delete("/chats", json=payload(ids))
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 0, (scenario_name, body)
        assert len(body["data"]["errors"]) == 1, (scenario_name, body)
        assert body["data"]["success_count"] == 5, (scenario_name, body)

        list_payload = rest_client.get("/chats").json()
        assert list_payload["code"] == 0, (scenario_name, list_payload)
        assert list_payload["data"]["chats"] == [], (scenario_name, list_payload)

    duplicate_ids = _reset_chat_batch(rest_client, "delete_duplicate_all")
    duplicate_all_res = rest_client.delete("/chats", json={"ids": duplicate_ids + duplicate_ids})
    assert duplicate_all_res.status_code == 200
    duplicate_all_payload = duplicate_all_res.json()
    assert duplicate_all_payload["code"] == 0, duplicate_all_payload
    assert duplicate_all_payload["data"]["success_count"] == 5, duplicate_all_payload
    assert len(duplicate_all_payload["data"]["errors"]) == 5, duplicate_all_payload
    assert all(error.startswith("Duplicate chat ids: ") for error in duplicate_all_payload["data"]["errors"]), duplicate_all_payload

    duplicate_one_ids = _reset_chat_batch(rest_client, "delete_duplicate_one")
    duplicate_one_res = rest_client.delete("/chats", json={"ids": [duplicate_one_ids[0], duplicate_one_ids[0]]})
    assert duplicate_one_res.status_code == 200
    duplicate_one_payload = duplicate_one_res.json()
    assert duplicate_one_payload["code"] == 0, duplicate_one_payload
    assert duplicate_one_payload["data"]["success_count"] == 1, duplicate_one_payload
    assert duplicate_one_payload["data"]["errors"] == [f"Duplicate chat ids: {duplicate_one_ids[0]}"], duplicate_one_payload

    all_missing_res = rest_client.delete("/chats", json={"ids": ["missing-1", "missing-2"]})
    assert all_missing_res.status_code == 200
    all_missing_payload = all_missing_res.json()
    assert all_missing_payload["code"] == 102, all_missing_payload
    assert "Chat(missing-1) not found." in all_missing_payload["message"], all_missing_payload
    assert "Chat(missing-2) not found." in all_missing_payload["message"], all_missing_payload

    repeated_ids = _reset_chat_batch(rest_client, "delete_repeated")
    first_res = rest_client.delete("/chats", json={"ids": repeated_ids})
    assert first_res.status_code == 200
    first_payload = first_res.json()
    assert first_payload["code"] == 0, first_payload
    assert first_payload["data"]["success_count"] == 5, first_payload

    second_res = rest_client.delete("/chats", json={"ids": repeated_ids})
    assert second_res.status_code == 200
    second_payload = second_res.json()
    assert second_payload["code"] == 102, second_payload
    for chat_id in repeated_ids:
        assert f"Chat({chat_id}) not found." in second_payload["message"], second_payload


@pytest.mark.p2
def test_chat_delete_concurrent_and_bulk_contract(rest_client, clear_chats):
    concurrent_ids = _reset_chat_batch(rest_client, "delete_concurrent", count=20)
    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(executor.map(lambda chat_id: rest_client.delete("/chats", json={"ids": [chat_id]}).json(), concurrent_ids))
    assert len(results) == 20, results
    assert all(result["code"] == 0 for result in results), results
    assert all(result["data"]["success_count"] == 1 for result in results), results

    list_after_concurrent = rest_client.get("/chats").json()
    assert list_after_concurrent["code"] == 0, list_after_concurrent
    assert list_after_concurrent["data"]["chats"] == [], list_after_concurrent

    bulk_ids = _reset_chat_batch(rest_client, "delete_bulk", count=100)
    bulk_res = rest_client.delete("/chats", json={"ids": bulk_ids})
    assert bulk_res.status_code == 200
    bulk_payload = bulk_res.json()
    assert bulk_payload["code"] == 0, bulk_payload
    assert bulk_payload["data"]["success_count"] == len(bulk_ids), bulk_payload


@pytest.mark.p1
def test_chat_list_requires_auth():
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.get("/chats")
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


@pytest.mark.p1
def test_chat_list_default_get_and_separate_lookup_contract(rest_client, clear_chats):
    ids = _reset_chat_batch(rest_client, "list_default")

    default_res = rest_client.get("/chats")
    assert default_res.status_code == 200
    default_payload = default_res.json()
    assert default_payload["code"] == 0, default_payload
    assert len(default_payload["data"]["chats"]) == 5, default_payload
    assert default_payload["data"]["total"] == 5, default_payload

    valid_get_res = rest_client.get(f"/chats/{ids[0]}")
    assert valid_get_res.status_code == 200
    valid_get_payload = valid_get_res.json()
    assert valid_get_payload["code"] == 0, valid_get_payload
    assert valid_get_payload["data"]["id"] == ids[0], valid_get_payload

    invalid_get_res = rest_client.get("/chats/unknown")
    assert invalid_get_res.status_code == 200
    invalid_get_payload = invalid_get_res.json()
    assert invalid_get_payload["code"] == 109, invalid_get_payload
    assert invalid_get_payload["message"] == "No authorization.", invalid_get_payload

    for chat_id, keywords, expected_count in ((ids[0], "list_default_0", 1), (ids[0], "list_default_1", 1), (ids[0], "unknown", 0)):
        get_res = rest_client.get(f"/chats/{chat_id}")
        list_res = rest_client.get("/chats", params={"keywords": keywords})
        assert get_res.status_code == 200, (keywords, get_res.text)
        assert list_res.status_code == 200, (keywords, list_res.text)
        get_payload = get_res.json()
        list_payload = list_res.json()
        assert get_payload["code"] == 0, (keywords, get_payload)
        assert list_payload["code"] == 0, (keywords, list_payload)
        assert len(list_payload["data"]["chats"]) == expected_count, (keywords, list_payload)


@pytest.mark.p2
def test_chat_list_keyword_and_invalid_param_contract(rest_client, clear_chats):
    _reset_chat_batch(rest_client, "list_keyword")
    cases = [
        ("keywords none", {"keywords": None}, 5, None),
        ("keywords empty", {"keywords": ""}, 5, None),
        ("keywords exact", {"keywords": "list_keyword_1"}, 1, "list_keyword_1"),
        ("keywords unknown", {"keywords": "unknown"}, 0, None),
        ("invalid params ignored", {"a": "b"}, 5, None),
    ]

    for scenario_name, params, expected_count, expected_name in cases:
        res = rest_client.get("/chats", params=params)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 0, (scenario_name, payload)
        assert len(payload["data"]["chats"]) == expected_count, (scenario_name, payload)
        if expected_name is not None:
            assert payload["data"]["chats"][0]["name"] == expected_name, (scenario_name, payload)


@pytest.mark.p2
def test_chat_list_page_and_page_size_contract(rest_client, clear_chats):
    cases = [
        ("page none", {"page": None, "page_size": 2}, 0, lambda total: total, ""),
        ("page zero", {"page": 0, "page_size": 2}, 0, lambda total: total, ""),
        ("page two", {"page": 2, "page_size": 2}, 0, lambda total: min(max(total - 2, 0), 2), ""),
        ("page three", {"page": 3, "page_size": 2}, 0, lambda total: min(max(total - 4, 0), 2), ""),
        ("page string", {"page": "3", "page_size": 2}, 0, lambda total: min(max(total - 4, 0), 2), ""),
        ("page negative", {"page": -1, "page_size": 2}, 100, None, "ProgrammingError(1064"),
        ("page alpha", {"page": "a", "page_size": 2}, 100, None, "ValueError(\"invalid literal for int() with base 10: 'a'\")"),
        ("page_size none", {"page_size": None}, 0, lambda total: total, ""),
        ("page_size zero", {"page_size": 0}, 0, lambda total: total, ""),
        ("page_size one", {"page_size": 1}, 0, lambda total: total, ""),
        ("page_size six", {"page_size": 6}, 0, lambda total: total, ""),
        ("page_size string", {"page_size": "1"}, 0, lambda total: total, ""),
        ("page_size negative", {"page_size": -1}, 0, lambda total: total, ""),
        ("page_size alpha", {"page_size": "a"}, 100, None, "ValueError(\"invalid literal for int() with base 10: 'a'\")"),
    ]

    for scenario_name, params, expected_code, expected_count_fn, expected_message in cases:
        _reset_chat_batch(rest_client, f"list_page_{scenario_name.replace(' ', '_')}")
        baseline_payload = rest_client.get("/chats").json()
        assert baseline_payload["code"] == 0, (scenario_name, baseline_payload)
        baseline_total = baseline_payload["data"]["total"]

        res = rest_client.get("/chats", params=params)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            assert len(payload["data"]["chats"]) == expected_count_fn(baseline_total), (scenario_name, payload)
            assert payload["data"]["total"] == baseline_total, (scenario_name, payload)
        else:
            assert expected_message in payload["message"], (scenario_name, payload)


@pytest.mark.p2
def test_chat_list_sorting_contract(rest_client, clear_chats):
    _reset_chat_batch(rest_client, "list_sort")
    ascending_names = [f"list_sort_{i}" for i in range(5)]
    descending_names = list(reversed(ascending_names))
    cases = [
        ("orderby none", {"orderby": None}, 0, descending_names, ""),
        ("orderby create", {"orderby": "create_time"}, 0, descending_names, ""),
        ("orderby update", {"orderby": "update_time"}, 0, descending_names, ""),
        ("orderby name ascending", {"orderby": "name", "desc": "False"}, 0, ascending_names, ""),
        ("orderby unknown", {"orderby": "unknown"}, 100, None, "AttributeError(\"type object 'Dialog' has no attribute 'unknown'\")"),
        ("desc none", {"desc": None}, 0, descending_names, ""),
        ("desc true", {"desc": "true"}, 0, descending_names, ""),
        ("desc True", {"desc": "True"}, 0, descending_names, ""),
        ("desc bool true", {"desc": True}, 0, descending_names, ""),
        ("desc false", {"desc": "false"}, 0, ascending_names, ""),
        ("desc False", {"desc": "False"}, 0, ascending_names, ""),
        ("desc bool false", {"desc": False}, 0, ascending_names, ""),
        ("desc False update_time", {"desc": "False", "orderby": "update_time"}, 0, ascending_names, ""),
        ("desc unknown", {"desc": "unknown"}, 0, descending_names, ""),
    ]

    for scenario_name, params, expected_code, expected_names, expected_message in cases:
        res = rest_client.get("/chats", params=params)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            assert _chat_names(payload) == expected_names, (scenario_name, payload)
        else:
            assert expected_message in payload["message"], (scenario_name, payload)


@pytest.mark.p2
def test_chat_list_concurrent_and_dataset_delete_contract(rest_client, clear_chats, ensure_parsed_document):
    _reset_chat_batch(rest_client, "list_concurrent")
    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(executor.map(lambda _idx: rest_client.get("/chats").json(), range(10)))
    assert len(results) == 10, results
    assert all(result["code"] == 0 for result in results), results
    assert all(len(result["data"]["chats"]) == 5 for result in results), results

    dataset_id, _ = ensure_parsed_document()
    create_res = rest_client.post("/chats", json={"name": "list_after_dataset_delete", "dataset_ids": [dataset_id]})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload

    delete_dataset_res = rest_client.delete("/datasets", json={"ids": [dataset_id]})
    assert delete_dataset_res.status_code == 200
    delete_dataset_payload = delete_dataset_res.json()
    assert delete_dataset_payload["code"] == 0, delete_dataset_payload

    list_res = rest_client.get("/chats", params={"keywords": "list_after_dataset_delete"})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert len(list_payload["data"]["chats"]) == 1, list_payload


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _DummyArgs(dict):
    def get(self, key, default=None):
        return super().get(key, default)

    def getlist(self, key):
        value = self.get(key, [])
        if value is None:
            return []
        if isinstance(value, list):
            return value
        return [value]


class _StubHeaders:
    def __init__(self):
        self._items = []

    def add_header(self, key, value):
        self._items.append((key, value))

    def get(self, key, default=None):
        for existing_key, value in reversed(self._items):
            if existing_key == key:
                return value
        return default


class _StubResponse:
    def __init__(self, body=None, mimetype=None, content_type=None):
        self.body = body
        self.mimetype = mimetype
        self.content_type = content_type
        self.headers = _StubHeaders()


class _DummyUploadFile:
    def __init__(self, filename):
        self.filename = filename
        self.saved_path = None

    async def save(self, path):
        self.saved_path = path


def _passthrough_login_required(func):
    @wraps(func)
    async def _wrapper(*args, **kwargs):
        return await func(*args, **kwargs)

    return _wrapper


class _DummyKB:
    def __init__(self, kid="kb-1", embd_id="embd@factory", chunk_num=1, name="Dataset A", status="1"):
        self.id = kid
        self.embd_id = embd_id
        self.chunk_num = chunk_num
        self.name = name
        self.status = status


class _DummyDialogRecord:
    def __init__(self, data=None):
        self._data = data or {
            "id": "chat-1",
            "name": "chat-name",
            "description": "desc",
            "icon": "icon.png",
            "kb_ids": ["kb-1"],
            "llm_id": "glm-4",
            "llm_setting": {"temperature": 0.1},
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
                "prologue": "hello",
                "quote": True,
            },
            "similarity_threshold": 0.2,
            "vector_similarity_weight": 0.3,
            "top_n": 6,
            "top_k": 1024,
            "rerank_id": "",
            "meta_data_filter": {},
            "tenant_id": "tenant-1",
        }

    def to_dict(self):
        return deepcopy(self._data)


def _run(coro):
    return asyncio.run(coro)


async def _collect_stream(body):
    items = []
    if hasattr(body, "__aiter__"):
        async for item in body:
            if isinstance(item, bytes):
                item = item.decode("utf-8")
            items.append(item)
    else:
        for item in body:
            if isinstance(item, bytes):
                item = item.decode("utf-8")
            items.append(item)
    return items


def _load_chat_routes_unit_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[3]
    module_name = "test_chat_restful_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "chat_api.py"

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args=_DummyArgs())
    quart_mod.Response = _StubResponse
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_pkg = ModuleType("api.apps")
    apps_pkg.__path__ = [str(repo_root / "api" / "apps")]
    apps_pkg.current_user = SimpleNamespace(id="tenant-1")
    apps_pkg.login_required = _passthrough_login_required
    monkeypatch.setitem(sys.modules, "api.apps", apps_pkg)
    api_pkg.apps = apps_pkg

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    common_constants_mod = ModuleType("common.constants")

    class _StubLLMType(str, Enum):
        CHAT = "chat"
        IMAGE2TEXT = "image2text"
        RERANK = "rerank"
        SPEECH2TEXT = "speech2text"
        TTS = "tts"

    class _StubRetCode(int, Enum):
        SUCCESS = 0
        DATA_ERROR = 102
        OPERATING_ERROR = 103
        AUTHENTICATION_ERROR = 109

    class _StubStatusEnum(str, Enum):
        VALID = "1"
        INVALID = "0"

    common_constants_mod.LLMType = _StubLLMType
    common_constants_mod.RetCode = _StubRetCode
    common_constants_mod.StatusEnum = _StubStatusEnum
    from common.constants import MAXIMUM_PAGE_NUMBER as _MPN, MAXIMUM_TASK_PAGE_NUMBER as _MTPN
    common_constants_mod.MAXIMUM_PAGE_NUMBER = _MPN
    common_constants_mod.MAXIMUM_TASK_PAGE_NUMBER = _MTPN
    monkeypatch.setitem(sys.modules, "common.constants", common_constants_mod)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "generated-chat-id"

    async def _thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    misc_utils_mod.thread_pool_exec = _thread_pool_exec
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    settings_mod = ModuleType("common.settings")
    settings_mod.STORAGE_IMPL = type("_StorageImpl", (), {"rm": staticmethod(lambda *_args, **_kwargs: None)})()
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)

    dialog_service_mod = ModuleType("api.db.services.dialog_service")

    class _StubDialogService:
        model = SimpleNamespace(
            _meta=SimpleNamespace(
                fields={
                    "id": None,
                    "tenant_id": None,
                    "name": None,
                    "description": None,
                    "icon": None,
                    "kb_ids": None,
                    "llm_id": None,
                    "llm_setting": None,
                    "prompt_config": None,
                    "similarity_threshold": None,
                    "vector_similarity_weight": None,
                    "top_n": None,
                    "top_k": None,
                    "rerank_id": None,
                    "meta_data_filter": None,
                    "created_by": None,
                    "create_time": None,
                    "create_date": None,
                    "update_time": None,
                    "update_date": None,
                    "status": None,
                }
            )
        )

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def get_by_id(_chat_id):
            return False, None

        @staticmethod
        def update_by_id(_chat_id, _payload):
            return True

        @staticmethod
        def get_by_tenant_ids(*_args, **_kwargs):
            return [], 0

    dialog_service_mod.DialogService = _StubDialogService
    dialog_service_mod.async_ask = lambda *_args, **_kwargs: None
    dialog_service_mod.async_chat = lambda *_args, **_kwargs: None
    dialog_service_mod.gen_mindmap = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.db.services.dialog_service", dialog_service_mod)

    conversation_service_mod = ModuleType("api.db.services.conversation_service")

    class _StubConversationService:
        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_list(*_args, **_kwargs):
            return []

        @staticmethod
        def get_by_id(_session_id):
            return False, None

        @staticmethod
        def update_by_id(_session_id, _payload):
            return True

        @staticmethod
        def delete_by_id(_session_id):
            return True

        @staticmethod
        def save(**_kwargs):
            return True

    conversation_service_mod.ConversationService = _StubConversationService
    conversation_service_mod.structure_answer = lambda *_args, **_kwargs: {}
    monkeypatch.setitem(sys.modules, "api.db.services.conversation_service", conversation_service_mod)

    kb_service_mod = ModuleType("api.db.services.knowledgebase_service")

    class _StubKnowledgebaseService:
        @staticmethod
        def accessible(**_kwargs):
            return []

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_by_id(_kb_id):
            return False, None

    kb_service_mod.KnowledgebaseService = _StubKnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_service_mod)

    llm_service_mod = ModuleType("api.db.services.llm_service")
    llm_service_mod.LLMBundle = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", llm_service_mod)

    search_service_mod = ModuleType("api.db.services.search_service")
    search_service_mod.SearchService = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "api.db.services.search_service", search_service_mod)

    tenant_model_service_mod = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_service_mod.get_model_config_from_provider_instance = lambda *_args, **_kwargs: {}
    tenant_model_service_mod.get_tenant_default_model_by_type = lambda *_args, **_kwargs: {}
    tenant_model_service_mod.get_api_key = lambda *_args, **_kwargs: SimpleNamespace(id=1)
    tenant_model_service_mod.split_model_name = lambda model: (model.split("@")[0],"default", "factory")
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")

    class _StubTenantService:
        @staticmethod
        def get_by_id(_tenant_id):
            return True, SimpleNamespace(llm_id="glm-4")

        @staticmethod
        def get_joined_tenants_by_user_id(_user_id):
            return [{"tenant_id": "tenant-1"}, {"tenant_id": "team-tenant-2"}]

    class _StubUserTenantService:
        @staticmethod
        def query(**_kwargs):
            return []

    user_service_mod.UserService = type("UserService", (), {})
    user_service_mod.TenantService = _StubTenantService
    user_service_mod.UserTenantService = _StubUserTenantService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    chunk_feedback_service_mod = ModuleType("api.db.services.chunk_feedback_service")
    chunk_feedback_service_mod.ChunkFeedbackService = type(
        "ChunkFeedbackService",
        (),
        {"apply_feedback": staticmethod(lambda **_kwargs: {"success_count": 0, "fail_count": 0, "chunk_ids": []})},
    )
    monkeypatch.setitem(sys.modules, "api.db.services.chunk_feedback_service", chunk_feedback_service_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    def _check_duplicate_ids(ids, label):
        counts = {}
        for item in ids or []:
            counts[item] = counts.get(item, 0) + 1
        duplicate_messages = [f"Duplicate {label} ids: {item}" for item, count in counts.items() if count > 1]
        return list(dict.fromkeys(ids or [])), duplicate_messages

    api_utils_mod.check_duplicate_ids = _check_duplicate_ids
    api_utils_mod.get_data_error_result = lambda message="": {"code": 102, "data": None, "message": message}
    api_utils_mod.get_json_result = lambda data=None, message="", code=0: {"code": code, "data": data, "message": message}
    api_utils_mod.get_request_json = lambda: _AwaitableValue({})
    api_utils_mod.server_error_response = lambda ex: {"code": 500, "data": None, "message": str(ex)}
    api_utils_mod.validate_request = lambda *_args, **_kwargs: (lambda func: func)
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = [str(repo_root / "rag")]
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_prompts_pkg = ModuleType("rag.prompts")
    rag_prompts_pkg.__path__ = [str(repo_root / "rag" / "prompts")]
    monkeypatch.setitem(sys.modules, "rag.prompts", rag_prompts_pkg)

    rag_prompts_generator_mod = ModuleType("rag.prompts.generator")
    rag_prompts_generator_mod.chunks_format = lambda reference: reference.get("chunks", []) if isinstance(reference, dict) else []
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", rag_prompts_generator_mod)

    rag_prompts_template_mod = ModuleType("rag.prompts.template")
    rag_prompts_template_mod.load_prompt = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "rag.prompts.template", rag_prompts_template_mod)

    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _set_route_unit_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(deepcopy(payload)))


@pytest.mark.p2
def test_chat_session_create_and_update_guard_matrix_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)

    _set_route_unit_request_json(monkeypatch, module, {"name": "session"})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(module.create_session.__wrapped__("chat-1"))
    assert res["message"] == "No authorization."

    dia = SimpleNamespace(prompt_config={"prologue": "hello"})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [dia])
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, dia))
    monkeypatch.setattr(module.ConversationService, "save", lambda **_kwargs: None)
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (False, None))
    res = _run(module.create_session.__wrapped__("chat-1"))
    assert "Fail to create a session" in res["message"]

    _set_route_unit_request_json(monkeypatch, module, {})
    monkeypatch.setattr(module.ConversationService, "query", lambda **_kwargs: [])
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert res["message"] == "Session not found!"

    monkeypatch.setattr(module.ConversationService, "query", lambda **_kwargs: [SimpleNamespace(id="session-1")])
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert res["message"] == "No authorization."

    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    _set_route_unit_request_json(monkeypatch, module, {"message": []})
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert "`messages` cannot be changed." in res["message"]

    _set_route_unit_request_json(monkeypatch, module, {"reference": []})
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert "`reference` cannot be changed." in res["message"]

    _set_route_unit_request_json(monkeypatch, module, {"name": ""})
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert "`name` can not be empty." in res["message"]

    _set_route_unit_request_json(monkeypatch, module, {"name": "renamed"})
    monkeypatch.setattr(module.ConversationService, "update_by_id", lambda *_args, **_kwargs: False)
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert res["message"] == "Session not found!"


@pytest.mark.p2
def test_chat_session_list_projection_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "page": 1,
                    "page_size": 30,
                    "orderby": "create_time",
                    "desc": "true",
                    "id": None,
                    "name": None,
                    "user_id": None,
                }.get(key, default)
            )
        ),
    )
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(
        module.ConversationService,
        "get_list",
        lambda *_args, **_kwargs: [
            {
                "id": "session-1",
                "dialog_id": "chat-1",
                "message": [{"role": "assistant", "content": "hello"}],
                "reference": [],
            }
        ],
    )

    res = _run(module.list_sessions.__wrapped__("chat-1"))
    assert res["data"][0]["chat_id"] == "chat-1"
    assert res["data"][0]["messages"][0]["content"] == "hello"

    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "page": 1,
                    "page_size": 0,
                    "orderby": "create_time",
                    "desc": "true",
                    "id": None,
                    "name": None,
                    "user_id": None,
                }.get(key, default)
            )
        ),
    )
    res = _run(module.list_sessions.__wrapped__("chat-1"))
    assert res["data"] == []


@pytest.mark.p2
def test_chat_session_delete_routes_partial_duplicate_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    _set_route_unit_request_json(monkeypatch, module, {})
    res = _run(module.delete_sessions.__wrapped__("chat-1"))
    assert res["code"] == 0

    monkeypatch.setattr(module.ConversationService, "delete_by_id", lambda *_args, **_kwargs: True)

    def _conversation_query(**kwargs):
        if "dialog_id" in kwargs and "id" not in kwargs:
            return [SimpleNamespace(id="seed")]
        if kwargs.get("id") == "ok":
            return [SimpleNamespace(id="ok")]
        return []

    monkeypatch.setattr(module.ConversationService, "query", _conversation_query)
    _set_route_unit_request_json(monkeypatch, module, {"ids": ["ok", "bad"]})
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
    res = _run(module.delete_sessions.__wrapped__("chat-1"))
    assert res["code"] == 0
    assert res["data"]["success_count"] == 1
    assert res["data"]["errors"] == ["The chat doesn't own the session bad"]

    _set_route_unit_request_json(monkeypatch, module, {"ids": ["bad"]})
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
    res = _run(module.delete_sessions.__wrapped__("chat-1"))
    assert res["message"] == "The chat doesn't own the session bad"

    _set_route_unit_request_json(monkeypatch, module, {"ids": ["ok", "ok"]})
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (["ok"], ["Duplicate session ids: ok"]))
    res = _run(module.delete_sessions.__wrapped__("chat-1"))
    assert res["code"] == 0
    assert res["data"]["success_count"] == 1
    assert res["data"]["errors"] == ["Duplicate session ids: ok"]


@pytest.mark.p2
def test_chat_audio_transcription_routes_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    monkeypatch.setattr(module, "Response", _StubResponse)
    monkeypatch.setattr(module.tempfile, "mkstemp", lambda suffix: (11, f"/tmp/audio{suffix}"))
    monkeypatch.setattr(module.os, "close", lambda _fd: None)

    def _set_request(form, files):
        monkeypatch.setattr(module, "request", SimpleNamespace(form=_AwaitableValue(form), files=_AwaitableValue(files)))

    _set_request({"stream": "false"}, {})
    res = _run(module.transcription.__wrapped__())
    assert "Missing 'file' in multipart form-data" in res["message"]

    _set_request({"stream": "false"}, {"file": _DummyUploadFile("bad.txt")})
    res = _run(module.transcription.__wrapped__())
    assert "Unsupported audio format: .txt" in res["message"]

    _set_request({"stream": "false"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(
        module,
        "get_tenant_default_model_by_type",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(LookupError("Tenant not found!")),
    )
    res = _run(module.transcription.__wrapped__())
    assert res["message"] == "Tenant not found!"

    _set_request({"stream": "false"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(
        module,
        "get_tenant_default_model_by_type",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(Exception("No default ASR model is set")),
    )
    res = _run(module.transcription.__wrapped__())
    assert res["message"] == "No default ASR model is set"

    class _SyncASR:
        def transcription(self, _path):
            return "transcribed text"

        def stream_transcription(self, _path):
            return []

    _set_request({"stream": "false"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(module, "get_tenant_default_model_by_type", lambda *_args, **_kwargs: {"llm_name": "asr-x"})
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _SyncASR())
    monkeypatch.setattr(module.os, "remove", lambda _path: (_ for _ in ()).throw(RuntimeError("cleanup fail")))
    res = _run(module.transcription.__wrapped__())
    assert res["code"] == 0
    assert res["data"]["text"] == "transcribed text"

    class _StreamASR:
        def transcription(self, _path):
            return ""

        def stream_transcription(self, _path):
            yield {"event": "partial", "text": "hello"}

    _set_request({"stream": "true"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _StreamASR())
    monkeypatch.setattr(module.os, "remove", lambda _path: None)
    resp = _run(module.transcription.__wrapped__())
    assert isinstance(resp, _StubResponse)
    assert resp.content_type == "text/event-stream"
    chunks = _run(_collect_stream(resp.body))
    assert any('"event": "partial"' in chunk for chunk in chunks)

    class _ErrorASR:
        def transcription(self, _path):
            return ""

        def stream_transcription(self, _path):
            raise RuntimeError("stream asr boom")

    _set_request({"stream": "true"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _ErrorASR())
    monkeypatch.setattr(module.os, "remove", lambda _path: (_ for _ in ()).throw(RuntimeError("cleanup boom")))
    resp = _run(module.transcription.__wrapped__())
    chunks = _run(_collect_stream(resp.body))
    assert any("stream asr boom" in chunk for chunk in chunks)


@pytest.mark.p2
def test_chat_audio_speech_routes_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    monkeypatch.setattr(module, "Response", _StubResponse)
    _set_route_unit_request_json(monkeypatch, module, {"text": "A。B"})

    monkeypatch.setattr(
        module,
        "get_tenant_default_model_by_type",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(LookupError("Tenant not found!")),
    )
    res = _run(module.tts.__wrapped__())
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(
        module,
        "get_tenant_default_model_by_type",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(Exception("No default TTS model is set")),
    )
    res = _run(module.tts.__wrapped__())
    assert res["message"] == "No default TTS model is set"

    class _TTSOk:
        def tts(self, txt):
            if not txt:
                return []
            yield f"chunk-{txt}".encode("utf-8")

    monkeypatch.setattr(module, "get_tenant_default_model_by_type", lambda *_args, **_kwargs: {"llm_name": "tts-x"})
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _TTSOk())
    resp = _run(module.tts.__wrapped__())
    assert resp.mimetype == "audio/mpeg"
    assert resp.headers.get("Cache-Control") == "no-cache"
    assert resp.headers.get("Connection") == "keep-alive"
    assert resp.headers.get("X-Accel-Buffering") == "no"
    chunks = _run(_collect_stream(resp.body))
    assert any("chunk-A" in chunk for chunk in chunks)
    assert any("chunk-B" in chunk for chunk in chunks)

    class _TTSErr:
        def tts(self, _txt):
            raise RuntimeError("tts boom")

    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _TTSErr())
    resp = _run(module.tts.__wrapped__())
    chunks = _run(_collect_stream(resp.body))
    assert any('"code": 500' in chunk and "**ERROR**: tts boom" in chunk for chunk in chunks)


@pytest.mark.p1
def test_chat_create_accepts_provider_scoped_rerank_id_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    saved = {}
    query_calls = []

    _set_route_unit_request_json(
        monkeypatch,
        module,
        {
            "name": "chat-a",
            "icon": "icon.png",
            "dataset_ids": ["kb-1"],
            "llm_id": "glm-4@@CI@ZHIPU-AI",
            "llm_setting": {"temperature": 0.8},
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
                "prologue": "Hi",
            },
            "rerank_id": "custom-reranker@OpenAI",
            "vector_similarity_weight": 0.25,
        },
    )
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4@CI@ZHIPU-AI")))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB()])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, _DummyKB()))

    def _split_model_name_and_factory(model_name):
        return {
            "glm-4@ZHIPU-AI": ("glm-4", "default", "ZHIPU-AI"),
            "glm-4@CI@ZHIPU-AI": ("glm-4", "CI", "ZHIPU-AI"),
            "custom-reranker@OpenAI": ("custom-reranker", "default", "OpenAI")
        }.get(model_name, (model_name, None))

    monkeypatch.setattr(module, "split_model_name", _split_model_name_and_factory)

    def _get_model_config_from_provider_instance(**kwargs):
        query_calls.append(kwargs)
        return {}

    monkeypatch.setattr(module, "get_model_config_from_provider_instance", _get_model_config_from_provider_instance)

    def _save(**kwargs):
        saved.update(kwargs)
        return True

    monkeypatch.setattr(module.DialogService, "save", _save)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(saved)))

    res = _run(module.create.__wrapped__())
    assert res["code"] == 0
    assert saved["rerank_id"] == "custom-reranker@OpenAI"
    assert {
        "tenant_id": "tenant-1",
        "model_name": "custom-reranker@OpenAI",
        "model_type": "rerank",
    } in query_calls


@pytest.mark.p1
def test_chat_create_allows_default_knowledge_placeholder_without_sources_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    saved = {}
    _set_route_unit_request_json(monkeypatch, module, {"name": "chat-a"})
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module, "get_api_key", lambda *_args, **_kwargs: SimpleNamespace(id=1))

    def _save(**kwargs):
        saved.update(kwargs)
        return True

    monkeypatch.setattr(module.DialogService, "save", _save)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(saved)))

    res = _run(module.create.__wrapped__())
    assert res["code"] == 0
    assert saved["kb_ids"] == []
    assert saved["prompt_config"]["system"].find("{knowledge}") >= 0
    assert saved["prompt_config"]["parameters"] == [{"key": "knowledge", "optional": False}]


@pytest.mark.p2
def test_chat_create_uses_direct_chat_fields_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    saved = {}
    _set_route_unit_request_json(
        monkeypatch,
        module,
        {
            "name": "chat-a",
            "icon": "icon.png",
            "dataset_ids": ["kb-1"],
            "llm_id": "glm-4",
            "llm_setting": {"temperature": 0.8},
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
                "prologue": "Hi",
            },
            "vector_similarity_weight": 0.25,
        },
    )
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB()])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, _DummyKB()))
    monkeypatch.setattr(module, "split_model_name", lambda model: (model.split("@")[0],"default", "factory"))

    def _save(**kwargs):
        saved.update(kwargs)
        return True

    monkeypatch.setattr(module.DialogService, "save", _save)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(saved)))

    res = _run(module.create.__wrapped__())
    assert res["code"] == 0
    assert saved["kb_ids"] == ["kb-1"]
    assert saved["prompt_config"]["prologue"] == "Hi"
    assert saved["llm_id"] == "glm-4"
    assert saved["llm_setting"]["temperature"] == 0.8
    assert res["data"]["dataset_ids"] == ["kb-1"]
    assert res["data"]["kb_names"] == ["Dataset A"]
    assert "kb_ids" not in res["data"]
    assert "prompt" not in res["data"]
    assert "llm" not in res["data"]
    assert "avatar" not in res["data"]


@pytest.mark.p2
def test_list_chats_defaults_to_authorized_owner_ids_when_omitted_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    captured = {}
    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "keywords": "",
                    "page": "1",
                    "page_size": "10",
                    "orderby": "create_time",
                    "desc": "true",
                    "id": None,
                    "name": None,
                }.get(key, default),
                getlist=lambda _key: [],
            )
        ),
    )

    def _get_by_tenant_ids(owner_ids, *_args, **_kwargs):
        captured["owner_ids"] = owner_ids
        return ([], 0)

    monkeypatch.setattr(module.DialogService, "get_by_tenant_ids", _get_by_tenant_ids)
    res = _run(module.list_chats.__wrapped__())
    assert res["code"] == 0
    assert set(captured["owner_ids"]) == {"tenant-1", "team-tenant-2"}


@pytest.mark.p2
def test_list_chats_rejects_unauthorized_owner_ids_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "keywords": "",
                    "page": "0",
                    "page_size": "0",
                    "orderby": "create_time",
                    "desc": "true",
                    "id": None,
                    "name": None,
                }.get(key, default),
                getlist=lambda key: ["foreign-tenant-id"] if key == "owner_ids" else [],
            )
        ),
    )
    res = _run(module.list_chats.__wrapped__())
    assert res["code"] == module.RetCode.OPERATING_ERROR
    assert "authorized owner_ids" in res["message"]


@pytest.mark.p2
def test_list_chats_returns_old_business_fields_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "keywords": "",
                    "page": 1,
                    "page_size": 20,
                    "orderby": "create_time",
                    "desc": "true",
                }.get(key, default),
                getlist=lambda _key: [],
            )
        ),
    )
    monkeypatch.setattr(module.DialogService, "get_by_tenant_ids", lambda *_args, **_kwargs: ([_DummyDialogRecord().to_dict()], 1))
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, _DummyKB()))

    res = _run(module.list_chats.__wrapped__())
    assert res["code"] == 0
    chat = res["data"]["chats"][0]
    assert chat["icon"] == "icon.png"
    assert chat["dataset_ids"] == ["kb-1"]
    assert chat["kb_names"] == ["Dataset A"]
    assert "kb_ids" not in chat
    assert chat["prompt_config"]["prologue"] == "hello"
    assert "dataset_names" not in chat
    assert "prompt" not in chat
    assert "llm" not in chat


@pytest.mark.p2
def test_patch_chat_drops_response_only_fields_before_update_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    updated = {}
    existing = _DummyDialogRecord().to_dict()
    payload = {
        "name": "renamed-chat",
        "description": existing["description"],
        "icon": existing["icon"],
        "dataset_ids": existing["kb_ids"],
        "kb_names": ["Dataset A"],
        "llm_id": existing["llm_id"],
        "llm_setting": existing["llm_setting"],
        "prompt_config": existing["prompt_config"],
        "similarity_threshold": existing["similarity_threshold"],
        "vector_similarity_weight": existing["vector_similarity_weight"],
        "top_n": existing["top_n"],
        "top_k": existing["top_k"],
        "rerank_id": existing["rerank_id"],
    }

    _set_route_unit_request_json(monkeypatch, module, payload)
    monkeypatch.setattr(module.DialogService, "query", lambda **kwargs: [] if "name" in kwargs else [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(existing)))
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB()])
    monkeypatch.setattr(module, "split_model_name", lambda model: (model.split("@")[0],"default", "factory"))
    monkeypatch.setattr(module, "get_api_key", lambda *args, **kwargs: SimpleNamespace(id=1))

    def _update(_chat_id, req):
        updated.update(req)
        return True

    monkeypatch.setattr(module.DialogService, "update_by_id", _update)
    res = _run(module.patch_chat.__wrapped__("chat-1"))
    assert res["code"] == 0
    assert updated["name"] == "renamed-chat"
    assert "kb_names" not in updated


@pytest.mark.p2
def test_patch_chat_merges_prompt_and_llm_settings_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    updated = {}
    existing = _DummyDialogRecord().to_dict()
    _set_route_unit_request_json(
        monkeypatch,
        module,
        {"prompt_config": {"prologue": "updated opener"}, "llm_setting": {"temperature": 0.9}},
    )
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(existing)))
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))

    def _update(_chat_id, payload):
        updated.update(payload)
        return True

    monkeypatch.setattr(module.DialogService, "update_by_id", _update)
    res = _run(module.patch_chat.__wrapped__("chat-1"))
    assert res["code"] == 0
    assert updated["prompt_config"]["system"] == "Answer with {knowledge}"
    assert updated["prompt_config"]["prologue"] == "updated opener"
    assert updated["llm_setting"]["temperature"] == 0.9


@pytest.mark.p2
def test_update_chat_allows_knowledge_placeholder_without_sources_unit(monkeypatch):
    module = _load_chat_routes_unit_module(monkeypatch)
    existing = _DummyDialogRecord().to_dict()
    _set_route_unit_request_json(
        monkeypatch,
        module,
        {
            "name": "chat-name",
            "description": "desc",
            "icon": "icon.png",
            "dataset_ids": [],
            "llm_id": "glm-4",
            "llm_setting": {"temperature": 0.1},
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
                "prologue": "hello",
                "quote": True,
            },
            "similarity_threshold": 0.2,
            "vector_similarity_weight": 0.3,
            "top_n": 6,
            "top_k": 1024,
            "rerank_id": "",
        },
    )
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(existing)))
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module, "split_model_name", lambda model: (model.split("@")[0], "default", "factory"))
    updated = {}

    def _update(_chat_id, payload):
        updated.update(payload)
        return True

    monkeypatch.setattr(module.DialogService, "update_by_id", _update)
    res = _run(module.update_chat.__wrapped__("chat-1"))
    assert res["code"] == 0
    assert updated["prompt_config"]["system"] == "Answer with {knowledge}"


@pytest.mark.p1
def test_chat_create_dataset_ids_contract(rest_client, clear_chats, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    cases = [
        ("empty dataset_ids", [], 0, "", []),
        ("owned parsed dataset", [dataset_id], 0, "", [dataset_id]),
        ("invalid dataset id", ["invalid_dataset_id"], 102, "You don't own the dataset invalid_dataset_id", None),
        ("dataset_ids wrong type", "invalid_dataset_id", 102, "`dataset_ids` should be a list.", None),
    ]

    for index, (scenario_name, dataset_ids, expected_code, expected_message, expected_dataset_ids) in enumerate(cases, start=1):
        res = rest_client.post(
            "/chats",
            json={"name": f"restful_chat_dataset_ids_{index}", "dataset_ids": dataset_ids},
        )
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            assert payload["data"]["dataset_ids"] == expected_dataset_ids, (scenario_name, payload)
        else:
            assert payload["message"] == expected_message, (scenario_name, payload)


@pytest.mark.p2
def test_chat_create_avatar_contract(rest_client, clear_chats, tmp_path):
    image_path = create_image_file(tmp_path / "restful_chat_avatar.png")
    encoded_avatar = encode_avatar(image_path)

    res = rest_client.post(
        "/chats",
        json={"name": "restful_chat_avatar", "dataset_ids": [], "icon": encoded_avatar},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"]["icon"] == encoded_avatar, payload


@pytest.mark.p2
def test_chat_create_llm_contract(rest_client, clear_chats, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    cases = [
        ("default llm", {}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {}),
        ("explicit llm_id", {"llm_id": "glm-4"}, 102, "`llm_id` glm-4 doesn't exist", None, None),
        ("unknown llm_id", {"llm_id": "unknown"}, 102, "`llm_id` unknown doesn't exist", None, None),
        ("temperature zero", {"llm_setting": {"temperature": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": 0}),
        ("temperature one", {"llm_setting": {"temperature": 1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": 1}),
        ("temperature negative one", {"llm_setting": {"temperature": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": -1}),
        ("temperature ten", {"llm_setting": {"temperature": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": 10}),
        ("temperature string", {"llm_setting": {"temperature": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": "a"}),
        ("top_p zero", {"llm_setting": {"top_p": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": 0}),
        ("top_p one", {"llm_setting": {"top_p": 1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": 1}),
        ("top_p negative one", {"llm_setting": {"top_p": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": -1}),
        ("top_p ten", {"llm_setting": {"top_p": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": 10}),
        ("top_p string", {"llm_setting": {"top_p": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": "a"}),
        ("presence_penalty zero", {"llm_setting": {"presence_penalty": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": 0}),
        ("presence_penalty one", {"llm_setting": {"presence_penalty": 1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": 1}),
        ("presence_penalty negative one", {"llm_setting": {"presence_penalty": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": -1}),
        ("presence_penalty ten", {"llm_setting": {"presence_penalty": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": 10}),
        ("presence_penalty string", {"llm_setting": {"presence_penalty": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": "a"}),
        ("frequency_penalty zero", {"llm_setting": {"frequency_penalty": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": 0}),
        ("frequency_penalty one", {"llm_setting": {"frequency_penalty": 1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": 1}),
        ("frequency_penalty negative one", {"llm_setting": {"frequency_penalty": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": -1}),
        ("frequency_penalty ten", {"llm_setting": {"frequency_penalty": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": 10}),
        ("frequency_penalty string", {"llm_setting": {"frequency_penalty": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": "a"}),
        ("max_token zero", {"llm_setting": {"max_token": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": 0}),
        ("max_token 1024", {"llm_setting": {"max_token": 1024}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": 1024}),
        ("max_token negative one", {"llm_setting": {"max_token": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": -1}),
        ("max_token ten", {"llm_setting": {"max_token": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": 10}),
        ("max_token string", {"llm_setting": {"max_token": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": "a"}),
        ("unknown llm setting key", {"llm_setting": {"unknown": "unknown"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"unknown": "unknown"}),
    ]

    for index, (scenario_name, extra_payload, expected_code, expected_message, expected_llm_id, expected_llm_setting) in enumerate(cases, start=1):
        payload = {
            "name": f"restful_chat_llm_{index}",
            "dataset_ids": [dataset_id],
        }
        payload.update(extra_payload)
        res = rest_client.post("/chats", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code == 0:
            assert body["data"]["llm_id"] == expected_llm_id, (scenario_name, body)
            assert body["data"]["llm_setting"] == expected_llm_setting, (scenario_name, body)
        else:
            assert body["message"] == expected_message, (scenario_name, body)


@pytest.mark.p2
def test_chat_create_prompt_contract(rest_client, clear_chats):
    cases = [
        (
            "default prompt config",
            {},
            {
                ("similarity_threshold",): 0.1,
                ("vector_similarity_weight",): 0.3,
                ("top_n",): 6,
                ("rerank_id",): "",
                ("prompt_config", "parameters"): [{"key": "knowledge", "optional": False}],
                ("prompt_config", "empty_response"): DEFAULT_CHAT_EMPTY_RESPONSE,
                ("prompt_config", "prologue"): DEFAULT_CHAT_PROLOGUE,
                ("prompt_config", "quote"): True,
                ("prompt_config", "system"): DEFAULT_CHAT_SYSTEM_PROMPT,
            },
        ),
        ("similarity_threshold zero", {"similarity_threshold": 0}, {("similarity_threshold",): 0}),
        ("similarity_threshold one", {"similarity_threshold": 1}, {("similarity_threshold",): 1}),
        ("similarity_threshold negative one", {"similarity_threshold": -1}, {("similarity_threshold",): -1.0}),
        ("similarity_threshold ten", {"similarity_threshold": 10}, {("similarity_threshold",): 10.0}),
        ("similarity_threshold string", {"similarity_threshold": "a"}, {("similarity_threshold",): 0.0}),
        ("vector_similarity_weight one", {"vector_similarity_weight": 1}, {("vector_similarity_weight",): 1}),
        ("vector_similarity_weight zero", {"vector_similarity_weight": 0}, {("vector_similarity_weight",): 0}),
        ("vector_similarity_weight two", {"vector_similarity_weight": 2}, {("vector_similarity_weight",): 2.0}),
        ("vector_similarity_weight negative nine", {"vector_similarity_weight": -9}, {("vector_similarity_weight",): -9.0}),
        ("vector_similarity_weight string", {"vector_similarity_weight": "a"}, {("vector_similarity_weight",): 0.0}),
        ("empty prompt parameters", {"prompt_config": {"parameters": []}}, {("prompt_config", "parameters"): []}),
        ("top_n zero", {"top_n": 0}, {("top_n",): 0}),
        ("top_n one", {"top_n": 1}, {("top_n",): 1}),
        ("top_n negative one", {"top_n": -1}, {("top_n",): -1}),
        ("top_n ten", {"top_n": 10}, {("top_n",): 10}),
        ("top_n string", {"top_n": "a"}, {("top_n",): 0}),
        ("empty_response plain text", {"prompt_config": {"empty_response": "Hello World"}}, {("prompt_config", "empty_response"): "Hello World"}),
        ("empty_response empty string", {"prompt_config": {"empty_response": ""}}, {("prompt_config", "empty_response"): ""}),
        ("empty_response punctuation", {"prompt_config": {"empty_response": "!@#$%^&*()"}}, {("prompt_config", "empty_response"): "!@#$%^&*()"}),
        ("empty_response chinese text", {"prompt_config": {"empty_response": "中文测试"}}, {("prompt_config", "empty_response"): "中文测试"}),
        ("empty_response integer", {"prompt_config": {"empty_response": 123}}, {("prompt_config", "empty_response"): 123}),
        ("empty_response boolean", {"prompt_config": {"empty_response": True}}, {("prompt_config", "empty_response"): True}),
        ("empty_response space", {"prompt_config": {"empty_response": " "}}, {("prompt_config", "empty_response"): " "}),
        ("prologue plain text", {"prompt_config": {"prologue": "Hello World"}}, {("prompt_config", "prologue"): "Hello World"}),
        ("prologue empty string", {"prompt_config": {"prologue": ""}}, {("prompt_config", "prologue"): ""}),
        ("prologue punctuation", {"prompt_config": {"prologue": "!@#$%^&*()"}}, {("prompt_config", "prologue"): "!@#$%^&*()"}),
        ("prologue chinese text", {"prompt_config": {"prologue": "中文测试"}}, {("prompt_config", "prologue"): "中文测试"}),
        ("prologue integer", {"prompt_config": {"prologue": 123}}, {("prompt_config", "prologue"): 123}),
        ("prologue boolean", {"prompt_config": {"prologue": True}}, {("prompt_config", "prologue"): True}),
        ("prologue space", {"prompt_config": {"prologue": " "}}, {("prompt_config", "prologue"): " "}),
        ("quote true", {"prompt_config": {"quote": True}}, {("prompt_config", "quote"): True}),
        ("quote false", {"prompt_config": {"quote": False}}, {("prompt_config", "quote"): False}),
        ("system prompt with knowledge prefix", {"prompt_config": {"system": "Hello World {knowledge}"}}, {("prompt_config", "system"): "Hello World {knowledge}"}),
        ("system prompt only knowledge", {"prompt_config": {"system": "{knowledge}"}}, {("prompt_config", "system"): "{knowledge}"}),
        ("system prompt punctuation", {"prompt_config": {"system": "!@#$%^&*() {knowledge}"}}, {("prompt_config", "system"): "!@#$%^&*() {knowledge}"}),
        ("system prompt chinese text", {"prompt_config": {"system": "中文测试 {knowledge}"}}, {("prompt_config", "system"): "中文测试 {knowledge}"}),
        ("system prompt plain text", {"prompt_config": {"system": "Hello World"}}, {("prompt_config", "system"): "Hello World"}),
        (
            "system prompt with explicit empty parameters",
            {"prompt_config": {"system": "Hello World", "parameters": []}},
            {("prompt_config", "system"): "Hello World", ("prompt_config", "parameters"): []},
        ),
        ("system prompt integer", {"prompt_config": {"system": 123}}, {("prompt_config", "system"): 123}),
        ("system prompt boolean", {"prompt_config": {"system": True}}, {("prompt_config", "system"): True}),
        ("unknown prompt_config key", {"prompt_config": {"unknown": "unknown"}}, {("prompt_config", "unknown"): "unknown"}),
    ]

    for index, (scenario_name, extra_payload, expected_values) in enumerate(cases, start=1):
        res = rest_client.post(
            "/chats",
            json={"name": f"restful_chat_prompt_{index}", "dataset_ids": [], **extra_payload},
        )
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 0, (scenario_name, payload)
        for path, expected_value in expected_values.items():
            assert _get_nested(payload["data"], path) == expected_value, (scenario_name, path, payload)


@pytest.mark.p2
def test_chat_create_additional_guards_contract(rest_client, clear_chats):
    cases = [
        ("reject tenant_id override", {"tenant_id": "tenant-should-not-pass"}, "`tenant_id` must not be provided."),
        ("reject unknown rerank_id", {"rerank_id": "unknown-rerank-model"}, "`rerank_id` unknown-rerank-model doesn't exist"),
    ]

    for index, (scenario_name, extra_payload, expected_message) in enumerate(cases, start=1):
        res = rest_client.post(
            "/chats",
            json={"name": f"restful_chat_guard_{index}", "dataset_ids": [], **extra_payload},
        )
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 102, (scenario_name, payload)
        assert expected_message in payload["message"], (scenario_name, payload)


@pytest.mark.p2
def test_chat_create_rejects_unparsed_document(rest_client, clear_chats, create_document):
    dataset_id, _ = create_document()
    res = rest_client.post(
        "/chats",
        json={"name": "restful_chat_unparsed_document", "dataset_ids": [dataset_id]},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert "doesn't own parsed file" in payload["message"], payload


@pytest.mark.p2
def test_chat_update_name_contract(rest_client, clear_chats):
    duplicate_res = rest_client.post("/chats", json={"name": "restful_chat_update_duplicate", "dataset_ids": []})
    assert duplicate_res.status_code == 200
    duplicate_payload = duplicate_res.json()
    assert duplicate_payload["code"] == 0, duplicate_payload

    target_res = rest_client.post("/chats", json={"name": "restful_chat_update_name_target", "dataset_ids": []})
    assert target_res.status_code == 200
    target_payload = target_res.json()
    assert target_payload["code"] == 0, target_payload
    chat_id = target_payload["data"]["id"]

    cases = [
        ("valid name", {"name": "valid_name"}, 0, "", "valid_name"),
        (
            "name too long",
            {"name": "a" * (CHAT_ASSISTANT_NAME_LIMIT + 1)},
            102,
            f"Chat name length is {CHAT_ASSISTANT_NAME_LIMIT + 1} which is larger than {CHAT_ASSISTANT_NAME_LIMIT}.",
            None,
        ),
        ("name wrong type", {"name": 1}, 102, "Chat name must be a string.", None),
        ("name empty", {"name": ""}, 102, "`name` cannot be empty.", None),
        ("duplicate lowercase", {"name": "restful_chat_update_duplicate"}, 102, "Duplicated chat name.", None),
        ("duplicate uppercase", {"name": "RESTFUL_CHAT_UPDATE_DUPLICATE"}, 102, "Duplicated chat name.", None),
    ]

    for scenario_name, patch_payload, expected_code, expected_message, expected_name in cases:
        res = rest_client.patch(f"/chats/{chat_id}", json=patch_payload)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            get_res = rest_client.get(f"/chats/{chat_id}")
            assert get_res.status_code == 200, (scenario_name, get_res.text)
            get_payload = get_res.json()
            assert get_payload["code"] == 0, (scenario_name, get_payload)
            assert get_payload["data"]["name"] == expected_name, (scenario_name, get_payload)
        else:
            assert payload["message"] == expected_message, (scenario_name, payload)


@pytest.mark.p2
def test_chat_update_dataset_ids_contract(rest_client, clear_chats, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    target_res = rest_client.post("/chats", json={"name": "restful_chat_update_dataset_target", "dataset_ids": []})
    assert target_res.status_code == 200
    target_payload = target_res.json()
    assert target_payload["code"] == 0, target_payload
    chat_id = target_payload["data"]["id"]

    cases = [
        ("empty dataset_ids", [], 0, "", []),
        ("owned parsed dataset", [dataset_id], 0, "", [dataset_id]),
        ("invalid dataset id", ["invalid_dataset_id"], 102, "You don't own the dataset invalid_dataset_id", None),
        ("dataset_ids wrong type", "invalid_dataset_id", 102, "`dataset_ids` should be a list.", None),
    ]

    for scenario_name, dataset_ids, expected_code, expected_message, expected_dataset_ids in cases:
        res = rest_client.put(
            f"/chats/{chat_id}",
            json={"name": "ragflow test", "dataset_ids": dataset_ids},
        )
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            get_res = rest_client.get(f"/chats/{chat_id}")
            assert get_res.status_code == 200, (scenario_name, get_res.text)
            get_payload = get_res.json()
            assert get_payload["code"] == 0, (scenario_name, get_payload)
            assert get_payload["data"]["name"] == "ragflow test", (scenario_name, get_payload)
            assert get_payload["data"]["dataset_ids"] == expected_dataset_ids, (scenario_name, get_payload)
        else:
            assert payload["message"] == expected_message, (scenario_name, payload)


@pytest.mark.p2
def test_chat_update_avatar_contract(rest_client, clear_chats, ensure_parsed_document, tmp_path):
    dataset_id, _ = ensure_parsed_document()
    create_res = rest_client.post("/chats", json={"name": "restful_chat_update_avatar_target", "dataset_ids": []})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    chat_id = create_payload["data"]["id"]

    image_path = create_image_file(tmp_path / "restful_chat_update_avatar.png")
    encoded_avatar = encode_avatar(image_path)

    res = rest_client.put(
        f"/chats/{chat_id}",
        json={"name": "avatar_test", "icon": encoded_avatar, "dataset_ids": [dataset_id]},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload

    get_res = rest_client.get(f"/chats/{chat_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["name"] == "avatar_test", get_payload
    assert get_payload["data"]["icon"] == encoded_avatar, get_payload
    assert get_payload["data"]["dataset_ids"] == [dataset_id], get_payload


@pytest.mark.p2
def test_chat_update_llm_contract(rest_client, clear_chats, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    cases = [
        ("default llm", {}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {}),
        ("explicit llm_id", {"llm_id": "glm-4"}, 102, "`llm_id` glm-4 doesn't exist", None, None),
        ("unknown llm_id", {"llm_id": "unknown"}, 102, "`llm_id` unknown doesn't exist", None, None),
        ("temperature zero", {"llm_setting": {"temperature": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": 0}),
        ("temperature one", {"llm_setting": {"temperature": 1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": 1}),
        ("temperature negative one", {"llm_setting": {"temperature": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": -1}),
        ("temperature ten", {"llm_setting": {"temperature": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": 10}),
        ("temperature string", {"llm_setting": {"temperature": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"temperature": "a"}),
        ("top_p zero", {"llm_setting": {"top_p": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": 0}),
        ("top_p one", {"llm_setting": {"top_p": 1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": 1}),
        ("top_p negative one", {"llm_setting": {"top_p": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": -1}),
        ("top_p ten", {"llm_setting": {"top_p": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": 10}),
        ("top_p string", {"llm_setting": {"top_p": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"top_p": "a"}),
        ("presence_penalty zero", {"llm_setting": {"presence_penalty": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": 0}),
        ("presence_penalty one", {"llm_setting": {"presence_penalty": 1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": 1}),
        ("presence_penalty negative one", {"llm_setting": {"presence_penalty": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": -1}),
        ("presence_penalty ten", {"llm_setting": {"presence_penalty": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": 10}),
        ("presence_penalty string", {"llm_setting": {"presence_penalty": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"presence_penalty": "a"}),
        ("frequency_penalty zero", {"llm_setting": {"frequency_penalty": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": 0}),
        ("frequency_penalty one", {"llm_setting": {"frequency_penalty": 1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": 1}),
        ("frequency_penalty negative one", {"llm_setting": {"frequency_penalty": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": -1}),
        ("frequency_penalty ten", {"llm_setting": {"frequency_penalty": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": 10}),
        ("frequency_penalty string", {"llm_setting": {"frequency_penalty": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"frequency_penalty": "a"}),
        ("max_token zero", {"llm_setting": {"max_token": 0}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": 0}),
        ("max_token 1024", {"llm_setting": {"max_token": 1024}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": 1024}),
        ("max_token negative one", {"llm_setting": {"max_token": -1}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": -1}),
        ("max_token ten", {"llm_setting": {"max_token": 10}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": 10}),
        ("max_token string", {"llm_setting": {"max_token": "a"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"max_token": "a"}),
        ("unknown llm setting key", {"llm_setting": {"unknown": "unknown"}}, 0, "", "glm-4-flash@CI@ZHIPU-AI", {"unknown": "unknown"}),
    ]

    for index, (scenario_name, extra_payload, expected_code, expected_message, expected_llm_id, expected_llm_setting) in enumerate(cases, start=1):
        create_res = rest_client.post(
            "/chats",
            json={"name": f"restful_chat_update_llm_target_{index}", "dataset_ids": [dataset_id]},
        )
        assert create_res.status_code == 200, (scenario_name, create_res.text)
        create_payload = create_res.json()
        assert create_payload["code"] == 0, (scenario_name, create_payload)
        chat_id = create_payload["data"]["id"]

        updated_name = f"llm_test_{index}"
        payload = {"name": updated_name, "dataset_ids": [dataset_id]}
        payload.update(extra_payload)
        res = rest_client.put(f"/chats/{chat_id}", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code == 0:
            get_res = rest_client.get(f"/chats/{chat_id}")
            assert get_res.status_code == 200, (scenario_name, get_res.text)
            get_payload = get_res.json()
            assert get_payload["code"] == 0, (scenario_name, get_payload)
            assert get_payload["data"]["name"] == updated_name, (scenario_name, get_payload)
            assert get_payload["data"]["llm_id"] == expected_llm_id, (scenario_name, get_payload)
            assert get_payload["data"]["llm_setting"] == expected_llm_setting, (scenario_name, get_payload)
        else:
            assert body["message"] == expected_message, (scenario_name, body)


@pytest.mark.p2
def test_chat_update_prompt_contract(rest_client, clear_chats, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    cases = [
        (
            "default prompt config",
            {},
            {
                ("similarity_threshold",): 0.1,
                ("vector_similarity_weight",): 0.3,
                ("top_n",): 6,
                ("prompt_config", "parameters"): [{"key": "knowledge", "optional": False}],
                ("prompt_config", "empty_response"): DEFAULT_CHAT_EMPTY_RESPONSE,
                ("prompt_config", "prologue"): DEFAULT_CHAT_PROLOGUE,
                ("prompt_config", "quote"): True,
                ("prompt_config", "system"): DEFAULT_CHAT_SYSTEM_PROMPT,
            },
        ),
        ("similarity_threshold zero", {"similarity_threshold": 0}, {("similarity_threshold",): 0}),
        ("similarity_threshold one", {"similarity_threshold": 1}, {("similarity_threshold",): 1}),
        ("similarity_threshold negative one", {"similarity_threshold": -1}, {("similarity_threshold",): -1.0}),
        ("similarity_threshold ten", {"similarity_threshold": 10}, {("similarity_threshold",): 10.0}),
        ("similarity_threshold string", {"similarity_threshold": "a"}, {("similarity_threshold",): 0.0}),
        ("vector_similarity_weight zero", {"vector_similarity_weight": 0}, {("vector_similarity_weight",): 0}),
        ("vector_similarity_weight one", {"vector_similarity_weight": 1}, {("vector_similarity_weight",): 1}),
        ("vector_similarity_weight negative one", {"vector_similarity_weight": -1}, {("vector_similarity_weight",): -1.0}),
        ("vector_similarity_weight ten", {"vector_similarity_weight": 10}, {("vector_similarity_weight",): 10.0}),
        ("vector_similarity_weight string", {"vector_similarity_weight": "a"}, {("vector_similarity_weight",): 0.0}),
        ("empty prompt parameters", {"prompt_config": {"parameters": []}}, {("prompt_config", "parameters"): []}),
        ("top_n zero", {"top_n": 0}, {("top_n",): 0}),
        ("top_n one", {"top_n": 1}, {("top_n",): 1}),
        ("top_n negative one", {"top_n": -1}, {("top_n",): -1}),
        ("top_n ten", {"top_n": 10}, {("top_n",): 10}),
        ("top_n string", {"top_n": "a"}, {("top_n",): 0}),
        ("empty_response plain text", {"prompt_config": {"empty_response": "Hello World"}}, {("prompt_config", "empty_response"): "Hello World"}),
        ("empty_response empty string", {"prompt_config": {"empty_response": ""}}, {("prompt_config", "empty_response"): ""}),
        ("empty_response punctuation", {"prompt_config": {"empty_response": "!@#$%^&*()"}}, {("prompt_config", "empty_response"): "!@#$%^&*()"}),
        ("empty_response chinese text", {"prompt_config": {"empty_response": "中文测试"}}, {("prompt_config", "empty_response"): "中文测试"}),
        ("empty_response integer", {"prompt_config": {"empty_response": 123}}, {("prompt_config", "empty_response"): 123}),
        ("empty_response boolean", {"prompt_config": {"empty_response": True}}, {("prompt_config", "empty_response"): True}),
        ("empty_response space", {"prompt_config": {"empty_response": " "}}, {("prompt_config", "empty_response"): " "}),
        ("prologue plain text", {"prompt_config": {"prologue": "Hello World"}}, {("prompt_config", "prologue"): "Hello World"}),
        ("prologue empty string", {"prompt_config": {"prologue": ""}}, {("prompt_config", "prologue"): ""}),
        ("prologue punctuation", {"prompt_config": {"prologue": "!@#$%^&*()"}}, {("prompt_config", "prologue"): "!@#$%^&*()"}),
        ("prologue chinese text", {"prompt_config": {"prologue": "中文测试"}}, {("prompt_config", "prologue"): "中文测试"}),
        ("prologue integer", {"prompt_config": {"prologue": 123}}, {("prompt_config", "prologue"): 123}),
        ("prologue boolean", {"prompt_config": {"prologue": True}}, {("prompt_config", "prologue"): True}),
        ("prologue space", {"prompt_config": {"prologue": " "}}, {("prompt_config", "prologue"): " "}),
        ("quote true", {"prompt_config": {"quote": True}}, {("prompt_config", "quote"): True}),
        ("quote false", {"prompt_config": {"quote": False}}, {("prompt_config", "quote"): False}),
        ("system prompt with knowledge prefix", {"prompt_config": {"system": "Hello World {knowledge}"}}, {("prompt_config", "system"): "Hello World {knowledge}"}),
        ("system prompt only knowledge", {"prompt_config": {"system": "{knowledge}"}}, {("prompt_config", "system"): "{knowledge}"}),
        ("system prompt punctuation", {"prompt_config": {"system": "!@#$%^&*() {knowledge}"}}, {("prompt_config", "system"): "!@#$%^&*() {knowledge}"}),
        ("system prompt chinese text", {"prompt_config": {"system": "中文测试 {knowledge}"}}, {("prompt_config", "system"): "中文测试 {knowledge}"}),
        ("system prompt plain text", {"prompt_config": {"system": "Hello World"}}, {("prompt_config", "system"): "Hello World"}),
        (
            "system prompt with explicit empty parameters",
            {"prompt_config": {"system": "Hello World", "parameters": []}},
            {("prompt_config", "system"): "Hello World", ("prompt_config", "parameters"): []},
        ),
        ("system prompt integer", {"prompt_config": {"system": 123}}, {("prompt_config", "system"): 123}),
        ("system prompt boolean", {"prompt_config": {"system": True}}, {("prompt_config", "system"): True}),
        ("unknown prompt key", {"unknown": "unknown"}, {}),
    ]

    for index, (scenario_name, extra_payload, expected_values) in enumerate(cases, start=1):
        create_res = rest_client.post(
            "/chats",
            json={"name": f"restful_chat_update_prompt_target_{index}", "dataset_ids": [dataset_id]},
        )
        assert create_res.status_code == 200, (scenario_name, create_res.text)
        create_payload = create_res.json()
        assert create_payload["code"] == 0, (scenario_name, create_payload)
        chat_id = create_payload["data"]["id"]

        updated_name = f"prompt_test_{index}"
        res = rest_client.put(
            f"/chats/{chat_id}",
            json={"name": updated_name, "dataset_ids": [dataset_id], **extra_payload},
        )
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 0, (scenario_name, payload)

        get_res = rest_client.get(f"/chats/{chat_id}")
        assert get_res.status_code == 200, (scenario_name, get_res.text)
        get_payload = get_res.json()
        assert get_payload["code"] == 0, (scenario_name, get_payload)
        assert get_payload["data"]["name"] == updated_name, (scenario_name, get_payload)
        assert get_payload["data"]["dataset_ids"] == [dataset_id], (scenario_name, get_payload)
        for path, expected_value in expected_values.items():
            assert _get_nested(get_payload["data"], path) == expected_value, (scenario_name, path, get_payload)


@pytest.mark.p2
def test_chat_update_mapping_and_validation_branches_p2(rest_client, clear_chats):
    duplicate_res = rest_client.post("/chats", json={"name": "restful_chat_update_mapping_duplicate", "dataset_ids": []})
    assert duplicate_res.status_code == 200
    duplicate_payload = duplicate_res.json()
    assert duplicate_payload["code"] == 0, duplicate_payload

    target_res = rest_client.post("/chats", json={"name": "restful_chat_update_mapping_target", "dataset_ids": []})
    assert target_res.status_code == 200
    target_payload = target_res.json()
    assert target_payload["code"] == 0, target_payload
    chat_id = target_payload["data"]["id"]

    unauthorized = rest_client.patch("/chats/invalid-chat-id", json={"name": "anything"})
    assert unauthorized.status_code == 200
    unauthorized_payload = unauthorized.json()
    assert unauthorized_payload["code"] == 109, unauthorized_payload
    assert unauthorized_payload["message"] == "No authorization.", unauthorized_payload

    quote_res = rest_client.patch(f"/chats/{chat_id}", json={"prompt_config": {"quote": False}})
    assert quote_res.status_code == 200
    quote_payload = quote_res.json()
    assert quote_payload["code"] == 0, quote_payload
    assert quote_payload["data"]["prompt_config"]["quote"] is False, quote_payload

    invalid_llm_res = rest_client.patch(
        f"/chats/{chat_id}",
        json={"llm_id": "unknown-llm-model", "llm_setting": {"model_type": "chat"}},
    )
    assert invalid_llm_res.status_code == 200
    invalid_llm_payload = invalid_llm_res.json()
    assert invalid_llm_payload["code"] == 102, invalid_llm_payload
    assert "`llm_id` unknown-llm-model doesn't exist" in invalid_llm_payload["message"], invalid_llm_payload

    invalid_rerank_res = rest_client.patch(f"/chats/{chat_id}", json={"rerank_id": "unknown-rerank-model"})
    assert invalid_rerank_res.status_code == 200
    invalid_rerank_payload = invalid_rerank_res.json()
    assert invalid_rerank_payload["code"] == 102, invalid_rerank_payload
    assert "`rerank_id` unknown-rerank-model doesn't exist" in invalid_rerank_payload["message"], invalid_rerank_payload

    empty_name_res = rest_client.patch(f"/chats/{chat_id}", json={"name": ""})
    assert empty_name_res.status_code == 200
    empty_name_payload = empty_name_res.json()
    assert empty_name_payload["code"] == 102, empty_name_payload
    assert empty_name_payload["message"] == "`name` cannot be empty.", empty_name_payload

    duplicate_name_res = rest_client.patch(f"/chats/{chat_id}", json={"name": "restful_chat_update_mapping_duplicate"})
    assert duplicate_name_res.status_code == 200
    duplicate_name_payload = duplicate_name_res.json()
    assert duplicate_name_payload["code"] == 102, duplicate_name_payload
    assert duplicate_name_payload["message"] == "Duplicated chat name.", duplicate_name_payload

    prompt_without_placeholder_res = rest_client.patch(
        f"/chats/{chat_id}",
        json={"prompt_config": {"system": "No required placeholder", "parameters": [{"key": "knowledge", "optional": False}]}},
    )
    assert prompt_without_placeholder_res.status_code == 200
    prompt_without_placeholder_payload = prompt_without_placeholder_res.json()
    assert prompt_without_placeholder_payload["code"] == 0, prompt_without_placeholder_payload

    icon_res = rest_client.patch(f"/chats/{chat_id}", json={"icon": "raw-avatar-value"})
    assert icon_res.status_code == 200
    icon_payload = icon_res.json()
    assert icon_payload["code"] == 0, icon_payload

    get_res = rest_client.get(f"/chats/{chat_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["prompt_config"]["system"] == "No required placeholder", get_payload
    assert get_payload["data"]["prompt_config"]["parameters"] == [{"key": "knowledge", "optional": False}], get_payload
    assert get_payload["data"]["icon"] == "raw-avatar-value", get_payload


@pytest.mark.p2
def test_chat_update_rejects_unparsed_document(rest_client, clear_chats, create_document):
    dataset_id, _ = create_document()
    create_res = rest_client.post("/chats", json={"name": "restful_chat_update_unparsed_target", "dataset_ids": []})
    assert create_res.status_code == 200, create_res.text
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    chat_id = create_payload["data"]["id"]

    res = rest_client.patch(f"/chats/{chat_id}", json={"dataset_ids": [dataset_id]})
    assert res.status_code == 200, res.text
    payload = res.json()
    assert payload["code"] == 102, payload
    assert "doesn't own parsed file" in payload["message"], payload
