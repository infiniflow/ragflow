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

from unittest.mock import MagicMock, patch

import pytest

from agent.tools.tushare import TuShare, TuShareParam


def _make_tool(param=None):
    # Bypass the canvas-bound __init__ (mirrors test_akshare.py) and stub the
    # canvas-touching helpers so we can exercise _invoke's execution path.
    tool = TuShare.__new__(TuShare)
    tool._param = param or TuShareParam()
    tool.check_if_canceled = lambda *a, **k: False
    out = {}
    tool.set_output = lambda k, v: out.__setitem__(k, v)
    tool.output = lambda k=None: out.get(k) if k else out
    return tool, out


def _response(code=0, msg=""):
    response = MagicMock()
    response.json.return_value = {
        "code": code,
        "msg": msg,
        "data": {
            "items": [
                [1, "Apple earnings beat expectations"],
                [2, "Google cloud revenue grows"],
                [3, "C++ regex special chars should not break filtering"],
            ],
            "fields": ["id", "content"],
        },
    }
    return response


def test_param_instantiates():
    TuShareParam()


def test_check_passes_with_defaults():
    TuShareParam().check()


def test_check_rejects_invalid_src():
    param = TuShareParam()
    param.src = "nope"
    with pytest.raises(ValueError):
        param.check()


def test_meta_exposes_query_parameter():
    # Regression: TuShare extended ComponentBase and defined no `meta`, so it
    # had no get_meta() and crashed agent_with_tools when added to an Agent.
    meta = TuShareParam().get_meta()
    params = meta["function"]["parameters"]
    assert "query" in params["properties"]
    assert "query" in params["required"]


@pytest.mark.p1
def test_tushare_filters_with_tool_query_when_param_empty():
    # Regression for the restored runtime path: TuShare only implemented the
    # legacy _run, so _invoke fell through to ComponentBase._invoke and raised
    # NotImplementedError.
    tool, out = _make_tool()

    with patch("agent.tools.tushare.requests.post", return_value=_response()):
        res = tool._invoke(query="Apple")

    assert "Apple" in res
    assert "Google" not in res
    assert out["formalized_content"] == res


@pytest.mark.p1
def test_tushare_prefers_explicit_param_keyword_over_tool_query():
    param = TuShareParam()
    param.keyword = "Google"
    tool, _ = _make_tool(param)

    with patch("agent.tools.tushare.requests.post", return_value=_response()):
        res = tool._invoke(query="Apple")

    assert "Google" in res
    assert "Apple" not in res


@pytest.mark.p1
def test_tushare_treats_keyword_as_literal_text():
    param = TuShareParam()
    param.keyword = "C++"
    tool, _ = _make_tool(param)

    with patch("agent.tools.tushare.requests.post", return_value=_response()):
        res = tool._invoke(query="ignored")

    assert "C++ regex special chars should not break filtering" in res


def test_invoke_empty_query_returns_empty():
    # Empty query short-circuits without calling the TuShare API.
    tool, out = _make_tool()
    with patch("agent.tools.tushare.requests.post") as post:
        assert tool._invoke(query="") == ""
    assert out.get("formalized_content") == ""
    post.assert_not_called()


def test_invoke_surfaces_api_error_message():
    tool, out = _make_tool()

    with patch("agent.tools.tushare.requests.post", return_value=_response(code=2002, msg="token invalid")):
        res = tool._invoke(query="Apple")

    assert res == "token invalid"
    assert out["_ERROR"] == "token invalid"


@pytest.mark.p1
def test_invoke_surfaces_error_on_request_failure():
    tool, out = _make_tool()

    with patch("agent.tools.tushare.requests.post", side_effect=RuntimeError("boom")):
        res = tool._invoke(query="Apple")

    assert "boom" in res
    assert "boom" in out["_ERROR"]
