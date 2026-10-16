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
"""Regression test for #16524.

``get_flatted_meta_by_kbs`` orders the doc-meta index by ``id`` so that
``search_after`` pagination can traverse more than ``MAX_RESULT_WINDOW``
(10,000) documents. ``ESConnection.search`` must therefore emit that sort
field in the request body. Previously the field was silently dropped with a
``continue`` (rationale: "id as 'text', not a 'keyword'"), which left the
query without any ``sort`` clause, so once ``offset + limit`` crossed
``MAX_RESULT_WINDOW`` the ``_search_with_search_after`` loop broke on its
first iteration (``hits[-1].get("sort")`` was ``None``) and the in-memory
fallback only saw the first ~10k documents.

The doc-meta index maps ``id`` as ``keyword`` (``conf/doc_meta_es_mapping.json``)
and the content index maps ``id`` as ``keyword`` via the dynamic ``kwd``
template (``conf/mapping.json``), so the sort is always safe to apply.
"""

from __future__ import annotations

import copy
import sys
import types
from unittest.mock import MagicMock

# Stub the heavy / circular-importing dependencies before importing
# ``rag.utils.es_conn`` so the module can be loaded without a live ES cluster
# or a fully-initialised ``common.settings`` (mirroring test_search_pagination.py).
_fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


_fake_query.FulltextQueryer = _DummyFulltextQueryer
_stubs = {
    "rag.nlp.query": _fake_query,
    "rag.nlp.rag_tokenizer": types.ModuleType("rag.nlp.rag_tokenizer"),
    "common.settings": types.ModuleType("common.settings"),
}
_previous = {name: sys.modules.get(name) for name in _stubs}
_previous_es_modules = {
    name: sys.modules.get(name)
    for name in ("common.doc_store.es_conn_base", "rag.utils.es_conn")
}
for _name, _module in _stubs.items():
    sys.modules.setdefault(_name, _module)

try:
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.utils.es_conn import MAX_RESULT_WINDOW, ESConnection as _ESConnection
finally:
    # These imports can retain the temporary common.settings module. Remove
    # only modules loaded here so later tests import their real dependencies.
    for _name, _prior in _previous_es_modules.items():
        if _prior is None:
            sys.modules.pop(_name, None)
    for _name, _prior in _previous.items():
        if _prior is None and sys.modules.get(_name) is _stubs[_name]:
            del sys.modules[_name]


def _resolve_es_connection_class():
    candidate = _ESConnection
    if isinstance(candidate, type):
        return candidate
    closure = getattr(candidate, "__closure__", None) or ()
    for cell in closure:
        contents = cell.cell_contents
        if isinstance(contents, type):
            return contents
    raise RuntimeError("Could not locate the ESConnection class in module scope")


def _make_es_connection():
    cls = _resolve_es_connection_class()
    conn = cls.__new__(cls)
    conn.logger = MagicMock()
    return conn


def _build_es_response(start: int, batch_size: int, total: int):
    hits = []
    for i in range(start, min(start + batch_size, total)):
        hits.append(
            {
                "_id": f"doc-{i:05d}",
                "_source": {"id": f"doc-{i:05d}", "kb_id": "kb-1", "meta_fields": {"canon": "1"}},
                # ES populates ``sort`` only when the request asked for one.
                # Including it here is what makes ``_search_with_search_after``
                # advance instead of breaking on its first iteration.
                "sort": [f"doc-{i:05d}"],
            }
        )
    return {"timed_out": False, "hits": {"total": {"value": total}, "hits": hits}}


def test_search_by_id_emits_sort_clause_for_elasticsearch():
    """``order_by.asc("id")`` must reach the ES request body as a sort clause."""
    conn = _make_es_connection()
    captured = {}

    def search_once(_index_names, query, **_kwargs):
        captured["query"] = query
        return _build_es_response(0, 5, 5)

    conn._es_search_once = search_once

    conn.search(
        ["*"],
        [],
        {"kb_id": ["kb-1"]},
        [],
        OrderByExpr().asc("id"),
        0,
        5,
        "ragflow_doc_meta_tenant-1",
        ["kb-1"],
    )

    assert captured["query"].get("sort") == [{"id": {"order": "asc", "unmapped_type": "keyword"}}]


def test_search_after_pagination_with_id_sort_returns_all_documents_beyond_max_result_window():
    """Reproduces the #16524 scenario.

    A doc-meta index with > 10k rows must be paginated through completely
    via ``search_after`` once ``offset + limit`` crosses
    ``MAX_RESULT_WINDOW``. Without the fix, the ``id`` sort is dropped, the
    ``search_after`` loop sees ``hits[-1]["sort"] is None``, breaks on the
    first iteration, and the caller only ever sees one batch.
    """
    conn = _make_es_connection()
    total = MAX_RESULT_WINDOW + 2_500
    page_size = 1_000

    calls: list[dict] = []

    def search_once(_index_names, query, **_kwargs):
        calls.append(copy.deepcopy(query))
        if "from" in query and "search_after" not in query:
            start = query["from"]
            return _build_es_response(start, page_size, total)
        search_after = query.get("search_after")
        if search_after is None:
            start = 0
        else:
            start = int(search_after[0].rsplit("-", 1)[-1]) + 1
        return _build_es_response(start, page_size, total)

    conn._es_search_once = search_once

    # Match the loop in ``DocMetadataService.get_flatted_meta_by_kbs``: 1k
    # per page, paginating with growing offsets.
    collected = []
    page_size = 1_000
    offset = 0
    while True:
        res = conn.search(
            ["*"],
            [],
            {"kb_id": ["kb-1"]},
            [],
            OrderByExpr().asc("id"),
            offset,
            page_size,
            "ragflow_doc_meta_tenant-1",
            ["kb-1"],
        )
        hits = res["hits"]["hits"]
        if not hits:
            break
        collected.extend(h["_source"]["id"] for h in hits)
        if len(hits) < page_size:
            break
        offset += page_size

    # Every document must be observed, not just the first ~10k.
    assert len(collected) == total
    assert collected == [f"doc-{i:05d}" for i in range(total)]
    # And the second call past the window must actually use search_after.
    saw_search_after = any("search_after" in call for call in calls)
    assert saw_search_after, "search_after must engage past MAX_RESULT_WINDOW"


def test_explicit_search_after_cursor_fetches_next_page_without_replaying_prior_pages():
    conn = _make_es_connection()
    calls = []

    def search_once(_index_names, query, **_kwargs):
        calls.append(copy.deepcopy(query))
        cursor = query.get("search_after")
        start = int(cursor[0].rsplit("-", 1)[-1]) + 1 if cursor else 0
        return _build_es_response(start, 1_000, MAX_RESULT_WINDOW + 2_500)

    conn._es_search_once = search_once
    cursor = None
    collected = []
    for _ in range(13):
        result = conn.search(
            ["*"], [], {"kb_id": ["kb-1"]}, [], OrderByExpr().asc("id"),
            0, 1_000, "ragflow_doc_meta_tenant-1", ["kb-1"],
            search_after=cursor,
        )
        hits = result["hits"]["hits"]
        collected.extend(hit["_id"] for hit in hits)
        cursor = hits[-1]["sort"]
    assert len(collected) == MAX_RESULT_WINDOW + 2_500
    assert collected == [f"doc-{i:05d}" for i in range(MAX_RESULT_WINDOW + 2_500)]
    assert len(calls) == 13
    assert all("from" not in query for query in calls[1:])
    assert calls[11]["search_after"] == ["doc-10999"]


def test_explicit_cursor_requires_unique_final_id_and_full_sort_tuple():
    conn = _make_es_connection()
    from pytest import raises

    with raises(ValueError, match="final unique sort field"):
        conn.search(["*"], [], {"kb_id": ["kb-1"]}, [],
                    OrderByExpr().asc("available_int"), 0, 100,
                    "ragflow_doc_meta_tenant-1", ["kb-1"], search_after=[1])
    order_by = OrderByExpr().asc("available_int").asc("id")
    with raises(ValueError, match="every returned sort value"):
        conn.search(["*"], [], {"kb_id": ["kb-1"]}, [],
                    order_by, 0, 100, "ragflow_doc_meta_tenant-1", ["kb-1"],
                    search_after=[1])
