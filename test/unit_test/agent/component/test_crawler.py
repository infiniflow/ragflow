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
import contextlib
import gc

import pytest

# Crawler imports the `crawl4ai` SDK at module load; skip where absent.
pytest.importorskip("crawl4ai")

from agent.tools.crawler import Crawler, CrawlerParam  # noqa: E402


@pytest.fixture(autouse=True)
def _close_event_loops():
    yield
    asyncio.set_event_loop(None)
    for obj in gc.get_objects():
        if isinstance(obj, asyncio.AbstractEventLoop) and not obj.is_closed() and not obj.is_running():
            obj.close()


def _make_tool():
    # Bypass the canvas-bound init and stub the canvas-touching helpers so we can
    # exercise the invoke execution path.
    crawler = Crawler.__new__(Crawler)
    crawler._param = CrawlerParam()
    crawler.check_if_canceled = lambda *a, **k: False
    out = {}
    crawler.set_output = lambda k, v: out.__setitem__(k, v)
    crawler.output = lambda k=None: out.get(k) if k else out
    return crawler, out


def test_param_instantiates():
    # Regression: CrawlerParam extends ToolParamBase, whose init reads
    # self.meta["parameters"]. Without meta, constructing the param raised
    # AttributeError, so any canvas containing a Crawler node failed to load.
    CrawlerParam()


def test_check_passes_with_defaults():
    CrawlerParam().check()


def test_meta_exposes_query_parameter():
    # The tool descriptor must advertise a required query parameter (the URL
    # to crawl) so an Agent LLM can call it. query matches the frontend
    # form field and the {sys.query} convention shared by the other tools.
    meta = CrawlerParam().get_meta()
    params = meta["function"]["parameters"]
    assert "query" in params["properties"]
    assert "query" in params["required"]


def test_check_rejects_invalid_extract_type():
    param = CrawlerParam()
    param.extract_type = "pdf"
    with pytest.raises(ValueError):
        param.check()


def test_invoke_returns_content_and_sets_formalized_content(monkeypatch):
    # Regression for the restored runtime path: _invoke(query=...) must fetch
    # the page, return its content, and write it to formalized_content.
    import common.ssrf_guard as ssrf

    monkeypatch.setattr(ssrf, "assert_url_is_safe", lambda url: ("example.com", "93.184.216.34"))
    monkeypatch.setattr(ssrf, "pin_dns_global", lambda *a, **k: contextlib.nullcontext())

    crawler, out = _make_tool()

    async def fake_get_web(url):
        return "PAGE CONTENT for " + url

    crawler.get_web = fake_get_web

    result = crawler._invoke(query="http://example.com")

    assert result == "PAGE CONTENT for http://example.com"
    assert out["formalized_content"] == "PAGE CONTENT for http://example.com"


def test_invoke_empty_query_returns_empty():
    # Empty query short-circuits without crawling.
    crawler, out = _make_tool()
    called = []

    async def fake_get_web(url):
        called.append(url)
        return "should not be used"

    crawler.get_web = fake_get_web

    assert crawler._invoke(query="") == ""
    assert out.get("formalized_content") == ""
    assert called == []


def test_invoke_rejects_unsafe_url(monkeypatch):
    # An unsafe URL is rejected before any crawl is attempted.
    import common.ssrf_guard as ssrf

    def _reject(url):
        raise ValueError("blocked")

    monkeypatch.setattr(ssrf, "assert_url_is_safe", _reject)

    crawler, out = _make_tool()
    called = []

    async def fake_get_web(url):
        called.append(url)
        return "should not be used"

    crawler.get_web = fake_get_web

    assert crawler._invoke(query="http://169.254.169.254/") == "URL not valid"
    assert out.get("_ERROR") == "URL not valid"
    assert called == []
