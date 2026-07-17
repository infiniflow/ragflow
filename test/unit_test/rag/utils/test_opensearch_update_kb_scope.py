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
``OSConnection.update`` accepted ``knowledgebaseId`` but never applied it, so
an update whose condition carried no ``kb_id`` of its own ran against every
document in the tenant index — all knowledge bases share ``ragflow_{tenant_id}``.

These tests pin the query that ``update`` builds, so the scoping cannot be
dropped again.
"""

from __future__ import annotations

import sys
import types
from unittest.mock import MagicMock

import pytest

opensearchpy = pytest.importorskip("opensearchpy")


def _install_module(name: str, **attrs) -> types.ModuleType:
    mod = sys.modules.get(name)
    if mod is None:
        mod = types.ModuleType(name)
        sys.modules[name] = mod
    for key, value in attrs.items():
        if not hasattr(mod, key):
            setattr(mod, key, value)
    return mod


def _install_module_stubs() -> None:
    """``opensearch_conn`` pulls ``common.settings`` (every storage backend) at
    module load; stub just what it captures so the real class can load."""
    _install_module(
        "common.settings",
        OS={"hosts": "stub", "username": "u", "password": "p"},
        ES={},
        DOC_ENGINE_INFINITY=False,
        DOC_ENGINE_OCEANBASE=False,
        DOC_ENGINE="opensearch",
        docStoreConn=None,
    )
    _install_module(
        "rag.nlp",
        is_english=lambda *_args, **_kwargs: False,
        rag_tokenizer=MagicMock(),
    )


_install_module_stubs()


def _resolve_os_connection_class():
    """``@singleton`` wraps the class in a closure; unwrap it to get the type."""
    from rag.utils import opensearch_conn

    candidate = opensearch_conn.OSConnection
    if isinstance(candidate, type):
        return candidate
    for cell in getattr(candidate, "__closure__", None) or ():
        if isinstance(cell.cell_contents, type):
            return cell.cell_contents
    raise RuntimeError("Could not locate the OSConnection class in module scope")


def _make_os_connection():
    cls = _resolve_os_connection_class()
    instance = cls.__new__(cls)
    instance.os = MagicMock()
    instance.info = {"version": {"number": "2.18.0"}}
    instance.mapping = {"settings": {}, "mappings": {}}
    return instance


@pytest.fixture
def captured_query(monkeypatch):
    """Capture the query handed to UpdateByQuery without touching a cluster."""
    from rag.utils import opensearch_conn

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

    monkeypatch.setattr(opensearch_conn, "UpdateByQuery", _FakeUpdateByQuery)
    return seen


def _filters(seen: dict) -> list[dict]:
    return seen["query"].to_dict()["bool"]["filter"]


def _kb_filters(seen: dict) -> list[dict]:
    return [f for f in _filters(seen) if "kb_id" in next(iter(f.values()), {})]


def test_update_without_kb_id_in_condition_is_still_scoped_to_the_knowledge_base(captured_query):
    # The pagerank-clear path: condition carries no kb_id of its own.
    conn = _make_os_connection()
    conn.update({"exists": "pagerank_fea"}, {"remove": "pagerank_fea"}, "ragflow_tenant1", "kb-A")
    assert {"term": {"kb_id": "kb-A"}} in _filters(captured_query)


def test_update_keeps_the_exists_filter_alongside_the_kb_scope(captured_query):
    conn = _make_os_connection()
    conn.update({"exists": "pagerank_fea"}, {"remove": "pagerank_fea"}, "ragflow_tenant1", "kb-A")
    assert {"exists": {"field": "pagerank_fea"}} in _filters(captured_query)


def test_update_scopes_to_exactly_one_knowledge_base(captured_query):
    conn = _make_os_connection()
    conn.update({"exists": "pagerank_fea"}, {"remove": "pagerank_fea"}, "ragflow_tenant1", "kb-A")
    assert len(_kb_filters(captured_query)) == 1


def test_update_by_doc_id_is_scoped_to_the_documents_knowledge_base(captured_query):
    # document_api_service.py passes {"doc_id": ...} with no kb_id.
    conn = _make_os_connection()
    conn.update({"doc_id": "doc-1"}, {"available_int": 0}, "ragflow_tenant1", "kb-A")
    assert {"term": {"kb_id": "kb-A"}} in _filters(captured_query)


def test_update_with_a_matching_kb_id_already_in_condition_targets_that_kb(captured_query):
    # dataset_api_service.py passes {"tag_kwd": t, "kb_id": [dataset_id]}.
    conn = _make_os_connection()
    conn.update({"tag_kwd": "t1", "kb_id": ["kb-A"]}, {"remove": {"tag_kwd": "t1"}}, "ragflow_tenant1", "kb-A")
    assert len(_kb_filters(captured_query)) == 1
    assert _kb_filters(captured_query)[0] in ({"term": {"kb_id": "kb-A"}}, {"terms": {"kb_id": ["kb-A"]}})
    assert {"term": {"tag_kwd": "t1"}} in _filters(captured_query)


def test_single_document_update_still_goes_through_the_by_id_path(captured_query):
    # Control: the `id` branch updates by _id and must not build a query at all.
    conn = _make_os_connection()
    conn.update({"id": "chunk-1"}, {"available_int": 0}, "ragflow_tenant1", "kb-A")
    assert "query" not in captured_query
    assert conn.os.update.called


def test_the_knowledgebase_argument_is_authoritative(captured_query):
    # Mirrors ESConnection.update: the kb scope comes from the argument, so a
    # caller cannot widen it past its own knowledge base via the condition.
    conn = _make_os_connection()
    conn.update({"exists": "pagerank_fea", "kb_id": "kb-B"}, {"remove": "pagerank_fea"}, "ragflow_tenant1", "kb-A")
    assert _kb_filters(captured_query) == [{"term": {"kb_id": "kb-A"}}]
