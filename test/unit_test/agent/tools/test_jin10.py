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

from agent.tools.jin10 import Jin10, Jin10Param


def _make_tool(param=None):
    # Bypass the canvas-bound __init__ (mirrors test_akshare.py) and stub the
    # canvas-touching helpers so we can exercise _invoke's execution path.
    tool = Jin10.__new__(Jin10)
    tool._param = param or Jin10Param()
    tool.check_if_canceled = lambda *a, **k: False
    out = {}
    tool.set_output = lambda k, v: out.__setitem__(k, v)
    tool.output = lambda k=None: out.get(k) if k else out
    return tool, out


def _response(payload):
    response = MagicMock()
    response.json.return_value = payload
    return response


def test_param_instantiates():
    Jin10Param()


def test_check_passes_with_defaults():
    Jin10Param().check()


def test_check_rejects_invalid_type():
    param = Jin10Param()
    param.type = "nope"
    with pytest.raises(ValueError):
        param.check()


def test_check_rejects_invalid_calendar_datatype():
    param = Jin10Param()
    param.calendar_datatype = "nope"
    with pytest.raises(ValueError):
        param.check()


def test_meta_exposes_query_parameter():
    # Regression: Jin10 extended ComponentBase and defined no `meta`, so it had
    # no get_meta() and crashed agent_with_tools when added to an Agent.
    meta = Jin10Param().get_meta()
    params = meta["function"]["parameters"]
    assert "query" in params["properties"]
    assert "query" in params["required"]


@pytest.mark.p1
def test_invoke_flash_returns_content_and_sets_formalized_content():
    # Regression for the restored runtime path: Jin10 only implemented the
    # legacy _run, so _invoke fell through to ComponentBase._invoke and raised
    # NotImplementedError.
    tool, out = _make_tool()
    payload = {"data": [{"data": {"content": "gold up"}}, {"data": {"content": "oil down"}}]}

    with patch("agent.tools.jin10.requests.get", return_value=_response(payload)):
        res = tool._invoke(query="gold")

    assert "gold up" in res
    assert "oil down" in res
    assert out["formalized_content"] == res


def test_invoke_passes_query_as_contain_filter():
    tool, _ = _make_tool()
    payload = {"data": []}

    with patch("agent.tools.jin10.requests.get", return_value=_response(payload)) as get:
        tool._invoke(query="gold")

    body = get.call_args.kwargs["data"]
    assert '"contain": "gold"' in body


@pytest.mark.p1
def test_invoke_calendar_returns_markdown_table():
    param = Jin10Param()
    param.type = "calendar"
    tool, out = _make_tool(param)
    payload = {"data": [{"country": "US", "event": "CPI"}]}

    with patch("agent.tools.jin10.requests.get", return_value=_response(payload)):
        res = tool._invoke(query="CPI")

    assert "CPI" in res
    assert out["formalized_content"] == res


def test_invoke_empty_query_returns_empty():
    # Empty query short-circuits without calling the Jin10 API.
    tool, out = _make_tool()
    with patch("agent.tools.jin10.requests.get") as get:
        assert tool._invoke(query="") == ""
    assert out.get("formalized_content") == ""
    get.assert_not_called()


@pytest.mark.p1
def test_invoke_surfaces_error_on_request_failure():
    tool, out = _make_tool()

    with patch("agent.tools.jin10.requests.get", side_effect=RuntimeError("boom")):
        res = tool._invoke(query="gold")

    assert "boom" in res
    assert "boom" in out["_ERROR"]
