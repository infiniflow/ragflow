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
"""Document reads must page in reading order, not the legacy ordering.

``chunk_list`` backs RAPTOR and GraphRAG, and ``retrieval_by_toc`` concatenates
a document's TOC rows (one per parsing task). Leaving ``chunk_order_int`` out of
their sort keys keeps both on the legacy ordering for exactly the rows the
column exists to order: those without position data, and TOCs of a document
parsed in several page ranges.
"""

import json
import sys
import types

import pytest

import common.settings  # noqa: F401  # imported first: breaks the settings <-> redis_conn cycle
from rag.nlp.search import Dealer


class _CapturingDataStore:
    def __init__(self, rows=None):
        self.order_by = None
        self.rows = rows or {}

    def search(self, fields, highlight_fields, condition, match_expressions, order_by, offset, limit, index_names, kb_ids):
        self.order_by = order_by
        return object()

    def get_fields(self, _res, _fields):
        return self.rows


def _dealer(store):
    dealer = Dealer.__new__(Dealer)
    dealer.dataStore = store
    return dealer


def test_chunk_list_sorts_by_the_reading_order_column_first():
    store = _CapturingDataStore()

    _dealer(store).chunk_list("doc-1", "tenant-1", ["kb-1"], sort_by_position=True)

    assert store.order_by.fields == [
        ("chunk_order_int", 0),
        ("page_num_int", 0),
        ("position_int", 0),
        ("top_int", 0),
    ]


def test_chunk_list_without_position_sort_keeps_the_order_by_empty():
    store = _CapturingDataStore()

    _dealer(store).chunk_list("doc-1", "tenant-1", ["kb-1"])

    assert store.order_by.fields == []


@pytest.mark.asyncio
async def test_retrieval_by_toc_reads_the_toc_rows_in_range_order(monkeypatch):
    toc_row = {"content_with_weight": json.dumps([{"chunk": "1.1", "ids": ["c1"]}])}
    store = _CapturingDataStore(rows={"t1": toc_row})

    generator = types.ModuleType("rag.prompts.generator")

    async def _no_ids(*_args, **_kwargs):
        return []

    generator.relevant_chunks_with_toc = _no_ids
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", generator)

    chunks = [{"doc_id": "doc-1", "kb_id": "kb-1", "similarity": 0.5, "chunk_id": "c1"}]

    result = await _dealer(store).retrieval_by_toc("q", chunks, ["tenant-1"], None)

    assert result == chunks
    assert store.order_by.fields == [("chunk_order_int", 0)]
