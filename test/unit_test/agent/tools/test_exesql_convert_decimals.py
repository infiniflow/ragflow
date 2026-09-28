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
"""Unit tests for ExeSQL convert_decimals JSON serialization helpers.

Plain ``datetime.date`` / ``datetime`` / ``time`` values from SQL DATE/TIME
columns can survive as Python objects and must be serialized before canvas SSE
``json.dumps`` (issue #19250). ``datetime`` truncates to ``YYYY-MM-DD``;
``date`` / ``time`` use isoformat.
"""

import importlib.util
import json
import math
import sys
import types
from datetime import date, datetime, time
from decimal import Decimal
from pathlib import Path

import pytest

_REPO_ROOT = Path(__file__).resolve().parents[4]


def _load_exesql_module():
    with pytest.MonkeyPatch.context() as mp:
        for name in ("pandas", "psycopg2", "pyodbc", "pymysql"):
            if name not in sys.modules:
                mod = types.ModuleType(name)
                mod.connect = lambda *a, **k: None
                mp.setitem(sys.modules, name, mod)

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
            if pkg not in sys.modules:
                mp.setitem(sys.modules, pkg, types.ModuleType(pkg))
        mp.setitem(sys.modules, "agent.tools.base", base)

        conn_utils = types.ModuleType("common.connection_utils")
        conn_utils.timeout = lambda *a, **k: lambda f: f
        mp.setitem(sys.modules, "common.connection_utils", conn_utils)

        ssrf = types.ModuleType("common.ssrf_guard")
        ssrf.assert_host_is_safe = lambda h: h
        mp.setitem(sys.modules, "common.ssrf_guard", ssrf)

        spec = importlib.util.spec_from_file_location("exesql_convert_uut", _REPO_ROOT / "agent" / "tools" / "exesql.py")
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        return mod


_exesql_mod = _load_exesql_module()
convert_decimals = _exesql_mod.convert_decimals


@pytest.mark.p2
def test_convert_decimals_date_datetime_time():
    # datetime must truncate to YYYY-MM-DD; plain date/time keep isoformat.
    payload = {
        "start_date": date(2026, 7, 27),
        "ts": datetime(2026, 7, 27, 14, 30, 0),
        "t": time(9, 15, 30),
    }
    out = convert_decimals(payload)
    assert out == {
        "start_date": "2026-07-27",
        "ts": "2026-07-27",
        "t": "09:15:30",
    }
    json.dumps(out)  # must not raise


@pytest.mark.p2
def test_convert_decimals_decimal_and_nan_still_work():
    payload = {
        "amount": Decimal("12.34"),
        "bad": float("nan"),
        "inf": float("inf"),
        "ok": 1.5,
    }
    out = convert_decimals(payload)
    assert out["amount"] == pytest.approx(12.34)
    assert out["bad"] is None
    assert out["inf"] is None
    assert out["ok"] == 1.5
    json.dumps(out)


@pytest.mark.p2
def test_convert_decimals_recurses_lists():
    rows = [{"d": date(2026, 7, 28), "n": Decimal("1")}, [date(2026, 1, 1), math.nan]]
    out = convert_decimals(rows)
    assert out == [{"d": "2026-07-28", "n": 1.0}, ["2026-01-01", None]]
    json.dumps(out)
