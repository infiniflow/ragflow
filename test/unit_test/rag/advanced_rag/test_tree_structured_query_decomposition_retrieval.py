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
"""Regression tests for TreeStructuredQueryDecompositionRetrieval._async_update_chunk_info.

The historical implementation rebuilt ``cids``/``dids`` as a plain list on
every merge round and checked membership with ``in`` — O(n) per item, O(n^2)
per round. The fix tracks a set alongside the ordered list. These tests pin
the observable behavior (dedup + relative order across rounds) so the
quadratic pattern cannot silently come back.
"""

import pytest

from rag.advanced_rag.tree_structured_query_decomposition_retrieval import (
    TreeStructuredQueryDecompositionRetrieval,
)


def _retrieval() -> TreeStructuredQueryDecompositionRetrieval:
    return TreeStructuredQueryDecompositionRetrieval(chat_mdl=None, prompt_config={})


def _chunk(cid: str) -> dict:
    return {"chunk_id": cid, "content_with_weight": f"content-{cid}"}


def _doc(did: str) -> dict:
    return {"doc_id": did, "doc_name": f"name-{did}"}


@pytest.mark.p1
@pytest.mark.asyncio
async def test_first_round_assigns_results_directly():
    retrieval = _retrieval()
    chunk_info = {"total": 0, "chunks": [], "doc_aggs": []}
    kbinfos = {"total": 2, "chunks": [_chunk("c1"), _chunk("c2")], "doc_aggs": [_doc("d1")]}

    await retrieval._async_update_chunk_info(chunk_info, kbinfos)

    assert chunk_info["chunks"] == kbinfos["chunks"]
    assert chunk_info["doc_aggs"] == kbinfos["doc_aggs"]
    assert chunk_info["total"] == 2


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_round_dedups_against_existing_ids_and_preserves_order():
    retrieval = _retrieval()
    chunk_info = {
        "total": 2,
        "chunks": [_chunk("c1"), _chunk("c2")],
        "doc_aggs": [_doc("d1")],
    }
    # c2 already exists (existing-ID overlap), c2 also repeats within this
    # same incoming batch (duplicate-input overlap) — only c3/c4 and d2 are new.
    kbinfos = {
        "total": 3,
        "chunks": [_chunk("c2"), _chunk("c3"), _chunk("c2"), _chunk("c4")],
        "doc_aggs": [_doc("d1"), _doc("d2")],
    }

    await retrieval._async_update_chunk_info(chunk_info, kbinfos)

    assert [c["chunk_id"] for c in chunk_info["chunks"]] == ["c1", "c2", "c3", "c4"]
    assert [d["doc_id"] for d in chunk_info["doc_aggs"]] == ["d1", "d2"]
    assert chunk_info["total"] == 5


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_round_tolerates_unhashable_chunk_and_doc_ids():
    """Malformed provenance (e.g. a chunk_id/doc_id that's a list/dict instead
    of a string) must not raise TypeError just because dedup is set-backed."""
    retrieval = _retrieval()
    chunk_info = {
        "total": 1,
        "chunks": [{"chunk_id": ["nested", "id"], "content_with_weight": "c1"}],
        "doc_aggs": [{"doc_id": {"bad": "id"}, "doc_name": "d1"}],
    }
    kbinfos = {
        "total": 1,
        "chunks": [{"chunk_id": ["nested", "id"], "content_with_weight": "dup"}, _chunk("c2")],
        "doc_aggs": [{"doc_id": {"bad": "id"}, "doc_name": "dup"}, _doc("d2")],
    }

    await retrieval._async_update_chunk_info(chunk_info, kbinfos)

    # the malformed existing entries are deduped (canonical-key based), new valid ones are appended
    assert len(chunk_info["chunks"]) == 2
    assert chunk_info["chunks"][1]["chunk_id"] == "c2"
    assert len(chunk_info["doc_aggs"]) == 2
    assert chunk_info["doc_aggs"][1]["doc_id"] == "d2"


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_round_dedups_dicts_regardless_of_key_order():
    """dict equality ignores key order, so a dict doc_id/chunk_id with the same
    keys/values in a different order must still be treated as a duplicate."""
    retrieval = _retrieval()
    chunk_info = {
        "total": 1,
        "chunks": [{"chunk_id": {"a": 1, "b": 2}, "content_with_weight": "c1"}],
        "doc_aggs": [],
    }
    kbinfos = {
        "total": 1,
        # same dict, keys reordered -> must dedup, not be treated as new
        "chunks": [{"chunk_id": {"b": 2, "a": 1}, "content_with_weight": "dup"}, _chunk("c2")],
        "doc_aggs": [],
    }

    await retrieval._async_update_chunk_info(chunk_info, kbinfos)

    assert len(chunk_info["chunks"]) == 2
    assert chunk_info["chunks"][1]["chunk_id"] == "c2"


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_round_does_not_collide_string_with_malformed_repr():
    """A real string chunk_id that happens to equal repr(<malformed value>)
    must not be treated as a duplicate of that malformed value."""
    retrieval = _retrieval()
    malformed = ["nested", "id"]
    lookalike = repr(malformed)  # e.g. "['nested', 'id']"
    chunk_info = {
        "total": 1,
        "chunks": [{"chunk_id": malformed, "content_with_weight": "c1"}],
        "doc_aggs": [],
    }
    kbinfos = {
        "total": 1,
        "chunks": [{"chunk_id": lookalike, "content_with_weight": "c2"}],
        "doc_aggs": [],
    }

    await retrieval._async_update_chunk_info(chunk_info, kbinfos)

    # both must be kept -> they are not the same chunk despite the repr() collision
    assert len(chunk_info["chunks"]) == 2
    assert chunk_info["chunks"][1]["chunk_id"] == lookalike


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_round_treats_nested_list_and_tuple_as_distinct():
    """A list and a tuple with equal elements are never equal in Python, so a
    chunk_id nesting one of each must not be deduped against the other."""
    retrieval = _retrieval()
    chunk_info = {
        "total": 1,
        "chunks": [{"chunk_id": {"ids": ["a", "b"]}, "content_with_weight": "c1"}],
        "doc_aggs": [],
    }
    kbinfos = {
        "total": 1,
        "chunks": [{"chunk_id": {"ids": ("a", "b")}, "content_with_weight": "c2"}],
        "doc_aggs": [],
    }

    await retrieval._async_update_chunk_info(chunk_info, kbinfos)

    assert [chunk["chunk_id"]["ids"] for chunk in chunk_info["chunks"]] == [
        ["a", "b"],
        ("a", "b"),
    ]


@pytest.mark.p1
@pytest.mark.asyncio
async def test_multiple_merge_rounds_keep_accumulating_without_duplicates():
    retrieval = _retrieval()
    chunk_info = {"total": 0, "chunks": [], "doc_aggs": []}

    await retrieval._async_update_chunk_info(chunk_info, {"total": 1, "chunks": [_chunk("c1")], "doc_aggs": [_doc("d1")]})
    await retrieval._async_update_chunk_info(chunk_info, {"total": 1, "chunks": [_chunk("c1"), _chunk("c2")], "doc_aggs": [_doc("d1"), _doc("d2")]})
    await retrieval._async_update_chunk_info(chunk_info, {"total": 1, "chunks": [_chunk("c2"), _chunk("c3")], "doc_aggs": [_doc("d2"), _doc("d3")]})

    assert [c["chunk_id"] for c in chunk_info["chunks"]] == ["c1", "c2", "c3"]
    assert [d["doc_id"] for d in chunk_info["doc_aggs"]] == ["d1", "d2", "d3"]
    assert chunk_info["total"] == 3
