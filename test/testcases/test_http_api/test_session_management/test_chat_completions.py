#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import requests
import pytest
import types
import json
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from session_stub import load_session_module
from common import (
    bulk_upload_documents,
    chat_completions,
    create_chat_assistant,
    create_session_with_chat_assistant,
    delete_chat_assistants,
    delete_session_with_chat_assistants,
    HEADERS,
    list_documents,
    parse_documents,
)
from configs import HOST_ADDRESS, VERSION
from utils import wait_for


@wait_for(200, 1, "Document parsing timeout")
def _parse_done(auth, dataset_id, document_ids=None):
    res = list_documents(auth, dataset_id)
    target_docs = res["data"]["docs"]
    if document_ids is None:
        return all(doc.get("run") == "DONE" for doc in target_docs)
    target_ids = set(document_ids)
    for doc in target_docs:
        if doc.get("id") in target_ids and doc.get("run") != "DONE":
            return False
    return True


class TestChatCompletions:
    @pytest.mark.p3
    def test_chat_completion_stream_false_with_session(self, HttpApiAuth, add_dataset_func, tmp_path, request):
        dataset_id = add_dataset_func
        document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == 0, res
        _parse_done(HttpApiAuth, dataset_id, document_ids)

        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_test", "dataset_ids": [dataset_id]})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_session_with_chat_assistants(HttpApiAuth, chat_id))
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        res = create_session_with_chat_assistant(HttpApiAuth, chat_id, {"name": "session_for_completion"})
        assert res["code"] == 0, res
        session_id = res["data"]["id"]

        res = chat_completions(
            HttpApiAuth,
            chat_id,
            {"question": "hello", "stream": False, "session_id": session_id},
        )
        assert res["code"] == 0, res
        assert isinstance(res["data"], dict), res
        for key in ["answer", "reference", "audio_binary", "id", "session_id"]:
            assert key in res["data"], res
        assert res["data"]["session_id"] == session_id, res

    @pytest.mark.p2
    def test_chat_completion_invalid_chat(self, HttpApiAuth):
        res = chat_completions(
            HttpApiAuth,
            "invalid_chat_id",
            {"question": "hello", "stream": False, "session_id": "invalid_session"},
        )
        assert res["code"] == 102, res
        assert "You don't own the chat" in res.get("message", ""), res

    @pytest.mark.p2
    def test_chat_completion_invalid_session(self, HttpApiAuth, request):
        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_invalid_session", "dataset_ids": []})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_session_with_chat_assistants(HttpApiAuth, chat_id))
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        res = chat_completions(
            HttpApiAuth,
            chat_id,
            {"question": "hello", "stream": False, "session_id": "invalid_session"},
        )
        assert res["code"] == 102, res
        assert "You don't own the session" in res.get("message", ""), res

    @pytest.mark.p2
    def test_chat_completion_invalid_metadata_condition(self, HttpApiAuth, request):
        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_invalid_meta", "dataset_ids": []})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_session_with_chat_assistants(HttpApiAuth, chat_id))
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        res = create_session_with_chat_assistant(HttpApiAuth, chat_id, {"name": "session_for_meta"})
        assert res["code"] == 0, res
        session_id = res["data"]["id"]

        res = chat_completions(
            HttpApiAuth,
            chat_id,
            {
                "question": "hello",
                "stream": False,
                "session_id": session_id,
                "metadata_condition": "invalid",
            },
        )
        assert res["code"] == 102, res
        assert "metadata_condition" in res.get("message", ""), res

    @pytest.mark.p3
    def test_chat_completion_stream_minimal(self, HttpApiAuth, add_dataset_func, tmp_path, request):
        dataset_id = add_dataset_func
        document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == 0, res
        _parse_done(HttpApiAuth, dataset_id, document_ids)

        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_stream_test", "dataset_ids": [dataset_id]})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_session_with_chat_assistants(HttpApiAuth, chat_id))
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        res = create_session_with_chat_assistant(HttpApiAuth, chat_id, {"name": "session_for_stream"})
        assert res["code"] == 0, res
        session_id = res["data"]["id"]

        url = f"{HOST_ADDRESS}/api/{VERSION}/chats/{chat_id}/completions"
        stream_res = requests.post(
            url=url,
            headers=HEADERS,
            auth=HttpApiAuth,
            json={"question": "hello", "stream": True, "session_id": session_id},
            stream=True,
        )
        data_events = []
        terminal = False
        try:
            content_type = stream_res.headers.get("Content-Type", "")
            assert "text/event-stream" in content_type, content_type
            for line in stream_res.iter_lines(decode_unicode=True):
                if not line:
                    continue
                if line.startswith("data:"):
                    data_events.append(line)
                    if '"data": true' in line or '"data":true' in line:
                        terminal = True
                        break
        finally:
            stream_res.close()

        assert data_events, "Expected at least one data event"
        assert terminal, "Expected terminal data event"

    @pytest.mark.p3
    def test_sequence2txt_and_tts_validation_only(self, HttpApiAuth):
        sequence_url = f"{HOST_ADDRESS}/api/{VERSION}/sequence2txt"
        res = requests.post(url=sequence_url, auth=HttpApiAuth, data={"stream": "false"})
        body = res.json()
        assert body.get("code") != 0, body
        assert "Missing 'file'" in body.get("message", ""), body

        res = requests.post(
            url=sequence_url,
            auth=HttpApiAuth,
            files={"file": ("bad.txt", b"invalid")},
            data={"stream": "false"},
        )
        body = res.json()
        assert body.get("code") != 0, body
        assert "Unsupported audio format" in body.get("message", ""), body

        tts_url = f"{HOST_ADDRESS}/api/{VERSION}/tts"
        res = requests.post(url=tts_url, headers=HEADERS, auth=HttpApiAuth, json={"text": "hello"})
        body = res.json()
        assert body.get("code") != 0, body
        assert "No default TTS model is set" in body.get("message", ""), body


@pytest.mark.asyncio
async def test_chat_completion_empty_payload_defaults_question(monkeypatch):
    mod = load_session_module(monkeypatch)
    captured = {}

    async def _get_request_json():
        return {}

    async def _rag_completion(_tenant_id, _chat_id, **req):
        captured["req"] = req
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=[])])
    mod.rag_completion = _rag_completion

    resp = await mod.chat_completion("tenant", "chat")
    assert resp.mimetype == "text/event-stream"
    assert captured["req"]["question"] == ""


@pytest.mark.asyncio
async def test_chat_completion_metadata_condition_filters_doc_ids(monkeypatch):
    mod = load_session_module(monkeypatch)
    captured = {}

    async def _get_request_json():
        return {
            "question": "hi",
            "metadata_condition": {"conditions": [{"name": "author"}]},
            "stream": True,
        }

    async def _rag_completion(_tenant_id, _chat_id, **req):
        captured["req"] = req
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=["kb"])])
    mod.DocMetadataService.get_flatted_meta_by_kbs = classmethod(lambda cls, _kb_ids: [{"id": "doc1"}])
    mod.meta_filter = lambda _metas, _conditions, _logic: ["doc1", "doc2"]
    mod.rag_completion = _rag_completion

    await mod.chat_completion("tenant", "chat")
    assert captured["req"]["doc_ids"] == "doc1,doc2"

    mod.meta_filter = lambda _metas, _conditions, _logic: []
    await mod.chat_completion("tenant", "chat")
    assert captured["req"]["doc_ids"] == "-999"

    async def _get_request_json_no_conditions():
        return {"question": "hi", "metadata_condition": {"logic": "and"}, "doc_ids": "keep", "stream": True}

    mod.get_request_json = _get_request_json_no_conditions
    mod.meta_filter = lambda _metas, _conditions, _logic: []
    await mod.chat_completion("tenant", "chat")
    assert "doc_ids" not in captured["req"]


@pytest.mark.asyncio
async def test_chat_completion_stream_headers(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"question": "hi", "stream": True}

    async def _rag_completion(_tenant_id, _chat_id, **_req):
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=[])])
    mod.rag_completion = _rag_completion

    resp = await mod.chat_completion("tenant", "chat")
    assert "text/event-stream" in resp.headers.get("Content-Type", "")
    assert resp.headers.get("Cache-control") == "no-cache"
    assert resp.headers.get("Connection") == "keep-alive"
    assert resp.headers.get("X-Accel-Buffering") == "no"


@pytest.mark.asyncio
async def test_sequence2txt_validation_and_upload(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.form = mod._awaitable({"stream": "false"})
    mod._stub_request.files = mod._awaitable({})

    resp = await mod.sequence2txt("tenant")
    assert resp["code"] != 0
    assert "Missing 'file'" in resp["message"]

    class _Uploaded:
        def __init__(self, filename):
            self.filename = filename
            self.saved_path = None

        async def save(self, path):
            self.saved_path = path

    bad_file = _Uploaded("bad.txt")
    mod._stub_request.files = mod._awaitable({"file": bad_file})
    resp = await mod.sequence2txt("tenant")
    assert resp["code"] != 0
    assert "Unsupported audio format" in resp["message"]

    good_file = _Uploaded("audio.wav")
    mod._stub_request.files = mod._awaitable({"file": good_file})
    monkeypatch.setattr(mod.tempfile, "mkstemp", lambda suffix=None: (1, "/tmp/audio.wav"))
    monkeypatch.setattr(mod.os, "close", lambda _fd: None)
    mod.TenantService.get_info_by = classmethod(lambda cls, _tenant_id: [])
    resp = await mod.sequence2txt("tenant")
    assert resp["code"] != 0
    assert good_file.saved_path == "/tmp/audio.wav"


@pytest.mark.asyncio
async def test_sequence2txt_tenant_and_asr_errors(monkeypatch):
    mod = load_session_module(monkeypatch)

    class _Uploaded:
        def __init__(self, filename):
            self.filename = filename

        async def save(self, _path):
            return None

    mod._stub_request.form = mod._awaitable({"stream": "false"})
    mod._stub_request.files = mod._awaitable({"file": _Uploaded("audio.wav")})
    monkeypatch.setattr(mod.tempfile, "mkstemp", lambda suffix=None: (1, "/tmp/audio.wav"))
    monkeypatch.setattr(mod.os, "close", lambda _fd: None)

    mod.TenantService.get_info_by = classmethod(lambda cls, _tenant_id: [])
    resp = await mod.sequence2txt("tenant")
    assert resp["code"] != 0
    assert resp["message"] == "Tenant not found!"

    mod.TenantService.get_info_by = classmethod(lambda cls, _tenant_id: [{"asr_id": "" , "tenant_id": "t"}])
    resp = await mod.sequence2txt("tenant")
    assert resp["code"] != 0
    assert resp["message"] == "No default ASR model is set"


@pytest.mark.asyncio
async def test_sequence2txt_transcription_paths(monkeypatch):
    mod = load_session_module(monkeypatch)

    class _Uploaded:
        def __init__(self, filename):
            self.filename = filename

        async def save(self, _path):
            return None

    mod._stub_request.form = mod._awaitable({"stream": "false"})
    mod._stub_request.files = mod._awaitable({"file": _Uploaded("audio.wav")})
    monkeypatch.setattr(mod.tempfile, "mkstemp", lambda suffix=None: (1, "/tmp/audio.wav"))
    monkeypatch.setattr(mod.os, "close", lambda _fd: None)
    removed = {"path": None}
    monkeypatch.setattr(mod.os, "remove", lambda path: removed.__setitem__("path", path))

    mod.TenantService.get_info_by = classmethod(lambda cls, _tenant_id: [{"asr_id": "asr", "tenant_id": "t"}])
    resp = await mod.sequence2txt("tenant")
    assert resp["code"] == 0
    assert removed["path"] == "/tmp/audio.wav"

    mod._stub_request.form = mod._awaitable({"stream": "true"})
    mod._stub_request.files = mod._awaitable({"file": _Uploaded("audio.wav")})

    class _LLM:
        def __init__(self, *_args, **_kwargs):
            pass

        def stream_transcription(self, _path):
            raise RuntimeError("stream error")

    mod.LLMBundle = _LLM
    resp = await mod.sequence2txt("tenant")
    events = []
    async for line in resp.response:
        events.append(line)
    assert any("error" in line for line in events)


@pytest.mark.asyncio
async def test_tts_errors_and_stream(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"text": "hello"}

    mod.get_request_json = _get_request_json
    mod.TenantService.get_info_by = classmethod(lambda cls, _tenant_id: [])
    resp = await mod.tts("tenant")
    assert resp["code"] != 0
    assert resp["message"] == "Tenant not found!"

    mod.TenantService.get_info_by = classmethod(lambda cls, _tenant_id: [{"tts_id": "" , "tenant_id": "t"}])
    resp = await mod.tts("tenant")
    assert resp["code"] != 0
    assert resp["message"] == "No default TTS model is set"


@pytest.mark.asyncio
async def test_tts_stream_error_chunk(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"text": "hello"}

    class _LLM:
        def __init__(self, *_args, **_kwargs):
            pass

        def tts(self, _text):
            raise RuntimeError("boom")

    mod.get_request_json = _get_request_json
    mod.TenantService.get_info_by = classmethod(lambda cls, _tenant_id: [{"tts_id": "tts", "tenant_id": "t"}])
    mod.LLMBundle = _LLM

    resp = await mod.tts("tenant")
    assert resp.mimetype == "audio/mpeg"
    payloads = list(resp.response)
    assert payloads
    assert b"**ERROR**" in payloads[0]
