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

"""
Unit tests for the Invoke component's header variable interpolation.

These tests exercise the real Invoke._invoke method, verifying that
{variable} placeholders in HTTP header values are resolved via canvas
variable lookup (issue #13277).
"""

import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest


def _load_invoke_module(monkeypatch):
    """Load the real Invoke class with monkeypatched stubs that are
    automatically cleaned up after each test."""
    repo_root = Path(__file__).resolve().parents[4]

    # -- lightweight stubs (auto-restored by monkeypatch) --------------------

    quart = ModuleType("quart")
    quart.make_response = lambda *a, **kw: None
    quart.jsonify = lambda *a, **kw: None
    monkeypatch.setitem(sys.modules, "quart", quart)

    pd = ModuleType("pandas")
    pd.DataFrame = type("DataFrame", (), {})
    monkeypatch.setitem(sys.modules, "pandas", pd)

    deepdoc = ModuleType("deepdoc")
    deepdoc.__path__ = []
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc)
    deepdoc_parser = ModuleType("deepdoc.parser")
    deepdoc_parser.HtmlParser = MagicMock
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser)
    monkeypatch.setitem(sys.modules, "xgboost", ModuleType("xgboost"))

    # -- common package and submodules ---------------------------------------

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    constants = ModuleType("common.constants")

    class _RetCode:
        SUCCESS = 0
        EXCEPTION_ERROR = 100

    constants.RetCode = _RetCode
    monkeypatch.setitem(sys.modules, "common.constants", constants)

    conn_spec = importlib.util.spec_from_file_location("common.connection_utils", repo_root / "common" / "connection_utils.py")
    conn_mod = importlib.util.module_from_spec(conn_spec)
    monkeypatch.setitem(sys.modules, "common.connection_utils", conn_mod)
    conn_spec.loader.exec_module(conn_mod)

    misc_spec = importlib.util.spec_from_file_location("common.misc_utils", repo_root / "common" / "misc_utils.py")
    misc_mod = importlib.util.module_from_spec(misc_spec)
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_mod)
    misc_spec.loader.exec_module(misc_mod)

    # -- agent package (bare stubs to skip __init__ auto-import) -------------

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = [str(repo_root / "agent")]
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)

    agent_settings = ModuleType("agent.settings")
    agent_settings.FLOAT_ZERO = 1e-8
    agent_settings.PARAM_MAXDEPTH = 5
    monkeypatch.setitem(sys.modules, "agent.settings", agent_settings)

    component_pkg = ModuleType("agent.component")
    component_pkg.__path__ = [str(repo_root / "agent" / "component")]
    monkeypatch.setitem(sys.modules, "agent.component", component_pkg)

    # -- load the real base.py and invoke.py ---------------------------------

    base_spec = importlib.util.spec_from_file_location("agent.component.base", repo_root / "agent" / "component" / "base.py")
    base_mod = importlib.util.module_from_spec(base_spec)
    monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)
    base_spec.loader.exec_module(base_mod)

    invoke_spec = importlib.util.spec_from_file_location("agent.component.invoke", repo_root / "agent" / "component" / "invoke.py")
    invoke_mod = importlib.util.module_from_spec(invoke_spec)
    monkeypatch.setitem(sys.modules, "agent.component.invoke", invoke_mod)
    invoke_spec.loader.exec_module(invoke_mod)

    return invoke_mod


def _make_invoke(module, *, url="http://example.com", method="get", headers="", variables=None, proxy="", timeout_sec=60, clean_html=False, datatype="json", variable_values=None):
    """Build an Invoke instance with a mocked canvas."""
    variable_values = variable_values or {}

    canvas = MagicMock()
    canvas.get_variable_value = MagicMock(side_effect=lambda k: variable_values.get(k, ""))
    canvas.is_canceled = MagicMock(return_value=False)

    param = module.InvokeParam.__new__(module.InvokeParam)
    param.url = url
    param.method = method
    param.headers = headers
    param.variables = variables or []
    param.proxy = proxy
    param.timeout = timeout_sec
    param.clean_html = clean_html
    param.datatype = datatype
    param.max_retries = 0
    param.delay_after_error = 0
    param.outputs = {}
    param.inputs = {}

    inst = module.Invoke.__new__(module.Invoke)
    inst._canvas = canvas
    inst._param = param
    inst._id = "invoke_test"

    return inst


@pytest.mark.p2
def test_header_single_variable(monkeypatch):
    module = _load_invoke_module(monkeypatch)
    invoke = _make_invoke(
        module,
        headers=json.dumps({"Authorization": "Bearer {auth_token}"}),
        variable_values={"auth_token": "secret123"},
    )
    mock_get = MagicMock(return_value=SimpleNamespace(text="ok"))
    monkeypatch.setattr(module.requests, "get", mock_get)
    invoke._invoke()
    assert mock_get.call_args[1]["headers"]["Authorization"] == "Bearer secret123"


@pytest.mark.p2
def test_header_multiple_variables(monkeypatch):
    module = _load_invoke_module(monkeypatch)
    invoke = _make_invoke(
        module,
        headers=json.dumps(
            {
                "Authorization": "Bearer {token}",
                "X-Request-Id": "{req_id}",
                "Content-Type": "application/json",
            }
        ),
        variable_values={"token": "tok_abc", "req_id": "id-42"},
    )
    mock_get = MagicMock(return_value=SimpleNamespace(text="ok"))
    monkeypatch.setattr(module.requests, "get", mock_get)
    invoke._invoke()
    h = mock_get.call_args[1]["headers"]
    assert h["Authorization"] == "Bearer tok_abc"
    assert h["X-Request-Id"] == "id-42"
    assert h["Content-Type"] == "application/json"


@pytest.mark.p2
def test_header_no_variables_unchanged(monkeypatch):
    module = _load_invoke_module(monkeypatch)
    invoke = _make_invoke(
        module,
        headers=json.dumps({"Content-Type": "application/json"}),
    )
    mock_get = MagicMock(return_value=SimpleNamespace(text="ok"))
    monkeypatch.setattr(module.requests, "get", mock_get)
    invoke._invoke()
    assert mock_get.call_args[1]["headers"]["Content-Type"] == "application/json"


@pytest.mark.p2
def test_header_empty(monkeypatch):
    module = _load_invoke_module(monkeypatch)
    invoke = _make_invoke(module, headers="")
    mock_get = MagicMock(return_value=SimpleNamespace(text="ok"))
    monkeypatch.setattr(module.requests, "get", mock_get)
    invoke._invoke()
    assert mock_get.call_args[1]["headers"] == {}


@pytest.mark.p2
def test_header_component_ref_variable(monkeypatch):
    module = _load_invoke_module(monkeypatch)
    invoke = _make_invoke(
        module,
        headers=json.dumps({"Authorization": "Bearer {begin@token}"}),
        variable_values={"begin@token": "my_token"},
    )
    mock_get = MagicMock(return_value=SimpleNamespace(text="ok"))
    monkeypatch.setattr(module.requests, "get", mock_get)
    invoke._invoke()
    assert mock_get.call_args[1]["headers"]["Authorization"] == "Bearer my_token"


@pytest.mark.p2
def test_header_env_variable(monkeypatch):
    module = _load_invoke_module(monkeypatch)
    invoke = _make_invoke(
        module,
        headers=json.dumps({"Authorization": "Bearer {env.api_key}"}),
        variable_values={"env.api_key": "env_secret"},
    )
    mock_get = MagicMock(return_value=SimpleNamespace(text="ok"))
    monkeypatch.setattr(module.requests, "get", mock_get)
    invoke._invoke()
    assert mock_get.call_args[1]["headers"]["Authorization"] == "Bearer env_secret"


@pytest.mark.p2
def test_header_missing_variable_becomes_empty(monkeypatch):
    module = _load_invoke_module(monkeypatch)
    invoke = _make_invoke(
        module,
        headers=json.dumps({"Authorization": "Bearer {nonexistent}"}),
        variable_values={},
    )
    mock_get = MagicMock(return_value=SimpleNamespace(text="ok"))
    monkeypatch.setattr(module.requests, "get", mock_get)
    invoke._invoke()
    assert mock_get.call_args[1]["headers"]["Authorization"] == "Bearer "


@pytest.mark.p2
def test_header_variable_with_post(monkeypatch):
    module = _load_invoke_module(monkeypatch)
    invoke = _make_invoke(
        module,
        method="post",
        headers=json.dumps({"Authorization": "Bearer {token}"}),
        variable_values={"token": "post_token"},
    )
    mock_post = MagicMock(return_value=SimpleNamespace(text="ok"))
    monkeypatch.setattr(module.requests, "post", mock_post)
    invoke._invoke()
    assert mock_post.call_args[1]["headers"]["Authorization"] == "Bearer post_token"


@pytest.mark.p2
def test_header_variable_with_put(monkeypatch):
    module = _load_invoke_module(monkeypatch)
    invoke = _make_invoke(
        module,
        method="put",
        headers=json.dumps({"Authorization": "Bearer {token}"}),
        variable_values={"token": "put_token"},
    )
    mock_put = MagicMock(return_value=SimpleNamespace(text="ok"))
    monkeypatch.setattr(module.requests, "put", mock_put)
    invoke._invoke()
    assert mock_put.call_args[1]["headers"]["Authorization"] == "Bearer put_token"
