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
"""Sorting on ``id`` must follow the live index mapping.

Indices created before ``id`` was matched by the keyword template map it as
``text`` with an ``id.keyword`` sub-field, which is the only sortable form.
Newer indices map ``id`` itself as ``keyword``. Dropping the sort leaves the
search_after deep-pagination path without sort values, so every page past
``index.max_result_window`` comes back empty.
"""

from unittest.mock import MagicMock

import pytest

from common import settings  # noqa: F401  # resolves the es_conn_base <-> settings import cycle
from common.doc_store.doc_store_base import OrderByExpr
from rag.utils import es_conn

pytestmark = pytest.mark.p2

_KEYWORD_ID = {"full_name": "id", "mapping": {"id": {"type": "keyword"}}}
_TEXT_ID = {"full_name": "id", "mapping": {"id": {"type": "text", "fields": {"keyword": {"type": "keyword", "ignore_above": 256}}}}}


def _connection(field_mappings: dict):
    factory = es_conn.ESConnection
    cls = next(cell.cell_contents for cell in factory.__closure__ or () if isinstance(cell.cell_contents, type))
    conn = cls.__new__(cls)
    conn.logger = MagicMock()
    conn.es = MagicMock()
    conn.es.indices.get_field_mapping.return_value = field_mappings
    return conn


def _sort_for(conn, index_names, offset=0, limit=10):
    captured = []

    def search_once(_index_names, query, **_kwargs):
        captured.append(query)
        return {"timed_out": False, "hits": {"total": {"value": 0}, "hits": []}}

    conn._es_search_once = search_once
    conn.search(["id"], [], {}, [], OrderByExpr().asc("id"), offset, limit, index_names, ["kb-1"])
    return captured[0].get("sort")


def test_sorts_on_id_when_index_maps_id_as_keyword():
    conn = _connection({"ragflow_t": {"mappings": {"id": _KEYWORD_ID}}})

    assert _sort_for(conn, "ragflow_t") == [{"id": {"order": "asc", "unmapped_type": "keyword"}}]


def test_sorts_on_id_keyword_when_legacy_index_maps_id_as_text():
    conn = _connection({"ragflow_t": {"mappings": {"id": _TEXT_ID}}})

    assert _sort_for(conn, "ragflow_t") == [{"id.keyword": {"order": "asc", "unmapped_type": "keyword"}}]


def test_sorts_on_id_when_no_index_has_mapped_id_yet():
    conn = _connection({"ragflow_t": {"mappings": {}}})

    assert _sort_for(conn, "ragflow_t") == [{"id": {"order": "asc", "unmapped_type": "keyword"}}]


def test_drops_id_sort_when_indices_disagree_on_the_sortable_field():
    conn = _connection(
        {
            "ragflow_a": {"mappings": {"id": _KEYWORD_ID}},
            "ragflow_b": {"mappings": {"id": _TEXT_ID}},
        }
    )

    assert _sort_for(conn, ["ragflow_a", "ragflow_b"]) is None


def test_legacy_index_pages_past_max_result_window_with_search_after():
    conn = _connection({"ragflow_t": {"mappings": {"id": _TEXT_ID}}})
    queries = []

    def search_once(_index_names, query, **_kwargs):
        queries.append(query)
        after = query.get("search_after", [""])[0]
        start = int(after) + 1 if after else 0
        hits = [{"_id": f"{i:06d}", "_source": {"id": f"{i:06d}"}, "sort": [f"{i:06d}"]} for i in range(start, start + query["size"])]
        return {"timed_out": False, "hits": {"total": {"value": 20000}, "hits": hits}}

    conn._es_search_once = search_once
    res = conn.search(["id"], [], {}, [], OrderByExpr().asc("id"), 10000, 1000, "ragflow_t", ["kb-1"])

    assert [h["_id"] for h in res["hits"]["hits"]][:2] == ["010000", "010001"]
    assert len(res["hits"]["hits"]) == 1000
    assert all("from" not in q for q in queries)
    assert all(q["sort"] == [{"id.keyword": {"order": "asc", "unmapped_type": "keyword"}}] for q in queries)
