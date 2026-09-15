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
"""Result-mapping regression tests for the Keenable agent tool.

The Keenable API returns both ``description`` and ``snippet`` on every result.
``description`` is frequently empty and ``snippet`` carries the page text, so
reading ``description`` handed retrieval a title and a URL with no content at
all. These tests apply the ``get_content`` getter the way ``_retrieve_chunks``
does, since that getter is where the bug lived.

``agent.tools.keenable`` is loaded in isolation (its package ``__init__`` would
auto-discover every tool and pull in the full agent framework), with the agent
base classes stubbed so only the real mapping runs.
"""

import importlib.util
import sys
import types
from pathlib import Path
from types import SimpleNamespace

_REPO_ROOT = Path(__file__).resolve().parents[4]


def _load_keenable_module():
    base = types.ModuleType("agent.tools.base")

    class _ToolParamBase:
        def __init__(self):
            pass

    class _ToolBase:
        def __init__(self, *a, **k):
            pass

    base.ToolParamBase = _ToolParamBase
    base.ToolBase = _ToolBase
    base.ToolMeta = dict
    for pkg in ("agent", "agent.tools"):
        sys.modules.setdefault(pkg, types.ModuleType(pkg))
    sys.modules["agent.tools.base"] = base

    # Neutralize the @timeout decorator so _invoke is a plain method.
    conn_utils = types.ModuleType("common.connection_utils")
    conn_utils.timeout = lambda *a, **k: lambda f: f
    sys.modules["common.connection_utils"] = conn_utils

    spec = importlib.util.spec_from_file_location("keenable_uut", _REPO_ROOT / "agent" / "tools" / "keenable.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


_keenable_mod = _load_keenable_module()
KeenableSearch = _keenable_mod.KeenableSearch


def _build_tool():
    cpn = KeenableSearch.__new__(KeenableSearch)
    cpn._canvas = SimpleNamespace()
    cpn._param = SimpleNamespace(mode="pro", top_n=10, api_key="", max_retries=0, delay_after_error=0)
    cpn.check_if_canceled = lambda *a, **k: False

    captured = {}
    out = {}

    def fake_retrieve(res_list, get_title=None, get_url=None, get_content=None, **_kw):
        # The real _retrieve_chunks applies these getters to every result;
        # replicate that so the tests actually exercise them.
        items = list(res_list)
        captured["rendered"] = [{"title": get_title(r), "url": get_url(r), "content": get_content(r)} for r in items]
        out["formalized_content"] = "FC"

    cpn._retrieve_chunks = fake_retrieve
    cpn.set_output = lambda key, value: out.__setitem__(key, value)
    cpn.output = lambda key=None: out.get(key) if key else out
    return cpn, captured, out


def _respond_with(monkeypatch, results):
    monkeypatch.setattr(_keenable_mod, "_request", lambda *a, **k: {"results": results})


def test_content_comes_from_snippet(monkeypatch):
    # A realistic result: `description` empty, `snippet` carrying the page text.
    _respond_with(
        monkeypatch,
        [{"title": "One", "url": "https://example.com/one", "description": "", "snippet": "First page text"}],
    )
    cpn, captured, _ = _build_tool()

    cpn._invoke(query="anything")

    assert captured["rendered"] == [{"title": "One", "url": "https://example.com/one", "content": "First page text"}]


def test_content_falls_back_to_description(monkeypatch):
    _respond_with(monkeypatch, [{"title": "One", "url": "https://example.com/one", "description": "A description"}])
    cpn, captured, _ = _build_tool()

    cpn._invoke(query="anything")

    assert captured["rendered"][0]["content"] == "A description"


def test_content_is_kept_whole_with_newlines_normalized(monkeypatch):
    # Snippets arrive as raw page text. These chunks feed retrieval, so the text
    # must not be truncated — only the newlines it arrives with are normalized.
    body = "line one\n\nline two" + " padding" * 500
    _respond_with(
        monkeypatch,
        [{"title": "One", "url": "https://example.com/one", "description": "", "snippet": body}],
    )
    cpn, captured, _ = _build_tool()

    cpn._invoke(query="anything")

    content = captured["rendered"][0]["content"]
    assert "\n" not in content
    assert content.startswith("line one line two")
    assert content == " ".join(body.split())


def test_missing_text_yields_empty_content(monkeypatch):
    _respond_with(monkeypatch, [{"title": "One", "url": "https://example.com/one"}])
    cpn, captured, _ = _build_tool()

    cpn._invoke(query="anything")

    assert captured["rendered"][0]["content"] == ""


def test_blank_query_short_circuits():
    cpn, _, out = _build_tool()

    assert cpn._invoke(query="") == ""
    assert out["formalized_content"] == ""
