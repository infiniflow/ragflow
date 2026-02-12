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


class _Headers(dict):
    def set(self, key, value):
        self[key] = value

    def add_header(self, key, value):
        self[key] = value


class _Response:
    def __init__(self, data, mimetype=None, content_type=None):
        self.data = data
        self.mimetype = mimetype or content_type
        self.headers = _Headers()


class _StrEnum(str):
    def __new__(cls, value):
        return str.__new__(cls, value)

    @property
    def value(self):
        return str(self)


class _Args:
    def __init__(self, state):
        self._state = state

    def get(self, key, default=None):
        return self._state.args.get(key, default)

    def getlist(self, key):
        value = self._state.args.get(key)
        if value is None:
            return []
        if isinstance(value, list):
            return value
        return [value]


class _Files(dict):
    def getlist(self, key):
        value = self.get(key)
        if value is None:
            return []
        if isinstance(value, list):
            return value
        return [value]


class _Request:
    def __init__(self, state):
        self._state = state
        self._args = _Args(state)

    @property
    def args(self):
        return self._args

    @property
    def form(self):
        async def _coro():
            return self._state.form
        return _coro()

    @property
    def files(self):
        async def _coro():
            return self._state.files
        return _coro()


class _FakeQuery:
    def __init__(self, rows):
        self._rows = rows

    def dicts(self):
        return list(self._rows)


class _State:
    def __init__(self):
        self.args = {}
        self.form = {}
        self.files = _Files()
        self.json = {}
        self.selenium_requests = []
        self.selenium_page_source = ""
        self.html_sections = ["section"]
        self.storage = {}


def _install_stub(monkeypatch, name, attrs=None):
    mod = types.ModuleType(name)
    if attrs:
        for key, value in attrs.items():
            setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    if "." in name:
        parent_name, child_name = name.rsplit(".", 1)
        parent = sys.modules.get(parent_name)
        if parent is not None:
            setattr(parent, child_name, mod)
    return mod


def _ensure_module(monkeypatch, name):
    if name in sys.modules:
        return sys.modules[name]
    return _install_stub(monkeypatch, name)


def _load_document_app(monkeypatch, tmp_path):
    root = Path(__file__).resolve().parents[4]
    file_path = root / "api" / "apps" / "document_app.py"
    state = _State()

    _ensure_module(monkeypatch, "api")
    _ensure_module(monkeypatch, "api.common")
    _ensure_module(monkeypatch, "api.db")
    _ensure_module(monkeypatch, "api.db.services")
    _ensure_module(monkeypatch, "api.utils")
    _ensure_module(monkeypatch, "common")
    _ensure_module(monkeypatch, "common.file_utils")
    _ensure_module(monkeypatch, "common.metadata_utils")
    _ensure_module(monkeypatch, "common.misc_utils")
    _ensure_module(monkeypatch, "deepdoc")
    _ensure_module(monkeypatch, "deepdoc.parser")
    _ensure_module(monkeypatch, "rag")
    _ensure_module(monkeypatch, "rag.nlp")

    async def _make_response(data):
        return _Response(data)

    _install_stub(
        monkeypatch,
        "quart",
        {
            "request": _Request(state),
            "make_response": _make_response,
        },
    )

    class _RetCode:
        SUCCESS = 0
        ARGUMENT_ERROR = 101
        DATA_ERROR = 102
        OPERATING_ERROR = 103
        AUTHENTICATION_ERROR = 109
        SERVER_ERROR = 500

    class _ParserType:
        PICTURE = _StrEnum("picture")
        AUDIO = _StrEnum("audio")
        PRESENTATION = _StrEnum("presentation")
        EMAIL = _StrEnum("email")

    class _TaskStatus:
        UNSTART = _StrEnum("0")
        RUNNING = _StrEnum("1")
        CANCEL = _StrEnum("2")
        DONE = _StrEnum("3")

    class _FileType:
        PDF = _StrEnum("pdf")
        DOC = _StrEnum("doc")
        VISUAL = _StrEnum("visual")
        AURAL = _StrEnum("aural")
        VIRTUAL = _StrEnum("virtual")
        FOLDER = _StrEnum("folder")
        OTHER = _StrEnum("other")

    _install_stub(
        monkeypatch,
        "common.constants",
        {
            "RetCode": _RetCode,
            "VALID_TASK_STATUS": {_TaskStatus.UNSTART.value, _TaskStatus.RUNNING.value, _TaskStatus.CANCEL.value, _TaskStatus.DONE.value},
            "ParserType": _ParserType,
            "TaskStatus": _TaskStatus,
        },
    )

    _install_stub(
        monkeypatch,
        "api.constants",
        {
            "FILE_NAME_LEN_LIMIT": 64,
            "IMG_BASE64_PREFIX": "data:image",
        },
    )

    _install_stub(
        monkeypatch,
        "api.db",
        {
            "VALID_FILE_TYPES": {_FileType.PDF, _FileType.DOC, _FileType.VISUAL, _FileType.AURAL, _FileType.VIRTUAL, _FileType.FOLDER, _FileType.OTHER},
            "FileType": _FileType,
        },
    )

    class _Task:
        doc_id = "doc_id"

    _install_stub(monkeypatch, "api.db.db_models", {"Task": _Task})

    def _duplicate_name(_query, name=None, kb_id=None):
        return name

    _install_stub(monkeypatch, "api.db.services", {"duplicate_name": _duplicate_name})

    class _DocumentService:
        @classmethod
        def accessible(cls, *_args, **_kwargs):
            return True

        @classmethod
        def accessible4deletion(cls, *_args, **_kwargs):
            return True

        @classmethod
        def query(cls, **_kwargs):
            return []

        @classmethod
        def get_by_id(cls, _doc_id):
            return False, None

        @classmethod
        def get_by_ids(cls, _doc_ids):
            return _FakeQuery([])

        @classmethod
        def get_by_kb_id(cls, *_args, **_kwargs):
            return [], 0

        @classmethod
        def get_filter_by_kb_id(cls, *_args, **_kwargs):
            return {}, 0

        @classmethod
        def get_thumbnails(cls, _doc_ids):
            return []

        @classmethod
        def insert(cls, data):
            return types.SimpleNamespace(to_dict=lambda: data, to_json=lambda: data, **data)

        @classmethod
        def update_by_id(cls, *_args, **_kwargs):
            return True

        @classmethod
        def update_parser_config(cls, *_args, **_kwargs):
            return True

        @classmethod
        def run(cls, *_args, **_kwargs):
            return None

        @classmethod
        def clear_chunk_num_when_rerun(cls, *_args, **_kwargs):
            return None

        @classmethod
        def increment_chunk_num(cls, *_args, **_kwargs):
            return True

        @classmethod
        def delete_chunk_images(cls, *_args, **_kwargs):
            return None

        @classmethod
        def get_tenant_id(cls, _doc_id):
            return "tenant"

    def _doc_upload_and_parse(_conversation_id, _file_objs, _user_id):
        return ["doc-id"]

    _install_stub(
        monkeypatch,
        "api.db.services.document_service",
        {
            "DocumentService": _DocumentService,
            "doc_upload_and_parse": _doc_upload_and_parse,
        },
    )

    class _DocMetadataService:
        @classmethod
        def get_flatted_meta_by_kbs(cls, _kb_ids):
            return {}

        @classmethod
        def get_document_metadata(cls, _doc_id):
            return {}

        @classmethod
        def get_metadata_summary(cls, _kb_id, _doc_ids):
            return {}

        @classmethod
        def batch_update_metadata(cls, *_args, **_kwargs):
            return 0

        @classmethod
        def update_document_metadata(cls, *_args, **_kwargs):
            return True

    _install_stub(monkeypatch, "api.db.services.doc_metadata_service", {"DocMetadataService": _DocMetadataService})

    class _File2DocumentService:
        @classmethod
        def get_by_document_id(cls, _doc_id):
            return []

        @classmethod
        def get_storage_address(cls, doc_id=None):
            return "bucket", f"{doc_id}.bin"

    _install_stub(monkeypatch, "api.db.services.file2document_service", {"File2DocumentService": _File2DocumentService})

    class _FileService:
        @classmethod
        def upload_document(cls, *_args, **_kwargs):
            return [], []

        @classmethod
        def get_root_folder(cls, _user_id):
            return {"id": "root"}

        @classmethod
        def init_knowledgebase_docs(cls, *_args, **_kwargs):
            return None

        @classmethod
        def get_kb_folder(cls, *_args, **_kwargs):
            return {"id": "kb-root"}

        @classmethod
        def new_a_file_from_kb(cls, *_args, **_kwargs):
            return {"id": "kb-folder"}

        @classmethod
        def add_file_from_kb(cls, *_args, **_kwargs):
            return None

        @classmethod
        def delete_docs(cls, *_args, **_kwargs):
            return None

        @classmethod
        def parse_docs(cls, _files, _user_id):
            return "parsed"

        @classmethod
        def get_by_id(cls, _file_id):
            return True, types.SimpleNamespace(id=_file_id)

        @classmethod
        def update_by_id(cls, *_args, **_kwargs):
            return True

        @classmethod
        def upload_info(cls, *_args, **_kwargs):
            return {"ok": True}

    _install_stub(monkeypatch, "api.db.services.file_service", {"FileService": _FileService})

    class _KnowledgebaseService:
        @classmethod
        def get_by_id(cls, _kb_id):
            kb = types.SimpleNamespace(id=_kb_id, tenant_id="tenant", name="kb", parser_id="parser", parser_config={}, pipeline_id="pipeline")
            return True, kb

        @classmethod
        def query(cls, **_kwargs):
            return []

    _install_stub(monkeypatch, "api.db.services.knowledgebase_service", {"KnowledgebaseService": _KnowledgebaseService})

    class _TaskService:
        @classmethod
        def query(cls, *_args, **_kwargs):
            return []

        @classmethod
        def filter_delete(cls, *_args, **_kwargs):
            return None

    def _cancel_all_task_of(_doc_id):
        return None

    _install_stub(monkeypatch, "api.db.services.task_service", {"TaskService": _TaskService, "cancel_all_task_of": _cancel_all_task_of})

    class _UserTenantService:
        @classmethod
        def query(cls, **_kwargs):
            return []

    _install_stub(monkeypatch, "api.db.services.user_service", {"UserTenantService": _UserTenantService})

    def _get_uuid():
        return "uuid"

    async def _thread_pool_exec(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    _install_stub(monkeypatch, "common.misc_utils", {"get_uuid": _get_uuid, "thread_pool_exec": _thread_pool_exec})

    def _get_project_base_directory():
        return str(tmp_path)

    _install_stub(monkeypatch, "common.file_utils", {"get_project_base_directory": _get_project_base_directory})

    def _get_json_result(code=_RetCode.SUCCESS, message="success", data=None):
        payload = {"code": code, "message": message}
        if data is not None:
            payload["data"] = data
        return payload

    def _get_data_error_result(message=""):
        return {"code": _RetCode.DATA_ERROR, "message": message}

    def _server_error_response(exc):
        return {"code": _RetCode.SERVER_ERROR, "message": str(exc)}

    async def _get_request_json():
        return state.json

    def _validate_request(*_args, **_kwargs):
        def decorator(func):
            return func
        return decorator

    _install_stub(
        monkeypatch,
        "api.utils.api_utils",
        {
            "get_json_result": _get_json_result,
            "get_data_error_result": _get_data_error_result,
            "server_error_response": _server_error_response,
            "validate_request": _validate_request,
            "get_request_json": _get_request_json,
        },
    )

    def _filename_type(_filename):
        return _FileType.PDF

    def _thumbnail(_filename, _blob):
        return "thumb"

    _install_stub(monkeypatch, "api.utils.file_utils", {"filename_type": _filename_type, "thumbnail": _thumbnail})

    def _is_valid_url(url):
        return url.startswith("http")

    def _html2pdf(_url):
        return b"blob"

    _install_stub(
        monkeypatch,
        "api.utils.web_utils",
        {
            "CONTENT_TYPE_MAP": {"png": "image/png", "txt": "text/plain"},
            "html2pdf": _html2pdf,
            "is_valid_url": _is_valid_url,
        },
    )

    _install_stub(
        monkeypatch,
        "common.metadata_utils",
        {
            "meta_filter": lambda _metas, _conds, _logic: [],
            "convert_conditions": lambda conditions: conditions,
            "turn2jsonschema": lambda metadata: {"schema": metadata},
        },
    )

    class _Search:
        @staticmethod
        def index_name(tenant_id):
            return f"idx-{tenant_id}"

    class _Tokenizer:
        @staticmethod
        def tokenize(text):
            return [text]

        @staticmethod
        def fine_grained_tokenize(tokens):
            return tokens

    _install_stub(monkeypatch, "rag.nlp", {"search": _Search, "rag_tokenizer": _Tokenizer})

    class _Storage:
        def obj_exist(self, *_args, **_kwargs):
            return False

        def put(self, *_args, **_kwargs):
            return None

        def get(self, *_args, **_kwargs):
            return b"data"

    class _DocStore:
        def index_exist(self, *_args, **_kwargs):
            return False

        def update(self, *_args, **_kwargs):
            return True

        def delete(self, *_args, **_kwargs):
            return None

    _install_stub(monkeypatch, "common.settings", {"STORAGE_IMPL": _Storage(), "docStoreConn": _DocStore()})

    class _RAGFlowHtmlParser:
        def parser_txt(self, _html):
            return state.html_sections

    _install_stub(monkeypatch, "deepdoc.parser.html_parser", {"RAGFlowHtmlParser": _RAGFlowHtmlParser})

    def _check_kb_team_permission(_kb, _user_id):
        return True

    _install_stub(monkeypatch, "api.common.check_team_permission", {"check_kb_team_permission": _check_kb_team_permission})

    class _ChromeOptions:
        def add_argument(self, _arg):
            return None

        def add_experimental_option(self, _key, _value):
            return None

    class _Chrome:
        def __init__(self, options=None):
            self.options = options
            self.requests = state.selenium_requests
            self.page_source = state.selenium_page_source
            self._url = None

        def get(self, url):
            self._url = url

        def quit(self):
            return None

    _install_stub(monkeypatch, "seleniumwire.webdriver", {"Chrome": _Chrome, "ChromeOptions": _ChromeOptions})

    _install_stub(
        monkeypatch,
        "api.apps",
        {
            "current_user": types.SimpleNamespace(id="user-id"),
            "login_required": lambda func: func,
            "manager": _DummyManager(),
        },
    )

    module_name = f"document_app_test_{id(state)}"
    module = types.ModuleType(module_name)
    module.__file__ = str(file_path)
    module.__dict__["manager"] = _DummyManager()
    sys.modules[module_name] = module
    source = file_path.read_text(encoding="utf-8")
    exec(compile(source, str(file_path), "exec"), module.__dict__)
    return module, state


@pytest.fixture()
def document_app(monkeypatch, tmp_path):
    return _load_document_app(monkeypatch, tmp_path)


@pytest.fixture()
def file_factory():
    def _make_file(name="file.txt"):
        class _File:
            def __init__(self, filename):
                self.filename = filename
                self.stream = types.SimpleNamespace(close=lambda: None)
                self.closed = False

            def close(self):
                self.closed = True

        return _File(name)
    return _make_file
