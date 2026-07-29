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

import logging

import pytest

import agent.tools.querit as querit_module
from agent.tools.querit import QueritSearch, QueritSearchParam


class _FakeResponse:
    def __init__(self, payload, status_code=200):
        self._payload = payload
        self.status_code = status_code

    def json(self):
        return self._payload

    def raise_for_status(self):
        if self.status_code >= 400:
            raise querit_module.requests.HTTPError(
                f"{self.status_code} error",
                response=self,
            )


def _make_tool(api_key="test-api-key"):
    tool = QueritSearch.__new__(QueritSearch)
    param = QueritSearchParam()
    param.api_key = api_key
    param.delay_after_error = 0
    tool._param = param
    tool.check_if_canceled = lambda *args, **kwargs: False

    captured = {}
    outputs = {}

    def fake_retrieve(results, get_title, get_url, get_content, get_score):
        items = list(results)
        captured["references"] = [
            {
                "title": get_title(item),
                "url": get_url(item),
                "content": get_content(item),
                "score": get_score(item),
            }
            for item in items
        ]
        outputs["formalized_content"] = "FORMALIZED"

    tool._retrieve_chunks = fake_retrieve
    tool.set_output = lambda key, value: outputs.__setitem__(key, value)
    tool.output = lambda key=None: outputs.get(key) if key else outputs
    return tool, captured, outputs


def test_minimal_search_posts_defaults_and_preserves_raw_response(monkeypatch):
    raw_response = {
        "took": "300ms",
        "error_code": 200,
        "error_msg": "",
        "search_id": 11099848653006015581,
        "query_context": {"query": "expanded query", "count": 99},
        "results": {
            "result": [
                {
                    "title": "LLM progress",
                    "url": "https://example.com/llm",
                    "snippet": "Recent LLM progress summary",
                    "custom": {"request_id": "kept"},
                }
            ]
        },
    }
    calls = []

    def fake_post(url, **kwargs):
        calls.append((url, kwargs))
        return _FakeResponse(raw_response)

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, captured, outputs = _make_tool()

    result = tool._invoke(query="latest LLM progress")

    assert result == "FORMALIZED"
    assert calls == [
        (
            "https://api.querit.ai/v1/search",
            {
                "headers": {
                    "Accept": "application/json",
                    "Authorization": "Bearer test-api-key",
                    "Content-Type": "application/json",
                },
                "json": {
                    "query": "latest LLM progress",
                    "count": 10,
                    "chunksPerDoc": 3,
                },
                "timeout": querit_module.DEFAULT_TIMEOUT,
            },
        )
    ]
    assert captured["references"] == [
        {
            "title": "LLM progress",
            "url": "https://example.com/llm",
            "content": "Recent LLM progress summary",
            "score": 1,
        }
    ]
    assert outputs["json"] == raw_response
    assert outputs["json"]["search_id"] == 11099848653006015581


def test_search_maps_flat_filters_to_querit_request(monkeypatch):
    calls = []

    def fake_post(url, **kwargs):
        calls.append((url, kwargs))
        return _FakeResponse({"results": {"result": []}})

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, outputs = _make_tool()

    result = tool._invoke(
        query="AI search news",
        count=5,
        chunks_per_doc=1,
        site_include=["example.com"],
        site_exclude=["archive.example.com"],
        time_range="w1",
        country_include=["US", "CA"],
        language_include=["en", "fr"],
    )

    assert result == ""
    assert calls[0][1]["json"] == {
        "query": "AI search news",
        "count": 5,
        "chunksPerDoc": 1,
        "filters": {
            "sites": {
                "include": ["example.com"],
                "exclude": ["archive.example.com"],
            },
            "timeRange": {"date": "w1"},
            "geo": {"countries": {"include": ["US", "CA"]}},
            "languages": {"include": ["en", "fr"]},
        },
    }
    assert outputs["json"] == {"results": {"result": []}}


def test_unauthorized_response_is_not_retried(monkeypatch):
    calls = []

    def fake_post(url, **kwargs):
        calls.append((url, kwargs))
        return _FakeResponse({"error": "unauthorized"}, status_code=401)

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, outputs = _make_tool()

    result = tool._invoke(query="AI news")

    assert len(calls) == 1
    assert "401" in result
    assert "401" in outputs["_ERROR"]


def test_blank_node_api_key_falls_back_to_environment(monkeypatch):
    received_authorization = []

    def fake_post(_url, **kwargs):
        received_authorization.append(kwargs["headers"]["Authorization"])
        return _FakeResponse({"results": {"result": []}})

    monkeypatch.setenv("QUERIT_API_KEY", "environment-key")
    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, outputs = _make_tool(api_key="   ")

    result = tool._invoke(query="AI news")

    assert result == ""
    assert outputs.get("_ERROR", "") == ""
    assert received_authorization == ["Bearer environment-key"]


def test_missing_api_key_fails_before_network(monkeypatch):
    calls = []
    monkeypatch.delenv("QUERIT_API_KEY", raising=False)
    monkeypatch.setattr(querit_module.requests, "post", lambda *args, **kwargs: calls.append((args, kwargs)))
    tool, _, outputs = _make_tool(api_key="")

    result = tool._invoke(query="AI news")

    assert calls == []
    assert "QUERIT_API_KEY" in result
    assert "QUERIT_API_KEY" in outputs["_ERROR"]


def test_empty_query_returns_empty_outputs_without_network(monkeypatch):
    calls = []
    monkeypatch.setattr(querit_module.requests, "post", lambda *args, **kwargs: calls.append((args, kwargs)))
    tool, _, outputs = _make_tool()

    result = tool._invoke(query="")

    assert result == ""
    assert calls == []
    assert outputs == {"formalized_content": "", "json": {}}


def test_explicit_none_omits_chunks_per_doc(monkeypatch):
    payloads = []

    def fake_post(_url, **kwargs):
        payloads.append(kwargs["json"])
        return _FakeResponse({"results": {"result": []}})

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, _ = _make_tool()

    tool._invoke(query="AI news", chunks_per_doc=None)

    assert payloads == [{"query": "AI news", "count": 10}]


def test_runtime_values_override_node_defaults(monkeypatch):
    payloads = []

    def fake_post(_url, **kwargs):
        payloads.append(kwargs["json"])
        return _FakeResponse({"results": {"result": []}})

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, _ = _make_tool()
    tool._param.count = 8
    tool._param.chunks_per_doc = 2
    tool._param.site_include = ["node.example.com"]

    tool._invoke(
        query="AI news",
        count=1,
        site_include=[],
        time_range="2026-01-01to2026-01-31",
    )

    assert payloads == [
        {
            "query": "AI news",
            "count": 1,
            "chunksPerDoc": 2,
            "filters": {"timeRange": {"date": "2026-01-01to2026-01-31"}},
        }
    ]


@pytest.mark.parametrize(
    ("runtime_values", "expected_error"),
    [
        ({"query": 123}, "query"),
        ({"query": "AI news", "count": 0}, "count"),
        ({"query": "AI news", "count": 1.5}, "count"),
        ({"query": "AI news", "chunks_per_doc": 0}, "chunks_per_doc"),
        ({"query": "AI news", "chunks_per_doc": 4}, "chunks_per_doc"),
        ({"query": "AI news", "time_range": "past_week"}, "time_range"),
        ({"query": "AI news", "site_include": ["example.com", 1]}, "site_include"),
    ],
)
def test_invalid_runtime_values_fail_before_network(monkeypatch, runtime_values, expected_error):
    calls = []
    monkeypatch.setattr(querit_module.requests, "post", lambda *args, **kwargs: calls.append((args, kwargs)))
    tool, _, outputs = _make_tool()

    result = tool._invoke(**runtime_values)

    assert calls == []
    assert expected_error in result
    assert expected_error in outputs["_ERROR"]


def test_retryable_http_errors_are_retried_until_success(monkeypatch):
    responses = [
        _FakeResponse({"error": "rate limited"}, status_code=429),
        _FakeResponse({"error": "temporary"}, status_code=503),
        _FakeResponse({"results": {"result": []}, "request_id": "success"}),
    ]
    calls = []

    def fake_post(*args, **kwargs):
        calls.append((args, kwargs))
        return responses.pop(0)

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, outputs = _make_tool()

    result = tool._invoke(query="AI news")

    assert result == ""
    assert len(calls) == 3
    assert outputs["json"]["request_id"] == "success"


def test_retry_wait_uses_configured_delay(monkeypatch):
    responses = [
        _FakeResponse({"error": "rate limited"}, status_code=429),
        _FakeResponse({"results": {"result": []}}),
    ]
    delays = []

    monkeypatch.setattr(querit_module.requests, "post", lambda *args, **kwargs: responses.pop(0))
    monkeypatch.setattr(querit_module.time, "sleep", delays.append)
    tool, _, _ = _make_tool()
    tool._param.delay_after_error = 0.25

    result = tool._invoke(query="AI news")

    assert result == ""
    assert delays == [0.25]


def test_cancellation_stops_before_retry(monkeypatch):
    calls = []

    def fake_post(*args, **kwargs):
        calls.append((args, kwargs))
        return _FakeResponse({"error": "rate limited"}, status_code=429)

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, outputs = _make_tool()
    cancellation_checks = iter([False, False, True])
    tool.check_if_canceled = lambda *args, **kwargs: next(cancellation_checks)

    result = tool._invoke(query="AI news")

    assert result is None
    assert len(calls) == 1
    assert "_ERROR" not in outputs


def test_network_errors_are_retried_until_success(monkeypatch):
    outcomes = [
        querit_module.requests.ConnectTimeout("first timeout"),
        querit_module.requests.ConnectionError("second failure"),
        _FakeResponse({"results": {"result": []}}),
    ]
    calls = []

    def fake_post(*args, **kwargs):
        calls.append((args, kwargs))
        outcome = outcomes.pop(0)
        if isinstance(outcome, Exception):
            raise outcome
        return outcome

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, outputs = _make_tool()

    result = tool._invoke(query="AI news")

    assert result == ""
    assert len(calls) == 3
    assert outputs["json"] == {"results": {"result": []}}


def test_invalid_json_response_is_not_retried(monkeypatch):
    calls = []

    class _InvalidJSONResponse(_FakeResponse):
        def json(self):
            raise querit_module.requests.JSONDecodeError("invalid JSON", "not-json", 0)

    def fake_post(*args, **kwargs):
        calls.append((args, kwargs))
        return _InvalidJSONResponse(None)

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, outputs = _make_tool()

    result = tool._invoke(query="AI news")

    assert len(calls) == 1
    assert "invalid JSON" in result
    assert "invalid JSON" in outputs["_ERROR"]


def test_persistent_retryable_error_stops_after_three_attempts(monkeypatch):
    calls = []

    def fake_post(*args, **kwargs):
        calls.append((args, kwargs))
        return _FakeResponse({"error": "temporary"}, status_code=500)

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, outputs = _make_tool()

    result = tool._invoke(query="AI news")

    assert len(calls) == 3
    assert "500" in result
    assert "500" in outputs["_ERROR"]


def test_non_object_response_is_rejected(monkeypatch):
    monkeypatch.setattr(
        querit_module.requests,
        "post",
        lambda *args, **kwargs: _FakeResponse(["unexpected"]),
    )
    tool, _, outputs = _make_tool()

    result = tool._invoke(query="AI news")

    assert "JSON object" in result
    assert "JSON object" in outputs["_ERROR"]


@pytest.mark.parametrize(
    ("raw_response", "expected_error"),
    [
        ({"results": []}, "results must be an object"),
        ({"results": None}, "results must be an object"),
        ({"results": {"result": {}}}, "results.result must be an array"),
        ({"results": {"result": None}}, "results.result must be an array"),
    ],
)
def test_invalid_result_container_types_are_rejected(
    monkeypatch,
    raw_response,
    expected_error,
):
    monkeypatch.setattr(
        querit_module.requests,
        "post",
        lambda *args, **kwargs: _FakeResponse(raw_response),
    )
    tool, _, outputs = _make_tool()

    result = tool._invoke(query="AI news")

    assert expected_error in result
    assert expected_error in outputs["_ERROR"]


def test_results_with_missing_optional_fields_do_not_fail(monkeypatch):
    raw_response = {
        "results": {
            "result": [
                {"title": "Title only"},
                {"url": "https://example.com"},
                {"snippet": "Snippet only"},
                "unexpected",
            ]
        }
    }
    monkeypatch.setattr(
        querit_module.requests,
        "post",
        lambda *args, **kwargs: _FakeResponse(raw_response),
    )
    tool, captured, outputs = _make_tool()

    result = tool._invoke(query="AI news")

    assert result == "FORMALIZED"
    assert len(captured["references"]) == 3
    assert outputs["json"] == raw_response


def test_non_string_reference_fields_match_go_string_coercion(monkeypatch):
    raw_response = {
        "results": {
            "result": [
                {
                    "title": 123,
                    "url": 456,
                    "snippet": 789,
                }
            ]
        }
    }
    monkeypatch.setattr(
        querit_module.requests,
        "post",
        lambda *args, **kwargs: _FakeResponse(raw_response),
    )
    tool, captured, outputs = _make_tool()

    result = tool._invoke(query="AI news")

    assert result == "FORMALIZED"
    assert captured["references"] == [
        {
            "title": "123",
            "url": "456",
            "content": "789",
            "score": 1,
        }
    ]
    assert outputs["json"] == raw_response


def test_api_key_is_redacted_from_errors_and_logs(monkeypatch, caplog):
    secret = f"secret-test-key-{id(caplog)}"

    def fake_post(*args, **kwargs):
        raise querit_module.requests.ConnectionError(f"connection failed for {secret}")

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _, outputs = _make_tool(api_key=secret)

    with caplog.at_level(logging.ERROR):
        result = tool._invoke(query="AI news")

    assert secret not in result
    assert secret not in outputs["_ERROR"]
    assert secret not in caplog.text
    assert "[REDACTED]" in result


def test_querit_classes_are_available_through_dynamic_tool_discovery():
    import agent.tools as tools_package

    assert tools_package.QueritSearch is QueritSearch
    assert tools_package.QueritSearchParam is QueritSearchParam


def test_tool_metadata_exposes_only_supported_runtime_parameters():
    metadata = QueritSearchParam().get_meta()["function"]

    assert metadata["name"] == "querit_search"
    assert metadata["parameters"]["required"] == ["query"]
    assert set(metadata["parameters"]["properties"]) == {
        "query",
        "count",
        "chunks_per_doc",
        "site_include",
        "site_exclude",
        "time_range",
        "country_include",
        "language_include",
    }
    assert "api_key" not in metadata["parameters"]["properties"]
