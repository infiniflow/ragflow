#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

"""Tests for GraphRAG/RAPTOR checkpoint/resume logic.

These tests call the real service functions in api.db.services.task_checkpoint,
monkeypatching only the doc-store layer (settings.docStoreConn.search /
get_fields).  This ensures bugs in pagination, query construction, and error
handling are caught -- a local re-implementation of the logic would not detect
those failures.
"""

import importlib.util
import json
import pathlib
import sys
from unittest.mock import MagicMock

import networkx as nx
import pytest

# ---------------------------------------------------------------------------
# Load the real task_checkpoint module.
#
# conftest.py mocks 'api.db.services' as a MagicMock (needed by other graphrag
# imports), which would turn any submodule import into another MagicMock.
# We load the file directly via importlib so the actual implementation is
# exercised, then register it under its canonical dotted name so internal
# imports like `from common import settings` resolve to the same MagicMocks
# that conftest already installed.
# ---------------------------------------------------------------------------
_CHECKPOINT_PATH = (
    pathlib.Path(__file__).parents[4] / "api" / "db" / "services" / "task_checkpoint.py"
)
_spec = importlib.util.spec_from_file_location(
    "api.db.services.task_checkpoint", _CHECKPOINT_PATH
)
_checkpoint_mod = importlib.util.module_from_spec(_spec)
sys.modules["api.db.services.task_checkpoint"] = _checkpoint_mod
_spec.loader.exec_module(_checkpoint_mod)

from api.db.services.task_checkpoint import (  # noqa: E402
    has_raptor_chunks,
    load_subgraph_from_store,
)

# The conftest installed common.settings as a MagicMock.  Grab it so we can
# attach controlled return values to docStoreConn in individual tests.
import common.settings as _settings  # noqa: E402  (MagicMock, from conftest)


# ---------------------------------------------------------------------------
# Shared helpers
# ---------------------------------------------------------------------------

def _make_subgraph(doc_id: str) -> nx.Graph:
    sg = nx.Graph()
    sg.add_node("ENTITY_A", description="test entity A", source_id=[doc_id])
    sg.add_node("ENTITY_B", description="test entity B", source_id=[doc_id])
    sg.add_edge(
        "ENTITY_A", "ENTITY_B",
        description="related", source_id=[doc_id], weight=1.0, keywords=[],
    )
    sg.graph["source_id"] = [doc_id]
    return sg


def _to_store_content(sg: nx.Graph) -> str:
    return json.dumps(nx.node_link_data(sg, edges="edges"), ensure_ascii=False)


def _single_page_mocks(field_map: dict):
    """Return (search_mock, get_fields_mock) that simulate a single-page store."""
    sentinel = object()
    call_count = {"n": 0}

    def _get_fields(_res, _fields):
        call_count["n"] += 1
        return field_map if call_count["n"] == 1 else {}

    return MagicMock(return_value=sentinel), MagicMock(side_effect=_get_fields)


# ---------------------------------------------------------------------------
# Tests for load_subgraph_from_store
# ---------------------------------------------------------------------------

class TestLoadSubgraphFromStore:

    @pytest.mark.p1
    @pytest.mark.asyncio
    async def test_loads_existing_subgraph(self, monkeypatch):
        """Subgraph present in the store is loaded and returned as nx.Graph."""
        doc_id = "doc_001"
        sg = _make_subgraph(doc_id)
        field_map = {"chunk_001": {"content_with_weight": _to_store_content(sg), "source_id": [doc_id]}}
        s, gf = _single_page_mocks(field_map)
        monkeypatch.setattr(_settings.docStoreConn, "search", s)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", gf)

        result = await load_subgraph_from_store("t1", "kb1", doc_id)

        assert result is not None
        assert isinstance(result, nx.Graph)
        assert result.has_node("ENTITY_A") and result.has_node("ENTITY_B")
        assert result.graph["source_id"] == [doc_id]

    @pytest.mark.p1
    @pytest.mark.asyncio
    async def test_returns_none_when_no_subgraph(self, monkeypatch):
        """Empty store returns None without raising."""
        s, gf = _single_page_mocks({})
        monkeypatch.setattr(_settings.docStoreConn, "search", s)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", gf)

        assert await load_subgraph_from_store("t1", "kb1", "doc_missing") is None

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_filters_by_doc_id_in_python(self, monkeypatch):
        """Only the chunk whose source_id matches the requested doc_id is returned."""
        sg_a, sg_b = _make_subgraph("doc_a"), _make_subgraph("doc_b")
        field_map = {
            "chunk_a": {"content_with_weight": _to_store_content(sg_a), "source_id": ["doc_a"]},
            "chunk_b": {"content_with_weight": _to_store_content(sg_b), "source_id": ["doc_b"]},
        }
        s, gf = _single_page_mocks(field_map)
        monkeypatch.setattr(_settings.docStoreConn, "search", s)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", gf)

        result = await load_subgraph_from_store("t1", "kb1", "doc_b")

        assert result is not None
        assert result.graph["source_id"] == ["doc_b"]

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_handles_string_source_id(self, monkeypatch):
        """source_id stored as a bare string (not list) is normalised correctly."""
        doc_id = "doc_str"
        sg = _make_subgraph(doc_id)
        field_map = {"chunk_001": {"content_with_weight": _to_store_content(sg), "source_id": doc_id}}
        s, gf = _single_page_mocks(field_map)
        monkeypatch.setattr(_settings.docStoreConn, "search", s)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", gf)

        result = await load_subgraph_from_store("t1", "kb1", doc_id)

        assert result is not None
        assert result.graph["source_id"] == [doc_id]

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_skips_malformed_json_returns_none(self, monkeypatch):
        """Malformed JSON is logged and skipped; function returns None (not raises)."""
        field_map = {"chunk_bad": {"content_with_weight": "not valid json{{{", "source_id": ["doc_bad"]}}
        s, gf = _single_page_mocks(field_map)
        monkeypatch.setattr(_settings.docStoreConn, "search", s)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", gf)

        result = await load_subgraph_from_store("t1", "kb1", "doc_bad")
        assert result is None

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_paginates_when_match_is_on_second_page(self, monkeypatch):
        """When the matching doc is beyond the first page, pagination finds it."""
        doc_id = "doc_page2"
        sg = _make_subgraph(doc_id)

        page1 = {f"other_{i}": {"content_with_weight": "", "source_id": [f"other_{i}"]} for i in range(256)}
        page2 = {"chunk_target": {"content_with_weight": _to_store_content(sg), "source_id": [doc_id]}}

        sentinel = object()
        call_count = {"n": 0}

        def _get_fields(_res, _fields):
            call_count["n"] += 1
            if call_count["n"] == 1:
                return page1
            if call_count["n"] == 2:
                return page2
            return {}

        monkeypatch.setattr(_settings.docStoreConn, "search", MagicMock(return_value=sentinel))
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", MagicMock(side_effect=_get_fields))

        result = await load_subgraph_from_store("t1", "kb1", doc_id)

        assert result is not None
        assert result.graph["source_id"] == [doc_id]

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_doc_store_exception_returns_none(self, monkeypatch):
        """A doc-store exception is caught; function returns None safely."""
        monkeypatch.setattr(
            _settings.docStoreConn, "search", MagicMock(side_effect=RuntimeError("db down"))
        )

        assert await load_subgraph_from_store("t1", "kb1", "doc_001") is None


# ---------------------------------------------------------------------------
# Tests for has_raptor_chunks
# ---------------------------------------------------------------------------

class TestHasRaptorChunks:

    @pytest.mark.p1
    def test_returns_true_when_raptor_chunk_exists(self, monkeypatch):
        """Doc store returns a RAPTOR row -> True."""
        monkeypatch.setattr(_settings.docStoreConn, "search", MagicMock(return_value=object()))
        monkeypatch.setattr(
            _settings.docStoreConn, "get_fields",
            MagicMock(return_value={"chunk_r": {"raptor_kwd": "raptor"}}),
        )

        assert has_raptor_chunks("doc_001", "t1", "kb1") is True

    @pytest.mark.p1
    def test_returns_false_when_no_raptor_chunks(self, monkeypatch):
        """Doc store returns empty -> False."""
        monkeypatch.setattr(_settings.docStoreConn, "search", MagicMock(return_value=object()))
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", MagicMock(return_value={}))

        assert has_raptor_chunks("doc_001", "t1", "kb1") is False

    @pytest.mark.p1
    def test_queries_specifically_for_raptor_kwd(self, monkeypatch):
        """raptor_kwd must be in the search condition so a non-RAPTOR leading chunk
        cannot produce a false-negative result."""
        captured = {}

        def _capture(fields, filters, condition, *_a, **_kw):
            captured["condition"] = condition
            return object()

        monkeypatch.setattr(_settings.docStoreConn, "search", _capture)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", MagicMock(return_value={}))

        has_raptor_chunks("doc_001", "t1", "kb1")

        assert "raptor_kwd" in captured["condition"]

    @pytest.mark.p2
    def test_returns_false_on_doc_store_exception(self, monkeypatch):
        """Exception is caught; function returns False without crashing."""
        monkeypatch.setattr(
            _settings.docStoreConn, "search", MagicMock(side_effect=RuntimeError("db down"))
        )

        assert has_raptor_chunks("doc_001", "t1", "kb1") is False


# ---------------------------------------------------------------------------
# End-to-end workflow test
# ---------------------------------------------------------------------------

class TestCheckpointResumeWorkflow:

    @pytest.mark.p1
    @pytest.mark.asyncio
    async def test_resume_finds_completed_docs_skips_new_ones(self, monkeypatch):
        """3 docs completed before crash; on resume each is found, new doc is not."""
        completed = ["doc_1", "doc_2", "doc_3"]
        field_map = {
            f"chunk_{d}": {"content_with_weight": _to_store_content(_make_subgraph(d)), "source_id": [d]}
            for d in completed
        }

        monkeypatch.setattr(_settings.docStoreConn, "search", MagicMock(return_value=object()))
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", MagicMock(return_value=field_map))

        for doc_id in completed:
            result = await load_subgraph_from_store("t1", "kb1", doc_id)
            assert result is not None
            assert result.graph["source_id"] == [doc_id]

        result = await load_subgraph_from_store("t1", "kb1", "doc_4_new")
        assert result is None
