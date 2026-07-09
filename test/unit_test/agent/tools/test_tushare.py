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


class _Canvas:
    def is_canceled(self):
        return False


def _tushare(param=None):
    cpn = TuShare.__new__(TuShare)
    cpn._canvas = _Canvas()
    cpn._param = param or TuShareParam()
    return cpn


def _response():
    response = MagicMock()
    response.json.return_value = {
        "code": 0,
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


@pytest.mark.p1
def test_tushare_filters_with_upstream_keyword_when_param_empty():
    cpn = _tushare()

    with patch.object(TuShare, "get_input", return_value={"content": ["Apple"]}):
        with patch("agent.tools.tushare.requests.post", return_value=_response()):
            result = cpn._run([])

    text = result.iloc[0]["content"]
    assert "Apple" in text
    assert "Google" not in text


@pytest.mark.p1
def test_tushare_prefers_explicit_param_keyword_over_upstream_input():
    param = TuShareParam()
    param.keyword = "Google"
    cpn = _tushare(param)

    with patch.object(TuShare, "get_input", return_value={"content": ["Apple"]}):
        with patch("agent.tools.tushare.requests.post", return_value=_response()):
            result = cpn._run([])

    text = result.iloc[0]["content"]
    assert "Google" in text
    assert "Apple" not in text


@pytest.mark.p1
def test_tushare_treats_keyword_as_literal_text():
    param = TuShareParam()
    param.keyword = "C++"
    cpn = _tushare(param)

    with patch.object(TuShare, "get_input", return_value={"content": ["ignored"]}):
        with patch("agent.tools.tushare.requests.post", return_value=_response()):
            result = cpn._run([])

    text = result.iloc[0]["content"]
    assert "C++ regex special chars should not break filtering" in text
