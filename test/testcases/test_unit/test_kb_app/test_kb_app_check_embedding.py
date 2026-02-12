import types

import numpy as np
import pytest

pytestmark = pytest.mark.p2


@pytest.mark.asyncio
async def test_check_embedding_sample_processing(kb_app, monkeypatch):
    mod, state = kb_app

    state.json = {"kb_id": "kb-1", "embd_id": "embd", "check_num": 5}
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]

    state.doc_store_search_results = [
        {"total": 5},
        {"ids": ["c1"]},
        {"ids": []},
        {"ids": ["c2"]},
        {"ids": ["c3"]},
        {"ids": ["c4"]},
    ]
    state.doc_store_total = 5
    state.doc_store_docs = {
        "c1": {"doc_id": "d1", "docnm_kwd": "Doc1", "content_with_weight": "hello", "question_kwd": [], "content_vec": "1\t0"},
        "c2": {"doc_id": "d2", "docnm_kwd": "Doc2", "content_with_weight": "__toggle__", "question_kwd": [], "list_vec": [1.0, 0.0]},
        "c3": {"doc_id": "d3", "docnm_kwd": "Doc3", "content_with_weight": "text", "question_kwd": [], "not_vec": 1},
        "c4": {"doc_id": "d4", "docnm_kwd": "Doc4", "content_with_weight": "text", "question_kwd": [], "bad_vec": {"a": 1}},
    }

    monkeypatch.setattr(mod.random, "sample", lambda population, n: list(range(n)))

    class _Toggle:
        def __init__(self):
            self._first = True

        def __bool__(self):
            if self._first:
                self._first = False
                return True
            return False

    import re as _re

    class _ReWrapper:
        @staticmethod
        def sub(pattern, repl, string, *args, **kwargs):
            if string == "__toggle__":
                return _Toggle()
            return _re.sub(pattern, repl, string, *args, **kwargs)

    monkeypatch.setattr(mod, "re", _ReWrapper())

    def _encode(_inputs):
        return [np.array([1.0, 0.0]), np.array([0.9, 0.1])], None

    monkeypatch.setattr(mod.LLMBundle, "encode", lambda *_args, **_kwargs: _encode(None))

    res = await mod.check_embedding()
    assert res["code"] == 0
    summary = res["data"]["summary"]
    assert summary["avg_cos_sim"] > 0.9
    assert summary["match_mode"] in {"content_only", "title+content"}
    reasons = {item.get("reason") for item in res["data"]["results"] if "reason" in item}
    assert "no_text" in reasons
    assert "no_stored_vector" in reasons


@pytest.mark.asyncio
async def test_check_embedding_similarity_below_threshold(kb_app, monkeypatch):
    mod, state = kb_app

    state.json = {"kb_id": "kb-1", "embd_id": "embd", "check_num": 1}
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    state.doc_store_search_results = [
        {"total": 1},
        {"ids": ["c1"]},
    ]
    state.doc_store_total = 1
    state.doc_store_docs = {
        "c1": {"doc_id": "d1", "docnm_kwd": "Doc1", "content_with_weight": "hello", "question_kwd": [], "content_vec": [0.0, 0.0]},
    }
    monkeypatch.setattr(mod.random, "sample", lambda population, n: list(range(n)))

    def _encode(_inputs):
        return [np.array([0.0, 1.0]), np.array([0.0, 1.0])], None

    monkeypatch.setattr(mod.LLMBundle, "encode", lambda *_args, **_kwargs: _encode(None))

    res = await mod.check_embedding()
    assert res["code"] == mod.RetCode.NOT_EFFECTIVE
    assert "below 0.9" in res["message"]


@pytest.mark.asyncio
async def test_check_embedding_encode_exception(kb_app, monkeypatch):
    mod, state = kb_app

    state.json = {"kb_id": "kb-1", "embd_id": "embd", "check_num": 1}
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    state.doc_store_search_results = [
        {"total": 1},
        {"ids": ["c1"]},
    ]
    state.doc_store_total = 1
    state.doc_store_docs = {
        "c1": {"doc_id": "d1", "docnm_kwd": "Doc1", "content_with_weight": "hello", "question_kwd": [], "content_vec": "1\t0"},
    }
    monkeypatch.setattr(mod.random, "sample", lambda population, n: list(range(n)))

    def _raise(_inputs):
        raise Exception("encode-fail")

    monkeypatch.setattr(mod.LLMBundle, "encode", lambda *_args, **_kwargs: _raise(None))

    res = await mod.check_embedding()
    assert res["code"] == 102
    assert "Embedding failure" in res["message"]


@pytest.mark.asyncio
async def test_check_embedding_no_chunks_raises(kb_app, monkeypatch):
    mod, state = kb_app

    state.json = {"kb_id": "kb-1", "embd_id": "embd", "check_num": 1}
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    state.doc_store_search_results = [{"total": 0}]
    state.doc_store_total = 0
    monkeypatch.setattr(mod.random, "sample", lambda population, n: list(range(n)))

    with pytest.raises(UnboundLocalError):
        await mod.check_embedding()
