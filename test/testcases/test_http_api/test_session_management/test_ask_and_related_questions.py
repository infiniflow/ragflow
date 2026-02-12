import sys
from pathlib import Path
import types
import json

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from session_stub import load_session_module


@pytest.mark.asyncio
async def test_ask_about_validation_and_dataset_checks(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {}

    mod.get_request_json = _get_request_json
    resp = await mod.ask_about("tenant")
    assert resp["code"] != 0
    assert "`question` is required." in resp["message"]

    async def _get_request_json_missing_dataset():
        return {"question": "hi"}

    mod.get_request_json = _get_request_json_missing_dataset
    resp = await mod.ask_about("tenant")
    assert "`dataset_ids` is required." in resp["message"]

    async def _get_request_json_bad_dataset_type():
        return {"question": "hi", "dataset_ids": "bad"}

    mod.get_request_json = _get_request_json_bad_dataset_type
    resp = await mod.ask_about("tenant")
    assert "`dataset_ids` should be a list." in resp["message"]

    async def _get_request_json_not_owned():
        return {"question": "hi", "dataset_ids": ["kb1"]}

    mod.get_request_json = _get_request_json_not_owned
    mod.KnowledgebaseService.accessible = classmethod(lambda cls, kb_id, user_id: False)
    resp = await mod.ask_about("tenant")
    assert "You don't own the dataset" in resp["message"]

    mod.KnowledgebaseService.accessible = classmethod(lambda cls, kb_id, user_id: True)
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(chunk_num=0)])
    resp = await mod.ask_about("tenant")
    assert "doesn't own parsed file" in resp["message"]


@pytest.mark.asyncio
async def test_ask_about_stream_headers_and_error_payload(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"question": "hi", "dataset_ids": ["kb1"]}

    async def _async_ask(*_args, **_kwargs):
        raise RuntimeError("boom")
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.KnowledgebaseService.accessible = classmethod(lambda cls, kb_id, user_id: True)
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(chunk_num=1)])
    mod.async_ask = _async_ask

    resp = await mod.ask_about("tenant")
    assert "text/event-stream" in resp.headers.get("Content-Type", "")
    lines = []
    async for line in resp.response:
        lines.append(line)
    assert any("**ERROR**" in line for line in lines)
    assert any('"data": true' in line or '"data":true' in line for line in lines)


@pytest.mark.asyncio
async def test_related_questions_missing_question(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {}

    mod.get_request_json = _get_request_json
    resp = await mod.related_questions("tenant")
    assert resp["code"] != 0
    assert "`question` is required." in resp["message"]


@pytest.mark.asyncio
async def test_related_questions_industry_prompt_and_normalize(monkeypatch):
    mod = load_session_module(monkeypatch)
    captured = {}

    async def _get_request_json():
        return {"question": "foo", "industry": "finance"}

    class _LLM:
        def __init__(self, *_args, **_kwargs):
            pass

        async def async_chat(self, prompt, _messages, _conf):
            captured["prompt"] = prompt
            return "1. alpha\n2. beta"

    mod.get_request_json = _get_request_json
    mod.LLMBundle = _LLM
    resp = await mod.related_questions("tenant")
    assert resp["code"] == 0
    assert resp["data"] == ["alpha", "beta"]
    assert "industry: finance" in captured["prompt"]
