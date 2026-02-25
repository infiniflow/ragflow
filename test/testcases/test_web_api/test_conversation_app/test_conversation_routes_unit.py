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

import asyncio
import importlib.util
import json
import sys
from copy import deepcopy
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _DummyRequest:
    def __init__(self, *, args=None, headers=None, form=None, files=None):
        self.args = args or {}
        self.headers = headers or {}
        self.form = _AwaitableValue(form or {})
        self.files = _AwaitableValue(files or {})
        self.method = "POST"
        self.content_length = 0


class _DummyConversation:
    def __init__(self, *, conv_id="conv-1", dialog_id="dialog-1", message=None, reference=None):
        self.id = conv_id
        self.dialog_id = dialog_id
        self.message = message if message is not None else []
        self.reference = reference if reference is not None else []

    def to_dict(self):
        return {
            "id": self.id,
            "dialog_id": self.dialog_id,
            "message": deepcopy(self.message),
            "reference": deepcopy(self.reference),
        }


class _DummyDialog:
    def __init__(self, *, dialog_id="dialog-1", tenant_id="tenant-1", icon="avatar.png"):
        self.id = dialog_id
        self.tenant_id = tenant_id
        self.icon = icon
        self.prompt_config = {"prologue": "hello"}
        self.llm_id = ""
        self.llm_setting = {}

    def to_dict(self):
        return {
            "id": self.id,
            "icon": self.icon,
            "tenant_id": self.tenant_id,
            "prompt_config": deepcopy(self.prompt_config),
        }


class _DummyUploadedFile:
    def __init__(self, filename):
        self.filename = filename
        self.saved_path = None

    async def save(self, path):
        self.saved_path = path
        Path(path).write_bytes(b"audio-bytes")


def _run(coro):
    return asyncio.run(coro)


def _load_conversation_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    deepdoc_pkg = ModuleType("deepdoc")
    deepdoc_parser_pkg = ModuleType("deepdoc.parser")
    deepdoc_parser_pkg.__path__ = []

    class _StubPdfParser:
        pass

    class _StubExcelParser:
        pass

    class _StubDocxParser:
        pass

    deepdoc_parser_pkg.PdfParser = _StubPdfParser
    deepdoc_parser_pkg.ExcelParser = _StubExcelParser
    deepdoc_parser_pkg.DocxParser = _StubDocxParser
    deepdoc_pkg.parser = deepdoc_parser_pkg
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser_pkg)

    deepdoc_excel_module = ModuleType("deepdoc.parser.excel_parser")
    deepdoc_excel_module.RAGFlowExcelParser = _StubExcelParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.excel_parser", deepdoc_excel_module)

    deepdoc_parser_utils = ModuleType("deepdoc.parser.utils")
    deepdoc_parser_utils.get_text = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", deepdoc_parser_utils)
    monkeypatch.setitem(sys.modules, "xgboost", ModuleType("xgboost"))

    apps_mod = ModuleType("api.apps")
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    module_name = "test_conversation_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "conversation_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _set_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(deepcopy(payload)))


async def _read_sse_text(response):
    chunks = []
    async for chunk in response.response:
        if isinstance(chunk, bytes):
            chunks.append(chunk.decode("utf-8"))
        else:
            chunks.append(chunk)
    return "".join(chunks)


@pytest.mark.p2
def test_set_conversation_update_create_and_errors(monkeypatch):
    module = _load_conversation_module(monkeypatch)

    long_name = "n" * 300
    create_payload = {
        "conversation_id": "conv-new",
        "dialog_id": "dialog-1",
        "is_new": True,
        "name": long_name,
    }
    _set_request_json(monkeypatch, module, create_payload)

    saved = {}
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialog()))
    monkeypatch.setattr(module.ConversationService, "save", lambda **kwargs: saved.update(kwargs) or True)
    res = _run(module.set_conversation())
    assert res["code"] == 0
    assert len(res["data"]["name"]) == 255
    assert saved["user_id"] == "user-1"

    update_payload = {
        "conversation_id": "conv-1",
        "dialog_id": "dialog-1",
        "is_new": False,
        "name": "rename",
    }
    _set_request_json(monkeypatch, module, update_payload)
    monkeypatch.setattr(module.ConversationService, "update_by_id", lambda *_args, **_kwargs: False)
    res = _run(module.set_conversation())
    assert "Conversation not found" in res["message"]

    _set_request_json(monkeypatch, module, update_payload)
    monkeypatch.setattr(module.ConversationService, "update_by_id", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (False, None))
    res = _run(module.set_conversation())
    assert "Fail to update" in res["message"]

    _set_request_json(monkeypatch, module, update_payload)
    monkeypatch.setattr(module.ConversationService, "update_by_id", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (True, _DummyConversation(conv_id="conv-1")))
    res = _run(module.set_conversation())
    assert res["code"] == 0
    assert res["data"]["id"] == "conv-1"

    _set_request_json(monkeypatch, module, update_payload)

    def _raise_update(*_args, **_kwargs):
        raise RuntimeError("update boom")

    monkeypatch.setattr(module.ConversationService, "update_by_id", _raise_update)
    res = _run(module.set_conversation())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "update boom" in res["message"]

    missing_dialog_payload = {
        "conversation_id": "conv-2",
        "dialog_id": "dialog-missing",
        "is_new": True,
        "name": "create",
    }
    _set_request_json(monkeypatch, module, missing_dialog_payload)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (False, None))
    res = _run(module.set_conversation())
    assert res["message"] == "Dialog not found"

    _set_request_json(monkeypatch, module, missing_dialog_payload)

    def _raise_dialog(_id):
        raise RuntimeError("dialog boom")

    monkeypatch.setattr(module.DialogService, "get_by_id", _raise_dialog)
    res = _run(module.set_conversation())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "dialog boom" in res["message"]


@pytest.mark.p2
def test_get_and_getsse_authorization_and_reference_paths(monkeypatch):
    module = _load_conversation_module(monkeypatch)

    conv = _DummyConversation(reference=[{"doc": "d"}, ["already-formatted"]])
    monkeypatch.setattr(module, "request", _DummyRequest(args={"conversation_id": "conv-1"}))
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (True, conv))
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(icon="bot-avatar")])
    monkeypatch.setattr(module, "chunks_format", lambda _ref: [{"chunk": "normalized"}])

    res = _run(module.get())
    assert res["code"] == 0
    assert res["data"]["avatar"] == "bot-avatar"
    assert res["data"]["reference"][0]["chunks"] == [{"chunk": "normalized"}]

    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (False, None))
    res = _run(module.get())
    assert res["message"] == "Conversation not found!"

    monkeypatch.setattr(module, "request", _DummyRequest(args={"conversation_id": "conv-1"}))
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (True, conv))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(module.get())
    assert res["code"] == module.RetCode.OPERATING_ERROR
    assert "Only owner of conversation" in res["message"]

    def _raise_get(*_args, **_kwargs):
        raise RuntimeError("get boom")

    monkeypatch.setattr(module.ConversationService, "get_by_id", _raise_get)
    res = _run(module.get())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "get boom" in res["message"]

    monkeypatch.setattr(module, "request", _DummyRequest(headers={"Authorization": "Bearer"}))
    res = module.getsse("dialog-1")
    assert "Authorization is not valid" in res["message"]

    monkeypatch.setattr(module, "request", _DummyRequest(headers={"Authorization": "Bearer token-1"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = module.getsse("dialog-1")
    assert "API key is invalid" in res["message"]

    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace()])
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (False, None))
    res = module.getsse("dialog-1")
    assert res["message"] == "Dialog not found!"

    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialog()))
    res = module.getsse("dialog-1")
    assert res["code"] == 0
    assert res["data"]["avatar"] == "avatar.png"
    assert "icon" not in res["data"]

    def _raise_getsse(_id):
        raise RuntimeError("getsse boom")

    monkeypatch.setattr(module.DialogService, "get_by_id", _raise_getsse)
    res = module.getsse("dialog-1")
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "getsse boom" in res["message"]


@pytest.mark.p2
def test_rm_and_list_conversation_guards(monkeypatch):
    module = _load_conversation_module(monkeypatch)

    _set_request_json(monkeypatch, module, {"conversation_ids": ["conv-1"]})
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (False, None))
    res = _run(module.rm())
    assert "Conversation not found" in res["message"]

    conv = _DummyConversation(conv_id="conv-1", dialog_id="dialog-1")
    _set_request_json(monkeypatch, module, {"conversation_ids": ["conv-1"]})
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (True, conv))
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(module.rm())
    assert res["code"] == module.RetCode.OPERATING_ERROR

    deleted = []
    _set_request_json(monkeypatch, module, {"conversation_ids": ["conv-1"]})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="dialog-1")])
    monkeypatch.setattr(module.ConversationService, "delete_by_id", lambda cid: deleted.append(cid) or True)
    res = _run(module.rm())
    assert res["code"] == 0
    assert res["data"] is True
    assert deleted == ["conv-1"]

    _set_request_json(monkeypatch, module, {"conversation_ids": ["conv-1"]})

    def _raise_rm(*_args, **_kwargs):
        raise RuntimeError("rm boom")

    monkeypatch.setattr(module.ConversationService, "get_by_id", _raise_rm)
    res = _run(module.rm())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "rm boom" in res["message"]

    monkeypatch.setattr(module, "request", _DummyRequest(args={"dialog_id": "dialog-1"}))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(module.list_conversation())
    assert res["code"] == module.RetCode.OPERATING_ERROR
    assert "Only owner of dialog" in res["message"]

    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="dialog-1")])
    monkeypatch.setattr(module.ConversationService, "model", SimpleNamespace(create_time="create_time"))
    monkeypatch.setattr(module.ConversationService, "query", lambda **_kwargs: [_DummyConversation(conv_id="c1"), _DummyConversation(conv_id="c2")])
    res = _run(module.list_conversation())
    assert res["code"] == 0
    assert [x["id"] for x in res["data"]] == ["c1", "c2"]

    def _raise_list(**_kwargs):
        raise RuntimeError("list boom")

    monkeypatch.setattr(module.ConversationService, "query", _raise_list)
    res = _run(module.list_conversation())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "list boom" in res["message"]


@pytest.mark.p2
def test_completion_stream_and_nonstream_branches(monkeypatch):
    module = _load_conversation_module(monkeypatch)

    conv = _DummyConversation(conv_id="conv-1", dialog_id="dialog-1", reference=[])
    dia = _DummyDialog(dialog_id="dialog-1", tenant_id="tenant-1")
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (True, conv))
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, dia))
    monkeypatch.setattr(module, "structure_answer", lambda _conv, ans, message_id, conv_id: {"answer": ans["answer"], "id": message_id, "conversation_id": conv_id, "reference": []})

    updates = []
    monkeypatch.setattr(module.ConversationService, "update_by_id", lambda conv_id, payload: updates.append((conv_id, payload)) or True)

    stream_payload = {
        "conversation_id": "conv-1",
        "messages": [
            {"role": "system", "content": "ignored"},
            {"role": "assistant", "content": "ignored-first-assistant"},
            {"role": "user", "content": "hello", "id": "m-1"},
        ],
        "stream": True,
    }

    async def _stream_ok(_dia, sanitized, *_args, **_kwargs):
        assert [m["role"] for m in sanitized] == ["user"]
        yield {"answer": "sse-ok"}

    monkeypatch.setattr(module, "async_chat", _stream_ok)
    _set_request_json(monkeypatch, module, stream_payload)
    resp = _run(module.completion.__wrapped__())
    assert resp.headers["Content-Type"].startswith("text/event-stream")
    sse_text = _run(_read_sse_text(resp))
    assert "sse-ok" in sse_text
    assert '"data": true' in sse_text
    assert updates

    async def _stream_error(_dia, _sanitized, *_args, **_kwargs):
        raise RuntimeError("stream explode")
        if False:
            yield {"answer": "never"}

    monkeypatch.setattr(module, "async_chat", _stream_error)
    _set_request_json(monkeypatch, module, stream_payload)
    resp = _run(module.completion.__wrapped__())
    sse_text = _run(_read_sse_text(resp))
    assert "**ERROR**: stream explode" in sse_text

    async def _non_stream(_dia, _sanitized, **_kwargs):
        yield {"answer": "plain-ok"}

    monkeypatch.setattr(module, "async_chat", _non_stream)
    _set_request_json(
        monkeypatch,
        module,
        {
            "conversation_id": "conv-1",
            "messages": [{"role": "user", "content": "plain", "id": "m-2"}],
            "stream": False,
        },
    )
    res = _run(module.completion.__wrapped__())
    assert res["code"] == 0
    assert res["data"]["answer"] == "plain-ok"

    monkeypatch.setattr(module.TenantLLMService, "get_api_key", lambda **_kwargs: False)
    _set_request_json(
        monkeypatch,
        module,
        {
            "conversation_id": "conv-1",
            "messages": [{"role": "user", "content": "embed", "id": "m-3"}],
            "llm_id": "bad-model",
            "stream": False,
        },
    )
    res = _run(module.completion.__wrapped__())
    assert "Cannot use specified model bad-model" in res["message"]

    monkeypatch.setattr(module.TenantLLMService, "get_api_key", lambda **_kwargs: "api-key")
    _set_request_json(
        monkeypatch,
        module,
        {
            "conversation_id": "conv-1",
            "messages": [{"role": "user", "content": "embed", "id": "m-4"}],
            "llm_id": "glm-4",
            "temperature": 0.7,
            "top_p": 0.2,
            "stream": False,
        },
    )
    res = _run(module.completion.__wrapped__())
    assert res["code"] == 0
    assert dia.llm_id == "glm-4"
    assert dia.llm_setting == {"temperature": 0.7, "top_p": 0.2}

    _set_request_json(
        monkeypatch,
        module,
        {
            "conversation_id": "missing",
            "messages": [{"role": "user", "content": "x", "id": "m-5"}],
            "stream": False,
        },
    )
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (False, None))
    res = _run(module.completion.__wrapped__())
    assert res["message"] == "Conversation not found!"

    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (True, conv))
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (False, None))
    _set_request_json(
        monkeypatch,
        module,
        {
            "conversation_id": "conv-1",
            "messages": [{"role": "user", "content": "x", "id": "m-6"}],
            "stream": False,
        },
    )
    res = _run(module.completion.__wrapped__())
    assert res["message"] == "Dialog not found!"

    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (_ for _ in ()).throw(RuntimeError("completion boom")))
    _set_request_json(
        monkeypatch,
        module,
        {
            "conversation_id": "conv-1",
            "messages": [{"role": "user", "content": "x", "id": "m-7"}],
            "stream": False,
        },
    )
    res = _run(module.completion.__wrapped__())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR
    assert "completion boom" in res["message"]


@pytest.mark.p2
def test_sequence2txt_validation_and_transcription_paths(monkeypatch):
    module = _load_conversation_module(monkeypatch)

    monkeypatch.setattr(module, "request", _DummyRequest(form={"stream": "false"}, files={}))
    res = _run(module.sequence2txt())
    assert "Missing 'file'" in res["message"]

    bad_file = _DummyUploadedFile("audio.txt")
    monkeypatch.setattr(module, "request", _DummyRequest(form={"stream": "false"}, files={"file": bad_file}))
    res = _run(module.sequence2txt())
    assert "Unsupported audio format" in res["message"]

    wav_file = _DummyUploadedFile("audio.wav")
    monkeypatch.setattr(module, "request", _DummyRequest(form={"stream": "false"}, files={"file": wav_file}))
    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _uid: [])
    res = _run(module.sequence2txt())
    assert res["message"] == "Tenant not found!"

    wav_file = _DummyUploadedFile("audio.wav")
    monkeypatch.setattr(module, "request", _DummyRequest(form={"stream": "false"}, files={"file": wav_file}))
    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _uid: [{"tenant_id": "tenant-1", "asr_id": ""}])
    res = _run(module.sequence2txt())
    assert res["message"] == "No default ASR model is set"

    class _SyncAsr:
        def transcription(self, _path):
            return "transcribed text"

        def stream_transcription(self, _path):
            return []

    wav_file = _DummyUploadedFile("audio.wav")
    monkeypatch.setattr(module, "request", _DummyRequest(form={"stream": "false"}, files={"file": wav_file}))
    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _uid: [{"tenant_id": "tenant-1", "asr_id": "asr-model"}])
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _SyncAsr())
    monkeypatch.setattr(module.os, "remove", lambda _path: (_ for _ in ()).throw(RuntimeError("remove failed")))
    res = _run(module.sequence2txt())
    assert res["code"] == 0
    assert res["data"]["text"] == "transcribed text"

    class _StreamAsr:
        def transcription(self, _path):
            return ""

        def stream_transcription(self, _path):
            yield {"event": "partial", "text": "hello"}

    wav_file = _DummyUploadedFile("audio.wav")
    monkeypatch.setattr(module, "request", _DummyRequest(form={"stream": "true"}, files={"file": wav_file}))
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _StreamAsr())
    resp = _run(module.sequence2txt())
    assert resp.headers["Content-Type"].startswith("text/event-stream")
    sse_text = _run(_read_sse_text(resp))
    assert '"event": "partial"' in sse_text

    class _ErrorStreamAsr:
        def transcription(self, _path):
            return ""

        def stream_transcription(self, _path):
            raise RuntimeError("stream asr boom")

    wav_file = _DummyUploadedFile("audio.wav")
    monkeypatch.setattr(module, "request", _DummyRequest(form={"stream": "true"}, files={"file": wav_file}))
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _ErrorStreamAsr())
    resp = _run(module.sequence2txt())
    sse_text = _run(_read_sse_text(resp))
    assert "stream asr boom" in sse_text


@pytest.mark.p2
def test_tts_request_parse_entry(monkeypatch):
    module = _load_conversation_module(monkeypatch)
    _set_request_json(monkeypatch, module, {"text": "hello"})
    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _uid: [])
    res = _run(module.tts())
    assert res["message"] == "Tenant not found!"
