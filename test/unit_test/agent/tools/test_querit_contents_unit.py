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
from agent.tools.querit import QueritContents, QueritContentsParam


class _FakeResponse:
    def __init__(self, payload, status_code=200):
        self._payload = payload
        self.status_code = status_code

    def json(self):
        return self._payload

    def raise_for_status(self):
        if self.status_code >= 400:
            raise querit_module.requests.HTTPError(f"{self.status_code} error", response=self)


def _make_tool(api_key="test-api-key"):
    tool = QueritContents.__new__(QueritContents)
    param = QueritContentsParam()
    param.api_key = api_key
    param.delay_after_error = 0
    tool._param = param
    tool.check_if_canceled = lambda *args, **kwargs: False

    outputs = {}
    tool.set_output = lambda key, value: outputs.__setitem__(key, value)
    tool.output = lambda key=None: outputs.get(key) if key else outputs
    return tool, outputs


def test_contents_posts_documented_defaults_and_preserves_response(monkeypatch):
    raw_response = {
        "error_code": 0,
        "error_msg": "",
        "search_id": "crawl-1",
        "results": [
            {
                "id": "1",
                "url": "https://example.com/article",
                "content": "# Article",
                "extrasMeta": {"title": "Article", "siteName": "Example"},
            }
        ],
        "statuses": [{"id": "1", "status": "success"}],
        "searchTime": 1,
    }
    calls = []

    def fake_post(url, **kwargs):
        calls.append((url, kwargs))
        return _FakeResponse(raw_response)

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, outputs = _make_tool()

    result = tool._invoke(urls=["https://example.com/article"])

    assert result == raw_response
    assert calls == [
        (
            "https://api.querit.ai/v1/contents",
            {
                "headers": {
                    "Accept": "application/json",
                    "Authorization": "Bearer test-api-key",
                    "Content-Type": "application/json",
                },
                "json": {
                    "urls": ["https://example.com/article"],
                    "format": "markdown",
                    "crawlTimeout": 10,
                    "extrasMeta": False,
                },
                "timeout": querit_module.DEFAULT_TIMEOUT,
            },
        )
    ]
    assert outputs["json"] == raw_response


def test_contents_accepts_canvas_url_string_and_runtime_options(monkeypatch):
    calls = []

    def fake_post(url, **kwargs):
        calls.append(kwargs)
        return _FakeResponse({"results": [], "statuses": []})

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, _ = _make_tool()

    tool._invoke(
        urls="https://one.example, https://two.example",
        format="html",
        crawl_timeout=60,
        extras_meta=True,
    )

    assert calls[0]["json"] == {
        "urls": ["https://one.example", "https://two.example"],
        "format": "html",
        "crawlTimeout": 60,
        "extrasMeta": True,
    }
    assert calls[0]["timeout"] == 65


def test_contents_param_normalizes_canvas_url_string_before_validation():
    param = QueritContentsParam()
    param.urls = "https://one.example, https://two.example"
    param.format = "markdown"
    param.crawl_timeout = 10
    param.extras_meta = False

    param.check()

    assert param.urls == ["https://one.example", "https://two.example"]
    assert param.get_input_form() == {"urls": {"name": "URLs", "type": "line"}}


@pytest.mark.parametrize(
    ("kwargs", "message"),
    [
        ({"urls": []}, "between 1 and 10"),
        ({"urls": [f"https://{index}.example" for index in range(11)]}, "between 1 and 10"),
        ({"urls": ["example.com"]}, "absolute HTTP or HTTPS"),
        ({"urls": ["file:///tmp/page"]}, "absolute HTTP or HTTPS"),
        ({"urls": ["https://example.com"], "format": "xml"}, "format must be"),
        ({"urls": ["https://example.com"], "crawl_timeout": 0}, "integer from 1 to 60"),
        ({"urls": ["https://example.com"], "extras_meta": 1}, "must be a boolean"),
    ],
)
def test_contents_rejects_invalid_inputs(kwargs, message):
    tool, outputs = _make_tool()

    result = tool._invoke(**kwargs)

    assert message in result
    assert message in outputs["_ERROR"]


@pytest.mark.parametrize(
    ("payload", "message"),
    [
        ([], "JSON object"),
        ({"results": {}}, "results must be an array"),
        ({"results": [], "statuses": {}}, "statuses must be an array"),
    ],
)
def test_contents_rejects_malformed_response(monkeypatch, payload, message):
    monkeypatch.setattr(querit_module.requests, "post", lambda *args, **kwargs: _FakeResponse(payload))
    tool, outputs = _make_tool()

    result = tool._invoke(urls=["https://example.com"])

    assert message in result
    assert message in outputs["_ERROR"]


def test_contents_redacts_api_key_from_errors_and_logs(monkeypatch, caplog):
    secret = f"secret-contents-key-{id(caplog)}"

    def fake_post(*args, **kwargs):
        raise querit_module.requests.ConnectionError(f"connection failed for {secret}")

    monkeypatch.setattr(querit_module.requests, "post", fake_post)
    tool, outputs = _make_tool(api_key=secret)

    with caplog.at_level(logging.ERROR):
        result = tool._invoke(urls=["https://example.com"])

    assert secret not in result
    assert secret not in outputs["_ERROR"]
    assert secret not in caplog.text
    assert "[REDACTED]" in result


def test_contents_contract_and_dynamic_discovery():
    import agent.tools as tools_package

    metadata = QueritContentsParam().get_meta()["function"]
    assert metadata["name"] == "querit_contents"
    assert metadata["parameters"]["required"] == ["urls"]
    assert set(metadata["parameters"]["properties"]) == {
        "urls",
        "format",
        "crawl_timeout",
        "extras_meta",
    }
    assert "api_key" not in metadata["parameters"]["properties"]
    assert tools_package.QueritContents is QueritContents
    assert tools_package.QueritContentsParam is QueritContentsParam
