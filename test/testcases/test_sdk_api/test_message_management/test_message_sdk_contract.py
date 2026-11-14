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
"""SDK wire contracts. These tests never require a Go server or model provider."""

import json
from urllib.parse import parse_qs, urlsplit

import pytest
import requests
from ragflow_sdk import Memory, RAGFlow


@pytest.fixture
def sdk():
    return RAGFlow("contract-key", "https://unused.invalid")


def assert_request(calls, method, path, query=None, body=None):
    assert len(calls) == 1
    request = calls[0]
    url = urlsplit(request.url)
    assert (request.method, url.scheme, url.netloc, url.path) == (method, "https", "unused.invalid", f"/api/v1{path}")
    assert request.headers["Authorization"] == "Bearer contract-key"
    assert parse_qs(url.query, keep_blank_values=True) == (query or {})
    assert (json.loads(request.body) if request.body else None) == body


@pytest.mark.parametrize("memory_ids", [["mem-1"], ["mem-1", "mem-2"]])
@pytest.mark.parametrize("user_id", [None, "external-user"])
def test_add_request_and_response(sdk, mock_api, memory_ids, user_id):
    calls = mock_api({"code": 0, "message": "All add to task.", "data": None})
    arguments = dict(memory_id=memory_ids, agent_id="agent", session_id="session", user_input="question", agent_response="answer")
    if user_id is not None:
        arguments["user_id"] = user_id
    assert sdk.add_message(**arguments) == "All add to task."
    assert_request(calls, "POST", "/messages", body={**arguments, "user_id": user_id or ""})


@pytest.mark.parametrize("missing", ["memory_id", "agent_id", "session_id", "user_input", "agent_response"])
def test_add_requires_arguments(sdk, missing):
    arguments = dict(memory_id=["memory"], agent_id="agent", session_id="session", user_input="question", agent_response="answer")
    del arguments[missing]
    with pytest.raises(TypeError, match=missing):
        sdk.add_message(**arguments)


@pytest.mark.parametrize(
    "filters,query",
    [
        ({}, {"page": ["1"], "page_size": ["50"]}),
        ({"agent_id": ["a1", "a2"], "keywords": "session-fragment", "page": 2, "page_size": 3}, {"agent_id": ["a1", "a2"], "keywords": ["session-fragment"], "page": ["2"], "page_size": ["3"]}),
    ],
)
def test_list_request_and_response(sdk, mock_api, filters, query):
    data = {"storage_type": "table", "messages": {"total_count": 1, "message_list": [{"message_id": 7, "extract": [], "task": {}}]}}
    calls = mock_api({"code": 0, "data": data})
    assert Memory(sdk, {"id": "memory"}).list_memory_messages(**filters) == data
    assert_request(calls, "GET", "/memories/memory", query=query)


@pytest.mark.parametrize(
    "filters,query",
    [
        ({}, {"memory_id": ["m1", "m2"], "limit": ["10"]}),
        ({"agent_id": "agent", "session_id": "session", "limit": 2}, {"memory_id": ["m1", "m2"], "agent_id": ["agent"], "session_id": ["session"], "limit": ["2"]}),
    ],
)
def test_recent_request_and_response(sdk, mock_api, filters, query):
    data = [{"memory_id": "m1", "message_id": 7, "content": "stored conversation"}]
    calls = mock_api({"code": 0, "data": data})
    assert sdk.get_recent_messages(["m1", "m2"], **filters) == data
    assert_request(calls, "GET", "/messages", query=query)


@pytest.mark.parametrize(
    "filters,extra",
    [
        ({}, {"similarity_threshold": ["0.2"], "keywords_similarity_weight": ["0.7"], "top_n": ["10"]}),
        (
            {"agent_id": "agent", "session_id": "session", "user_id": "user", "similarity_threshold": 0.1, "keywords_similarity_weight": 0.6, "top_n": 4},
            {"agent_id": ["agent"], "session_id": ["session"], "user_id": ["user"], "similarity_threshold": ["0.1"], "keywords_similarity_weight": ["0.6"], "top_n": ["4"]},
        ),
    ],
)
def test_search_request_and_response(sdk, mock_api, filters, extra):
    data = [{"memory_id": "m1", "message_id": 7, "content": "Coriander leaves and seeds"}]
    calls = mock_api({"code": 0, "data": data})
    assert sdk.search_message("coriander leaves?", ["m1", "m2"], **filters) == data
    assert_request(calls, "GET", "/messages/search", query={"query": ["coriander leaves?"], "memory_id": ["m1", "m2"], **extra})


@pytest.mark.parametrize(
    "operation,method,path,body,data,expected",
    [
        ("content", "GET", "/messages/memory:7/content", None, {"content": "conversation", "content_embed": [0.25]}, {"content": "conversation", "content_embed": [0.25]}),
        ("forget", "DELETE", "/messages/memory:7", {}, None, True),
        ("false", "PUT", "/messages/memory:7", {"status": False}, None, True),
        ("true", "PUT", "/messages/memory:7", {"status": True}, None, True),
    ],
)
def test_composite_message_requests(sdk, mock_api, operation, method, path, body, data, expected):
    calls = mock_api({"code": 0, "data": data})
    assert invoke(sdk, operation) == expected
    assert_request(calls, method, path, body=body)


def invoke(sdk, operation):
    memory = Memory(sdk, {"id": "memory"})
    if operation == "add":
        return sdk.add_message(["memory"], "agent", "session", "question", "answer", user_id="user")
    if operation == "list":
        return memory.list_memory_messages()
    if operation == "recent":
        return sdk.get_recent_messages(["memory"])
    if operation == "search":
        return sdk.search_message("query", ["memory"])
    if operation == "content":
        return memory.get_message_content(7)
    if operation == "forget":
        return memory.forget_message(7)
    return memory.update_message_status(7, operation == "true")


@pytest.mark.parametrize("operation", ["add", "list", "recent", "search", "content", "forget", "false", "true"])
@pytest.mark.parametrize("code,status,message", [(401, 401, "Invalid access token"), (404, 200, "Message not found"), (500, 200, "message store unavailable")])
def test_error_propagation(sdk, mock_api, operation, code, status, message):
    calls = mock_api({"code": code, "message": message}, status=status)
    with pytest.raises(Exception) as error:
        invoke(sdk, operation)
    assert str(error.value) == message
    assert len(calls) == 1


@pytest.mark.parametrize("operation", ["add", "list", "recent", "search", "content", "forget", "false", "true"])
def test_transport_failure_is_not_swallowed(sdk, monkeypatch, operation):
    def fail(*args, **kwargs):
        raise requests.Timeout("transport timed out")

    monkeypatch.setattr(requests.sessions.Session, "send", fail)
    with pytest.raises(requests.Timeout, match="transport timed out"):
        invoke(sdk, operation)
