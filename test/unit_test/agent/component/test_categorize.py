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
"""Regression tests for category selection in the Categorize component
(agent/component/categorize.py)."""

import sys
from types import ModuleType, SimpleNamespace

import pytest


def _stub(monkeypatch, name, **attrs):
    """Register a stub module under `name` for the duration of the test."""
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


@pytest.fixture()
def categorize_cls(monkeypatch):
    """Import Categorize with the DB/LLM service imports stubbed out."""
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        resolve_model_config=lambda *_a, **_k: None,
        resolve_model_type=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "api.db.services.dialog_service", _stream_with_think_delta=lambda *_a, **_k: None)
    # rag.llm dynamically imports every model SDK, so the package is stubbed too.
    _stub(monkeypatch, "rag.llm", __path__=[])
    _stub(monkeypatch, "rag.llm.chat_model", ERROR_PREFIX="**ERROR**")

    from agent.component.categorize import Categorize

    return Categorize


def _pick(categorize_cls, counts):
    # Category selection does not access instance state, so initialization is unnecessary.
    return categorize_cls._pick_category(counts)


@pytest.mark.p1
def test_substring_category_does_not_steal_the_route(categorize_cls):
    """An answer naming the longer category must not route to its substring (#19410)."""
    assert _pick(categorize_cls, {"Billing": 1, "Billing Dispute": 1}) == "Billing Dispute"


@pytest.mark.p1
def test_highest_count_wins_over_name_length(categorize_cls):
    """A shorter category mentioned more often still wins."""
    assert _pick(categorize_cls, {"Billing": 3, "Billing Dispute": 1}) == "Billing"


@pytest.mark.p1
def test_equal_count_and_length_keeps_first_category(categorize_cls):
    """Full ties keep the previous insertion-order behavior."""
    assert _pick(categorize_cls, {"Billing": 1, "Support": 1}) == "Billing"


@pytest.mark.p1
def test_single_category_is_selected(categorize_cls):
    assert _pick(categorize_cls, {"Other": 2}) == "Other"
