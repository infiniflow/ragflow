import json
import sys
import types
from pathlib import Path


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _Headers(dict):
    def add_header(self, key, value):
        self[key] = value


class _Response:
    def __init__(self, response, mimetype=None, content_type=None):
        self.response = response
        self.mimetype = mimetype or content_type
        self.headers = _Headers()


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _coro():
            return self._value

        return _coro().__await__()


class _StubRequest:
    def __init__(self):
        self.args = {}
        self.headers = {}
        self.form = _AwaitableValue({})
        self.files = _AwaitableValue({})


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


def load_session_module(monkeypatch):
    root = Path(__file__).resolve().parents[3]
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

    stub_request = _StubRequest()

    _install_stub(
        monkeypatch,
        "quart",
        {
            "Response": _Response,
            "jsonify": lambda payload: payload,
            "request": stub_request,
        },
    )

    _install_stub(monkeypatch, "common.token_utils", {"num_tokens_from_string": lambda text: len(text or "")})

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

    async def _retrieval(*_args, **_kwargs):
        return {"chunks": []}

    _install_stub(
        monkeypatch,
        "common.settings",
        {
            "retriever": types.SimpleNamespace(retrieval=_retrieval),
            "kg_retriever": types.SimpleNamespace(retrieval=_retrieval),
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

        @classmethod
        def get_by_id(cls, _id):
            return False, None

    _install_stub(
        monkeypatch,
        "api.db.services.dialog_service",
        {
            "DialogService": _DialogService,
            "async_ask": None,
            "async_chat": None,
            "gen_mindmap": None,
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
        def get_by_id(cls, _id):
            return False, None

        @classmethod
        def update_by_id(cls, _id, _req):
            return True

        @classmethod
        def get_list(cls, *_args, **_kwargs):
            return []

        @classmethod
        def delete_by_id(cls, _id):
            return True

    async def _async_completion(*_args, **_kwargs):
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
            "async_completion": _async_completion,
            "async_iframe_completion": _iframe_completion,
        },
    )

    class _API4ConversationService:
        @classmethod
        def save(cls, **_kwargs):
            return True

        @classmethod
        def query(cls, **_kwargs):
            return []

        @classmethod
        def get_list(cls, *_args, **_kwargs):
            return 0, []

        @classmethod
        def delete_by_id(cls, _id):
            return True

    _install_stub(monkeypatch, "api.db.services.api_service", {"API4ConversationService": _API4ConversationService})

    class _UserCanvasService:
        @classmethod
        def get_by_id(cls, _id):
            return False, None

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
        {"UserCanvasService": _UserCanvasService, "completion_openai": _completion_openai, "completion": _agent_completion},
    )

    class _DocMetadataService:
        @classmethod
        def get_flatted_meta_by_kbs(cls, _kb_ids):
            return []

        @classmethod
        def get_metadata_for_documents(cls, _doc_ids, _kb_id):
            return {}

    _install_stub(monkeypatch, "api.db.services.doc_metadata_service", {"DocMetadataService": _DocMetadataService})

    class _KnowledgebaseService:
        @classmethod
        def accessible(cls, _kb_id, _tenant_id):
            return True

        @classmethod
        def query(cls, **_kwargs):
            return []

        @classmethod
        def get_by_id(cls, _kb_id):
            return False, None

    _install_stub(monkeypatch, "api.db.services.knowledgebase_service", {"KnowledgebaseService": _KnowledgebaseService})

    class _SearchService:
        @classmethod
        def get_detail(cls, _search_id):
            return {}

        @classmethod
        def query(cls, **_kwargs):
            return []

    _install_stub(monkeypatch, "api.db.services.search_service", {"SearchService": _SearchService})

    class _UserTenantService:
        @classmethod
        def query(cls, **_kwargs):
            return []

    class _TenantService:
        @classmethod
        def get_info_by(cls, _tenant_id):
            return []

    _install_stub(
        monkeypatch,
        "api.db.services.user_service",
        {"TenantService": _TenantService, "UserTenantService": _UserTenantService},
    )

    class _LLMBundle:
        def __init__(self, *_args, **_kwargs):
            self._answer = "1. foo\n2. bar"

        async def async_chat(self, *_args, **_kwargs):
            return self._answer

        def transcription(self, _path):
            return "transcribed"

        def stream_transcription(self, _path):
            yield {"event": "data", "text": "hello"}

        def tts(self, _text):
            yield b"audio"

    _install_stub(monkeypatch, "api.db.services.llm_service", {"LLMBundle": _LLMBundle})

    class _Canvas:
        def __init__(self, dsl, *_args, **_kwargs):
            self._dsl = dsl
            self.id = "canvas-id"
            self.title = "canvas-title"
            self.avatar = "canvas-avatar"

        def reset(self):
            return None

        def get_prologue(self):
            return "prologue"

        def get_component_input_form(self, _name):
            return {"fields": []}

        def get_mode(self):
            return "mode"

        def __str__(self):
            return self._dsl

    _install_stub(monkeypatch, "agent.canvas", {"Canvas": _Canvas})
    _install_stub(monkeypatch, "common.misc_utils", {"get_uuid": lambda: "uuid"})

    module_name = "test_http_session_module"
    module = types.ModuleType(module_name)
    module.__file__ = str(file_path)
    module.manager = _DummyManager()
    sys.modules[module_name] = module
    source = file_path.read_text(encoding="utf-8")
    exec(compile(source, str(file_path), "exec"), module.__dict__)
    module._stub_request = stub_request
    module._awaitable = _AwaitableValue
    return module
