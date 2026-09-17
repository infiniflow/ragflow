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

import agent.tools.youcom as youcom_module
from agent.tools.youcom import YouComSearch, YouComSearchParam

KEYLESS_URL = "https://api.you.com/v1/agents/search"
KEYED_URL = "https://api.you.com/v1/search"


class _FakeResponse:
    def __init__(self, payload, status_code=200):
        self._payload = payload
        self.status_code = status_code
        self.url = f"{KEYLESS_URL}?query=my%20private%20query&count=10"

    def json(self):
        return self._payload

    def raise_for_status(self):
        if self.status_code >= 400:
            raise youcom_module.requests.HTTPError(
                f"{self.status_code} Client Error for url: {self.url}",
                response=self,
            )


def _make_tool(api_key=""):
    tool = YouComSearch.__new__(YouComSearch)
    param = YouComSearchParam()
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
        "results": {
            "web": [
                {
                    "url": "https://example.com/a",
                    "title": "A",
                    "description": "Meta description.",
                    "snippets": ["First passage.", "Second passage."],
                }
            ],
            "news": [
                {
                    "url": "https://news.example.com/b",
                    "title": "B",
                    "description": "News description only.",
                }
            ],
        }
    }


def _capture_get(monkeypatch, response=None):
    calls = []

    def fake_get(url, headers=None, params=None, timeout=None):
        calls.append({"url": url, "headers": headers, "params": params})
        return response if response is not None else _FakeResponse(_payload())

    monkeypatch.setattr(youcom_module.requests, "get", fake_get)
    return calls


def test_keyless_search_uses_the_public_endpoint_without_an_auth_header(monkeypatch):
    calls = _capture_get(monkeypatch)
    tool, _captured, outputs = _make_tool(api_key="")

    tool._invoke(query="What is RAGFlow?")

    assert calls[0]["url"] == KEYLESS_URL
    # The keyless endpoint rejects an auth header, so none may be sent.
    assert "X-API-Key" not in calls[0]["headers"]
    assert calls[0]["params"]["query"] == "What is RAGFlow?"
    assert outputs["formalized_content"] == "FORMALIZED"


def test_keyed_search_uses_the_authenticated_endpoint(monkeypatch):
    calls = _capture_get(monkeypatch)
    tool, _captured, _outputs = _make_tool(api_key="  ydc-test  ")

    tool._invoke(query="What is RAGFlow?")

    assert calls[0]["url"] == KEYED_URL
    assert calls[0]["headers"]["X-API-Key"] == "ydc-test"


def test_requests_identify_ragflow(monkeypatch):
    calls = _capture_get(monkeypatch)
    tool, _captured, _outputs = _make_tool()

    tool._invoke(query="What is RAGFlow?")

    assert calls[0]["headers"]["User-Agent"] == "RAGFlow youdotcom-integration/infiniflow-ragflow"


def test_web_and_news_are_merged_with_passages_preferred(monkeypatch):
    _capture_get(monkeypatch)
    tool, captured, outputs = _make_tool()

    tool._invoke(query="What is RAGFlow?")

    assert captured["references"] == [
        {"title": "A", "url": "https://example.com/a", "content": "First passage. Second passage."},
        {"title": "B", "url": "https://news.example.com/b", "content": "News description only."},
    ]
    assert len(outputs["json"]) == 2


def test_top_n_caps_the_merged_sections(monkeypatch):
    payload = {
        "results": {
            "web": [{"url": f"https://example.com/w{i}", "title": f"W{i}", "description": "d"} for i in range(6)],
            "news": [{"url": f"https://example.com/n{i}", "title": f"N{i}", "description": "d"} for i in range(6)],
        }
    }
    calls = _capture_get(monkeypatch, _FakeResponse(payload))
    tool, captured, _outputs = _make_tool()
    tool._param.top_n = 4

    tool._invoke(query="q")

    assert calls[0]["params"]["count"] == 4
    # `count` applies per section, so the merged list is trimmed on our side.
    assert [r["title"] for r in captured["references"]] == ["W0", "W1", "W2", "W3"]


def test_freshness_is_forwarded_only_when_a_window_is_set(monkeypatch):
    calls = _capture_get(monkeypatch)
    tool, _captured, _outputs = _make_tool()

    tool._invoke(query="q", freshness="week")
    assert calls[0]["params"]["freshness"] == "week"

    # Blank and `any` both mean no restriction.
    tool._invoke(query="q", freshness="")
    assert "freshness" not in calls[1]["params"]

    tool._invoke(query="q", freshness="any")
    assert "freshness" not in calls[2]["params"]


def test_an_unsupported_freshness_is_rejected_before_the_request(monkeypatch):
    calls = _capture_get(monkeypatch)
    tool, _captured, _outputs = _make_tool()

    try:
        tool._invoke(query="q", freshness="decade")
    except ValueError as e:
        assert "decade" in str(e)
        assert calls == []
        return
    raise AssertionError("expected an unsupported freshness to raise")


def test_the_published_schema_restricts_freshness():
    freshness = YouComSearchParam().get_meta()["function"]["parameters"]["properties"]["freshness"]

    assert freshness["enum"] == ["any", "day", "week", "month", "year"]


def test_blank_query_short_circuits(monkeypatch):
    calls = _capture_get(monkeypatch)
    tool, _captured, outputs = _make_tool()

    assert tool._invoke(query="") == ""
    assert calls == []
    assert outputs["formalized_content"] == ""


def test_failures_never_log_the_query_or_the_key(monkeypatch, caplog):
    """The query is a URL parameter, so the requests error message contains it."""
    _capture_get(monkeypatch, _FakeResponse({}, status_code=402))
    tool, _captured, _outputs = _make_tool(api_key="ydc-secret")

    with caplog.at_level("ERROR"):
        result = tool._invoke(query="my private query")

    assert "my%20private%20query" not in caplog.text
    assert "ydc-secret" not in caplog.text
    assert "my%20private%20query" not in str(result)
    assert "HTTPError" in str(result)


def _capture_sleep(monkeypatch):
    """Record the retry delays without patching the stdlib for other threads."""
    slept = []

    class _Clock:
        @staticmethod
        def sleep(seconds):
            slept.append(seconds)

    monkeypatch.setattr(youcom_module, "time", _Clock)
    return slept


def test_a_failed_final_attempt_does_not_sleep(monkeypatch):
    """The delay only buys something when another attempt follows it."""
    _capture_get(monkeypatch, _FakeResponse({}, status_code=402))
    slept = _capture_sleep(monkeypatch)
    tool, _captured, _outputs = _make_tool()
    tool._param.max_retries = 0
    tool._param.delay_after_error = 5

    tool._invoke(query="q")

    assert slept == []


def test_retries_sleep_between_attempts(monkeypatch):
    calls = _capture_get(monkeypatch, _FakeResponse({}, status_code=402))
    slept = _capture_sleep(monkeypatch)
    tool, _captured, _outputs = _make_tool()
    tool._param.max_retries = 2
    tool._param.delay_after_error = 5

    tool._invoke(query="q")

    assert len(calls) == 3
    # Two gaps between three attempts, and nothing after the last one.
    assert slept == [5, 5]


def test_param_check_rejects_a_non_positive_top_n():
    param = YouComSearchParam()
    param.top_n = 0

    try:
        param.check()
    except Exception:
        return
    raise AssertionError("expected check() to reject top_n=0")
