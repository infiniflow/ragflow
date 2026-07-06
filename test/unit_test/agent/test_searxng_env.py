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
"""Tests for SEARXNG_URL env-var fallback in agent/tools/searxng.py.

These tests bypass ToolBase.__init__ via ``__new__`` and inject only the
attributes _invoke touches, so the precedence chain
(per-tool > kwargs > env) can be exercised without a Canvas runtime.
"""
import contextlib
from types import SimpleNamespace

import pytest

from agent.tools import searxng as searxng_module
from agent.tools.searxng import SearXNG


class _FakeGetResponse:
    def __init__(self, payload):
        self._payload = payload

    def raise_for_status(self):
        return None

    def json(self):
        return self._payload


def _make_searxng(per_tool_url: str = "") -> SearXNG:
    inst = SearXNG.__new__(SearXNG)
    inst._param = SimpleNamespace(
        searxng_url=per_tool_url,
        max_retries=0,
        delay_after_error=0,
        top_n=5,
    )
    # Stubs for ToolBase plumbing _invoke would otherwise call.
    inst.check_if_canceled = lambda *_a, **_k: False
    inst.set_output = lambda *_a, **_k: None
    inst._retrieve_chunks = lambda *_a, **_k: None
    inst.output = lambda *_a, **_k: "ok"
    return inst


@pytest.fixture
def captured_get(monkeypatch):
    """Patch requests.get in the searxng module and capture call args."""
    calls = []

    def fake_get(url, params=None, timeout=None, **kwargs):
        calls.append({"url": url, "params": params, "timeout": timeout})
        return _FakeGetResponse({"results": []})

    monkeypatch.setattr(searxng_module.requests, "get", fake_get)
    # SSRF guard always returns a valid hostname/ip pair in these tests.
    monkeypatch.setattr(searxng_module, "assert_url_is_safe", lambda u: ("host", "203.0.113.1"))
    # pin_dns is a contextmanager that we don't care about for the URL test.
    monkeypatch.setattr(searxng_module, "pin_dns", lambda *_a, **_k: contextlib.nullcontext())
    return calls


def test_falls_back_to_env_var_when_no_per_tool_url(captured_get, monkeypatch):
    monkeypatch.setenv("SEARXNG_URL", "http://env-searxng:9999")
    _make_searxng(per_tool_url="")._invoke(query="cats")
    assert len(captured_get) == 1
    assert captured_get[0]["url"] == "http://env-searxng:9999/search"


def test_per_tool_url_wins_over_env_var(captured_get, monkeypatch):
    monkeypatch.setenv("SEARXNG_URL", "http://env-searxng:9999")
    _make_searxng(per_tool_url="http://per-tool:1234")._invoke(query="cats")
    assert captured_get[0]["url"] == "http://per-tool:1234/search"


def test_kwargs_url_wins_over_env_var(captured_get, monkeypatch):
    monkeypatch.setenv("SEARXNG_URL", "http://env-searxng:9999")
    _make_searxng(per_tool_url="")._invoke(query="cats", searxng_url="http://kwargs:5555")
    assert captured_get[0]["url"] == "http://kwargs:5555/search"


def test_per_tool_url_wins_over_kwargs(captured_get, monkeypatch):
    monkeypatch.setenv("SEARXNG_URL", "http://env-searxng:9999")
    _make_searxng(per_tool_url="http://per-tool:1234")._invoke(
        query="cats", searxng_url="http://kwargs:5555"
    )
    assert captured_get[0]["url"] == "http://per-tool:1234/search"


def test_no_sources_set_skips_http_call(captured_get, monkeypatch):
    """No URL at all should produce no HTTP call (existing try-run behaviour)."""
    monkeypatch.delenv("SEARXNG_URL", raising=False)
    result = _make_searxng(per_tool_url="")._invoke(query="cats")
    assert result == ""
    assert captured_get == []


def test_empty_query_skips_http_call(captured_get, monkeypatch):
    monkeypatch.setenv("SEARXNG_URL", "http://env-searxng:9999")
    result = _make_searxng(per_tool_url="")._invoke(query="")
    assert result == ""
    assert captured_get == []


def test_whitespace_per_tool_url_falls_through_to_env(captured_get, monkeypatch):
    """A whitespace-only per-tool URL must not block the env fallback."""
    monkeypatch.setenv("SEARXNG_URL", "http://env-searxng:9999")
    _make_searxng(per_tool_url="   ")._invoke(query="cats")
    assert captured_get[0]["url"] == "http://env-searxng:9999/search"


def test_whitespace_kwargs_url_falls_through_to_env(captured_get, monkeypatch):
    """A whitespace-only kwargs URL must not block the env fallback."""
    monkeypatch.setenv("SEARXNG_URL", "http://env-searxng:9999")
    _make_searxng(per_tool_url="")._invoke(query="cats", searxng_url="   ")
    assert captured_get[0]["url"] == "http://env-searxng:9999/search"
