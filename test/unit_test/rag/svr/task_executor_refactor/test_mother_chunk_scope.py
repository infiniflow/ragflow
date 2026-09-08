#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

from rag.nlp.search import Dealer
from rag.svr.task_executor_refactor.chunk_service import ChunkService, make_mother_chunk_id


def test_mother_chunk_ids_are_scoped_to_the_source_document():
    assert make_mother_chunk_id("same summary", "doc-a") != make_mother_chunk_id("same summary", "doc-b")
    assert make_mother_chunk_id("same summary", "doc-a") == make_mother_chunk_id("same summary", "doc-a")

    chunks = [
        {"mom": "same summary", "doc_id": "doc-a", "kb_id": "kb-a"},
        {"mom": "same summary", "doc_id": "doc-b", "kb_id": "kb-b"},
    ]
    mothers = ChunkService._create_mother_chunks(chunks)

    assert chunks[0]["mom_id"] != chunks[1]["mom_id"]
    assert {mother["doc_id"] for mother in mothers} == {"doc-a", "doc-b"}


class _DataStore:
    def __init__(self, parent):
        self.parent = parent

    def get(self, _parent_id, _index_name, _dataset_ids):
        return self.parent


def _dealer(parent):
    dealer = Dealer.__new__(Dealer)
    dealer.dataStore = _DataStore(parent)
    return dealer


def _child(doc_id="doc-a", kb_id="kb-a"):
    return {
        "mom_id": "legacy-colliding-id",
        "content_ltks": "child content",
        "content_with_weight": "child content",
        "doc_id": doc_id,
        "kb_id": kb_id,
        "similarity": 0.8,
    }


def test_retrieval_falls_back_when_parent_metadata_is_from_another_document():
    child = _child()
    parent = {
        "content_with_weight": "parent from another document",
        "doc_id": "doc-b",
        "kb_id": "kb-b",
    }

    result = _dealer(parent).retrieval_by_children([child], ["tenant"])

    assert result == [child]
    assert result[0]["content_with_weight"] == "child content"


def test_retrieval_uses_parent_when_parent_metadata_matches_children():
    child = _child()
    parent = {
        "content_with_weight": "parent content",
        "doc_id": "doc-a",
        "kb_id": "kb-a",
    }

    result = _dealer(parent).retrieval_by_children([child], ["tenant"])

    assert result[0]["chunk_id"] == "legacy-colliding-id"
    assert result[0]["content_with_weight"] == "parent content"
    assert result[0]["doc_id"] == "doc-a"
