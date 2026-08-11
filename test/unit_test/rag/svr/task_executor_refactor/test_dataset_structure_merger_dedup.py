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
"""Regression tests for the quadratic dedup fix in dataset_structure_merger.py.

``_merge_bucket``, ``_merge_relations`` and ``_cleanup_deleted_docs`` used to
dedup by scanning a plain list (``item not in some_list``) on every row —
O(n^2) over a bucket/page. The fix tracks a ``seen`` set alongside each
ordered list. These tests pin dedup + relative-order behavior across
duplicate/interleaved/malformed inputs so the quadratic pattern (and any
accidental order regression from a bare ``set()`` swap) cannot come back
silently.
"""

import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from rag.svr.task_executor_refactor.dataset_structure_merger import (
    _cleanup_deleted_docs,
    _merge_bucket,
    _merge_relations,
)


def _entity_row(name: str, *, type_: str = "org", description: str | None = None, chunks=None, doc_id=None, mention_count=None) -> dict:
    payload = {"name": name, "type": type_}
    if description is not None:
        payload["description"] = description
    row: dict = {"content_with_weight": json.dumps(payload)}
    if chunks is not None:
        row["source_chunk_ids"] = chunks
    if doc_id is not None:
        row["doc_id"] = doc_id
    if mention_count is not None:
        row["mention_count_int"] = mention_count
    return row


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_bucket_dedups_chunks_docs_descriptions_and_names_per_group():
    rows = [
        _entity_row("Apple Inc", description="A tech company.", chunks=["c1", "c2"], doc_id="docA"),
        # duplicate doc_id (docA) and duplicate chunk (c2); longer name/description should win as "best"
        _entity_row("Apple Incorporated", description="Maker of iPhone and iPad devices worldwide.", chunks=["c2", "c3"], doc_id="docA"),
        # exact duplicate name + description of row 1 (existing-ID overlap); duplicate chunk c1
        _entity_row("Apple Inc", description="A tech company.", chunks=["c1", "c4"], doc_id="docB"),
        # malformed row: no chunks/doc_id/description at all, and a name duplicate of row 1 & 3
        _entity_row("Apple Inc"),
        # unrelated entity — must not leak into the "apple" group's dedup sets
        _entity_row("Widget Co", type_="product", chunks=["c5"], doc_id="docC"),
    ]

    rows_out = await _merge_bucket("tenant1", "kb1", "compile1", None, rows, None)

    by_name_kwd = {r["name_kwd"]: r for r in rows_out}
    assert set(by_name_kwd) == {"apple inc", "widget co"}

    apple = by_name_kwd["apple inc"]
    assert apple["source_chunk_ids"] == ["c1", "c2", "c3", "c4"]
    assert apple["doc_ids_kwd"] == ["docA", "docB"]
    assert apple["mention_count_int"] == 4  # one implicit mention per row, including the malformed one

    payload_out = json.loads(apple["content_with_weight"])
    assert payload_out["description"] == "Maker of iPhone and iPad devices worldwide."
    assert payload_out["name"] == "Apple Incorporated"

    widget = by_name_kwd["widget co"]
    assert widget["source_chunk_ids"] == ["c5"]
    assert widget["doc_ids_kwd"] == ["docC"]


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_bucket_tolerates_unhashable_chunk_ids_and_descriptions():
    """A malformed source_chunk_ids entry or description (list/dict instead of
    a string) must not raise TypeError just because dedup is set-backed."""
    rows = [
        _entity_row("Gadget Co", description=["not", "a", "string"], chunks=[["bad", "id"], "c1"], doc_id="docA"),
        # exact duplicate malformed chunk id + malformed description -> both deduped via repr()
        _entity_row("Gadget Co", description=["not", "a", "string"], chunks=[["bad", "id"], "c2"], doc_id="docA"),
    ]

    rows_out = await _merge_bucket("tenant1", "kb1", "compile1", None, rows, None)

    assert len(rows_out) == 1
    row = rows_out[0]
    assert row["source_chunk_ids"] == [["bad", "id"], "c1", "c2"]


def _relation_row(*, from_="apple", to="iphone", type_="produces", chunks=None, doc_id=None, use_row_level_fallback=False) -> dict:
    if use_row_level_fallback:
        # "type" stays in the payload (no row-level fallback exists for it);
        # only "from"/"to" are exercised via from_entity_kwd/to_entity_kwd.
        row: dict = {"content_with_weight": json.dumps({"type": type_}), "from_entity_kwd": from_, "to_entity_kwd": to}
    else:
        row = {"content_with_weight": json.dumps({"from": from_, "to": to, "type": type_})}
    if chunks is not None:
        row["source_chunk_ids"] = chunks
    if doc_id is not None:
        row["doc_id"] = doc_id
    return row


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_relations_dedups_chunks_and_docs_per_group():
    rows = [
        _relation_row(chunks=["c1", "c2"], doc_id="docA"),
        _relation_row(chunks=["c2", "c3"], doc_id="docA"),  # duplicate doc + chunk
        _relation_row(chunks=["c1", "c4"], doc_id="docB"),  # duplicate chunk
        _relation_row(use_row_level_fallback=True),  # malformed: no chunks/doc_id, falls back to row-level entity kwds
        _relation_row(to="ipad", chunks=["c5"], doc_id="docC"),  # different group — must stay isolated
    ]

    rows_out = await _merge_relations("tenant1", "kb1", "compile1", None, rows, None)

    by_target = {r["to_entity_kwd"]: r for r in rows_out}
    assert set(by_target) == {"iphone", "ipad"}

    iphone = by_target["iphone"]
    assert iphone["source_chunk_ids"] == ["c1", "c2", "c3", "c4"]
    assert iphone["doc_ids_kwd"] == ["docA", "docB"]

    ipad = by_target["ipad"]
    assert ipad["source_chunk_ids"] == ["c5"]
    assert ipad["doc_ids_kwd"] == ["docC"]


def _fake_get_fields(res, fields):
    return {row["id"]: {f: row.get(f) for f in fields} for row in res}


@pytest.mark.p1
@pytest.mark.asyncio
async def test_cleanup_deleted_docs_dedups_deleted_ids_and_preserves_live_docs():
    # Pass 1: two deletion markers point at the same doc (e.g. a retried
    # record_doc_deletion call) — the dedup must collapse them.
    pass1_rows = [
        {"id": "meta1", "deleted_doc_id": "docA"},
        {"id": "meta2", "deleted_doc_id": "docA"},
        {"id": "meta3", "deleted_doc_id": "docB"},
    ]
    pass2_rows = [
        # duplicate + interleaved deleted ids, one live doc left over -> update, order preserved
        {"id": "entity1", "doc_ids_kwd": ["docA", "docA", "docB", "docC"]},
        # only deleted docs left -> ghost, delete
        {"id": "entity2", "doc_ids_kwd": ["docA"]},
        # malformed provenance (not a list) must not crash the pass
        {"id": "entity3", "doc_ids_kwd": "docA"},
    ]

    mock_conn = MagicMock()
    mock_conn.index_exist.return_value = True
    mock_conn.search.side_effect = [pass1_rows, pass2_rows]
    mock_conn.get_fields.side_effect = _fake_get_fields

    mock_settings = MagicMock()
    mock_settings.docStoreConn = mock_conn

    with (
        patch("rag.svr.task_executor_refactor.dataset_structure_merger.settings", mock_settings),
        patch("rag.svr.task_executor_refactor.dataset_structure_merger._index_delete", new_callable=AsyncMock) as mock_index_delete,
    ):
        ok = await _cleanup_deleted_docs("tenant1", "kb1", "compile1", None)

    assert ok is True
    mock_index_delete.assert_awaited_once_with("tenant1", "kb1", {"id": ["entity2", "entity3"]})

    update_calls = [c for c in mock_conn.update.call_args_list]
    assert len(update_calls) == 1
    args = update_calls[0].args
    assert args[0] == {"id": ["entity1"]}
    assert args[1] == {"doc_ids_kwd": ["docC"]}


@pytest.mark.p1
@pytest.mark.asyncio
async def test_cleanup_deleted_docs_tolerates_unhashable_doc_ids_kwd_entries():
    """A doc_ids_kwd entry that's a list/dict (malformed provenance) must not
    raise TypeError just because the membership filter is set-backed."""
    pass1_rows = [{"id": "meta1", "deleted_doc_id": "docA"}]
    pass2_rows = [
        {"id": "entity1", "doc_ids_kwd": ["docA", ["bad", "entry"], "docC"]},
    ]

    mock_conn = MagicMock()
    mock_conn.index_exist.return_value = True
    mock_conn.search.side_effect = [pass1_rows, pass2_rows]
    mock_conn.get_fields.side_effect = _fake_get_fields

    mock_settings = MagicMock()
    mock_settings.docStoreConn = mock_conn

    with (
        patch("rag.svr.task_executor_refactor.dataset_structure_merger.settings", mock_settings),
        patch("rag.svr.task_executor_refactor.dataset_structure_merger._index_delete", new_callable=AsyncMock),
    ):
        ok = await _cleanup_deleted_docs("tenant1", "kb1", "compile1", None)

    assert ok is True
    update_calls = mock_conn.update.call_args_list
    assert len(update_calls) == 1
    args = update_calls[0].args
    assert args[0] == {"id": ["entity1"]}
    assert args[1] == {"doc_ids_kwd": [["bad", "entry"], "docC"]}
