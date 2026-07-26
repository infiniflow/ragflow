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

# GoogleScholar imports the `scholarly` SDK at module load; skip where absent.
pytest.importorskip("scholarly")

import agent.tools.googlescholar as gs_module  # noqa: E402
from agent.tools.googlescholar import GoogleScholar, GoogleScholarParam  # noqa: E402


def _fake_pubs(n, consumed=None):
    """A lazy generator, exactly like scholarly.search_pubs.

    When ``consumed`` (a one-element list) is passed, it counts how many items
    are actually pulled from the generator, so a test can prove the tool stops
    after top_n instead of draining the whole result stream.
    """
    for i in range(n):
        if consumed is not None:
            consumed[0] += 1
        yield {"bib": {"title": f"t{i}", "author": ["A"], "abstract": "x"}, "pub_url": f"u{i}"}


def _make_tool(top_n):
    # Bypass the canvas-bound __init__ (mirrors test_pubmed_unit.py) and stub the
    # canvas-touching helpers so we can exercise _invoke's generator handling.
    gs = GoogleScholar.__new__(GoogleScholar)
    param = GoogleScholarParam()
    param.top_n = top_n
    gs._param = param
    gs.check_if_canceled = lambda *a, **k: False

    captured = {}
    out = {}

    def fake_retrieve(res_list, **_kw):
        # The real _retrieve_chunks iterates its argument, which exhausts a
        # generator; replicate that to expose the original bug.
        items = list(res_list)
        captured["chunks_count"] = len(items)
        out["formalized_content"] = "FC"

    gs._retrieve_chunks = fake_retrieve
    gs.set_output = lambda k, v: out.__setitem__(k, v)
    gs.output = lambda k=None: out.get(k) if k else out
    return gs, captured, out


def test_respects_top_n(monkeypatch):
    # Regression: top_n was never applied; the unbounded generator was passed
    # straight to _retrieve_chunks. Assert both that _retrieve_chunks saw a
    # sliced list AND that only top_n items were actually pulled from the
    # generator (the stream is not drained past top_n).
    consumed = [0]
    monkeypatch.setattr(gs_module.scholarly, "search_pubs", lambda *a, **k: _fake_pubs(30, consumed))
    gs, captured, _ = _make_tool(top_n=5)
    gs._invoke(query="q")
    assert captured["chunks_count"] == 5
    assert consumed[0] == 5


def test_json_output_not_exhausted(monkeypatch):
    # Regression: json was set from the already-consumed generator -> always [].
    monkeypatch.setattr(gs_module.scholarly, "search_pubs", lambda *a, **k: _fake_pubs(30))
    gs, _, out = _make_tool(top_n=5)
    gs._invoke(query="q")
    assert len(out["json"]) == 5
    assert out["json"], "json output must not be empty when there are results"


def test_empty_query_short_circuits(monkeypatch):
    # An empty query must short-circuit without hitting the SDK, and must clear
    # json so a reused instance can't surface stale results from a prior call.
    calls = [0]

    def spy(*a, **k):
        calls[0] += 1
        return _fake_pubs(30)

    monkeypatch.setattr(gs_module.scholarly, "search_pubs", spy)
    gs, _, out = _make_tool(top_n=5)
    out["json"] = ["stale"]  # simulate leftover output from a previous call
    assert gs._invoke(query="") == ""
    assert calls[0] == 0, "search_pubs must not be called for an empty query"
    assert out.get("formalized_content") == ""
    assert out.get("json") == []
