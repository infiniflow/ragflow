"""Keep the real cross-server search gate sensitive to broken results."""

from copy import deepcopy
from types import SimpleNamespace

import pytest

from test.testcases.restful_api import test_search_datasets_consistency as gate


def _data():
    return {
        "total": 3,
        "chunks": [{"chunk_id": "c1", "kb_id": "kb1", "doc_id": "d1", "similarity": 0.8, "term_similarity": 0.7, "vector_similarity": 0.9}],
    }


def _response(data):
    return SimpleNamespace(status_code=200, text="synthetic response", json=lambda: {"code": 0, "data": data})


@pytest.mark.p0
@pytest.mark.parametrize("option", [{"keyword": True}, {"rerank_id": "reranker"}, {"cross_languages": ["English"]}])
def test_llm_search_rejects_empty_peer_result(monkeypatch, option):
    client = SimpleNamespace(token="synthetic", post=lambda *args, **kwargs: _response(_data()))
    monkeypatch.setattr(gate.requests, "post", lambda *args, **kwargs: _response({"chunks": [], "total": 0}))
    with pytest.raises(AssertionError, match="no chunks"):
        gate.search_and_compare(client, "kb1", {"question": "known answer", **option})


@pytest.mark.p0
@pytest.mark.parametrize(
    "mutation, message",
    [
        (lambda data: data["chunks"][0].update(kb_id="foreign"), "requested datasets"),
        (lambda data: data["chunks"][0].update(doc_id="foreign"), "requested documents"),
        (lambda data: data["chunks"][0].update(similarity=float("nan")), "invalid similarity"),
        (lambda data: data["chunks"].append(deepcopy(data["chunks"][0])), "duplicate chunk"),
        (lambda data: data.update(total=0), "invalid total"),
    ],
)
def test_search_rejects_invalid_results(mutation, message):
    data = _data()
    mutation(data)
    with pytest.raises(AssertionError, match=message):
        gate.assert_search_invariants(data, ["kb1"], {"doc_ids": ["d1"]}, require_results=True)


@pytest.mark.p0
def test_llm_search_accepts_score_drift_and_returns_actual_total(monkeypatch):
    client = SimpleNamespace(token="synthetic", post=lambda *args, **kwargs: _response(_data()))
    peer = _data()
    peer["chunks"][0]["similarity"] = 0.5
    monkeypatch.setattr(gate.requests, "post", lambda *args, **kwargs: _response(peer))
    assert gate.search_and_compare(client, "kb1", {"question": "known answer", "keyword": True}) == (3, 1)
