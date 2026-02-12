import sys
from pathlib import Path
import types
import json

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from session_stub import load_session_module


@pytest.mark.asyncio
async def test_searchbots_ask_auth_and_streaming(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "invalid"}
    resp = await mod.ask_about_embedded()
    assert resp["code"] != 0
    assert "Authorization is not valid" in resp["message"]

    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"question": "hi", "kb_ids": ["kb1"], "search_id": "s1"}

    captured = {}

    async def _async_ask(_q, _kb_ids, _uid, search_config=None):
        captured["search_config"] = search_config
        yield {"answer": "ok"}

    mod.get_request_json = _get_request_json
    mod.SearchService.get_detail = classmethod(lambda cls, _id: {"search_config": {"k": "v"}})
    mod.async_ask = _async_ask

    resp = await mod.ask_about_embedded()
    assert "text/event-stream" in resp.headers.get("Content-Type", "")
    assert captured["search_config"] == {"k": "v"}


@pytest.mark.asyncio
async def test_searchbots_ask_stream_error_payload(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"question": "hi", "kb_ids": ["kb1"]}

    async def _async_ask(*_args, **_kwargs):
        raise RuntimeError("boom")
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.async_ask = _async_ask
    resp = await mod.ask_about_embedded()
    lines = []
    async for line in resp.response:
        lines.append(line)
    assert any("**ERROR**" in line for line in lines)


@pytest.mark.asyncio
async def test_searchbots_retrieval_auth_and_validation(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "invalid"}
    resp = await mod.retrieval_test_embedded()
    assert resp["code"] != 0
    assert "Authorization is not valid" in resp["message"]

    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id=None)])

    async def _get_request_json():
        return {"question": "q", "kb_id": []}

    mod.get_request_json = _get_request_json
    resp = await mod.retrieval_test_embedded()
    assert resp["code"] != 0

    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    resp = await mod.retrieval_test_embedded()
    assert resp["code"] == mod.RetCode.DATA_ERROR
    assert "Please specify dataset" in resp["message"]


@pytest.mark.asyncio
async def test_searchbots_retrieval_search_config_overrides_and_filters(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"question": "q", "kb_id": ["kb1"], "search_id": "s1"}

    captured = {}

    async def _apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl, local_doc_ids):
        captured["filter"] = meta_data_filter
        return ["doc1"]

    async def _retrieval(_q, _embd_mdl, _tenant_ids, _kb_ids, _page, _size, similarity_threshold, vector_similarity_weight, top, local_doc_ids, **_kwargs):
        captured["similarity_threshold"] = similarity_threshold
        captured["vector_similarity_weight"] = vector_similarity_weight
        captured["top"] = top
        return {"chunks": []}

    mod.get_request_json = _get_request_json
    mod.apply_meta_data_filter = _apply_meta_data_filter
    mod.SearchService.get_detail = classmethod(
        lambda cls, _id: {"search_config": {"meta_data_filter": {"method": "auto"}, "similarity_threshold": 0.9, "vector_similarity_weight": 0.2, "top_k": 5}}
    )
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="kb1")])
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, _kb_id: (True, types.SimpleNamespace(tenant_id="t", embd_id="embd")))
    mod.settings.retriever.retrieval = _retrieval

    resp = await mod.retrieval_test_embedded()
    assert resp["code"] == 0
    assert captured["filter"]["method"] == "auto"
    assert captured["similarity_threshold"] == 0.9
    assert captured["vector_similarity_weight"] == 0.2
    assert captured["top"] == 5


@pytest.mark.asyncio
async def test_searchbots_retrieval_tenant_permission_and_kb_missing(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"question": "q", "kb_id": ["kb1"]}

    mod.get_request_json = _get_request_json
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="other")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: [])

    resp = await mod.retrieval_test_embedded()
    assert resp["code"] == mod.RetCode.OPERATING_ERROR

    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="kb1")])
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, _kb_id: (False, None))
    resp = await mod.retrieval_test_embedded()
    assert resp["code"] != 0
    assert "Knowledgebase not found" in resp["message"]


@pytest.mark.asyncio
async def test_searchbots_retrieval_full_path_with_kg_and_cleanup(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {
            "question": "q",
            "kb_id": ["kb1"],
            "use_kg": True,
            "rerank_id": "rerank",
            "keyword": True,
            "cross_languages": ["en"],
        }

    async def _retrieval(_q, _embd_mdl, _tenant_ids, _kb_ids, _page, _size, _st, _vsw, _top, _local_doc_ids, **_kwargs):
        return {"chunks": [{"vector": [1], "content": "c"}]}

    async def _kg_retrieval(*_args, **_kwargs):
        return {"content_with_weight": "kg"}

    mod.get_request_json = _get_request_json
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="kb1")])
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, _kb_id: (True, types.SimpleNamespace(tenant_id="t", embd_id="embd")))
    mod.settings.retriever.retrieval = _retrieval
    mod.settings.kg_retriever.retrieval = _kg_retrieval

    async def _cross_languages(*_args, **_kwargs):
        return "translated"

    async def _keyword_extraction(*_args, **_kwargs):
        return " kw"

    mod.cross_languages = _cross_languages
    mod.keyword_extraction = _keyword_extraction
    mod.label_question = lambda *_args, **_kwargs: ["label"]

    resp = await mod.retrieval_test_embedded()
    assert resp["code"] == 0
    chunks = resp["data"]["chunks"]
    assert "vector" not in chunks[0]
    assert resp["data"]["labels"] == ["label"]


@pytest.mark.asyncio
async def test_searchbots_retrieval_exception_mapping(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"question": "q", "kb_id": ["kb1"]}

    mod.get_request_json = _get_request_json
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="kb1")])
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, _kb_id: (True, types.SimpleNamespace(tenant_id="t", embd_id="embd")))

    async def _retrieval(*_args, **_kwargs):
        raise Exception("err not_found")

    mod.settings.retriever.retrieval = _retrieval
    resp = await mod.retrieval_test_embedded()
    assert resp["code"] == mod.RetCode.DATA_ERROR

    async def _retrieval2(*_args, **_kwargs):
        raise Exception("boom")

    mod.settings.retriever.retrieval = _retrieval2
    resp = await mod.retrieval_test_embedded()
    assert resp["code"] == 500


@pytest.mark.asyncio
async def test_searchbots_related_questions_auth_and_prompt(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "invalid"}
    resp = await mod.related_questions_embedded()
    assert resp["code"] != 0

    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id=None)])
    async def _get_request_json():
        return {"question": "q", "search_id": "s1"}
    mod.get_request_json = _get_request_json
    resp = await mod.related_questions_embedded()
    assert resp["code"] != 0

    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    mod.SearchService.get_detail = classmethod(lambda cls, _id: {"search_config": {"chat_id": "c", "llm_setting": {"temperature": 0.5}}})
    captured = {}

    class _LLM:
        def __init__(self, *_args, **_kwargs):
            pass

        async def async_chat(self, prompt, _messages, _conf):
            captured["prompt"] = prompt
            return "1. alpha\n2. beta"

    mod.LLMBundle = _LLM
    resp = await mod.related_questions_embedded()
    assert resp["code"] == 0
    assert resp["data"] == ["alpha", "beta"]
    assert "related_question" in captured["prompt"]


@pytest.mark.asyncio
async def test_searchbots_detail_permission_and_not_found(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "invalid"}
    resp = await mod.detail_share_embedded()
    assert resp["code"] != 0

    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    mod._stub_request.args = {"search_id": "s1"}
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    mod.SearchService.query = classmethod(lambda cls, **_kwargs: [])
    resp = await mod.detail_share_embedded()
    assert resp["code"] == mod.RetCode.OPERATING_ERROR

    mod.SearchService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="s1")])
    mod.SearchService.get_detail = classmethod(lambda cls, _id: None)
    resp = await mod.detail_share_embedded()
    assert resp["code"] != 0
    assert "Can't find this Search App" in resp["message"]

    def _raise(_id):
        raise RuntimeError("boom")

    mod.SearchService.get_detail = classmethod(lambda cls, _id: _raise(_id))
    resp = await mod.detail_share_embedded()
    assert resp["code"] == 500


@pytest.mark.asyncio
async def test_searchbots_mindmap_auth_and_error_mapping(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "invalid"}
    resp = await mod.mindmap()
    assert resp["code"] != 0

    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"question": "q", "kb_ids": ["kb1"]}

    mod.get_request_json = _get_request_json
    mod.gen_mindmap = lambda *_args, **_kwargs: {"error": "boom"}
    resp = await mod.mindmap()
    assert resp["code"] == 500
