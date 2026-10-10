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

"""Parent (mother) chunk IDs must be scoped to their dataset and document.

All datasets of a tenant share one index, so an ID derived from the parent
text alone lets identical text in another document overwrite the parent row.
"""

import pytest

from rag.svr.task_executor_refactor.chunk_service import ChunkService, make_mother_chunk_id


def test_mother_chunk_id_is_deterministic():
    assert make_mother_chunk_id("kb-a", "doc-a", "same parent") == make_mother_chunk_id("kb-a", "doc-a", "same parent")


@pytest.mark.parametrize(
    "other",
    [
        ("kb-b", "doc-a", "same parent"),
        ("kb-a", "doc-b", "same parent"),
        ("kb-a", "doc-a", "other parent"),
    ],
)
def test_mother_chunk_id_differs_across_scope(other):
    assert make_mother_chunk_id("kb-a", "doc-a", "same parent") != make_mother_chunk_id(*other)


def test_mother_chunk_id_matches_go_parent_chunk_id():
    # Same value as Go parentChunkID("kb-a", "doc-a", "same parent") in
    # internal/ingestion/task/indexdoc, so both runtimes address one parent row.
    assert make_mother_chunk_id("kb-a", "doc-a", "same parent") == "13d8508623d3d8b2"


def test_create_mother_chunks_keeps_one_parent_per_document():
    chunks = [
        {"mom": "same parent", "doc_id": "doc-a", "kb_id": ["kb-a"]},
        {"mom": "same parent", "doc_id": "doc-a", "kb_id": ["kb-a"]},
        {"mom": "same parent", "doc_id": "doc-b", "kb_id": ["kb-a"]},
    ]

    mothers = ChunkService._create_mother_chunks(chunks, "kb-a")

    assert chunks[0]["mom_id"] == chunks[1]["mom_id"] == make_mother_chunk_id("kb-a", "doc-a", "same parent")
    assert chunks[2]["mom_id"] == make_mother_chunk_id("kb-a", "doc-b", "same parent")
    assert [(m["id"], m["doc_id"]) for m in mothers] == [(chunks[0]["mom_id"], "doc-a"), (chunks[2]["mom_id"], "doc-b")]
