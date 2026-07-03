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
"""SSRF-guard regression tests for the ExeSQL agent tool.

The DB host/port are node-author-controlled and connected to server-side, so
``ExeSQL._invoke`` must reject hosts that resolve to non-public addresses
(loopback, link-local/metadata, RFC1918) before opening any connection, and
must dial the validated/resolved public IP for allowed hosts — mirroring the
``test_db_connection`` endpoint guard (PR #14860).

``agent.tools.exesql`` is loaded in isolation (its package ``__init__`` would
auto-discover every tool and pull in the full agent framework), with the heavy
DB drivers and the agent base classes stubbed so only the real SSRF guard runs.
"""

import importlib.util
import sys
import types
from pathlib import Path
from types import SimpleNamespace

import pytest

_REPO_ROOT = Path(__file__).resolve().parents[4]


class _RecordingPyMySQL:
    """Fake pymysql whose connect() records the host it was asked to dial."""

    def __init__(self):
        self.dialed_host = None

    def connect(self, *args, **kwargs):
        self.dialed_host = kwargs.get("host")
        raise RuntimeError("connection attempted")  # stop before real DB I/O


_fake_pymysql = _RecordingPyMySQL()


def _load_exesql_module():
    # Stub the heavy drivers and the agent base so the module imports cleanly.
    for name in ("pandas", "psycopg2", "pyodbc"):
        mod = types.ModuleType(name)
        mod.connect = lambda *a, **k: None
        sys.modules.setdefault(name, mod)

    pymysql_stub = types.ModuleType("pymysql")
    pymysql_stub.connect = _fake_pymysql.connect
    sys.modules["pymysql"] = pymysql_stub

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

    spec = importlib.util.spec_from_file_location("exesql_uut", _REPO_ROOT / "agent" / "tools" / "exesql.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


_exesql_mod = _load_exesql_module()
ExeSQL = _exesql_mod.ExeSQL


def _build_exesql(host, db_type="mysql"):
    cpn = ExeSQL.__new__(ExeSQL)
    cpn._canvas = SimpleNamespace()
    cpn._param = SimpleNamespace(
        host=host,
        port=3306,
        db_type=db_type,
        database="db",
        username="u",
        password="p",
    )
    # Neutralize the component machinery that runs before the host check.
    cpn.check_if_canceled = lambda *_a, **_k: False
    cpn.get_input_elements_from_text = lambda _sql: {}
    cpn.set_input_value = lambda *_a, **_k: None
    cpn.string_format = lambda sql, _args: sql
    return cpn


@pytest.mark.p2
@pytest.mark.parametrize("host", ["127.0.0.1", "169.254.169.254", "10.0.0.5"])
def test_internal_host_rejected_before_connect(host):
    _fake_pymysql.dialed_host = None
    cpn = _build_exesql(host)
    with pytest.raises(Exception) as ei:
        cpn._invoke(sql="SELECT 1")
    assert "not allowed" in str(ei.value)
    # The SSRF guard must fire before any connection is attempted.
    assert _fake_pymysql.dialed_host is None


@pytest.mark.p2
def test_empty_host_rejected():
    cpn = _build_exesql("")
    with pytest.raises(Exception) as ei:
        cpn._invoke(sql="SELECT 1")
    assert "not allowed" in str(ei.value)


@pytest.mark.p2
def test_public_host_dials_validated_ip(monkeypatch):
    # Public host: pretend it resolves to a public IP, and ensure the driver is
    # dialed with that validated IP (not the raw hostname).
    monkeypatch.setattr(_exesql_mod, "assert_host_is_safe", lambda _h: "93.184.216.34")
    _fake_pymysql.dialed_host = None

    cpn = _build_exesql("db.example.com")
    with pytest.raises(Exception):
        cpn._invoke(sql="SELECT 1")  # RuntimeError from the recording connect
    assert _fake_pymysql.dialed_host == "93.184.216.34"
