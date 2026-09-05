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
"""Tests for the CRAWL4AI_SERVER_URL remote-server fallback in agent/tools/crawler.py."""
from types import SimpleNamespace

import pytest

from agent.tools import crawler as crawler_module
from agent.tools.crawler import Crawler


def _make_crawler(extract_type: str = "markdown") -> Crawler:
    """Build a Crawler instance without going through ToolBase.__init__.

    _fetch_remote only touches self._param.extract_type and HTTP, so a
    SimpleNamespace param is enough to exercise it in isolation.
    """
    inst = Crawler.__new__(Crawler)
    inst._param = SimpleNamespace(extract_type=extract_type)
    return inst


class _FakeResponse:
    def __init__(self, payload):
        self._payload = payload

    def raise_for_status(self):
        return None

    def json(self):
        return self._payload


@pytest.fixture
def post_returning(monkeypatch):
    """Factory that swaps requests.post to return a chosen payload, recording
    the single call it expects to receive."""

    def install(payload):
        calls = []

        def fake_post(url, json=None, timeout=None, **kwargs):
            calls.append({"url": url, "json": json, "timeout": timeout})
            return _FakeResponse(payload)

        monkeypatch.setattr(crawler_module.requests, "post", fake_post)
        return calls

    return install


# ---------------------------------------------------------------------------
# extract_type branches
# ---------------------------------------------------------------------------

def test_markdown_dict_returns_raw_markdown(post_returning):
    calls = post_returning({"results": [{"markdown": {"raw_markdown": "hello", "fit_markdown": "ignored"}}]})
    out = _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out == "hello"
    assert len(calls) == 1


def test_markdown_dict_falls_back_to_fit_markdown(post_returning):
    post_returning({"results": [{"markdown": {"fit_markdown": "fit-only", "raw_markdown": ""}}]})
    out = _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out == "fit-only"


def test_markdown_bare_string_supported(post_returning):
    """Older crawl4ai servers return a bare string in markdown."""
    post_returning({"results": [{"markdown": "old-style"}]})
    out = _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out == "old-style"


def test_markdown_missing_returns_empty_string(post_returning):
    post_returning({"results": [{}]})
    out = _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out == ""


def test_html_returns_cleaned_html(post_returning):
    post_returning({"results": [{"cleaned_html": "<p>hi</p>"}]})
    out = _make_crawler("html")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out == "<p>hi</p>"


def test_html_missing_returns_empty_string(post_returning):
    post_returning({"results": [{}]})
    out = _make_crawler("html")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out == ""


def test_content_passes_through_extracted_content(post_returning):
    post_returning({"results": [{"extracted_content": "stuff"}]})
    out = _make_crawler("content")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out == "stuff"


def test_content_preserves_none_when_no_extractor(post_returning):
    """In-process crawl4ai also returns None for extracted_content without an
    extraction strategy — the remote path must match that contract."""
    post_returning({"results": [{"extracted_content": None}]})
    out = _make_crawler("content")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out is None


def test_empty_results_returns_empty_string(post_returning):
    post_returning({"results": []})
    out = _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out == ""


def test_missing_results_key_returns_empty_string(post_returning):
    post_returning({})
    out = _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert out == ""


# ---------------------------------------------------------------------------
# HTTP shape: URL, payload, timeout
# ---------------------------------------------------------------------------

def test_posts_to_crawl_endpoint_with_urls_list(post_returning):
    calls = post_returning({"results": [{"markdown": "x"}]})
    _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com/page")
    assert calls[0]["url"] == "http://crawl4ai:11235/crawl"
    assert calls[0]["json"] == {"urls": ["https://example.com/page"]}


def test_default_timeout_is_120_seconds(post_returning, monkeypatch):
    monkeypatch.delenv("CRAWL4AI_REQUEST_TIMEOUT", raising=False)
    calls = post_returning({"results": [{"markdown": "x"}]})
    _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert calls[0]["timeout"] == 120


def test_request_timeout_env_var_is_honoured(post_returning, monkeypatch):
    monkeypatch.setenv("CRAWL4AI_REQUEST_TIMEOUT", "37")
    calls = post_returning({"results": [{"markdown": "x"}]})
    _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert calls[0]["timeout"] == 37


@pytest.mark.parametrize("bad_value", ["abc", "0", "-5", "", "  "])
def test_invalid_timeout_env_falls_back_to_default(post_returning, monkeypatch, bad_value):
    """Non-integer, zero, or negative timeouts must not crash the crawl."""
    monkeypatch.setenv("CRAWL4AI_REQUEST_TIMEOUT", bad_value)
    calls = post_returning({"results": [{"markdown": "x"}]})
    _make_crawler("markdown")._fetch_remote("http://crawl4ai:11235", "https://example.com")
    assert calls[0]["timeout"] == 120
