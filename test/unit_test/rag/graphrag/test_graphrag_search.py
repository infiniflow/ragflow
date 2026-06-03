import asyncio
import inspect
import pytest
from unittest.mock import MagicMock
from copy import deepcopy
from rag.graphrag.search import KGSearch


def _make_kg_search():
    ks = KGSearch.__new__(KGSearch)
    ks.dataStore = MagicMock()
    ks.dataStore.search.return_value = MagicMock(hits=MagicMock(hits=[]))
    return ks


def test_get_relevant_ents_by_keywords_is_coroutine():
    assert inspect.iscoroutinefunction(KGSearch.get_relevant_ents_by_keywords)


def test_get_relevant_relations_by_txt_is_coroutine():
    assert inspect.iscoroutinefunction(KGSearch.get_relevant_relations_by_txt)


@pytest.mark.asyncio
async def test_get_vector_awaited_in_ents():
    ks = _make_kg_search()
    awaited = []
    async def fake_get_vector(txt, emb, dim, sim_thr):
        awaited.append(txt)
        return MagicMock()
    ks.get_vector = fake_get_vector
    await ks.get_relevant_ents_by_keywords(
        keywords=["e1"], filters={}, idxnms=["idx"], kb_ids=["kb1"], emb_mdl=MagicMock()
    )
    assert len(awaited) == 1
    dense_exprs = ks.dataStore.search.call_args[0][3]
    for expr in dense_exprs:
        assert not inspect.iscoroutine(expr)


@pytest.mark.asyncio
async def test_get_vector_awaited_in_relations():
    ks = _make_kg_search()
    awaited = []
    async def fake_get_vector(txt, emb, dim, sim_thr):
        awaited.append(txt)
        return MagicMock()
    ks.get_vector = fake_get_vector
    await ks.get_relevant_relations_by_txt(
        txt="query", filters={}, idxnms=["idx"], kb_ids=["kb1"], emb_mdl=MagicMock()
    )
    assert len(awaited) == 1
    dense_exprs = ks.dataStore.search.call_args[0][3]
    for expr in dense_exprs:
        assert not inspect.iscoroutine(expr)


@pytest.mark.asyncio
async def test_ents_empty_keywords_returns_empty():
    ks = _make_kg_search()
    result = await ks.get_relevant_ents_by_keywords(
        keywords=[], filters={}, idxnms=[], kb_ids=[], emb_mdl=MagicMock()
    )
    assert result == {}
    ks.dataStore.search.assert_not_called()


@pytest.mark.asyncio
async def test_relations_empty_txt_returns_empty():
    ks = _make_kg_search()
    result = await ks.get_relevant_relations_by_txt(
        txt="", filters={}, idxnms=[], kb_ids=[], emb_mdl=MagicMock()
    )
    assert result == {}
    ks.dataStore.search.assert_not_called()


@pytest.mark.asyncio
async def test_filters_not_mutated():
    ks = _make_kg_search()
    async def fake_get_vector(*a, **kw):
        return MagicMock()
    ks.get_vector = fake_get_vector
    original = {"some_key": "some_value"}
    original_copy = deepcopy(original)
    await ks.get_relevant_ents_by_keywords(
        keywords=["e1"], filters=original, idxnms=["idx"], kb_ids=["kb1"], emb_mdl=MagicMock()
    )
    assert original == original_copy
