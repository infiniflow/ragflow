"""Exact registration contracts for the owned search and bot HTTP surfaces."""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


ROOT = Path(__file__).resolve().parents[4]


def _load_fixture(name, relative_path):
    spec = importlib.util.spec_from_file_location(name, ROOT / relative_path)
    fixture = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(fixture)
    return fixture


SEARCH_FIXTURE = _load_fixture(
    "api_files_chat_search_route_contract_fixture",
    "test/testcases/test_web_api/test_search_app/test_search_routes_unit.py",
)
BOT_FIXTURE = _load_fixture(
    "api_files_chat_bot_route_contract_fixture",
    "test/unit_test/api/apps/restful_apis/test_agentbots_access_control.py",
)
CHAT_FIXTURE = _load_fixture(
    "api_files_chat_chat_route_contract_fixture",
    "test/testcases/restful_api/test_user_tenant_routes_unit.py",
)

EXPECTED_SEARCH_ROUTES = {
    ("/searches", ("POST",), "create"),
    ("/searches", ("GET",), "list_searches"),
    ("/searches/<search_id>", ("GET",), "detail"),
    ("/searches/<search_id>", ("PUT",), "update"),
    ("/searches/<search_id>", ("DELETE",), "delete_search"),
    ("/searches/<search_id>/completion", ("POST",), "completion"),
    ("/searches/<search_id>/completions", ("POST",), "completion"),
}

EXPECTED_BOT_ROUTES = {
    ("/chatbots/<dialog_id>/completions", ("POST",), "chatbot_completions"),
    ("/chatbots/<dialog_id>/info", ("GET",), "chatbots_inputs"),
    ("/agentbots/<agent_id>/completions", ("POST",), "agent_bot_completions"),
    ("/agentbots/<agent_id>/inputs", ("GET",), "begin_inputs"),
    ("/agentbots/<agent_id>/upload", ("POST",), "upload_agent_bot_file"),
    ("/agentbots/<shared_id>/logs/<message_id>", ("GET",), "agent_bot_logs"),
    ("/searchbots/ask", ("POST",), "ask_about_embedded"),
    ("/searchbots/retrieval_test", ("POST",), "retrieval_test_embedded"),
    ("/searchbots/related_questions", ("POST",), "related_questions_embedded"),
    ("/searchbots/detail", ("GET",), "detail_share_embedded"),
    ("/searchbots/mindmap", ("POST",), "mindmap"),
}

EXPECTED_CHAT_ROUTES = {
    ("/chats", ("POST",), "create"),
    ("/chats", ("GET",), "list_chats"),
    ("/chats/<chat_id>", ("GET",), "get_chat"),
    ("/chats/<chat_id>", ("PUT",), "update_chat"),
    ("/chats/<chat_id>", ("PATCH",), "patch_chat"),
    ("/chats/<chat_id>", ("DELETE",), "delete_chat"),
    ("/chats", ("DELETE",), "bulk_delete_chats"),
    ("/chats/<chat_id>/sessions", ("POST",), "create_session"),
    ("/chats/<chat_id>/sessions", ("GET",), "list_sessions"),
    ("/chats/<chat_id>/sessions/<session_id>", ("GET",), "get_session"),
    ("/chats/<chat_id>/sessions/<session_id>", ("PATCH",), "update_session"),
    ("/chats/<chat_id>/sessions", ("DELETE",), "delete_sessions"),
    ("/chats/<chat_id>/sessions/<session_id>/messages/<msg_id>", ("DELETE",), "delete_session_message"),
    ("/chats/<chat_id>/sessions/<session_id>/messages/<msg_id>/feedback", ("PUT",), "update_message_feedback"),
    ("/chat/audio/speech", ("POST",), "tts"),
    ("/chat/audio/transcription", ("POST",), "transcription"),
    ("/chat/mindmap", ("POST",), "mindmap"),
    ("/chat/recommendation", ("POST",), "recommendation"),
    ("/chat/completions", ("POST",), "session_completion"),
}

EXPECTED_CHAT_CHANNEL_ROUTES = {
    ("/chat-channels", ("POST",), "create_chat_channel"),
    ("/chat-channels", ("GET",), "list_chat_channel"),
    ("/chat-channels/dialogs", ("GET",), "list_chat_channel_dialogs"),
    ("/chat-channels/<channel_id>", ("GET",), "get_chat_channel"),
    ("/chat-channels/<channel_id>", ("PATCH",), "update_chat_channel"),
    ("/chat-channels/<channel_id>", ("DELETE",), "rm_chat_channel"),
    ("/chat-channels/<channel_id>/runtime", ("GET",), "get_chat_channel_runtime"),
}

EXPECTED_DOCUMENT_ROUTES = {
    ("/documents/upload", ("POST",), "upload_info"),
    ("/datasets/<dataset_id>/documents/<document_id>", ("PATCH",), "update_document"),
    ("/datasets/<dataset_id>/metadata/summary", ("GET",), "metadata_summary"),
    ("/datasets/<dataset_id>/metadata/update", ("POST",), "metadata_batch_update"),
    ("/datasets/<dataset_id>/documents", ("POST",), "upload_document"),
    ("/datasets/<dataset_id>/documents", ("GET",), "list_docs"),
    ("/datasets/<dataset_id>/documents", ("DELETE",), "delete_documents"),
    ("/datasets/<dataset_id>/documents/<document_id>/metadata/config", ("PUT",), "update_metadata_config"),
    ("/thumbnails", ("GET",), "list_thumbnails"),
    ("/datasets/<dataset_id>/documents/metadatas", ("PATCH",), "update_metadata"),
    ("/documents/ingest", ("POST",), "ingest"),
    ("/datasets/<dataset_id>/documents/parse", ("POST",), "parse_documents"),
    ("/datasets/<dataset_id>/documents/stop", ("POST",), "stop_parse_documents"),
    ("/documents/images/<image_id>", ("GET",), "get_document_image"),
    ("/documents/artifact/<filename>", ("GET",), "get_artifact"),
    ("/datasets/<dataset_id>/documents/batch-update-status", ("POST",), "batch_update_document_status"),
    ("/documents/<doc_id>/preview", ("GET",), "get"),
    ("/datasets/<dataset_id>/documents/<document_id>", ("GET",), "download"),
    ("/documents/<document_id>", ("GET",), "download_document"),
}


class _RecordingManager:
    def __init__(self):
        self.routes = []

    def route(self, path, *, methods, endpoint=None, **_kwargs):
        def decorator(func):
            self.routes.append((path, tuple(sorted(methods)), endpoint or func.__name__))
            return func

        return decorator


def _load_search(monkeypatch):
    monkeypatch.setattr(SEARCH_FIXTURE, "_DummyManager", _RecordingManager)
    return SEARCH_FIXTURE._load_search_api(monkeypatch)


def _load_bot(monkeypatch):
    monkeypatch.setattr(BOT_FIXTURE, "_PassthroughManager", _RecordingManager)
    return BOT_FIXTURE._load_bot_api(monkeypatch, accessible=True, calls={})


def _load_chat(monkeypatch):
    monkeypatch.setattr(CHAT_FIXTURE, "_DummyManager", _RecordingManager)
    return CHAT_FIXTURE._load_chat_routes_unit_module(monkeypatch)


def _load_chat_channel(monkeypatch):
    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(ROOT / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(ROOT / "api/apps")]
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    services_mod = ModuleType("api.db.services")
    services_mod.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_mod)
    for module_name, symbol in (
        ("chat_channel_service", "ChatChannelService"),
        ("dialog_service", "DialogService"),
        ("managed_resource_service", "ManagedResourceService"),
    ):
        service_mod = ModuleType(f"api.db.services.{module_name}")
        setattr(service_mod, symbol, SimpleNamespace())
        monkeypatch.setitem(sys.modules, service_mod.__name__, service_mod)

    utils_mod = ModuleType("api.utils.api_utils")
    utils_mod.get_data_error_result = lambda **_kwargs: None
    utils_mod.get_json_result = lambda **_kwargs: None

    async def _request_json():
        return {}

    utils_mod.get_request_json = _request_json
    utils_mod.validate_request = lambda *_args, **_kwargs: lambda func: func
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", utils_mod)

    constants_mod = ModuleType("common.constants")
    constants_mod.RetCode = SimpleNamespace(AUTHENTICATION_ERROR=109)
    constants_mod.StatusEnum = SimpleNamespace(VALID=SimpleNamespace(value="1"))
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)
    misc_mod = ModuleType("common.misc_utils")
    misc_mod.get_uuid = lambda: "uuid"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_mod)

    spec = importlib.util.spec_from_file_location(
        "api_files_chat_channel_route_contract_module",
        ROOT / "api/apps/restful_apis/chat_channel_api.py",
    )
    module = importlib.util.module_from_spec(spec)
    module.manager = _RecordingManager()
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    return module


def _stub_module(monkeypatch, name, **attrs):
    module = ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    monkeypatch.setitem(sys.modules, name, module)
    return module


def _load_document(monkeypatch):
    def _passthrough_decorator(func=None, **_kwargs):
        if func is None:
            return lambda decorated: decorated
        return func

    api_pkg = _stub_module(monkeypatch, "api")
    api_pkg.__path__ = [str(ROOT / "api")]
    apps_mod = _stub_module(
        monkeypatch,
        "api.apps",
        AUTH_JWT="jwt",
        AUTH_API="api",
        AUTH_BETA="beta",
        current_user=SimpleNamespace(id="user-1"),
        login_required=_passthrough_decorator,
    )
    apps_mod.__path__ = [str(ROOT / "api/apps")]
    _stub_module(
        monkeypatch,
        "api.constants",
        FILE_NAME_LEN_LIMIT=255,
        IMG_BASE64_PREFIX="data:image/",
    )
    _stub_module(monkeypatch, "api.apps.services").__path__ = []
    _stub_module(
        monkeypatch,
        "api.apps.services.document_api_service",
        validate_document_update_fields=lambda *_args, **_kwargs: (None, None),
        map_doc_keys=lambda value: value,
        map_doc_keys_with_run_status=lambda value, **_kwargs: value,
        update_document_name_only=lambda *_args, **_kwargs: None,
        update_chunk_method=lambda *_args, **_kwargs: None,
        update_document_status_only=lambda *_args, **_kwargs: None,
        reset_document_for_reparse=lambda *_args, **_kwargs: None,
    )

    db_mod = _stub_module(
        monkeypatch,
        "api.db",
        VALID_FILE_TYPES=set(),
        FileType=SimpleNamespace(),
    )
    db_mod.__path__ = []
    _stub_module(
        monkeypatch,
        "api.db.db_models",
        API4Conversation=SimpleNamespace(),
        DB=SimpleNamespace(connection_context=lambda: lambda func: func),
        Task=SimpleNamespace(),
    )
    services_mod = _stub_module(monkeypatch, "api.db.services", duplicate_name=lambda *_args, **_kwargs: None)
    services_mod.__path__ = []
    for module_name, symbol in (
        ("doc_metadata_service", "DocMetadataService"),
        ("document_service", "DocumentService"),
        ("file2document_service", "File2DocumentService"),
        ("file_service", "FileService"),
        ("knowledgebase_service", "KnowledgebaseService"),
        ("canvas_service", "UserCanvasService"),
    ):
        _stub_module(monkeypatch, f"api.db.services.{module_name}", **{symbol: SimpleNamespace()})
    _stub_module(
        monkeypatch,
        "api.db.services.task_service",
        TaskService=SimpleNamespace(),
        cancel_all_task_of=lambda *_args, **_kwargs: None,
    )

    utils_pkg = _stub_module(monkeypatch, "api.utils")
    utils_pkg.__path__ = []

    async def _request_json():
        return {}

    _stub_module(
        monkeypatch,
        "api.utils.api_utils",
        construct_json_result=lambda **_kwargs: None,
        get_data_error_result=lambda **_kwargs: None,
        get_error_data_result=lambda **_kwargs: None,
        get_result=lambda **_kwargs: None,
        get_json_result=lambda **_kwargs: None,
        server_error_response=lambda *_args, **_kwargs: None,
        add_managed_resource_owner_id_to_kwargs=_passthrough_decorator,
        add_tenant_id_to_kwargs=_passthrough_decorator,
        get_request_json=_request_json,
        get_error_argument_result=lambda **_kwargs: None,
        check_duplicate_ids=lambda values, _label: (values, []),
    )
    _stub_module(
        monkeypatch,
        "api.utils.pagination_utils",
        validate_rest_api_page_size=lambda value: value,
    )
    _stub_module(
        monkeypatch,
        "api.utils.validation_utils",
        UpdateDocumentReq=type("UpdateDocumentReq", (), {}),
        DeleteDocumentReq=type("DeleteDocumentReq", (), {}),
        format_validation_error_message=lambda *_args: "",
        validate_and_parse_json_request=lambda *_args, **_kwargs: None,
    )
    _stub_module(
        monkeypatch,
        "api.utils.file_utils",
        filename_type=lambda _name: "",
        thumbnail=lambda *_args, **_kwargs: None,
    )
    _stub_module(
        monkeypatch,
        "api.utils.file_response",
        apply_preview_file_response_headers=lambda response, **_kwargs: response,
    )
    _stub_module(
        monkeypatch,
        "api.utils.web_utils",
        CONTENT_TYPE_MAP={},
        html2pdf=lambda *_args, **_kwargs: None,
        is_valid_url=lambda _url: True,
        apply_safe_file_response_headers=lambda response, **_kwargs: response,
    )

    common_mod = _stub_module(monkeypatch, "common", settings=SimpleNamespace())
    common_mod.__path__ = []
    _stub_module(
        monkeypatch,
        "common.constants",
        ParserType=SimpleNamespace(),
        RetCode=SimpleNamespace(),
        TaskStatus=SimpleNamespace(),
        SANDBOX_ARTIFACT_BUCKET="sandbox",
    )
    _stub_module(
        monkeypatch,
        "common.metadata_utils",
        convert_conditions=lambda value: value,
        meta_filter=lambda *_args, **_kwargs: None,
        turn2jsonschema=lambda value: value,
    )
    _stub_module(
        monkeypatch,
        "common.misc_utils",
        get_uuid=lambda: "uuid",
        thread_pool_exec=lambda *_args, **_kwargs: None,
    )
    _stub_module(monkeypatch, "common.ssrf_guard", assert_url_is_safe=lambda _url: None)
    _stub_module(
        monkeypatch,
        "quart",
        request=SimpleNamespace(),
        make_response=lambda *_args, **_kwargs: None,
        send_file=lambda *_args, **_kwargs: None,
    )
    rag_mod = _stub_module(monkeypatch, "rag")
    rag_mod.__path__ = []
    rag_nlp_mod = _stub_module(monkeypatch, "rag.nlp", search=SimpleNamespace())
    rag_nlp_mod.__path__ = []

    spec = importlib.util.spec_from_file_location(
        "api_files_chat_document_route_contract_module",
        ROOT / "api/apps/restful_apis/document_api.py",
    )
    module = importlib.util.module_from_spec(spec)
    module.manager = _RecordingManager()
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    return module


def _assert_exact_inventory(module, expected):
    assert len(module.manager.routes) == len(expected)
    assert set(module.manager.routes) == expected


@pytest.mark.parametrize(
    ("loader", "expected"),
    (
        (_load_search, EXPECTED_SEARCH_ROUTES),
        (_load_bot, EXPECTED_BOT_ROUTES),
        (_load_chat, EXPECTED_CHAT_ROUTES),
        (_load_chat_channel, EXPECTED_CHAT_CHANNEL_ROUTES),
        (_load_document, EXPECTED_DOCUMENT_ROUTES),
    ),
    ids=("search", "bot", "chat", "chat-channel", "document"),
)
def test_api_files_chat_route_inventory_is_exact(monkeypatch, loader, expected):
    _assert_exact_inventory(loader(monkeypatch), expected)


@pytest.mark.parametrize(
    ("loader", "expected"),
    (
        (_load_search, EXPECTED_SEARCH_ROUTES),
        (_load_bot, EXPECTED_BOT_ROUTES),
        (_load_chat, EXPECTED_CHAT_ROUTES),
        (_load_chat_channel, EXPECTED_CHAT_CHANNEL_ROUTES),
        (_load_document, EXPECTED_DOCUMENT_ROUTES),
    ),
    ids=("search", "bot", "chat", "chat-channel", "document"),
)
def test_api_files_chat_route_inventory_rejects_changed_registration(monkeypatch, loader, expected):
    module = loader(monkeypatch)
    module.manager.routes[-1] = ("/api-files-chat/unexpected", ("POST",), "unexpected")

    with pytest.raises(AssertionError):
        _assert_exact_inventory(module, expected)
