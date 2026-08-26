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
`tools: []` must never reach the provider.

Servers that validate their request body reject an empty array outright:

    400 - Value error, `tools` must not be an empty array.
          Either provide at least one tool or omit the field entirely.

The LiteLLM path already guards this in `_construct_completion_args`; the
base OpenAI path did not.
"""

import pytest

from rag.llm.chat_model import Base

pytestmark = pytest.mark.p1

SCHEMAS = [{"type": "function", "function": {"name": "rag", "parameters": {}}}]


def _model(tools):
    """A Base instance without running __init__ (which would build an API client)."""
    mdl = Base.__new__(Base)
    mdl.tools = tools
    return mdl


def test_no_tools_bound_omits_the_fields_entirely():
    assert _model([])._tool_request_kwargs() == {}


def test_bound_tools_are_sent_with_auto_choice():
    assert _model(SCHEMAS)._tool_request_kwargs() == {"tools": SCHEMAS, "tool_choice": "auto"}


def test_explicit_tools_argument_wins_over_self_tools():
    """The streaming loop snapshots `tools = self.tools` and passes it back in."""
    assert _model(SCHEMAS)._tool_request_kwargs([]) == {}
    assert _model([])._tool_request_kwargs(SCHEMAS) == {"tools": SCHEMAS, "tool_choice": "auto"}


def test_tool_choice_is_never_sent_without_tools():
    """`tool_choice: auto` alone is just as invalid as an empty `tools` array."""
    assert "tool_choice" not in _model([])._tool_request_kwargs()
    assert "tool_choice" not in _model(None)._tool_request_kwargs()
