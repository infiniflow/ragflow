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
"""Unit tests for knowledge-base scoping in ``OSConnection.update``.

``ESConnection.update`` pins every update to one knowledge base with
``condition["kb_id"] = knowledgebase_id`` before it builds the query, and
``OSConnection.search``/``delete`` do the same. The multi-document branch of
``OSConnection.update`` accepted ``knowledgebaseId`` but never applied it, so an
update whose condition carried no ``kb_id`` of its own ran against every document
in the tenant index — all knowledge bases share ``ragflow_{tenant_id}``.

These tests pin the query ``update`` builds, so the scoping cannot be dropped again.
"""

from __future__ import annotations

import sys
import types
from unittest.mock import MagicMock, patch

import pytest

opensearchpy = pytest.importorskip("opensearchpy")


def _stub(name: str, **attrs) -> types.ModuleType:
    mod = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    return mod


def _unwrap(candidate):
    """``@singleton`` wraps the class in a closure; unwrap it to get the type."""
    if isinstance(candidate, type):
        return candidate
    for cell in getattr(candidate, "__closure__", None) or ():
        if isinstance(cell.cell_contents, type):
            return cell.cell_contents
    raise RuntimeError("Could not locate the OSConnection class in module scope")


@pytest.fixture
def os_conn():
    """An OSConnection whose query builder is captured, with no live cluster.

    ``opensearch_conn`` imports ``common.settings`` — which pulls every storage
    backend (Infinity, OceanBase, Azure, MinIO, GCS …) — and ``rag.nlp`` at module
    load, and neither is needed here: the settings values are only read in
    ``__init__``, which these tests bypass via ``__new__``.

    The stubs stay inside ``patch.dict`` rather than being installed at module
    scope: module-scope mutation runs during collection and would leave the stubs
    in the global cache for every test collected afterwards, so a run that reached
    this file before importing the real ``common.settings`` would hand the stub to
    everything downstream. ``patch.dict`` restores ``sys.modules`` afterwards,
    including dropping the ``opensearch_conn`` imported under the stubs.
    """
    stubs = {
        "common.settings": _stub(
            "common.settings",
            OS={"hosts": "stub", "username": "u", "password": "p"},
            ES={},
            DOC_ENGINE_INFINITY=False,
            DOC_ENGINE_OCEANBASE=False,
            DOC_ENGINE="opensearch",
            docStoreConn=None,
        ),
        "rag.nlp": _stub("rag.nlp", is_english=lambda *_a, **_k: False, rag_tokenizer=MagicMock()),
    }
    seen: dict = {}

    class _FakeUpdateByQuery:
        def __init__(self, index=None):
            seen["index"] = index

        def using(self, _client):
            return self

        def query(self, q):
            seen["query"] = q
            return self

        def script(self, **_kwargs):
            return self

        def params(self, **_kwargs):
            return self

        def execute(self):
            return MagicMock()

    with patch.dict(sys.modules, stubs):
        sys.modules.pop("rag.utils.opensearch_conn", None)
        from rag.utils import opensearch_conn

        with patch.object(opensearch_conn, "UpdateByQuery", _FakeUpdateByQuery):
            cls = _unwrap(opensearch_conn.OSConnection)
            conn = cls.__new__(cls)
            conn.os = MagicMock()
            conn.info = {"version": {"number": "2.18.0"}}
            conn.mapping = {"settings": {}, "mappings": {}}
            yield conn, seen


def _filters(seen: dict) -> list[dict]:
    return seen["query"].to_dict()["bool"]["filter"]


def _kb_filters(seen: dict) -> list[dict]:
    return [f for f in _filters(seen) if "kb_id" in next(iter(f.values()), {})]


def test_update_without_kb_id_in_condition_is_still_scoped_to_the_knowledge_base(os_conn):
    # The pagerank-clear path: condition carries no kb_id of its own.
    conn, seen = os_conn
    conn.update({"exists": "pagerank_fea"}, {"remove": "pagerank_fea"}, "ragflow_tenant1", "kb-A")
    assert {"term": {"kb_id": "kb-A"}} in _filters(seen)


def test_update_keeps_the_exists_filter_alongside_the_kb_scope(os_conn):
    conn, seen = os_conn
    conn.update({"exists": "pagerank_fea"}, {"remove": "pagerank_fea"}, "ragflow_tenant1", "kb-A")
    assert {"exists": {"field": "pagerank_fea"}} in _filters(seen)


def test_update_scopes_to_exactly_one_knowledge_base(os_conn):
    conn, seen = os_conn
    conn.update({"exists": "pagerank_fea"}, {"remove": "pagerank_fea"}, "ragflow_tenant1", "kb-A")
    assert len(_kb_filters(seen)) == 1


def test_update_by_doc_id_is_scoped_to_the_documents_knowledge_base(os_conn):
    # document_api_service.py passes {"doc_id": ...} with no kb_id.
    conn, seen = os_conn
    conn.update({"doc_id": "doc-1"}, {"available_int": 0}, "ragflow_tenant1", "kb-A")
    assert {"term": {"kb_id": "kb-A"}} in _filters(seen)


def test_update_with_a_matching_kb_id_already_in_condition_targets_that_kb(os_conn):
    # dataset_api_service.py passes {"tag_kwd": t, "kb_id": [dataset_id]}.
    conn, seen = os_conn
    conn.update({"tag_kwd": "t1", "kb_id": ["kb-A"]}, {"remove": {"tag_kwd": "t1"}}, "ragflow_tenant1", "kb-A")
    assert len(_kb_filters(seen)) == 1
    assert _kb_filters(seen)[0] in ({"term": {"kb_id": "kb-A"}}, {"terms": {"kb_id": ["kb-A"]}})
    assert {"term": {"tag_kwd": "t1"}} in _filters(seen)


def test_single_document_update_still_goes_through_the_by_id_path(os_conn):
    # Control: the `id` branch updates by _id and must not build a query at all.
    conn, seen = os_conn
    conn.update({"id": "chunk-1"}, {"available_int": 0}, "ragflow_tenant1", "kb-A")
    assert "query" not in seen
    assert conn.os.update.called


def test_the_knowledgebase_argument_is_authoritative(os_conn):
    # Mirrors ESConnection.update: the kb scope comes from the argument, so a
    # caller cannot widen it past its own knowledge base via the condition.
    conn, seen = os_conn
    conn.update({"exists": "pagerank_fea", "kb_id": "kb-B"}, {"remove": "pagerank_fea"}, "ragflow_tenant1", "kb-A")
    assert _kb_filters(seen) == [{"term": {"kb_id": "kb-A"}}]
