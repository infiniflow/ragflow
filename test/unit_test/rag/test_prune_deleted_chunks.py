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
import importlib.util
import sys
import types
from pathlib import Path

_ROOT = Path(__file__).parents[3]


def _load_search():
    fake_rag_nlp = types.ModuleType("rag.nlp")
    fake_rag_nlp.__path__ = []

    fake_tokenizer = types.ModuleType("rag.nlp.rag_tokenizer")
    fake_rag_nlp.rag_tokenizer = fake_tokenizer

    fake_query = types.ModuleType("rag.nlp.query")

    class _DummyFulltextQueryer:
        pass

    fake_query.FulltextQueryer = _DummyFulltextQueryer
    fake_rag_nlp.query = fake_query
    fake_settings = types.ModuleType("common.settings")

    stub_names = ("rag.nlp", "rag.nlp.rag_tokenizer", "rag.nlp.query", "common.settings")
    saved = {name: sys.modules.get(name) for name in stub_names}
    sys.modules["rag.nlp"] = fake_rag_nlp
    sys.modules["rag.nlp.rag_tokenizer"] = fake_tokenizer
    sys.modules["rag.nlp.query"] = fake_query
    sys.modules["common.settings"] = fake_settings

    import common

    saved_common_settings = getattr(common, "settings", None)
    common.settings = fake_settings

    spec = importlib.util.spec_from_file_location("rag.nlp.search_prune_test", _ROOT / "rag" / "nlp" / "search.py")
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules["rag.nlp.search_prune_test"] = module
    try:
        spec.loader.exec_module(module)
    finally:
        if saved_common_settings is None:
            if getattr(common, "settings", None) is fake_settings:
                delattr(common, "settings")
        else:
            common.settings = saved_common_settings
        for name, previous in saved.items():
            if previous is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = previous
    return module


_search = _load_search()
Dealer = _search.Dealer
is_kb_scoped_chunk = _search.is_kb_scoped_chunk


def test_is_kb_scoped_chunk_missing_or_empty_doc_id():
    assert is_kb_scoped_chunk({"kb_id": "kb1"}) is False
    assert is_kb_scoped_chunk({"kb_id": "kb1", "doc_id": ""}) is False


def test_is_kb_scoped_chunk_compilation_row_without_doc_id():
    assert is_kb_scoped_chunk({"kb_id": "kb1", "compile_kwd": "wiki_page"}) is True
    assert is_kb_scoped_chunk({"kb_id": "kb1", "doc_id": "", "compile_kwd": ["wiki_canonical_entity"]}) is True


def test_is_kb_scoped_chunk_sentinel_matches_kb():
    assert is_kb_scoped_chunk({"kb_id": "kb1", "doc_id": "kb1"}) is True
    assert is_kb_scoped_chunk({"kb_id": ["kb1"], "doc_id": ["kb1"]}) is True
    assert is_kb_scoped_chunk({"kb_id": "kb1", "doc_id": "kb1", "compile_kwd": "wiki_page"}) is True


def test_is_kb_scoped_chunk_real_document():
    assert is_kb_scoped_chunk({"kb_id": "kb1", "doc_id": "doc1"}) is False
    assert is_kb_scoped_chunk({"kb_id": "kb1", "doc_id": "doc1", "compile_kwd": "wiki_doc_page_source"}) is False
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


def test_prune_drops_ordinary_chunks_that_omit_doc_id():
    dealer = Dealer(dataStore=None)
    called = []

    async def existing(doc_ids):
        called.append(list(doc_ids))
        return {d for d in doc_ids if d == "doc-live"}

    dealer._existing_doc_ids = existing
    sres = _result(
        ["src-live", "src-orphan", "wiki-page"],
        {
            "src-live": {"doc_id": "doc-live", "kb_id": "kb1"},
            "src-orphan": {"kb_id": "kb1"},
            "wiki-page": {"kb_id": "kb1", "compile_kwd": "wiki_page"},
        },
    )
    out = asyncio.run(dealer._prune_deleted_chunks(sres))
    assert out.ids == ["src-live", "wiki-page"]
    assert "src-orphan" not in out.field
    assert called == [["doc-live"]]


def test_prune_drops_orphan_chunks_when_every_hit_omits_doc_id():
    dealer = Dealer(dataStore=None)
    called = []

    async def existing(doc_ids):
        called.append(list(doc_ids))
        return set()

    dealer._existing_doc_ids = existing
    sres = _result(
        ["src-orphan", "wiki-page"],
        {
            "src-orphan": {"kb_id": "kb1", "doc_id": ""},
            "wiki-page": {"kb_id": "kb1", "compile_kwd": "wiki_page"},
        },
    )
    out = asyncio.run(dealer._prune_deleted_chunks(sres))
    assert out.ids == ["wiki-page"]
    assert "src-orphan" not in out.field
    assert called == []


def test_prune_drops_all_ordinary_chunks_without_doc_id():
    dealer = Dealer(dataStore=None)
    called = []

    async def existing(doc_ids):
        called.append(list(doc_ids))
        return set()

    dealer._existing_doc_ids = existing
    sres = _result(["src-orphan"], {"src-orphan": {"kb_id": "kb1"}})
    out = asyncio.run(dealer._prune_deleted_chunks(sres))
    assert out.ids == []
    assert out.field == {}
    assert called == []


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


def test_prune_drops_ids_missing_from_fields_when_all_hits_are_kb_scoped():
    dealer = Dealer(dataStore=None)
    called = []

    async def existing(doc_ids):
        called.append(list(doc_ids))
        return set()

    dealer._existing_doc_ids = existing
    sres = _result(["wiki", "ghost"], {"wiki": {"doc_id": "kb1", "kb_id": "kb1"}})
    out = asyncio.run(dealer._prune_deleted_chunks(sres))
    assert out.ids == ["wiki"]
    assert "ghost" not in out.field
    assert called == []
