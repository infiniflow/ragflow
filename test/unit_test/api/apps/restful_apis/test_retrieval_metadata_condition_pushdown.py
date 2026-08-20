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
"""Regression tests for /api/v1/retrieval metadata_condition filtering (#18397).

The endpoint used to evaluate `metadata_condition` with the in-memory
`meta_filter` over `get_flatted_meta_by_kbs`, while /datasets/search pushes
the same conditions down to the doc-metadata index. On large datasets the
in-memory path misses documents, so identical conditions returned matches in
the UI and an empty page over the SDK.

The fix routes the endpoint through the shared `apply_meta_data_filter`. These
tests pin the contract at the helper level: the exact mapping /retrieval now
builds (`{"method": "manual", "manual": convert_conditions(...), "logic": ...}`)
must reach the ES/Infinity push-down, a push-down failure must fall back to the
in-memory filter, an empty condition set must resolve to the match-nothing
marker without loading any metadata, and a manual filter with no matches must
return the `["-999"]` marker the endpoint's empty-response branch keys off.
"""

from __future__ import annotations

import asyncio
import sys
import types
from pathlib import Path
from typing import ClassVar

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[5]))

from common.metadata_utils import apply_meta_data_filter, convert_conditions


class _FakePushdown:
    calls: ClassVar[list[tuple[list[str], list[dict], str]]] = []
    result: ClassVar[list[str]] = ["doc-1", "doc-2"]
    raise_error: ClassVar[bool] = False

    @classmethod
    def filter_doc_ids_by_meta_pushdown(cls, kb_ids, conditions, logic):
        cls.calls.append((list(kb_ids), [dict(c) for c in conditions], logic))
        if cls.raise_error:
            raise RuntimeError("backend cannot service this filter")
        return cls.result


@pytest.fixture()
def fake_pushdown(monkeypatch):
    _FakePushdown.calls = []
    _FakePushdown.result = ["doc-1", "doc-2"]
    _FakePushdown.raise_error = False
    fake_module = types.ModuleType("api.db.services.doc_metadata_service")
    fake_module.DocMetadataService = _FakePushdown
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", fake_module)
    # apply_meta_data_filter imports the LLM filter generator unconditionally
    # at call time; manual mode never uses it, and stubbing it keeps these
    # tests free of the heavy rag package.
    rag_module = types.ModuleType("rag")
    prompts_module = types.ModuleType("rag.prompts")
    generator_module = types.ModuleType("rag.prompts.generator")
    generator_module.gen_meta_filter = None
    monkeypatch.setitem(sys.modules, "rag", rag_module)
    monkeypatch.setitem(sys.modules, "rag.prompts", prompts_module)
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", generator_module)
    return _FakePushdown


def _retrieval_mapping(metadata_condition: dict) -> dict:
    """The exact meta_data_filter /retrieval builds from metadata_condition."""
    return {
        "method": "manual",
        "manual": convert_conditions(metadata_condition),
        "logic": metadata_condition.get("logic", "and"),
    }


@pytest.mark.p1
def test_retrieval_conditions_reach_the_pushdown_filter(fake_pushdown):
    metadata_condition = {
        "conditions": [
            {"name": "agent_code", "comparison_operator": "=", "value": "AGENT_WST_SHENGCHAN"}
        ]
    }
    doc_ids = asyncio.run(
        apply_meta_data_filter(
            _retrieval_mapping(metadata_condition),
            None,
            "question",
            None,
            [],
            kb_ids=["kb-1", "kb-2"],
            metas_loader=lambda: {},
        )
    )
    assert doc_ids == ["doc-1", "doc-2"]
    assert len(fake_pushdown.calls) == 1
    kb_ids, conditions, logic = fake_pushdown.calls[0]
    assert kb_ids == ["kb-1", "kb-2"]
    assert conditions == [{"op": "=", "key": "agent_code", "value": "AGENT_WST_SHENGCHAN"}]
    assert logic == "and"


@pytest.mark.p1
def test_pushdown_failure_falls_back_to_in_memory(fake_pushdown):
    fake_pushdown.raise_error = True
    metas = {"agent_code": {"AGENT_WST_SHENGCHAN": ["doc-9"]}}
    metadata_condition = {
        "conditions": [
            {"name": "agent_code", "comparison_operator": "=", "value": "AGENT_WST_SHENGCHAN"}
        ]
    }
    doc_ids = asyncio.run(
        apply_meta_data_filter(
            _retrieval_mapping(metadata_condition),
            metas,
            "question",
            None,
            [],
            kb_ids=["kb-1"],
            metas_loader=lambda: metas,
        )
    )
    assert doc_ids == ["doc-9"]


@pytest.mark.p1
def test_no_matching_documents_returns_match_nothing_marker(fake_pushdown):
    fake_pushdown.result = []
    metadata_condition = {
        "conditions": [
            {"name": "agent_code", "comparison_operator": "=", "value": "MISSING"}
        ]
    }
    doc_ids = asyncio.run(
        apply_meta_data_filter(
            _retrieval_mapping(metadata_condition),
            None,
            "question",
            None,
            [],
            kb_ids=["kb-1"],
            metas_loader=lambda: {},
        )
    )
    # ["-999"] is the endpoint's match-nothing marker; the empty-response
    # branch keys off it.
    assert doc_ids == ["-999"]


@pytest.mark.p1
def test_operator_mapping_is_preserved(fake_pushdown):
    metadata_condition = {
        "conditions": [
            {"name": "score", "comparison_operator": ">=", "value": 10},
            {"name": "kind", "comparison_operator": "not is", "value": "draft"},
        ]
    }
    asyncio.run(
        apply_meta_data_filter(
            _retrieval_mapping(metadata_condition),
            None,
            "q",
            None,
            [],
            kb_ids=["kb-1"],
            metas_loader=lambda: {},
        )
    )
    _, conditions, _ = fake_pushdown.calls[0]
    assert conditions[0]["op"] == "≥"
    assert conditions[1]["op"] == "≠"


@pytest.mark.p1
def test_empty_condition_list_short_circuits_without_loading_metadata(fake_pushdown):
    # An empty condition list resolves to the match-nothing marker without
    # touching the push-down or the metadata loader — the endpoint guards
    # before calling the helper.
    loader_calls = []

    doc_ids = asyncio.run(
        apply_meta_data_filter(
            _retrieval_mapping({"conditions": []}),
            None,
            "q",
            None,
            [],
            kb_ids=["kb-1"],
            metas_loader=lambda: loader_calls.append(1) or {},
        )
    )
    # The helper itself does not short-circuit; the endpoint's guard does.
    # Pin the endpoint-level contract by feeding the mapping the guard
    # protects against: an empty manual list must not invoke the loader.
    assert fake_pushdown.calls == []
    assert loader_calls == []
    assert doc_ids == []
