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
"""Regression test for category selection in `agent/component/categorize.py`.

The old logic counted `ans.lower().count(c.lower())` per category and
picked `max(counts)`. When one category name is a substring of another
("Billing" vs "Billing Dispute"), an answer naming the longer category
counted for BOTH and the tie was broken by dict insertion order, so
"Categorize" could route "Billing Dispute" to the "Billing" branch.

The fix ties-breaks top counts toward the longest (most specific)
category name.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock

import pytest


def _load_categorize_module(monkeypatch):
    """Load `agent.component.categorize` with its heavy deps stubbed."""
    repo_root = Path(__file__).resolve().parents[4]

    quart_stub = ModuleType("quart")
    quart_stub.make_response = MagicMock()
    quart_stub.jsonify = MagicMock()
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    constants_mod = ModuleType("common.constants")

    class _RetCode:
        SUCCESS = 0
        EXCEPTION_ERROR = 100

    class _LLMType:
        CHAT = "chat"

    constants_mod.RetCode = _RetCode
    constants_mod.LLMType = _LLMType
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)
    conn_spec = importlib.util.spec_from_file_location("common.connection_utils", repo_root / "common" / "connection_utils.py")
    conn_mod = importlib.util.module_from_spec(conn_spec)
    monkeypatch.setitem(sys.modules, "common.connection_utils", conn_mod)
    conn_spec.loader.exec_module(conn_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api", api_pkg)
    for name in ("api.db", "api.db.services", "api.db.joint_services", "rag", "rag.llm"):
        pkg = ModuleType(name)
        pkg.__path__ = []
        monkeypatch.setitem(sys.modules, name, pkg)
    llm_service = ModuleType("api.db.services.llm_service")
    llm_service.LLMBundle = object
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", llm_service)
    tenant_model_service = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_service.resolve_model_config = lambda *a, **k: {}
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_service)
    chat_model = ModuleType("rag.llm.chat_model")
    chat_model.ERROR_PREFIX = "**ERROR**"
    monkeypatch.setitem(sys.modules, "rag.llm.chat_model", chat_model)

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)
    component_pkg = ModuleType("agent.component")
    component_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "agent.component", component_pkg)
    llm_mod = ModuleType("agent.component.llm")

    class _LLMParam:
        pass

    class _LLM:
        pass

    llm_mod.LLMParam = _LLMParam
    llm_mod.LLM = _LLM
    monkeypatch.setitem(sys.modules, "agent.component.llm", llm_mod)

    spec = importlib.util.spec_from_file_location(
        "agent.component.categorize", repo_root / "agent" / "component" / "categorize.py"
    )
    mod = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "agent.component.categorize", mod)
    spec.loader.exec_module(mod)
    return mod


def test_overlapping_names_route_to_more_specific_category(monkeypatch):
    mod = _load_categorize_module(monkeypatch)
    categories = ["Billing", "Billing Dispute"]
    assert mod.Categorize._select_category("Billing Dispute", categories) == "Billing Dispute"
    assert mod.Categorize._select_category("billing dispute", categories) == "Billing Dispute"
    assert mod.Categorize._select_category("Billing", categories) == "Billing"


def test_no_match_returns_none(monkeypatch):
    mod = _load_categorize_module(monkeypatch)
    assert mod.Categorize._select_category("something else entirely", ["Billing", "Refund"]) is None


def test_distinct_names_keep_count_winner(monkeypatch):
    mod = _load_categorize_module(monkeypatch)
    categories = ["Refund", "Exchange"]
    assert mod.Categorize._select_category("Refund, refund, but not exchange", categories) == "Refund"
