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
import types

import pytest

# Stub the heavy / circular-importing dependencies before importing search,
# mirroring test_search_pagination.py so the module imports in isolation.
_fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


_fake_query.FulltextQueryer = _DummyFulltextQueryer

_STUBS = {
    "rag.nlp.query": _fake_query,
    "rag.nlp.rag_tokenizer": types.ModuleType("rag.nlp.rag_tokenizer"),
    "common.settings": types.ModuleType("common.settings"),
}

# Install the stubs only for the import below, then take them back out. Leaving
# them in sys.modules is process-global and breaks any later test that imports
# the real module, for example test/unit_test/common/test_settings_queue.py.
_previous = {name: sys.modules.get(name) for name in _STUBS}
for _name, _module in _STUBS.items():
    sys.modules.setdefault(_name, _module)

try:
    from rag.nlp.search import Dealer  # noqa: E402
finally:
    for _name, _prior in _previous.items():
        if _prior is not None:
            sys.modules[_name] = _prior
        elif sys.modules.get(_name) is _STUBS[_name]:
            del sys.modules[_name]


@pytest.fixture(autouse=True)
def _reset_doc_exists_cache():
    """Dealer._doc_exists_cache is class-level (so invalidations reach every
    retrieval path in the process); reset it between tests so each one starts
    from a clean slate."""
    Dealer._doc_exists_cache.clear()
    yield
    Dealer._doc_exists_cache.clear()


def _dealer():
    return Dealer.__new__(Dealer)


def _stub_document_service(monkeypatch, existing_ids_or_callable):
    """Stand in for DocumentService.get_by_ids(...).dicts(), which the method imports lazily.

    Pass either a static set of ids or a callable that returns the current
    set on each call — invalidation tests need the second form so they can
    mutate "MySQL state" between cache fills.
    """
    if callable(existing_ids_or_callable):

        def _resolve():
            return existing_ids_or_callable()

    else:

        def _resolve():
            return existing_ids_or_callable

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
            return _Rows([d for d in doc_ids if d in _resolve()])

    module = types.ModuleType("api.db.services.document_service")
    module.DocumentService = _DocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", module)
    return calls


@pytest.mark.asyncio
async def test_deleted_doc_stays_absent_on_the_cached_second_call(monkeypatch):
    """A doc proven deleted must not come back as existing while the cache is warm."""
    dealer = _dealer()
    calls = _stub_document_service(monkeypatch, set())

    first = await dealer._existing_doc_ids(["deleted-doc"])
    second = await dealer._existing_doc_ids(["deleted-doc"])

    assert first == set()
    assert second == set()
    # The point of caching the negative result: no second database round-trip.
    assert len(calls) == 1


@pytest.mark.asyncio
async def test_existing_doc_is_served_from_cache_without_a_second_query(monkeypatch):
    dealer = _dealer()
    calls = _stub_document_service(monkeypatch, {"live-doc"})

    first = await dealer._existing_doc_ids(["live-doc"])
    second = await dealer._existing_doc_ids(["live-doc"])

    assert first == {"live-doc"}
    assert second == {"live-doc"}
    assert len(calls) == 1


@pytest.mark.asyncio
async def test_mixed_batch_keeps_only_the_live_doc_when_cached(monkeypatch):
    dealer = _dealer()
    calls = _stub_document_service(monkeypatch, {"live-doc"})

    await dealer._existing_doc_ids(["live-doc", "deleted-doc"])
    second = await dealer._existing_doc_ids(["live-doc", "deleted-doc"])

    assert second == {"live-doc"}
    # A regression that re-queries would still return {"live-doc"}, so pin the
    # round-trip count too.
    assert len(calls) == 1


@pytest.mark.asyncio
async def test_invalidate_doc_exists_drops_a_positive_cached_entry(monkeypatch):
    """Regression for #19071: a doc cached as 'exists' must be evictable
    synchronously so a delete does not leak through _prune_deleted_chunks
    for the remaining TTL window."""
    dealer = _dealer()
    # Use a mutable fixture so the delete can flip the doc to "missing"
    # between the two cache lookups.
    existing = {"live-doc"}
    calls = _stub_document_service(monkeypatch, lambda: existing)

    first = await dealer._existing_doc_ids(["live-doc"])
    assert first == {"live-doc"}
    assert len(calls) == 1

    # DocumentService.remove_document calls invalidate_doc_exists right
    # after the MySQL row delete. The doc is now also gone from MySQL.
    existing.discard("live-doc")
    Dealer.invalidate_doc_exists("live-doc")

    second = await dealer._existing_doc_ids(["live-doc"])
    assert second == set()
    assert len(calls) == 2
    assert calls[-1] == ["live-doc"]


@pytest.mark.asyncio
async def test_invalidate_doc_exists_drops_a_negative_cached_entry_too(monkeypatch):
    """A delete-then-recache race must not preserve a stale negative result.
    Invalidate, then upsert the doc — the next retrieval should hit MySQL
    and report it as existing again."""
    dealer = _dealer()
    existing = set()
    calls = _stub_document_service(monkeypatch, lambda: existing)

    # Warm the cache with a negative result.
    await dealer._existing_doc_ids(["live-doc"])
    assert Dealer._doc_exists_cache["live-doc"][1] is False
    assert len(calls) == 1

    # The delete path invalidates AND a separate write path re-creates the doc.
    existing.add("live-doc")
    Dealer.invalidate_doc_exists("live-doc")

    second = await dealer._existing_doc_ids(["live-doc"])
    assert second == {"live-doc"}
    assert len(calls) == 2


def test_invalidate_doc_exists_handles_an_empty_call_silently():
    Dealer.invalidate_doc_exists()  # must not raise
    assert Dealer._doc_exists_cache == {}
