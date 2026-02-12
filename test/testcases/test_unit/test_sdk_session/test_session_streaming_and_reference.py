import json
import sys
import types
from pathlib import Path

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func
        return decorator


def _install_stub(monkeypatch, name, attrs=None):
    mod = types.ModuleType(name)
    if attrs:
        for key, value in attrs.items():
            setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _ensure_module(monkeypatch, name):
    if name in sys.modules:
        return sys.modules[name]
    return _install_stub(monkeypatch, name)


def _load_session_module(monkeypatch):
    root = Path(__file__).resolve().parents[4]
    file_path = root / "api" / "apps" / "sdk" / "session.py"

    _ensure_module(monkeypatch, "api")
    _ensure_module(monkeypatch, "api.db")
    _ensure_module(monkeypatch, "api.db.services")
    _ensure_module(monkeypatch, "api.utils")
    _ensure_module(monkeypatch, "common")
    _ensure_module(monkeypatch, "rag")
    _ensure_module(monkeypatch, "rag.prompts")
    _ensure_module(monkeypatch, "rag.app")
    _ensure_module(monkeypatch, "agent")

    class _Headers(dict):
        def add_header(self, key, value):
            self[key] = value

    class _Response:
        def __init__(self, response, mimetype=None, content_type=None):
            self.response = response
            self.mimetype = mimetype or content_type
            self.headers = _Headers()

    _install_stub(
        monkeypatch,
        "quart",
        {
            "Response": _Response,
            "jsonify": lambda payload: payload,
            "request": types.SimpleNamespace(args={}, headers={}),
        },
    )

    _install_stub(
        monkeypatch,
        "common.token_utils",
        {"num_tokens_from_string": lambda text: len(text or "")},
    )

    async def _apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl, local_doc_ids):
        return local_doc_ids

    _install_stub(
        monkeypatch,
        "common.metadata_utils",
        {
            "apply_meta_data_filter": _apply_meta_data_filter,
            "convert_conditions": lambda conditions: conditions,
            "meta_filter": lambda metas, conditions, logic: [],
        },
    )

    async def _cross_languages(_tenant_id, _arg, question, _langs):
        return question

    async def _keyword_extraction(_chat_mdl, _question):
        return ""

    def _chunks_format(reference):
        if isinstance(reference, dict):
            chunks = reference.get("chunks", [])
        else:
            chunks = reference or []
        normalized = []
        for chunk in chunks:
            if isinstance(chunk, dict):
                item = dict(chunk)
                if "id" not in item and "chunk_id" in item:
                    item["id"] = item["chunk_id"]
                if "content" not in item and "content_with_weight" in item:
                    item["content"] = item["content_with_weight"]
                normalized.append(item)
            else:
                normalized.append(chunk)
        return normalized

    _install_stub(
        monkeypatch,
        "rag.prompts.generator",
        {
            "cross_languages": _cross_languages,
            "keyword_extraction": _keyword_extraction,
            "chunks_format": _chunks_format,
        },
    )

    _install_stub(monkeypatch, "rag.prompts.template", {"load_prompt": lambda name: f"prompt:{name}"})
    _install_stub(monkeypatch, "rag.app.tag", {"label_question": lambda _q, _kb: []})

    class _EnumValue:
        def __init__(self, value):
            self.value = value

    class _RetCode:
        SUCCESS = 0
        DATA_ERROR = 102
        OPERATING_ERROR = 103
        AUTHENTICATION_ERROR = 109

    class _LLMType:
        CHAT = _EnumValue("chat")
        EMBEDDING = _EnumValue("embedding")
        RERANK = _EnumValue("rerank")
        SPEECH2TEXT = _EnumValue("asr")
        TTS = _EnumValue("tts")

    class _StatusEnum:
        VALID = types.SimpleNamespace(value="1")

    _install_stub(
        monkeypatch,
        "common.constants",
        {"RetCode": _RetCode, "LLMType": _LLMType, "StatusEnum": _StatusEnum},
    )

    _install_stub(
        monkeypatch,
        "common.settings",
        {
            "retriever": types.SimpleNamespace(retrieval=lambda *_args, **_kwargs: {}),
            "kg_retriever": types.SimpleNamespace(retrieval=lambda *_args, **_kwargs: {}),
        },
    )

    async def _get_request_json():
        return {}

    def _check_duplicate_ids(ids, _id_type="item"):
        return list(dict.fromkeys(ids)), []

    def _get_error_data_result(message="", code=_RetCode.DATA_ERROR):
        return {"code": code, "message": message}

    def _get_json_result(code=_RetCode.SUCCESS, message="success", data=None):
        payload = {"code": code, "message": message}
        if data is not None:
            payload["data"] = data
        return payload

    def _get_result(code=_RetCode.SUCCESS, message="", data=None, total=None):
        payload = {"code": code}
        if code == _RetCode.SUCCESS and data is not None:
            payload["data"] = data
        if code != _RetCode.SUCCESS:
            payload["message"] = message
        return payload

    def _get_data_openai(**kwargs):
        return kwargs

    def _server_error_response(exc):
        return {"code": 500, "message": str(exc)}

    def _token_required(func):
        return func

    def _validate_request(*_args, **_kwargs):
        def decorator(func):
            return func
        return decorator

    _install_stub(
        monkeypatch,
        "api.utils.api_utils",
        {
            "check_duplicate_ids": _check_duplicate_ids,
            "get_data_openai": _get_data_openai,
            "get_error_data_result": _get_error_data_result,
            "get_json_result": _get_json_result,
            "get_result": _get_result,
            "get_request_json": _get_request_json,
            "server_error_response": _server_error_response,
            "token_required": _token_required,
            "validate_request": _validate_request,
        },
    )

    class _APIToken:
        @classmethod
        def query(cls, **_kwargs):
            return []

    _install_stub(monkeypatch, "api.db.db_models", {"APIToken": _APIToken})

    class _DialogService:
        @classmethod
        def query(cls, **_kwargs):
            return []

    async def _async_chat(*_args, **_kwargs):
        if False:
            yield None

    async def _async_ask(*_args, **_kwargs):
        if False:
            yield None

    async def _gen_mindmap(*_args, **_kwargs):
        return {}

    _install_stub(
        monkeypatch,
        "api.db.services.dialog_service",
        {
            "DialogService": _DialogService,
            "async_chat": _async_chat,
            "async_ask": _async_ask,
            "gen_mindmap": _gen_mindmap,
        },
    )

    class _ConversationService:
        @classmethod
        def query(cls, **_kwargs):
            return []

        @classmethod
        def save(cls, **_kwargs):
            return True

        @classmethod
        def get_by_id(cls, _pid):
            return True, types.SimpleNamespace(to_dict=lambda: {})

        @classmethod
        def update_by_id(cls, _pid, _data):
            return 1

        @classmethod
        def delete_by_id(cls, _pid):
            return True

        @classmethod
        def get_list(cls, *_args, **_kwargs):
            return []

    async def _rag_completion(*_args, **_kwargs):
        if False:
            yield None

    async def _iframe_completion(*_args, **_kwargs):
        if False:
            yield None

    _install_stub(
        monkeypatch,
        "api.db.services.conversation_service",
        {
            "ConversationService": _ConversationService,
            "async_completion": _rag_completion,
            "async_iframe_completion": _iframe_completion,
        },
    )

    class _API4ConversationService:
        @classmethod
        def get_list(cls, *_args, **_kwargs):
            return 0, []

        @classmethod
        def query(cls, **_kwargs):
            return []

        @classmethod
        def save(cls, **_kwargs):
            return True

        @classmethod
        def delete_by_id(cls, _pid):
            return True

    _install_stub(monkeypatch, "api.db.services.api_service", {"API4ConversationService": _API4ConversationService})

    class _UserCanvasService:
        @classmethod
        def get_by_id(cls, _pid):
            return False, types.SimpleNamespace(dsl="{}", id="")

        @classmethod
        def query(cls, **_kwargs):
            return []

    async def _completion_openai(*_args, **_kwargs):
        if False:
            yield None

    async def _agent_completion(*_args, **_kwargs):
        if False:
            yield None

    _install_stub(
        monkeypatch,
        "api.db.services.canvas_service",
        {
            "UserCanvasService": _UserCanvasService,
            "completion_openai": _completion_openai,
            "completion": _agent_completion,
        },
    )

    class _DocMetadataService:
        @staticmethod
        def get_flatted_meta_by_kbs(_kb_ids):
            return []

        @staticmethod
        def get_metadata_for_documents(_doc_ids, _kb_id):
            return {}

    _install_stub(monkeypatch, "api.db.services.doc_metadata_service", {"DocMetadataService": _DocMetadataService})

    class _KnowledgebaseService:
        @classmethod
        def accessible(cls, *_args, **_kwargs):
            return True

        @classmethod
        def query(cls, **_kwargs):
            return []

        @classmethod
        def get_by_id(cls, _pid):
            return False, None

    _install_stub(monkeypatch, "api.db.services.knowledgebase_service", {"KnowledgebaseService": _KnowledgebaseService})

    class _LLMBundle:
        def __init__(self, *_args, **_kwargs):
            pass

        async def async_chat(self, *_args, **_kwargs):
            return ""

        def transcription(self, _path):
            return ""

        def stream_transcription(self, _path):
            return []

        def tts(self, _text):
            return []

        def get_component_input_form(self, _name):
            return {}

        def get_prologue(self):
            return ""

        def get_mode(self):
            return ""

    _install_stub(monkeypatch, "api.db.services.llm_service", {"LLMBundle": _LLMBundle})

    class _SearchService:
        @classmethod
        def get_detail(cls, _search_id):
            return {"search_config": {}}

        @classmethod
        def query(cls, **_kwargs):
            return []

    _install_stub(monkeypatch, "api.db.services.search_service", {"SearchService": _SearchService})

    class _TenantService:
        @classmethod
        def get_info_by(cls, _tenant_id):
            return []

    class _UserTenantService:
        @classmethod
        def query(cls, **_kwargs):
            return []

    _install_stub(
        monkeypatch,
        "api.db.services.user_service",
        {"TenantService": _TenantService, "UserTenantService": _UserTenantService},
    )

    class _Canvas:
        def __init__(self, _dsl, _tenant_id, _agent_id=None, canvas_id=None):
            self._dsl = _dsl
            self.id = canvas_id

        def reset(self):
            return None

        def get_prologue(self):
            return ""

        def get_component_input_form(self, _name):
            return {}

        def get_mode(self):
            return ""

        def __str__(self):
            return self._dsl

    _install_stub(monkeypatch, "agent.canvas", {"Canvas": _Canvas})
    _install_stub(monkeypatch, "common.misc_utils", {"get_uuid": lambda: "uuid"})

    module_name = "test_session_module"
    module = types.ModuleType(module_name)
    module.__file__ = str(file_path)
    module.manager = _DummyManager()
    sys.modules[module_name] = module
    source = file_path.read_text(encoding="utf-8")
    exec(compile(source, str(file_path), "exec"), module.__dict__)
    return module


@pytest.mark.p2
@pytest.mark.asyncio
async def test_openai_stream_reference_payload(monkeypatch):
    mod = _load_session_module(monkeypatch)

    async def _get_request_json():
        return {
            "model": "model",
            "messages": [
                {"role": "system", "content": "sys"},
                {"role": "user", "content": "hi"},
            ],
            "stream": True,
            "extra_body": {"reference": True},
        }

    async def _async_chat(_dia, _msg, _stream, **_kwargs):
        yield {"start_to_think": True}
        yield {"answer": "think"}
        yield {"end_to_think": True}
        yield {"answer": "hello"}
        yield {
            "final": True,
            "answer": "final",
            "reference": {"chunks": [{"chunk_id": "chunk-1", "content": "ref"}]},
        }

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=[])])
    mod.async_chat = _async_chat

    resp = await mod.chat_completion_openai_like("tenant", "chat")

    payloads = []
    done = False
    async for line in resp.response:
        if not line.startswith("data:"):
            continue
        data = line[5:].strip()
        if data == "[DONE]":
            done = True
            continue
        payloads.append(json.loads(data))

    assert done is True
    assert payloads

    last_payload = payloads[-1]
    delta = last_payload["choices"][0]["delta"]
    assert delta["reference"][0]["id"] == "chunk-1"
    assert delta["final_content"] == "final"
    assert last_payload["usage"]["prompt_tokens"] == len("hi")
    assert last_payload["usage"]["completion_tokens"] == len("think") + len("hello")
    assert last_payload["usage"]["total_tokens"] == last_payload["usage"]["prompt_tokens"] + last_payload["usage"]["completion_tokens"]

    assert any(
        p["choices"][0]["delta"].get("reasoning_content") == "think" for p in payloads
    )


@pytest.mark.p2
@pytest.mark.asyncio
async def test_agent_completions_stream_trace(monkeypatch):
    mod = _load_session_module(monkeypatch)

    async def _get_request_json():
        return {"stream": True, "return_trace": True}

    async def _agent_completion(*_args, **_kwargs):
        yield "data:" + json.dumps({"event": "node_finished", "data": {"component_id": "node-1"}}) + "\n\n"
        yield "data:" + json.dumps({"event": "message", "data": {"content": "hi"}}) + "\n\n"
        yield "data:" + json.dumps({"event": "message_end", "data": {"content": ""}}) + "\n\n"

    mod.get_request_json = _get_request_json
    mod.agent_completion = _agent_completion

    resp = await mod.agent_completions("tenant", "agent")

    outputs = []
    async for line in resp.response:
        outputs.append(line)

    assert outputs[-1] == "data:[DONE]\n\n"

    first_payload = json.loads(outputs[0][5:])
    assert "trace" in first_payload["data"]
    assert first_payload["data"]["trace"][0]["component_id"] == "node-1"


@pytest.mark.p2
def test_build_reference_chunks_metadata_filtering(monkeypatch):
    mod = _load_session_module(monkeypatch)

    class _DocMetadataService:
        @staticmethod
        def get_metadata_for_documents(_doc_ids, _kb_id):
            return {"doc-1": {"author": "alice", "year": 2024, "topic": "x"}}

    mod.DocMetadataService = _DocMetadataService

    reference = {"chunks": [{"dataset_id": "kb-1", "document_id": "doc-1"}]}
    chunks = mod._build_reference_chunks(reference, include_metadata=True, metadata_fields=["author", "year"])

    assert chunks[0]["document_metadata"] == {"author": "alice", "year": 2024}


@pytest.mark.p2
@pytest.mark.asyncio
async def test_list_session_reference_mapping_and_desc(monkeypatch):
    mod = _load_session_module(monkeypatch)

    captured = {}

    class _DialogService:
        @classmethod
        def query(cls, **_kwargs):
            return [types.SimpleNamespace(id="dialog")]

    def _get_list(_chat_id, _page, _page_size, _orderby, desc, _id, _name, _user_id=None):
        captured["desc"] = desc
        return [
            {
                "id": "sess-1",
                "dialog_id": "chat-1",
                "message": [
                    {"role": "user", "content": "hi", "prompt": "p1"},
                    {"role": "assistant", "content": "hello", "prompt": "p2"},
                ],
                "reference": [
                    {
                        "chunks": [
                            {
                                "chunk_id": "chunk-1",
                                "content_with_weight": "weighted",
                                "doc_id": "doc-1",
                                "docnm_kwd": "doc-name",
                                "kb_id": "kb-1",
                                "img_id": "img-1",
                                "position_int": [1, 2],
                            }
                        ]
                    }
                ],
            }
        ]

    mod.DialogService = _DialogService
    mod.ConversationService.get_list = classmethod(lambda cls, *args, **kwargs: _get_list(*args, **kwargs))

    mod.request.args = {
        "id": "sess-1",
        "page": "1",
        "page_size": "10",
        "orderby": "create_time",
        "desc": "False",
    }

    res = await mod.list_session("tenant", "chat-1")
    assert res["code"] == 0
    assert captured["desc"] is False

    messages = res["data"][0]["messages"]
    assert all("prompt" not in msg for msg in messages)
    assert messages[1]["reference"][0]["id"] == "chunk-1"


@pytest.mark.p2
@pytest.mark.asyncio
async def test_update_session_update_failure_mapping(monkeypatch):
    mod = _load_session_module(monkeypatch)

    async def _get_request_json():
        return {"name": "new-name"}

    mod.get_request_json = _get_request_json
    mod.ConversationService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="sess")])
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="dialog")])
    mod.ConversationService.update_by_id = classmethod(lambda cls, _pid, _data: 0)

    res = await mod.update("tenant", "chat-1", "sess-1")
    assert res["code"] == 102
    assert "Session updates error" in res.get("message", "")


@pytest.mark.p2
@pytest.mark.asyncio
async def test_create_session_internal_fetch_failure(monkeypatch):
    mod = _load_session_module(monkeypatch)

    async def _get_request_json():
        return {"name": "session"}

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(prompt_config={"prologue": "hi"})])
    mod.ConversationService.get_by_id = classmethod(lambda cls, _id: (False, None))

    res = await mod.create("tenant", "chat-1")
    assert res["code"] == 102
    assert "Fail to create a session" in res.get("message", "")


@pytest.mark.p2
@pytest.mark.asyncio
async def test_update_session_internal_failure(monkeypatch):
    mod = _load_session_module(monkeypatch)

    async def _get_request_json():
        return {"name": "new-name"}

    mod.get_request_json = _get_request_json
    mod.ConversationService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="sess")])
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="dialog")])
    mod.ConversationService.update_by_id = classmethod(lambda cls, _pid, _data: 0)

    res = await mod.update("tenant", "chat-1", "sess-1")
    assert res["code"] == 102
    assert "Session updates error" in res.get("message", "")


@pytest.mark.p2
def test_build_reference_chunks_no_metadata(monkeypatch):
    mod = _load_session_module(monkeypatch)
    reference = {"chunks": [{"dataset_id": "kb-1", "document_id": "doc-1"}]}
    chunks = mod._build_reference_chunks(reference, include_metadata=False)
    assert chunks
    assert "document_metadata" not in chunks[0]


@pytest.mark.p2
def test_build_reference_chunks_metadata_fields_filter(monkeypatch):
    mod = _load_session_module(monkeypatch)

    class _DocMetadataService:
        @staticmethod
        def get_metadata_for_documents(_doc_ids, _kb_id):
            return {"doc-1": {"author": "alice", "year": 2024, "topic": "x"}}

    mod.DocMetadataService = _DocMetadataService
    reference = {"chunks": [{"dataset_id": "kb-1", "document_id": "doc-1"}]}
    chunks = mod._build_reference_chunks(reference, include_metadata=True, metadata_fields=["topic"])
    assert chunks[0]["document_metadata"] == {"topic": "x"}
