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


class _Canvas:
    def is_canceled(self):
        return False


def _jin10(param=None):
    cpn = Jin10.__new__(Jin10)
    cpn._canvas = _Canvas()
    cpn._param = param or Jin10Param()
    return cpn


def _flash_response():
    response = MagicMock()
    response.json.return_value = {
        "data": [
            {"data": {"content": "央行公布最新利率"}},
            {"data": {"content": "美股三大指数收涨"}},
            {"data": {"content": "原油价格波动"}},
        ]
    }
    return response


@pytest.mark.p1
def test_jin10_param_exposes_tool_descriptor():
    # The Agent runtime builds each tool's function descriptor via get_meta();
    # without it, constructing an Agent with the tool crashes (#18447).
    meta = Jin10Param().get_meta()
    assert meta["function"]["name"] == "jin10_market_data"
    query = meta["function"]["parameters"]["properties"]["query"]
    assert query["type"] == "string"
    # required is the normalized top-level array; query is optional.
    assert meta["function"]["parameters"]["required"] == []


@pytest.mark.p1
def test_jin10_flash_returns_joined_content():
    cpn = _jin10()

    with patch("agent.tools.jin10.requests.get", return_value=_flash_response()):
        result = cpn._invoke()

    assert "央行公布最新利率" in result
    assert "美股三大指数收涨" in result


@pytest.mark.p1
def test_jin10_query_filters_content_client_side():
    cpn = _jin10()

    with patch("agent.tools.jin10.requests.get", return_value=_flash_response()):
        result = cpn._invoke(query="美股")

    assert "美股三大指数收涨" in result
    assert "央行公布最新利率" not in result


@pytest.mark.p1
def test_jin10_endpoint_filter_config_skips_client_side_filter():
    param = Jin10Param()
    param.filter = "美股"
    cpn = _jin10(param)

    with patch("agent.tools.jin10.requests.get", return_value=_flash_response()):
        result = cpn._invoke(query="原油")

    # contain/filter already configured server-side: the invoked query must
    # not double-filter the payload.
    assert "央行公布最新利率" in result


@pytest.mark.p1
def test_jin10_exception_is_surfaced_as_error_not_content():
    cpn = _jin10()

    with patch("agent.tools.jin10.requests.get", side_effect=OSError("network unreachable")):
        result = cpn._invoke()

    assert "network unreachable" in result
    assert cpn.error() == "network unreachable"
