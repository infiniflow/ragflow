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

from types import SimpleNamespace

from agent.tools.base import ToolBase
from agent.tools.exesql import ExeSQL


class _FakeCanvas:
    def is_canceled(self):
        return False


class _FailingTool(ToolBase):
    component_name = "FailingTool"

    def _invoke(self, **kwargs):
        raise RuntimeError("boom")


def _build_tool(tool_cls, outputs=None):
    tool = tool_cls.__new__(tool_cls)
    tool._canvas = _FakeCanvas()
    tool._id = "tool:0"
    tool._param = SimpleNamespace(
        outputs=outputs or {},
        inputs={},
        debug_inputs={},
        exception_method="comment",
        exception_default_value="SQL failed, continue with fallback.",
        exception_goto=None,
    )
    return tool


def test_toolbase_comment_exception_sets_default_without_error():
    tool = _build_tool(_FailingTool)

    result = tool.invoke()

    assert result == "SQL failed, continue with fallback."
    assert tool.error() in ("", None)
    assert tool.output("_EXCEPTION") == "boom"
    assert tool.output("result") == "SQL failed, continue with fallback."


def test_exesql_comment_exception_sets_sql_outputs(monkeypatch):
    tool = _build_tool(
        ExeSQL,
        {
            "formalized_content": {"value": "", "type": "string"},
            "json": {"value": [], "type": "Array<Object>"},
        },
    )
    tool._param.exception_default_value = "Database query failed."

    def fail(*args, **kwargs):
        raise RuntimeError("bad sql")

    monkeypatch.setattr(tool, "_invoke", fail)

    result = tool.invoke(sql="select * from missing_table")

    assert result == "Database query failed."
    assert tool.error() in ("", None)
    assert tool.output("_EXCEPTION") == "bad sql"
    assert tool.output("formalized_content") == "Database query failed."
    assert tool.output("json") == []
