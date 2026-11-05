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
"""Deep pagination past ``index.max_result_window`` on the OpenSearch backend.

Issue #21522: retrieval with a metadata condition over a dataset holding more
than 10,000 documents fails with

    RequestError(400, 'search_phase_execution_exception',
    'Result window is too large, from + size must be less than or equal to:
     [10000] but was [11000] ...')

``DocMetadataService.get_flatted_meta_by_kbs`` pages the doc-metadata index
1,000 rows at a time, so its 11th request asks for ``from=10000, size=1000``.
``ESConnection.search`` stops emitting ``from`` once ``offset + limit`` exceeds
``MAX_RESULT_WINDOW`` and walks the result set with ``search_after`` instead
(``_search_with_search_after``, resolving a sortable ``id`` field through the
index mapping). The OpenSearch connector applied the page unconditionally, so
the same caller that passes on Elasticsearch raises on OpenSearch.

These tests drive ``OSConnection.search`` against a fake client that enforces
the same window rejection as a real cluster, so the regression is observable
without a live OpenSearch.
"""

from __future__ import annotations

import copy
import sys
import types
from unittest.mock import MagicMock

import pytest

opensearchpy = pytest.importorskip("opensearchpy")

pytestmark = pytest.mark.p2

_WINDOW = 10000
_TOTAL_DOCS = 20000

_KEYWORD_ID = {"full_name": "id", "mapping": {"id": {"type": "keyword"}}}
_TEXT_ID = {
    "full_name": "id",
    "mapping": {"id": {"type": "text", "fields": {"keyword": {"type": "keyword", "ignore_above": 256}}}},
}
_UNSORTABLE_ID = {"full_name": "id", "mapping": {"id": {"type": "text"}}}


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
    """Keep ``opensearch_conn`` importable without the full storage stack."""
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


def _resolve_os_connection_class():
    """``OSConnection`` is wrapped by ``@singleton``; recover the real class."""
    from rag.utils import opensearch_conn

    candidate = opensearch_conn.OSConnection
    if isinstance(candidate, type):
        return candidate
    for cell in getattr(candidate, "__closure__", None) or ():
        contents = cell.cell_contents
        if isinstance(contents, type):
            return contents
    raise RuntimeError("Could not locate the OSConnection class in module scope")


def _connection(field_mappings: dict):
    cls = _resolve_os_connection_class()
    conn = cls.__new__(cls)
    conn.os = MagicMock()
    conn.os.indices.get_field_mapping.return_value = field_mappings
    conn.info = {"version": {"number": "2.18.0"}}
    conn.hybrid_search_enabled = False
    return conn


def _install_module_stubs() -> None:
    """Keep ``opensearch_conn`` importable without the full storage stack."""
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


@pytest.fixture(scope="module", autouse=True)
def _stubbed_imports():
    """Install the import stubs for this module only, then restore.

    ``opensearch_conn`` needs ``common.settings`` and ``rag.nlp`` at import
    time, but ``rag.nlp`` is a real package: leaving a ``types.ModuleType``
    stand-in in ``sys.modules`` would make a later ``import rag.nlp.delim`` in
    the same session fail with "'rag.nlp' is not a package". Install the stubs
    here and put the previous entries back when the module finishes, so
    collecting this file cannot affect collection of another one.
    """
    saved = {name: sys.modules.get(name) for name in ("common.settings", "rag.nlp")}
    _install_module_stubs()
    yield
    for name, module in saved.items():
        if module is None:
            sys.modules.pop(name, None)
        else:
            sys.modules[name] = module


def _fake_search(queries: list):
    """Serve a sorted universe and reject pages that exceed the window."""

    def search(index=None, body=None, **_kwargs):
        queries.append(copy.deepcopy(body))
        from_ = body.get("from", 0)
        size = body.get("size", 10)
        if from_ + size > _WINDOW:
            raise opensearchpy.exceptions.RequestError(
                400,
                "search_phase_execution_exception",
                f"Result window is too large, from + size must be less than or equal to: [{_WINDOW}] but was [{from_ + size}]. See the scroll api for a more efficient way to request large data sets.",
            )
        cursor = body.get("search_after")
        start = int(cursor[0]) + 1 if cursor else from_
        ids = [] if size == 0 else [f"{i:06d}" for i in range(start, min(start + size, _TOTAL_DOCS))]
        hits = [{"_id": i, "_source": {"id": i}, "sort": [i]} for i in ids]
        return {
            "timed_out": False,
            "hits": {"total": {"value": _TOTAL_DOCS, "relation": "eq"}, "hits": hits},
        }

    return search


def _deep_conn(queries: list, mapping: dict = _KEYWORD_ID):
    conn = _connection({"ragflow_kb": {"mappings": {"id": mapping}}})
    conn.os.search.side_effect = _fake_search(queries)
    return conn


def _search(conn, offset: int, limit: int, order_by=None):
    from common.doc_store.doc_store_base import OrderByExpr

    return conn.search(
        ["*"],
        [],
        {},
        [],
        order_by if order_by is not None else OrderByExpr().asc("id"),
        offset,
        limit,
        "ragflow_kb",
        ["kb-1"],
    )


def test_deep_page_is_served_through_search_after():
    """The 11th metadata page must not send ``from=10000, size=1000``."""
    queries = []
    conn = _deep_conn(queries)

    res = _search(conn, 10000, 1000)

    assert [h["_id"] for h in res["hits"]["hits"]][:2] == ["010000", "010001"]
    assert len(res["hits"]["hits"]) == 1000
    assert all("from" not in q for q in queries), "the deep page still asked for from=%s" % [q.get("from") for q in queries]


def test_window_edge_still_uses_from():
    """A page that fits inside the window must keep the cheap ``from`` request."""
    queries = []
    conn = _deep_conn(queries)

    _search(conn, 9000, 1000)

    assert queries[0].get("from") == 9000
    assert "search_after" not in queries[0]


@pytest.mark.parametrize(
    "mapping, expected_field",
    [
        (_KEYWORD_ID, "id"),
        (_TEXT_ID, "id.keyword"),
        ({}, "id"),
    ],
)
def test_id_sort_follows_index_mapping(mapping, expected_field):
    """New indices map ``id`` as keyword, legacy ones only ``id.keyword``."""
    queries = []
    conn = _deep_conn(queries, mapping=mapping)

    _search(conn, 0, 10)

    assert list(queries[0]["sort"][0]) == [expected_field]


def test_unsortable_id_drops_the_sort_and_keeps_the_window_error():
    """Mirror of the ES negative case.

    ``search_after`` needs a stable cursor. When the index has no sortable
    ``id`` the sort cannot be kept, the deep page has to fall back to ``from``,
    and the window rejection must surface rather than be silently swallowed
    into an empty result.
    """
    queries = []
    conn = _deep_conn(queries, mapping=_UNSORTABLE_ID)

    with pytest.raises(opensearchpy.exceptions.RequestError):
        _search(conn, 10000, 1000)

    assert queries[-1].get("from") == 10000


def test_mapping_lookup_failure_falls_back_to_a_plain_id_sort():
    """A failed mapping read must not turn an id-ordered search into an error.

    Before the deep-pagination change an ``id`` sort sent a plain sort and made
    no extra request. The lookup added here runs outside the retry block, so a
    transient error or a cluster that denies the mapping-read privilege would
    otherwise fail every id-ordered search.
    """
    queries = []
    conn = _connection({})
    conn.os.indices.get_field_mapping.side_effect = RuntimeError("security_exception: no mapping read")
    conn.os.search.side_effect = _fake_search(queries)

    _search(conn, 0, 10)

    assert list(queries[0]["sort"][0]) == ["id"]


def test_deep_page_requires_a_unique_sort():
    """``search_after`` needs a total order; a page sort does not provide one.

    Sorting by ``page_num_int`` leaves ties, and with ties on the cursor value
    the walk either skips documents that share it at a batch boundary or sees
    ``next_search_after == search_after`` and stops early - a short page with no
    error, which is worse than the 400 this change set out to remove. Such a
    page therefore stays on ``from`` and keeps failing loudly, and the id
    tiebreaker is *not* appended: that would silently change which documents a
    deep page contains for every caller sorting on a position field.
    """
    from common.doc_store.doc_store_base import OrderByExpr

    queries = []
    conn = _deep_conn(queries)

    with pytest.raises(opensearchpy.exceptions.RequestError):
        _search(conn, 10000, 1000, order_by=OrderByExpr().asc("page_num_int"))

    body = queries[-1]
    assert body.get("from") == 10000
    assert "search_after" not in body
    assert [s for s in (body.get("sort") or []) if isinstance(s, dict) and "id" in s] == []


def test_deep_page_does_not_duplicate_an_existing_id_sort():
    """An id sort already provides the total order; do not append it twice."""
    queries = []
    conn = _deep_conn(queries)

    _search(conn, 10000, 1000)

    sorts = queries[0].get("sort") or []
    id_entries = [s for s in sorts if isinstance(s, dict) and "id" in s]
    assert len(id_entries) == 1, f"expected exactly one id sort, got: {sorts}"
