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

from agent.tools import qweather as qweather_mod
from agent.tools.qweather import QWeather, QWeatherParam


def _make_tool(**overrides):
    # Bypass the canvas-bound __init__ (mirrors test_pubmed_unit.py) and stub the
    # canvas-touching helpers so we can exercise _invoke's execution path.
    tool = QWeather.__new__(QWeather)
    param = QWeatherParam()
    param.web_apikey = "test-key"
    for k, v in overrides.items():
        setattr(param, k, v)
    tool._param = param
    tool.check_if_canceled = lambda *a, **k: False
    out = {}
    tool.set_output = lambda k, v: out.__setitem__(k, v)
    tool.output = lambda k=None: out.get(k) if k else out
    return tool, out


class _FakeResp:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


def _patch_requests(monkeypatch, payloads):
    """Return each payload in order for successive requests.get(...) calls."""
    calls = iter(payloads)

    def fake_get(*args, **kwargs):
        return _FakeResp(next(calls))

    monkeypatch.setattr(qweather_mod.requests, "get", fake_get)


def test_param_instantiates():
    QWeatherParam()


def test_check_passes_with_defaults():
    param = QWeatherParam()
    param.web_apikey = "test-key"
    param.check()


def test_meta_exposes_query_parameter():
    # Regression: QWeather extended ComponentBase and defined no `meta`, so it
    # had no get_meta() and crashed the Agent when added as a tool.
    meta = QWeatherParam().get_meta()
    params = meta["function"]["parameters"]
    assert "query" in params["properties"]
    assert "query" in params["required"]


def test_check_rejects_empty_apikey():
    param = QWeatherParam()
    param.web_apikey = ""
    with pytest.raises(ValueError):
        param.check()


def test_invoke_weather_now_returns_content(monkeypatch):
    # Regression for the restored runtime path: _invoke(query=...) resolves the
    # location, fetches current weather, returns the content, and mirrors it to
    # formalized_content.
    _patch_requests(
        monkeypatch,
        [
            {"code": "200", "location": [{"id": "101010100"}]},
            {"code": "200", "now": {"temp": "25", "text": "Sunny"}},
        ],
    )
    tool, out = _make_tool(type="weather", time_period="now")
    res = tool._invoke(query="Beijing")
    assert "Sunny" in res
    assert out["formalized_content"] == res


def test_invoke_empty_query_returns_empty():
    # Empty query short-circuits without any HTTP call.
    tool, out = _make_tool()
    assert tool._invoke(query="") == ""
    assert out.get("formalized_content") == ""


def test_invoke_location_lookup_error_returns_message(monkeypatch):
    # A non-200 from the geo lookup is a final, human-readable message (not a
    # retry) and is written to formalized_content.
    _patch_requests(monkeypatch, [{"code": "404"}])
    tool, out = _make_tool()
    res = tool._invoke(query="Nowhere")
    assert res.startswith("**Error**")
    assert "does not exist" in res
    assert out["formalized_content"] == res
