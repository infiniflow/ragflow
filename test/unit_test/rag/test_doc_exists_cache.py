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
#
"""Unit tests for the doc-existence cache behind Dealer._prune_deleted_chunks."""

import sys
import threading
import types
from collections import OrderedDict

import pytest

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

from rag.nlp.search import Dealer  # noqa: E402


def _dealer():
    dealer = Dealer.__new__(Dealer)
    dealer._doc_exists_cache = OrderedDict()
    dealer._doc_exists_lock = threading.Lock()
    return dealer


def _stub_document_service(monkeypatch, existing_ids):
    """Stand in for DocumentService.get_by_ids(...).dicts(), which the method imports lazily."""
    calls = []

    class _Rows:
        def __init__(self, ids):
            self._ids = ids

        def dicts(self):
            return [{"id": doc_id} for doc_id in self._ids]

    class _DocumentService:
        @staticmethod
        def get_by_ids(doc_ids):
            calls.append(list(doc_ids))
            return _Rows([d for d in doc_ids if d in existing_ids])

    module = types.ModuleType("api.db.services.document_service")
    module.DocumentService = _DocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", module)
    return calls


@pytest.mark.asyncio
async def test_deleted_doc_stays_absent_on_the_cached_second_call(monkeypatch):
    """A doc proven deleted must not come back as existing while the cache is warm."""
    dealer = _dealer()
    calls = _stub_document_service(monkeypatch, existing_ids=set())

    first = await dealer._existing_doc_ids(["deleted-doc"])
    second = await dealer._existing_doc_ids(["deleted-doc"])

    assert first == set()
    assert second == set()
    # The point of caching the negative result: no second database round-trip.
    assert len(calls) == 1


@pytest.mark.asyncio
async def test_existing_doc_is_served_from_cache_without_a_second_query(monkeypatch):
    dealer = _dealer()
    calls = _stub_document_service(monkeypatch, existing_ids={"live-doc"})

    first = await dealer._existing_doc_ids(["live-doc"])
    second = await dealer._existing_doc_ids(["live-doc"])

    assert first == {"live-doc"}
    assert second == {"live-doc"}
    assert len(calls) == 1


@pytest.mark.asyncio
async def test_mixed_batch_keeps_only_the_live_doc_when_cached(monkeypatch):
    dealer = _dealer()
    _stub_document_service(monkeypatch, existing_ids={"live-doc"})

    await dealer._existing_doc_ids(["live-doc", "deleted-doc"])
    second = await dealer._existing_doc_ids(["live-doc", "deleted-doc"])

    assert second == {"live-doc"}
