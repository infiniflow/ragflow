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

import agent.tools.sofya as sofya_module
from agent.tools.sofya import SofyaSearch, SofyaSearchParam

SEARCH_URL = "https://sofya.co/v1/search"


class _FakeResponse:
    def __init__(self, payload, status_code=200):
        self._payload = payload
        self.status_code = status_code
        self.url = SEARCH_URL

    def json(self):
        return self._payload

    def raise_for_status(self):
        if self.status_code >= 400:
            raise sofya_module.requests.HTTPError(
                f"{self.status_code} Client Error for url: {self.url}",
                response=self,
            )


def _make_tool(api_key="sofya-key"):
    tool = SofyaSearch.__new__(SofyaSearch)
    param = SofyaSearchParam()
    param.api_key = api_key
    param.delay_after_error = 0
    param.max_retries = 0
    tool._param = param
    tool.check_if_canceled = lambda *args, **kwargs: False

    captured = {}
    outputs = {}

    def fake_retrieve(results, get_title, get_url, get_content):
        captured["references"] = [{"title": get_title(item), "url": get_url(item), "content": get_content(item)} for item in results]
        outputs["formalized_content"] = "FORMALIZED"

    tool._retrieve_chunks = fake_retrieve
    tool.set_output = lambda key, value: outputs.__setitem__(key, value)
    tool.output = lambda key=None: outputs.get(key) if key else outputs
    return tool, captured, outputs


def _payload():
    return {
        "results": [
            {
                "url": "https://example.com/a",
                "title": "A",
                "description": "Snippet for A.",
                "content": "Page text for A.",
            },
            {
                "url": "https://example.com/b",
                "title": "B",
                "description": "Snippet for B.",
                "content": "",
            },
        ]
    }


def _capture_post(monkeypatch, response=None):
    calls = []

    def fake_post(url, headers=None, json=None, timeout=None):
        calls.append({"url": url, "headers": headers, "json": json})
        return response if response is not None else _FakeResponse(_payload())

    monkeypatch.setattr(sofya_module.requests, "post", fake_post)
    return calls


def test_search_posts_the_query_with_a_bearer_key(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, outputs = _make_tool(api_key="  sofya-key  ")

    tool._invoke(query="What is RAGFlow?")

    assert calls[0]["url"] == SEARCH_URL
    assert calls[0]["headers"]["Authorization"] == "Bearer sofya-key"
    assert calls[0]["json"]["query"] == "What is RAGFlow?"
    assert outputs["formalized_content"] == "FORMALIZED"


def test_requests_identify_ragflow(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, _outputs = _make_tool()

    tool._invoke(query="q")

    assert calls[0]["headers"]["User-Agent"] == "RAGFlow sofya-integration/infiniflow-ragflow"


def test_page_content_is_preferred_over_the_snippet(monkeypatch):
    _capture_post(monkeypatch)
    tool, captured, outputs = _make_tool()

    tool._invoke(query="q")

    assert captured["references"] == [
        {"title": "A", "url": "https://example.com/a", "content": "Page text for A."},
        # B has no page content, so its snippet is used.
        {"title": "B", "url": "https://example.com/b", "content": "Snippet for B."},
    ]
    assert len(outputs["json"]) == 2


def test_non_string_result_fields_do_not_raise(monkeypatch):
    payload = {"results": [{"url": 42, "title": True, "content": 7, "description": None}]}
    _capture_post(monkeypatch, _FakeResponse(payload))
    tool, captured, _outputs = _make_tool()

    tool._invoke(query="q")

    assert captured["references"] == [{"title": "True", "url": "42", "content": "7"}]


def test_whitespace_only_content_falls_back_to_the_snippet(monkeypatch):
    payload = {"results": [{"url": "https://example.com/c", "title": "C", "content": "   ", "description": "  Snippet for C.  "}]}
    _capture_post(monkeypatch, _FakeResponse(payload))
    tool, captured, _outputs = _make_tool()

    tool._invoke(query="q")

    assert captured["references"] == [{"title": "C", "url": "https://example.com/c", "content": "Snippet for C."}]


def test_a_result_with_no_text_at_all_yields_no_content(monkeypatch):
    payload = {"results": [{"url": "https://example.com/d", "title": "D", "content": " ", "description": " "}]}
    _capture_post(monkeypatch, _FakeResponse(payload))
    tool, captured, _outputs = _make_tool()

    tool._invoke(query="q")

    # An empty content is dropped by _retrieve_chunks rather than stored blank.
    assert captured["references"] == [{"title": "D", "url": "https://example.com/d", "content": ""}]


def test_a_results_field_that_is_not_a_list_yields_nothing(monkeypatch):
    _capture_post(monkeypatch, _FakeResponse({"results": None}))
    tool, captured, _outputs = _make_tool()

    tool._invoke(query="q")

    assert captured["references"] == []


def test_the_node_top_n_is_used_when_the_caller_gives_no_count(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, captured, _outputs = _make_tool()
    tool._param.top_n = 1

    tool._invoke(query="q")

    assert calls[0]["json"]["max_results"] == 1
    assert [r["title"] for r in captured["references"]] == ["A"]


def test_a_caller_supplied_count_wins_over_the_node_top_n(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, _outputs = _make_tool()
    tool._param.top_n = 10

    tool._invoke(query="q", max_results=3)

    assert calls[0]["json"]["max_results"] == 3


def test_the_result_count_is_clamped_to_the_supported_range(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, _outputs = _make_tool()

    tool._invoke(query="q", max_results=500)
    assert calls[0]["json"]["max_results"] == 20

    tool._invoke(query="q", max_results=0)
    assert calls[1]["json"]["max_results"] == 1

    # An unusable argument falls back to the node's Top N.
    tool._invoke(query="q", max_results="lots")
    assert calls[2]["json"]["max_results"] == tool._param.top_n


def test_an_unknown_search_depth_falls_back_to_the_default(monkeypatch, caplog):
    calls = _capture_post(monkeypatch)
    tool, _captured, _outputs = _make_tool()
    tool._param.search_depth = "advanced"

    with caplog.at_level("WARNING"):
        tool._invoke(query="q")

    assert calls[0]["json"]["search_depth"] == "basic"
    assert "advanced" in caplog.text


def test_the_search_depth_is_forwarded_when_it_is_supported(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, _outputs = _make_tool()
    tool._param.search_depth = "snippets"

    tool._invoke(query="q")

    assert calls[0]["json"]["search_depth"] == "snippets"


def test_freshness_is_forwarded_only_when_a_window_is_set(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, _outputs = _make_tool()

    tool._invoke(query="q", freshness="week")
    assert calls[0]["json"]["freshness"] == "week"

    # Blank and `any` both mean no restriction.
    tool._invoke(query="q", freshness="")
    assert "freshness" not in calls[1]["json"]

    tool._invoke(query="q", freshness="any")
    assert "freshness" not in calls[2]["json"]


def test_an_unsupported_freshness_is_rejected_before_the_request(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, _outputs = _make_tool()

    try:
        tool._invoke(query="q", freshness="decade")
    except ValueError as e:
        assert "decade" in str(e)
        assert calls == []
        return
    raise AssertionError("expected an unsupported freshness to raise")


def test_an_unsupported_topic_is_rejected_before_the_request(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, _outputs = _make_tool()

    try:
        tool._invoke(query="q", topic="sports")
    except ValueError as e:
        assert "sports" in str(e)
        assert calls == []
        return
    raise AssertionError("expected an unsupported topic to raise")


def test_the_topic_defaults_to_general(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, _outputs = _make_tool()

    tool._invoke(query="q")
    assert calls[0]["json"]["topic"] == "general"

    tool._invoke(query="q", topic="news")
    assert calls[1]["json"]["topic"] == "news"


def test_the_published_schema_restricts_the_enums():
    properties = SofyaSearchParam().get_meta()["function"]["parameters"]["properties"]

    assert properties["topic"]["enum"] == ["general", "news"]
    assert properties["freshness"]["enum"] == ["any", "day", "week", "month", "year"]


def test_blank_query_short_circuits(monkeypatch):
    calls = _capture_post(monkeypatch)
    tool, _captured, outputs = _make_tool()

    assert tool._invoke(query="") == ""
    assert calls == []
    assert outputs["formalized_content"] == ""


def test_failures_never_log_the_query_or_the_key(monkeypatch, caplog):
    _capture_post(monkeypatch, _FakeResponse({}, status_code=402))
    tool, _captured, _outputs = _make_tool(api_key="sofya-secret")

    with caplog.at_level("ERROR"):
        result = tool._invoke(query="my private query")

    assert "my private query" not in caplog.text
    assert "sofya-secret" not in caplog.text
    assert "my private query" not in str(result)
    assert "HTTPError" in str(result)


def _capture_sleep(monkeypatch):
    """Record the retry delays without patching the stdlib for other threads."""
    slept = []

    class _Clock:
        @staticmethod
        def sleep(seconds):
            slept.append(seconds)

    monkeypatch.setattr(sofya_module, "time", _Clock)
    return slept


def test_a_failed_final_attempt_does_not_sleep(monkeypatch):
    """The delay only buys something when another attempt follows it."""
    _capture_post(monkeypatch, _FakeResponse({}, status_code=402))
    slept = _capture_sleep(monkeypatch)
    tool, _captured, _outputs = _make_tool()
    tool._param.max_retries = 0
    tool._param.delay_after_error = 5

    tool._invoke(query="q")

    assert slept == []


def test_transient_failures_are_retried_with_a_delay(monkeypatch):
    calls = _capture_post(monkeypatch, _FakeResponse({}, status_code=503))
    slept = _capture_sleep(monkeypatch)
    tool, _captured, _outputs = _make_tool()
    tool._param.max_retries = 2
    tool._param.delay_after_error = 5

    tool._invoke(query="q")

    assert len(calls) == 3
    # Two gaps between three attempts, and nothing after the last one.
    assert slept == [5, 5]


def test_non_transient_failures_are_not_retried(monkeypatch):
    """A bad key or no credits fails the same way every time, so ask once."""
    calls = _capture_post(monkeypatch, _FakeResponse({}, status_code=402))
    slept = _capture_sleep(monkeypatch)
    tool, _captured, _outputs = _make_tool()
    tool._param.max_retries = 2
    tool._param.delay_after_error = 5

    result = tool._invoke(query="q")

    assert len(calls) == 1
    assert slept == []
    assert "HTTPError" in str(result)


def test_network_errors_are_retried(monkeypatch):
    slept = _capture_sleep(monkeypatch)
    attempts = []

    def failing_post(url, headers=None, json=None, timeout=None):
        attempts.append(url)
        raise sofya_module.requests.ConnectionError("connection reset")

    monkeypatch.setattr(sofya_module.requests, "post", failing_post)
    tool, _captured, _outputs = _make_tool()
    tool._param.max_retries = 1
    tool._param.delay_after_error = 2

    result = tool._invoke(query="q")

    assert len(attempts) == 2
    assert slept == [2]
    assert "ConnectionError" in str(result)


def test_param_check_requires_an_api_key():
    param = SofyaSearchParam()

    try:
        param.check()
    except Exception:
        return
    raise AssertionError("expected check() to reject a blank API key")


def test_param_check_rejects_an_unknown_search_depth():
    param = SofyaSearchParam()
    param.api_key = "sofya-key"
    param.search_depth = "advanced"

    try:
        param.check()
    except Exception:
        return
    raise AssertionError("expected check() to reject an unknown search depth")


def test_param_check_rejects_a_non_positive_top_n():
    param = SofyaSearchParam()
    param.api_key = "sofya-key"
    param.top_n = 0

    try:
        param.check()
    except Exception:
        return
    raise AssertionError("expected check() to reject top_n=0")
