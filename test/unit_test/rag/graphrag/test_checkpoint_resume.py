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

Calls the real implementations:
  - load_subgraph_from_store  (rag/graphrag/general/index.py)
  - has_raptor_chunks         (rag/svr/task_executor.py)

Both modules are loaded via importlib with their infrastructure dependencies
mocked, so the actual query logic, pagination, and error handling are exercised
without needing running services.
"""

import importlib.util
import json
import pathlib
import sys
from unittest.mock import MagicMock

import networkx as nx
import pytest

# ---------------------------------------------------------------------------
# Additional sys.modules mocks needed beyond what conftest already provides.
#
# conftest.py (same directory) mocks the heavy packages listed in
# _modules_to_mock.  We need a few more to satisfy index.py and
# task_executor.py's import-time dependencies.
# ---------------------------------------------------------------------------
_EXTRA_MOCKS = [
    # for index.py
    "api.db.services.document_service",
    # for task_executor.py
    "api.db",
    "api.db.services.knowledgebase_service",
    "api.db.services.pipeline_operation_log_service",
    "api.db.joint_services",
    "api.db.joint_services.memory_message_service",
    "api.db.joint_services.tenant_model_service",
    "api.db.services.doc_metadata_service",
    "api.db.services.llm_service",
    "api.db.services.file2document_service",
    "api.db.db_models",
    "common.metadata_utils",
    "common.log_utils",
    "common.config_utils",
    "common.versions",
    "common.token_utils",
    "common.signal_utils",
    "common.exceptions",
    "common.constants",
    "rag.utils.base64_image",
    "rag.utils.raptor_utils",
    "rag.prompts.generator",
    "rag.raptor",
    "rag.app",
    "rag.graphrag.utils",
]
for _m in _EXTRA_MOCKS:
    if _m not in sys.modules:
        sys.modules[_m] = MagicMock()

# ---------------------------------------------------------------------------
# Load the real implementations via importlib.
# ---------------------------------------------------------------------------
_ROOT = pathlib.Path(__file__).parents[4]


def _load_module(dotted_name: str, rel_path: str):
    path = _ROOT / rel_path
    spec = importlib.util.spec_from_file_location(dotted_name, path)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[dotted_name] = mod
    spec.loader.exec_module(mod)
    return mod


_index_mod = _load_module("rag.graphrag.general.index", "rag/graphrag/general/index.py")
_executor_mod = _load_module("rag.svr.task_executor", "rag/svr/task_executor.py")

load_subgraph_from_store = _index_mod.load_subgraph_from_store
has_raptor_chunks = _executor_mod.has_raptor_chunks

# settings is a MagicMock installed by conftest; grab it to monkeypatch docStoreConn.
import common.settings as _settings  # noqa: E402


# ---------------------------------------------------------------------------
# Shared helpers
# ---------------------------------------------------------------------------

def _make_subgraph(doc_id: str) -> nx.Graph:
    sg = nx.Graph()
    sg.add_node("ENTITY_A", description="test entity A", source_id=[doc_id])
    sg.add_node("ENTITY_B", description="test entity B", source_id=[doc_id])
    sg.add_edge("ENTITY_A", "ENTITY_B", description="related", source_id=[doc_id], weight=1.0, keywords=[])
    sg.graph["source_id"] = [doc_id]
    return sg


def _to_store_content(sg: nx.Graph) -> str:
    return json.dumps(nx.node_link_data(sg, edges="edges"), ensure_ascii=False)


def _single_page_mocks(field_map: dict):
    """search + get_fields mocks that simulate a single-page result."""
    sentinel = object()
    call_count = {"n": 0}

    def _get_fields(_res, _fields):
        call_count["n"] += 1
        return field_map if call_count["n"] == 1 else {}

    return MagicMock(return_value=sentinel), MagicMock(side_effect=_get_fields)


# ---------------------------------------------------------------------------
# Tests for load_subgraph_from_store  (rag/graphrag/general/index.py)
# ---------------------------------------------------------------------------

class TestLoadSubgraphFromStore:

    @pytest.mark.p1
    @pytest.mark.asyncio
    async def test_loads_existing_subgraph(self, monkeypatch):
        """Subgraph present in the store is returned as nx.Graph."""
        doc_id = "doc_001"
        sg = _make_subgraph(doc_id)
        field_map = {"chunk_001": {"content_with_weight": _to_store_content(sg), "source_id": [doc_id]}}
        s, gf = _single_page_mocks(field_map)
        monkeypatch.setattr(_settings.docStoreConn, "search", s)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", gf)

        result = await load_subgraph_from_store("t1", "kb1", doc_id)

        assert result is not None and isinstance(result, nx.Graph)
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
    async def test_passes_doc_id_in_search_condition(self, monkeypatch):
        """source_id (== doc_id) is included in the search condition so the doc
        store filters results directly rather than fetching all subgraphs."""
        captured = {}

        def _capture(fields, filters, condition, *_a, **_kw):
            captured["condition"] = condition
            return object()

        sg = _make_subgraph("doc_b")
        monkeypatch.setattr(_settings.docStoreConn, "search", _capture)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields",
                            MagicMock(return_value={"chunk_b": {"content_with_weight": _to_store_content(sg), "source_id": ["doc_b"]}}))

        result = await load_subgraph_from_store("t1", "kb1", "doc_b")
        assert result is not None and result.graph["source_id"] == ["doc_b"]
        assert captured["condition"]["source_id"] == ["doc_b"]

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_skips_malformed_json_returns_none(self, monkeypatch):
        """Malformed JSON is logged and skipped; None is returned (not raised)."""
        field_map = {"chunk_bad": {"content_with_weight": "not valid json{{{", "source_id": ["doc_bad"]}}
        s, gf = _single_page_mocks(field_map)
        monkeypatch.setattr(_settings.docStoreConn, "search", s)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", gf)

        assert await load_subgraph_from_store("t1", "kb1", "doc_bad") is None

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_issues_single_query_with_limit_one(self, monkeypatch):
        """Exactly one search call is issued with limit=1 — the doc store index
        does the filtering, so no pagination is required."""
        doc_id = "doc_single"
        sg = _make_subgraph(doc_id)
        search_calls: list[tuple] = []

        def _search(fields, filters, condition, order, orderby, offset, limit, *_a, **_kw):
            search_calls.append((offset, limit))
            return object()

        monkeypatch.setattr(_settings.docStoreConn, "search", _search)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields",
                            MagicMock(return_value={"chunk_t": {"content_with_weight": _to_store_content(sg), "source_id": [doc_id]}}))

        result = await load_subgraph_from_store("t1", "kb1", doc_id)
        assert result is not None
        assert len(search_calls) == 1, "must issue exactly one query"
        assert search_calls[0] == (0, 1), "must use offset=0, limit=1"

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_doc_store_exception_returns_none(self, monkeypatch):
        """A doc-store exception is caught; None is returned safely."""
        monkeypatch.setattr(_settings.docStoreConn, "search", MagicMock(side_effect=RuntimeError("db down")))
        assert await load_subgraph_from_store("t1", "kb1", "doc_001") is None


# ---------------------------------------------------------------------------
# Tests for has_raptor_chunks  (rag/svr/task_executor.py)
# ---------------------------------------------------------------------------

class TestHasRaptorChunks:

    @pytest.mark.p1
    @pytest.mark.asyncio
    async def test_returns_true_when_raptor_chunk_exists(self, monkeypatch):
        """Doc store returns a RAPTOR row -> True."""
        monkeypatch.setattr(_settings.docStoreConn, "search", MagicMock(return_value=object()))
        monkeypatch.setattr(_settings.docStoreConn, "get_fields",
                            MagicMock(return_value={"chunk_r": {"raptor_kwd": "raptor"}}))

        assert await has_raptor_chunks("doc_001", "t1", "kb1") is True

    @pytest.mark.p1
    @pytest.mark.asyncio
    async def test_returns_false_when_no_raptor_chunks(self, monkeypatch):
        """Doc store returns empty -> False."""
        monkeypatch.setattr(_settings.docStoreConn, "search", MagicMock(return_value=object()))
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", MagicMock(return_value={}))

        assert await has_raptor_chunks("doc_001", "t1", "kb1") is False

    @pytest.mark.p1
    @pytest.mark.asyncio
    async def test_queries_specifically_for_raptor_kwd(self, monkeypatch):
        """raptor_kwd is in the search condition so non-RAPTOR leading chunks
        cannot produce a false-negative."""
        captured = {}

        def _capture(fields, filters, condition, *_a, **_kw):
            captured["condition"] = condition
            return object()

        monkeypatch.setattr(_settings.docStoreConn, "search", _capture)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", MagicMock(return_value={}))

        await has_raptor_chunks("doc_001", "t1", "kb1")
        assert captured["condition"] == {"doc_id": "doc_001", "raptor_kwd": ["raptor"]}

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_returns_false_on_doc_store_exception(self, monkeypatch):
        """Exception is caught; False is returned without crashing."""
        monkeypatch.setattr(_settings.docStoreConn, "search", MagicMock(side_effect=RuntimeError("db down")))
        assert await has_raptor_chunks("doc_001", "t1", "kb1") is False


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
        # The doc store filters by source_id (doc_id) directly, so get_fields
        # should return only the matching chunk for each call.
        def _get_fields_by_doc(res, fields):
            # res is the sentinel from search; extract the doc_id it was called with
            return {k: v for k, v in field_map.items() if v["source_id"] == [_get_fields_by_doc.last_doc_id]}

        def _search(fields, filters, condition, *_a, **_kw):
            _get_fields_by_doc.last_doc_id = (condition or {}).get("source_id", [None])[0]
            return object()

        monkeypatch.setattr(_settings.docStoreConn, "search", _search)
        monkeypatch.setattr(_settings.docStoreConn, "get_fields", _get_fields_by_doc)

        for doc_id in completed:
            result = await load_subgraph_from_store("t1", "kb1", doc_id)
            assert result is not None and result.graph["source_id"] == [doc_id]

        assert await load_subgraph_from_store("t1", "kb1", "doc_4_new") is None
