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
"""KB-scoped wiki/compilation rows must survive deleted-document pruning."""

import asyncio
import sys
import types

# Stub the heavy / circular-importing dependencies before importing search,
# mirroring test_search_pagination.py so the module imports in isolation.
_fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


_fake_query.FulltextQueryer = _DummyFulltextQueryer
_fake_tokenizer = types.ModuleType("rag.nlp.rag_tokenizer")
sys.modules.setdefault("rag.nlp.query", _fake_query)
sys.modules.setdefault("rag.nlp.rag_tokenizer", _fake_tokenizer)
sys.modules.setdefault("common.settings", types.ModuleType("common.settings"))

from rag.nlp.search import Dealer, is_kb_scoped_chunk  # noqa: E402


def test_is_kb_scoped_chunk_missing_or_empty_doc_id():
    assert is_kb_scoped_chunk({"kb_id": "kb1"}) is True
    assert is_kb_scoped_chunk({"kb_id": "kb1", "doc_id": ""}) is True


def test_is_kb_scoped_chunk_sentinel_matches_kb():
    assert is_kb_scoped_chunk({"kb_id": "kb1", "doc_id": "kb1"}) is True
    assert is_kb_scoped_chunk({"kb_id": ["kb1"], "doc_id": ["kb1"]}) is True


def test_is_kb_scoped_chunk_real_document():
    assert is_kb_scoped_chunk({"kb_id": "kb1", "doc_id": "doc1"}) is False
    assert is_kb_scoped_chunk(None) is False


def _result(ids, field):
    return Dealer.SearchResult(total=len(ids), ids=list(ids), field=field)


def test_prune_keeps_wiki_pages_when_mixed_with_source_chunks():
    dealer = Dealer(dataStore=None)

    async def existing(doc_ids):
        return {d for d in doc_ids if d == "doc-live"}

    dealer._existing_doc_ids = existing
    sres = _result(
        ["src-live", "src-dead", "wiki-page"],
        {
            "src-live": {"doc_id": "doc-live", "kb_id": "kb1"},
            "src-dead": {"doc_id": "doc-gone", "kb_id": "kb1"},
            "wiki-page": {"doc_id": "kb1", "kb_id": "kb1", "compile_kwd": "wiki_page"},
        },
    )
    out = asyncio.run(dealer._prune_deleted_chunks(sres))
    assert out.ids == ["src-live", "wiki-page"]
    assert "src-dead" not in out.field
    assert "wiki-page" in out.field


def test_prune_keeps_wiki_pages_that_omit_doc_id():
    dealer = Dealer(dataStore=None)

    async def existing(doc_ids):
        return {d for d in doc_ids if d == "doc-live"}

    dealer._existing_doc_ids = existing
    sres = _result(
        ["src-live", "wiki-page"],
        {
            "src-live": {"doc_id": "doc-live", "kb_id": "kb1"},
            "wiki-page": {"kb_id": "kb1", "compile_kwd": "wiki_page"},
        },
    )
    out = asyncio.run(dealer._prune_deleted_chunks(sres))
    assert out.ids == ["src-live", "wiki-page"]


def test_prune_skips_lookup_when_all_hits_are_kb_scoped():
    dealer = Dealer(dataStore=None)
    called = []

    async def existing(doc_ids):
        called.append(list(doc_ids))
        return set()

    dealer._existing_doc_ids = existing
    sres = _result(["wiki"], {"wiki": {"doc_id": "kb1", "kb_id": "kb1"}})
    out = asyncio.run(dealer._prune_deleted_chunks(sres))
    assert out.ids == ["wiki"]
    assert called == []
