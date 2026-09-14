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

from agent.tools.akshare import AkShare, AkShareParam


def _make_tool(top_n=10):
    # Bypass the canvas-bound __init__ (mirrors test_pubmed_unit.py) and stub the
    # canvas-touching helpers so we can exercise _invoke's execution path.
    tool = AkShare.__new__(AkShare)
    param = AkShareParam()
    param.top_n = top_n
    tool._param = param
    tool.check_if_canceled = lambda *a, **k: False
    out = {}
    tool.set_output = lambda k, v: out.__setitem__(k, v)
    tool.output = lambda k=None: out.get(k) if k else out
    return tool, out


def _fake_news_df(n):
    import pandas as pd

    return pd.DataFrame(
        [
            {
                "新闻链接": f"https://u{i}",
                "新闻标题": f"title{i}",
                "新闻内容": f"content{i}",
                "发布时间": "2026-01-01",
                "文章来源": "src",
            }
            for i in range(n)
        ]
    )


def test_param_instantiates():
    AkShareParam()


def test_check_passes_with_defaults():
    AkShareParam().check()


def test_meta_exposes_query_parameter():
    # Regression: AkShare extended ComponentBase and defined no `meta`, so it
    # had no get_meta() and crashed agent_with_tools when added to an Agent.
    meta = AkShareParam().get_meta()
    params = meta["function"]["parameters"]
    assert "query" in params["properties"]
    assert "query" in params["required"]


def test_check_rejects_non_positive_top_n():
    param = AkShareParam()
    param.top_n = 0
    with pytest.raises(ValueError):
        param.check()


def test_invoke_returns_content_and_sets_formalized_content(monkeypatch):
    # Regression for the restored runtime path: _invoke(query=...) must fetch
    # news, return the formatted content, write it to formalized_content, and
    # respect top_n.
    pytest.importorskip("akshare")
    import akshare

    monkeypatch.setattr(akshare, "stock_news_em", lambda symbol: _fake_news_df(5))

    tool, out = _make_tool(top_n=2)
    res = tool._invoke(query="600519")

    assert "title0" in res and "https://u0" in res
    assert out["formalized_content"] == res
    # top_n is applied via .head(top_n): only 2 articles formatted.
    assert res.count("新闻内容:") == 2


def test_invoke_empty_query_returns_empty():
    # Empty query short-circuits without calling akshare.
    tool, out = _make_tool()
    assert tool._invoke(query="") == ""
    assert out.get("formalized_content") == ""
