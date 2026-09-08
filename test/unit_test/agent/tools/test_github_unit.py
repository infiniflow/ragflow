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

import agent.tools.github as gh_module
from agent.tools.github import GitHub, GitHubParam


class _FakeResp:
    """Stands in for the requests.Response; json() returns a canned payload."""

    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


def _make_tool():
    # Bypass the canvas-bound __init__ (mirrors test_googlescholar.py) and stub the
    # canvas-touching helpers so we can exercise _invoke's response handling. Zero the
    # error delay so the failing path doesn't sleep.
    g = GitHub.__new__(GitHub)
    param = GitHubParam()
    param.top_n = 10
    param.max_retries = 0
    param.delay_after_error = 0
    g._param = param
    g.check_if_canceled = lambda *a, **k: False

    captured = {}
    out = {}

    def fake_retrieve(res_list, **_kw):
        captured["chunks"] = list(res_list)
        out["formalized_content"] = "FC"

    g._retrieve_chunks = fake_retrieve
    g.set_output = lambda k, v: out.__setitem__(k, v)
    g.output = lambda k=None: out.get(k) if k else out
    return g, captured, out


def test_rate_limit_response_surfaces_message(monkeypatch):
    # Regression: the github search api returns {"message": ...} with no "items" on a
    # rate limit / invalid query. The tool used to raise KeyError('items'), reported to
    # the model as the opaque "GitHub error: 'items'". It must surface the real message.
    monkeypatch.setattr(
        gh_module.requests,
        "get",
        lambda *a, **k: _FakeResp({"message": "API rate limit exceeded for x.", "documentation_url": "https://docs.github.com/rest"}),
    )
    g, _, out = _make_tool()
    result = g._invoke(query="anything")
    assert "API rate limit exceeded for x." in result
    assert "'items'" not in result
    assert out.get("_ERROR") == "API rate limit exceeded for x."


def test_valid_response_returns_items(monkeypatch):
    items = [{"name": "n", "html_url": "u", "description": "d", "watchers": 3}]
    monkeypatch.setattr(gh_module.requests, "get", lambda *a, **k: _FakeResp({"items": items}))
    g, captured, out = _make_tool()
    g._invoke(query="anything")
    assert captured["chunks"] == items
    assert out["json"] == items
