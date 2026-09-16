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

import pytest

# Google imports the `serpapi` SDK at module load; skip where absent.
pytest.importorskip("serpapi")

import agent.tools.google as g_module  # noqa: E402
from agent.tools.google import Google, GoogleParam  # noqa: E402


class _FakeSearch:
    """Stands in for serpapi.GoogleSearch; get_dict() returns a canned response."""

    def __init__(self, result):
        self._result = result

    def get_dict(self):
        return self._result


def _make_tool():
    # Bypass the canvas-bound __init__ (mirrors test_googlescholar.py) and stub the
    # canvas-touching helpers so we can exercise _invoke's response handling. Zero the
    # error delay so the failing path doesn't sleep.
    g = Google.__new__(Google)
    param = GoogleParam()
    param.api_key = "k"
    param.max_retries = 0
    param.delay_after_error = 0
    g._param = param
    g.check_if_canceled = lambda *a, **k: False

    captured = {}
    out = {}

    def fake_retrieve(res_list, get_title=None, get_url=None, get_content=None, **_kw):
        # The real _retrieve_chunks applies these getters to every result; replicate
        # that so the tests actually exercise the lambdas, which is where the
        # per-result KeyErrors came from.
        items = list(res_list)
        captured["chunks"] = items
        captured["rendered"] = [{"title": get_title(r), "url": get_url(r), "content": get_content(r)} for r in items]
        out["formalized_content"] = "FC"

    g._retrieve_chunks = fake_retrieve
    g.set_output = lambda k, v: out.__setitem__(k, v)
    g.output = lambda k=None: out.get(k) if k else out
    return g, captured, out


def test_error_response_surfaces_serpapi_message(monkeypatch):
    # Regression: on an invalid key / exhausted quota / no-match, serpapi returns
    # {"error": ...} with no "organic_results". The tool used to raise
    # KeyError('organic_results'), reported to the model as the opaque
    # "Google error: 'organic_results'". It must surface serpapi's real message.
    monkeypatch.setattr(g_module, "GoogleSearch", lambda params: _FakeSearch({"error": "Invalid API key."}))
    g, _, out = _make_tool()
    result = g._invoke(q="anything")
    assert "Invalid API key." in result
    assert "organic_results" not in result
    assert out.get("_ERROR") == "Invalid API key."


def test_valid_response_returns_results(monkeypatch):
    results = [{"title": "t", "link": "u", "snippet": "s"}]
    monkeypatch.setattr(g_module, "GoogleSearch", lambda params: _FakeSearch({"organic_results": results}))
    g, captured, out = _make_tool()
    g._invoke(q="anything")
    assert captured["chunks"] == results
    assert captured["rendered"] == [{"title": "t", "url": "u", "content": "s"}]
    assert out["json"] == results


def test_result_missing_optional_fields_does_not_raise(monkeypatch):
    # Regression: get_content's fallback `r["snippet"]` was evaluated eagerly, so a
    # result carrying a description but no snippet raised KeyError('snippet') -- the
    # very failure mode this tool's error handling exists to prevent. title and link
    # may be absent on a result too.
    results = [{"about_this_result": {"source": {"description": "d"}}}]
    monkeypatch.setattr(g_module, "GoogleSearch", lambda params: _FakeSearch({"organic_results": results}))
    g, captured, _ = _make_tool()
    g._invoke(q="anything")
    assert captured["rendered"] == [{"title": "", "url": "", "content": "d"}]
