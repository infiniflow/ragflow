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

import pytest

pytest.importorskip("duckduckgo_search")

import agent.tools.duckduckgo as ddg_module  # noqa: E402
from agent.tools.duckduckgo import DuckDuckGo, DuckDuckGoParam  # noqa: E402


class _FakeDDGS:
    """Records which search method the tool chose."""

    calls: list[str] = []

    def __enter__(self):
        return self

    def __exit__(self, *_):
        return False

    def text(self, *_a, **_kw):
        _FakeDDGS.calls.append("text")
        return []

    def news(self, *_a, **_kw):
        _FakeDDGS.calls.append("news")
        return []


def _make_tool(channel):
    # Bypass the canvas-bound __init__ the way test_google_unit.py does, and stub
    # the canvas-touching helpers so _invoke's branch is the only thing exercised.
    tool = DuckDuckGo.__new__(DuckDuckGo)
    param = DuckDuckGoParam()
    param.max_retries = 0
    param.delay_after_error = 0
    param.channel = channel
    tool._param = param
    tool.check_if_canceled = lambda *a, **k: False
    tool._retrieve_chunks = lambda *a, **k: None
    tool.set_output = lambda *a, **k: None
    return tool


@pytest.fixture(autouse=True)
def _fake_ddgs(monkeypatch):
    _FakeDDGS.calls = []
    monkeypatch.setattr(ddg_module, "DDGS", _FakeDDGS)


@pytest.mark.p2
@pytest.mark.parametrize("channel", ["news"])
def test_news_channel_runs_a_news_search(channel):
    # The node's own parameter is what the canvas passes; selecting News must not
    # fall through to the general web search.
    _make_tool(channel)._invoke(query="q")
    assert _FakeDDGS.calls == ["news"]


@pytest.mark.p2
def test_news_channel_from_kwargs_runs_a_news_search():
    # An LLM tool call supplies `channel` in kwargs rather than on the param.
    _make_tool("text")._invoke(query="q", channel="news")
    assert _FakeDDGS.calls == ["news"]


@pytest.mark.p2
@pytest.mark.parametrize("channel", ["text", "general"])
def test_non_news_channels_run_a_text_search(channel):
    # Callers send either value for a web search; both must keep it.
    _make_tool(channel)._invoke(query="q")
    assert _FakeDDGS.calls == ["text"]


@pytest.mark.p2
def test_blank_channel_falls_back_to_the_param():
    # A blank kwarg must not be read as a channel; the node's setting wins.
    _make_tool("news")._invoke(query="q", channel="")
    assert _FakeDDGS.calls == ["news"]
