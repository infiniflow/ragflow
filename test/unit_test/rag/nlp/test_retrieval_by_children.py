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

"""``Dealer.retrieval_by_children`` must only replace child chunks with a
parent that belongs to the same dataset and document.

Parents indexed before IDs were scoped by dataset and document can be shared
by identical text in another dataset, and the document store looks them up by
ID alone.
"""

import pytest

# See test_search_rerank.py: `common.settings` must be initialised before
# `rag.nlp.search` to resolve an import cycle.
import common.settings  # noqa: F401

from rag.nlp.search import Dealer


class _DataStore:
    def __init__(self, parent):
        self.parent = parent

    def get(self, _chunk_id, _index_name, _dataset_ids):
        return self.parent


def _dealer(parent):
    dealer = Dealer.__new__(Dealer)
    dealer.dataStore = _DataStore(parent)
    return dealer


def _child(doc_id="doc-a", kb_id="kb-a"):
    return {
        "mom_id": "parent-id",
        "content_ltks": "child content",
        "content_with_weight": "child content",
        "doc_id": doc_id,
        "kb_id": kb_id,
        "similarity": 0.8,
    }


def _parent(doc_id="doc-a", kb_id="kb-a"):
    return {"content_with_weight": "parent content", "doc_id": doc_id, "kb_id": kb_id}


@pytest.mark.parametrize("kb_id", ["kb-a", ["kb-a"]])
def test_uses_parent_from_the_same_dataset_and_document(kb_id):
    result = _dealer(_parent(kb_id=kb_id)).retrieval_by_children([_child()], ["tenant"])

    assert len(result) == 1
    assert result[0]["chunk_id"] == "parent-id"
    assert result[0]["content_with_weight"] == "parent content"


@pytest.mark.parametrize(
    "parent",
    [
        _parent(kb_id="kb-b"),
        _parent(doc_id="doc-b"),
        _parent(doc_id="doc-b", kb_id="kb-b"),
    ],
    ids=["other-dataset", "other-document", "other-dataset-and-document"],
)
def test_falls_back_to_children_when_parent_is_out_of_scope(parent):
    child = _child()

    assert _dealer(parent).retrieval_by_children([child], ["tenant"]) == [child]


def test_falls_back_to_children_when_children_span_documents():
    children = [_child(), _child(doc_id="doc-b", kb_id="kb-b")]

    assert _dealer(_parent()).retrieval_by_children(list(children), ["tenant"]) == children
