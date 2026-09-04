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
"""Regression tests for SearXNG redirect SSRF protection."""

import contextlib
import importlib.util
import sys
import types
from pathlib import Path
from types import SimpleNamespace
from urllib.parse import urlparse

import pytest

_REPO_ROOT = Path(__file__).resolve().parents[4]


def _load_searxng_module():
    base = types.ModuleType("agent.tools.base")

    class _ToolParamBase:
        def __init__(self):
            pass

    class _ToolBase:
        def __init__(self, *args, **kwargs):
            pass

    base.ToolParamBase = _ToolParamBase
    base.ToolBase = _ToolBase
    base.ToolMeta = dict
    sys.modules.setdefault("agent", types.ModuleType("agent"))
    sys.modules.setdefault("agent.tools", types.ModuleType("agent.tools"))
    sys.modules["agent.tools.base"] = base

    connection_utils = types.ModuleType("common.connection_utils")
    connection_utils.timeout = lambda *args, **kwargs: lambda fn: fn
    sys.modules["common.connection_utils"] = connection_utils

    ssrf_guard = types.ModuleType("common.ssrf_guard")
    ssrf_guard.assert_url_is_safe = lambda url: (urlparse(url).hostname, "93.184.216.34")
    ssrf_guard.pin_dns = lambda *args, **kwargs: contextlib.nullcontext()
    sys.modules["common.ssrf_guard"] = ssrf_guard

    spec = importlib.util.spec_from_file_location("searxng_uut", _REPO_ROOT / "agent" / "tools" / "searxng.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


_searxng_mod = _load_searxng_module()


class _FakeResponse:
    def __init__(self, status_code, *, location=None, payload=None):
        self.status_code = status_code
        self.headers = {"Location": location} if location else {}
        self._payload = payload

    def raise_for_status(self):
        if self.status_code >= 400:
            raise _searxng_mod.requests.HTTPError(f"HTTP {self.status_code}")

    def json(self):
        return self._payload


def _make_tool():
    tool = _searxng_mod.SearXNG.__new__(_searxng_mod.SearXNG)
    tool._param = SimpleNamespace(
        searxng_url="https://search.example.test",
        max_retries=0,
        delay_after_error=0,
        top_n=10,
    )
    outputs = {}
    tool.check_if_canceled = lambda *args, **kwargs: False
    tool.set_output = lambda key, value: outputs.__setitem__(key, value)
    tool.output = lambda key=None: outputs.get(key) if key else outputs
    tool._retrieve_chunks = lambda *args, **kwargs: outputs.__setitem__("formalized_content", "FORMALIZED")
    return tool, outputs


@pytest.mark.p2
def test_redirect_to_link_local_is_rejected_before_following(monkeypatch):
    calls = []

    def fake_assert_url_is_safe(url):
        if "169.254.169.254" in url:
            raise ValueError("URL resolves to a non-public address")
        return "search.example.test", "93.184.216.34"

    def fake_get(url, **kwargs):
        calls.append((url, kwargs))
        return _FakeResponse(302, location="http://169.254.169.254/latest/meta-data")

    monkeypatch.setattr(_searxng_mod, "assert_url_is_safe", fake_assert_url_is_safe)
    monkeypatch.setattr(_searxng_mod.requests, "get", fake_get)
    tool, outputs = _make_tool()

    result = tool._invoke(query="private query")

    assert "SSRF guard blocked redirect" in result
    assert len(calls) == 1
    assert calls[0][1]["allow_redirects"] is False
    assert outputs["_ERROR"].startswith("SSRF guard blocked redirect")


@pytest.mark.p2
def test_safe_redirect_is_validated_and_followed_with_redirects_disabled(monkeypatch):
    validated = []
    calls = []
    responses = iter(
        [
            _FakeResponse(302, location="/search-next"),
            _FakeResponse(200, payload={"results": [{"title": "Result", "url": "https://example.com", "content": "text"}]}),
        ]
    )

    def fake_assert_url_is_safe(url):
        validated.append(url)
        return urlparse(url).hostname, "93.184.216.34"

    def fake_get(url, **kwargs):
        calls.append((url, kwargs))
        return next(responses)

    monkeypatch.setattr(_searxng_mod, "assert_url_is_safe", fake_assert_url_is_safe)
    monkeypatch.setattr(_searxng_mod.requests, "get", fake_get)
    tool, outputs = _make_tool()

    assert tool._invoke(query="safe query") == "FORMALIZED"
    assert validated == ["https://search.example.test", "https://search.example.test/search-next"]
    assert [url for url, _kwargs in calls] == ["https://search.example.test/search", "https://search.example.test/search-next"]
    assert all(kwargs["allow_redirects"] is False for _url, kwargs in calls)
    assert outputs["json"]
