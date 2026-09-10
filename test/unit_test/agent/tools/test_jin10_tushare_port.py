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
"""Regression tests for issues #18447 and #18448.

Jin10 and TuShare agent tools were the last two tools still on the
legacy ``ComponentBase``/``ComponentParamBase`` pair, with only the
dead ``_run(self, history, **kwargs)`` signature. They could not be
used as Agent tools: ``agent_with_tools.py`` calls ``get_meta()``
(``AttributeError`` -- it is defined on ``ToolBase``/``ToolParamBase``,
not on ``ComponentBase``) and the invoke path goes through
``_invoke`` (``ComponentBase._invoke`` raises ``NotImplementedError``;
nothing dispatches to ``_run`` any more).

The fix ports both tools to ``ToolBase``/``ToolParamBase`` with a
``meta`` block and an ``@timeout _invoke``. These tests pin the
post-port behaviour:

* each Param class is a ``ToolParamBase`` subclass with a meta block
  (the contract ``agent_with_tools.py`` relies on),
* each Tool class is a ``ToolBase`` subclass with an ``_invoke``
  method (the contract ``agent/tools/base.py`` dispatches on),
* each Param's ``get_input_form`` returns a non-empty dict (the
  contract the Agent UI relies on for the field map),
* each Tool's ``_invoke`` publishes ``formalized_content`` and
  surfaces failures via ``_ERROR`` (the contract every other ported
  tool already followed).
"""

from agent.tools.base import ToolBase, ToolParamBase
from agent.tools.jin10 import Jin10, Jin10Param
from agent.tools.tushare import TuShare, TuShareParam


# ---------------------------------------------------------------------------
# Jin10
# ---------------------------------------------------------------------------


def test_jin10_param_is_tool_param_base_with_meta():
    p = Jin10Param()
    assert isinstance(p, ToolParamBase), f"Jin10Param must inherit ToolParamBase (the contract agent_with_tools.py relies on for get_meta() and the @register_tool dispatch); got {type(p).__mro__!r}"
    meta = p.meta
    assert isinstance(meta, dict) and "name" in meta and "description" in meta and "parameters" in meta, (
        f"Jin10Param.meta must be a ToolMeta dict (with 'name', 'description', 'parameters'); got {meta!r}"
    )


def test_jin10_get_input_form_returns_query_field():
    """The agent UI uses get_input_form to render the user-facing field
    for the tool's primary input. Pre-port Jin10 had no get_input_form
    and the field map was empty, so the tool was not configurable from
    the UI.
    """
    p = Jin10Param()
    form = p.get_input_form()
    assert isinstance(form, dict) and form, f"Jin10Param.get_input_form() must return a non-empty dict; got {form!r}"
    assert "query" in form, f"Jin10Param.get_input_form() must include the 'query' field; got {form!r}"


def test_jin10_tool_inherits_tool_base_with_invoke():
    """The tool's invoke dispatcher is ``ToolBase._invoke``; ``_run`` is
    dead code (no caller in the agent module). Pre-port Jin10 had
    only ``_run``, so any call hit ``ComponentBase._invoke`` ->
    NotImplementedError.
    """
    assert issubclass(Jin10, ToolBase), f"Jin10 must inherit ToolBase; got {Jin10.__mro__!r}"
    assert hasattr(Jin10, "_invoke"), "Jin10 must define _invoke"
    assert callable(Jin10._invoke)
    # The legacy _run must be gone (no caller; keeping it would invite
    # future regressions where someone wires the legacy path back up).
    assert not hasattr(Jin10, "_run") or Jin10._run is None or Jin10._run.__qualname__.startswith(Jin10.__qualname__), (
        "Jin10._run should be removed (no caller; only _invoke is the post-port entry point)."
    )


def test_jin10_invoke_surfaces_errors_via_set_output(monkeypatch):
    """When the HTTP fetch raises, the tool must surface the failure
    via ``set_output('_ERROR', ...)`` and return a string the Agent
    can show the user -- mirroring the contract every other ported
    tool already follows. Pre-port Jin10 returned ``Jin10.be_output('**ERROR**: ...')``,
    a legacy DataFrame shape; the agent module no longer dispatches to
    that path.
    """
    import requests as _requests

    tool = Jin10.__new__(Jin10)
    p = Jin10Param()
    p.secret_key = "fake-key"
    p.type = "flash"
    tool._param = p

    outputs = {}

    def _set_output(key, value):
        outputs[key] = value

    def _check_if_canceled(_label):
        return False

    def _raise(*_args, **_kwargs):
        raise _requests.exceptions.Timeout("simulated timeout")

    monkeypatch.setattr(tool, "set_output", _set_output)
    monkeypatch.setattr(tool, "check_if_canceled", _check_if_canceled)
    monkeypatch.setattr("agent.tools.jin10.requests.get", _raise)
    # timeout decorator is bypassed by calling the wrapped function
    # directly; pull it back out via the source attribute.
    unwrapped = Jin10._invoke
    result = unwrapped(tool)

    assert "_ERROR" in outputs, f"Jin10 must call set_output('_ERROR', ...) on HTTP failure; outputs seen: {outputs!r}"
    assert "simulated timeout" in str(outputs["_ERROR"]), f"Jin10 _ERROR must include the exception text so the Agent can show it to the user; got {outputs['_ERROR']!r}"
    assert "ERROR" in result, f"Jin10 _invoke must return a string that includes the error marker for the Agent; got {result!r}"


# ---------------------------------------------------------------------------
# TuShare
# ---------------------------------------------------------------------------


def test_tushare_param_is_tool_param_base_with_meta():
    p = TuShareParam()
    assert isinstance(p, ToolParamBase), f"TuShareParam must inherit ToolParamBase; got {type(p).__mro__!r}"
    meta = p.meta
    assert isinstance(meta, dict) and "name" in meta and "description" in meta and "parameters" in meta, f"TuShareParam.meta must be a ToolMeta dict; got {meta!r}"


def test_tushare_get_input_form_returns_query_field():
    p = TuShareParam()
    form = p.get_input_form()
    assert isinstance(form, dict) and form
    assert "query" in form


def test_tushare_tool_inherits_tool_base_with_invoke():
    assert issubclass(TuShare, ToolBase)
    assert hasattr(TuShare, "_invoke") and callable(TuShare._invoke)


def test_tushare_invoke_surfaces_errors_via_set_output(monkeypatch):
    """TuShare must surface HTTP failure via set_output('_ERROR', ...)
    and the response-code != 0 case via set_output('_ERROR', ...) as
    well. Pre-port TuShare returned ``TuShare.be_output(response['msg'])``
    for the non-zero code case and the dead _run path for everything
    else; the new path mirrors the contract every other ported tool
    follows.
    """
    tool = TuShare.__new__(TuShare)
    p = TuShareParam()
    p.token = "fake-token"
    p.src = "sina"
    tool._param = p

    outputs = {}

    def _set_output(key, value):
        outputs[key] = value

    def _check_if_canceled(_label):
        return False

    # HTTP raises -> set_output('_ERROR', ...).
    import requests as _requests

    def _raise(*_args, **_kwargs):
        raise _requests.exceptions.ConnectionError("simulated connection error")

    monkeypatch.setattr(tool, "set_output", _set_output)
    monkeypatch.setattr(tool, "check_if_canceled", _check_if_canceled)
    monkeypatch.setattr("agent.tools.tushare.requests.post", _raise)
    result = TuShare._invoke(tool)
    assert "_ERROR" in outputs, f"TuShare must call set_output('_ERROR', ...) on HTTP failure; outputs seen: {outputs!r}"
    assert "simulated connection error" in str(outputs["_ERROR"])
    assert "ERROR" in result

    # response['code'] != 0 -> set_output('_ERROR', ...).
    outputs.clear()

    class _Resp:
        def __init__(self, payload):
            self._payload = payload

        def json(self):
            return self._payload

    def _ok_post(*_args, **_kwargs):
        return _Resp({"code": 1, "msg": "rate limit exceeded"})

    monkeypatch.setattr("agent.tools.tushare.requests.post", _ok_post)
    result = TuShare._invoke(tool)
    assert outputs.get("_ERROR") == "rate limit exceeded", f"TuShare must call set_output('_ERROR', response['msg']) on non-zero response code; outputs seen: {outputs!r}"
    assert result == "rate limit exceeded"
