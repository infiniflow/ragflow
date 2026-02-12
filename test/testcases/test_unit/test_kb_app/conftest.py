import sys
import types
from pathlib import Path

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func
        return decorator


class _Args:
    def __init__(self, state):
        self._state = state

    def get(self, key, default=None):
        return self._state.args.get(key, default)

    def __getitem__(self, key):
        return self._state.args[key]


class _Request:
    def __init__(self, state):
        self._state = state
        self._args = _Args(state)

    @property
    def args(self):
        return self._args


class _State:
    def __init__(self):
        self.args = {}
        self.json = {}
        self.kb_create_side_effect = []
        self.kb_save_side_effect = []
        self.kb_accessible4deletion = True
        self.kb_accessible = True
        self.kb_query_owner_result = [object()]
        self.kb_query_duplicate_result = []
        self.kb_query_result = [object()]
        self.kb_query_side_effect = None
        self.kb_get_by_id_side_effect = []
        self.kb_obj = None
        self.kb_detail = None
        self.kb_get_by_tenant_ids_result = ([], 0)
        self.kb_get_by_tenant_ids_exc = None
        self.kb_update_by_id_side_effect = []
        self.kb_update_by_id_result = True
        self.kb_delete_by_id_result = True
        self.kb_total_size = 0
        self.kb_total_size_exc = None
        self.docs_by_kb = []
        self.docs_total = 0
        self.docs_query = []
        self.remove_document_side_effect = []
        self.remove_document_result = True
        self.connectors_list = []
        self.connector_link_errors = []
        self.file2doc_result = []
        self.file_service_updates = []
        self.file_service_deletes = []
        self.meta_flat = {}
        self.kb_basic_info = {}
        self.pipeline_logs_result = ([], 0)
        self.pipeline_logs_exc = None
        self.pipeline_dataset_logs_result = ([], 0)
        self.pipeline_dataset_logs_exc = None
        self.pipeline_log_get_result = (False, None)
        self.pipeline_deleted_ids = []
        self.task_get_result = (False, None)
        self.queue_task_id = "task-1"
        self.user_tenants = []
        self.user_tenants_dicts = []
        self.joined_tenants = []
        self.retriever_tags = []
        self.retriever_search_result = None
        self.doc_store_search_results = []
        self.doc_store_total = 0
        self.doc_store_docs = {}
        self.doc_store_index_exists = False
        self.doc_store_deleted = []
        self.doc_store_updated = []
        self.doc_store_delete_idx = []
        self.redis_sets = []
        self.doc_engine_infinity = False


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


def _load_kb_app(monkeypatch, tmp_path):
    root = Path(__file__).resolve().parents[4]
    file_path = root / "api" / "apps" / "kb_app.py"
    state = _State()

    _ensure_module(monkeypatch, "api")
    _ensure_module(monkeypatch, "api.db")
    _ensure_module(monkeypatch, "api.db.services")
    _ensure_module(monkeypatch, "api.utils")
    _ensure_module(monkeypatch, "common")
    _ensure_module(monkeypatch, "common.doc_store")
    _ensure_module(monkeypatch, "common.doc_store.doc_store_base")
    _ensure_module(monkeypatch, "common.metadata_utils")
    _ensure_module(monkeypatch, "common.misc_utils")
    _ensure_module(monkeypatch, "rag")
    _ensure_module(monkeypatch, "rag.nlp")
    _ensure_module(monkeypatch, "rag.utils")
    _ensure_module(monkeypatch, "rag.utils.redis_conn")

    _install_stub(monkeypatch, "quart", {"request": _Request(state)})

    class _RetCode:
        SUCCESS = 0
        ARGUMENT_ERROR = 101
        DATA_ERROR = 102
        OPERATING_ERROR = 103
        AUTHENTICATION_ERROR = 109
        SERVER_ERROR = 500
        NOT_EFFECTIVE = 120

    class _PipelineTaskType:
        GRAPH_RAG = "graph_rag"
        RAPTOR = "raptor"
        MINDMAP = "mindmap"

    class _StatusEnum:
        VALID = types.SimpleNamespace(value="valid")

    class _FileSource:
        KNOWLEDGEBASE = "knowledgebase"

    class _LLMType:
        EMBEDDING = "embedding"

    _install_stub(
        monkeypatch,
        "common.constants",
        {
            "RetCode": _RetCode,
            "PipelineTaskType": _PipelineTaskType,
            "StatusEnum": _StatusEnum,
            "VALID_TASK_STATUS": {"0", "1", "2", "3"},
            "FileSource": _FileSource,
            "LLMType": _LLMType,
            "PAGERANK_FLD": "pagerank_fld",
        },
    )

    _install_stub(monkeypatch, "api.constants", {"DATASET_NAME_LIMIT": 128})

    _install_stub(monkeypatch, "api.db", {"VALID_FILE_TYPES": {"pdf", "doc", "txt"}})

    class _File:
        tenant_id = "tenant_id"
        source_type = "source_type"
        type = "type"
        name = "name"
        id = "id"

    _install_stub(monkeypatch, "api.db.db_models", {"File": _File})

    class _OrderByExpr:
        def __init__(self):
            return None

    _install_stub(monkeypatch, "common.doc_store.doc_store_base", {"OrderByExpr": _OrderByExpr})

    class _KB:
        def __init__(self, **kwargs):
            self.__dict__.update(kwargs)

        def to_dict(self):
            return dict(self.__dict__)

    state.kb_obj = _KB(id="kb-1", name="old_name", tenant_id="tenant-1", pagerank=0, graphrag_task_id="", raptor_task_id="", mindmap_task_id="")

    class KnowledgebaseService:
        @staticmethod
        def create_with_name(**_kwargs):
            if state.kb_create_side_effect:
                return state.kb_create_side_effect.pop(0)
            return False, {"code": _RetCode.SUCCESS}

        @staticmethod
        def save(**_kwargs):
            if state.kb_save_side_effect:
                result = state.kb_save_side_effect.pop(0)
                if isinstance(result, Exception):
                    raise result
                return result
            return True

        @staticmethod
        def accessible4deletion(_kb_id, _user_id):
            return state.kb_accessible4deletion

        @staticmethod
        def accessible(_kb_id, _user_id):
            return state.kb_accessible

        @staticmethod
        def query(**kwargs):
            if state.kb_query_side_effect:
                return state.kb_query_side_effect(kwargs)
            if "created_by" in kwargs:
                return state.kb_query_owner_result
            if "name" in kwargs:
                return state.kb_query_duplicate_result
            return state.kb_query_result

        @staticmethod
        def get_by_id(_kb_id):
            if state.kb_get_by_id_side_effect:
                return state.kb_get_by_id_side_effect.pop(0)
            return True, state.kb_obj

        @staticmethod
        def get_detail(_kb_id):
            return state.kb_detail

        @staticmethod
        def get_by_tenant_ids(*_args, **_kwargs):
            if state.kb_get_by_tenant_ids_exc:
                raise state.kb_get_by_tenant_ids_exc
            return state.kb_get_by_tenant_ids_result

        @staticmethod
        def update_by_id(_kb_id, _payload):
            if state.kb_update_by_id_side_effect:
                return state.kb_update_by_id_side_effect.pop(0)
            return state.kb_update_by_id_result

        @staticmethod
        def delete_by_id(_kb_id):
            return state.kb_delete_by_id_result

    class DocumentService:
        @staticmethod
        def get_total_size_by_kb_id(**_kwargs):
            if state.kb_total_size_exc:
                raise state.kb_total_size_exc
            return state.kb_total_size

        @staticmethod
        def get_by_kb_id(**_kwargs):
            return state.docs_by_kb, state.docs_total

        @staticmethod
        def query(**_kwargs):
            return state.docs_query

        @staticmethod
        def remove_document(_doc, _tenant_id):
            if state.remove_document_side_effect:
                result = state.remove_document_side_effect.pop(0)
                if isinstance(result, Exception):
                    raise result
                return result
            return state.remove_document_result

        @staticmethod
        def knowledgebase_basic_info(_kb_id):
            return state.kb_basic_info

    def queue_raptor_o_graphrag_tasks(**_kwargs):
        return state.queue_task_id

    _install_stub(
        monkeypatch,
        "api.db.services.document_service",
        {"DocumentService": DocumentService, "queue_raptor_o_graphrag_tasks": queue_raptor_o_graphrag_tasks},
    )

    class Connector2KbService:
        @staticmethod
        def list_connectors(_kb_id):
            return state.connectors_list

        @staticmethod
        def link_connectors(_kb_id, _connectors, _user_id):
            return state.connector_link_errors

    _install_stub(monkeypatch, "api.db.services.connector_service", {"Connector2KbService": Connector2KbService})

    class DocMetadataService:
        @staticmethod
        def get_flatted_meta_by_kbs(_kb_ids):
            return state.meta_flat

    _install_stub(monkeypatch, "api.db.services.doc_metadata_service", {"DocMetadataService": DocMetadataService})

    class File2DocumentService:
        @staticmethod
        def get_by_document_id(_doc_id):
            return state.file2doc_result

        @staticmethod
        def delete_by_document_id(_doc_id):
            return None

    _install_stub(monkeypatch, "api.db.services.file2document_service", {"File2DocumentService": File2DocumentService})

    class FileService:
        @staticmethod
        def filter_update(filters, payload):
            state.file_service_updates.append((filters, payload))
            return True

        @staticmethod
        def filter_delete(filters):
            state.file_service_deletes.append(filters)
            return True

    _install_stub(monkeypatch, "api.db.services.file_service", {"FileService": FileService})

    class PipelineOperationLogService:
        @staticmethod
        def get_file_logs_by_kb_id(*_args, **_kwargs):
            if state.pipeline_logs_exc:
                raise state.pipeline_logs_exc
            return state.pipeline_logs_result

        @staticmethod
        def get_dataset_logs_by_kb_id(*_args, **_kwargs):
            if state.pipeline_dataset_logs_exc:
                raise state.pipeline_dataset_logs_exc
            return state.pipeline_dataset_logs_result

        @staticmethod
        def delete_by_ids(ids):
            state.pipeline_deleted_ids.extend(ids)
            return True

        @staticmethod
        def get_by_id(_log_id):
            return state.pipeline_log_get_result

    _install_stub(monkeypatch, "api.db.services.pipeline_operation_log_service", {"PipelineOperationLogService": PipelineOperationLogService})

    class _Task:
        def __init__(self, **kwargs):
            self.__dict__.update(kwargs)

        def to_dict(self):
            return dict(self.__dict__)

    class TaskService:
        @staticmethod
        def get_by_id(_task_id):
            return state.task_get_result

    _install_stub(monkeypatch, "api.db.services.task_service", {"TaskService": TaskService, "GRAPH_RAPTOR_FAKE_DOC_ID": "fake-doc"})

    class TenantService:
        @staticmethod
        def get_joined_tenants_by_user_id(_user_id):
            return state.joined_tenants

    class UserTenantService:
        @staticmethod
        def query(user_id=None, **_kwargs):
            return state.user_tenants

        @staticmethod
        def get_tenants_by_user_id(_user_id):
            return state.user_tenants_dicts

    _install_stub(monkeypatch, "api.db.services.user_service", {"TenantService": TenantService, "UserTenantService": UserTenantService})

    import logging as _logging

    _orig_error = _logging.error

    def _safe_error(msg, *args, **kwargs):
        try:
            return _orig_error(msg, *args, **kwargs)
        except TypeError:
            return _orig_error(f"{msg} {args}")

    monkeypatch.setattr(_logging, "error", _safe_error)

    async def _thread_pool_exec(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    _install_stub(monkeypatch, "common.misc_utils", {"thread_pool_exec": _thread_pool_exec})

    def _get_json_result(code=_RetCode.SUCCESS, message="success", data=None):
        payload = {"code": code, "message": message}
        if data is not None:
            payload["data"] = data
        return payload

    def _get_data_error_result(message=""):
        return {"code": _RetCode.DATA_ERROR, "message": message}

    def _get_error_data_result(message=""):
        return {"code": _RetCode.DATA_ERROR, "message": message}

    def _server_error_response(exc):
        return {"code": _RetCode.SERVER_ERROR, "message": str(exc)}

    async def _get_request_json():
        return state.json

    def _validate_request(*_args, **_kwargs):
        def decorator(func):
            return func
        return decorator

    def _not_allowed_parameters(*_args, **_kwargs):
        def decorator(func):
            return func
        return decorator

    _install_stub(
        monkeypatch,
        "api.utils.api_utils",
        {
            "get_json_result": _get_json_result,
            "get_data_error_result": _get_data_error_result,
            "get_error_data_result": _get_error_data_result,
            "server_error_response": _server_error_response,
            "validate_request": _validate_request,
            "not_allowed_parameters": _not_allowed_parameters,
            "get_request_json": _get_request_json,
        },
    )

    _install_stub(monkeypatch, "api.db.services.knowledgebase_service", {"KnowledgebaseService": KnowledgebaseService})

    class _Search:
        @staticmethod
        def index_name(tenant_id):
            return f"idx-{tenant_id}"

    _install_stub(monkeypatch, "rag.nlp", {"search": _Search})

    class _Retriever:
        def all_tags(self, _tenant_id, _kb_ids):
            return list(state.retriever_tags)

        async def search(self, *_args, **_kwargs):
            return state.retriever_search_result

    class _DocStore:
        def index_exist(self, *_args, **_kwargs):
            return state.doc_store_index_exists

        def search(self, *_args, **_kwargs):
            if state.doc_store_search_results:
                return state.doc_store_search_results.pop(0)
            return {}

        def get_total(self, res):
            if isinstance(res, dict) and "total" in res:
                return res["total"]
            return state.doc_store_total

        def get_doc_ids(self, res):
            if isinstance(res, dict):
                return res.get("ids", [])
            return []

        def get(self, cid, *_args, **_kwargs):
            return state.doc_store_docs.get(cid, {})

        def update(self, *args, **kwargs):
            state.doc_store_updated.append((args, kwargs))
            return True

        def delete(self, *args, **kwargs):
            state.doc_store_deleted.append((args, kwargs))
            return True

        def delete_idx(self, *args, **kwargs):
            state.doc_store_delete_idx.append((args, kwargs))
            return True

    class _Storage:
        def remove_bucket(self, _bucket):
            return True

    _install_stub(monkeypatch, "common.settings", {"docStoreConn": _DocStore(), "retriever": _Retriever(), "STORAGE_IMPL": _Storage(), "DOC_ENGINE_INFINITY": state.doc_engine_infinity})

    class _Redis:
        def set(self, key, value):
            state.redis_sets.append((key, value))
            return True

    _install_stub(monkeypatch, "rag.utils.redis_conn", {"REDIS_CONN": _Redis()})

    class LLMBundle:
        def __init__(self, *_args, **_kwargs):
            return None

        def encode(self, _inputs):
            return [[1.0], [1.0]], None

    _install_stub(monkeypatch, "api.db.services.llm_service", {"LLMBundle": LLMBundle})

    _install_stub(monkeypatch, "common.metadata_utils", {"turn2jsonschema": lambda metadata: {"schema": metadata}})

    _install_stub(
        monkeypatch,
        "api.apps",
        {
            "current_user": types.SimpleNamespace(id="user-id"),
            "login_required": lambda func: func,
            "manager": _DummyManager(),
        },
    )

    module_name = f"kb_app_test_{id(state)}"
    module = types.ModuleType(module_name)
    module.__file__ = str(file_path)
    module.__dict__["manager"] = _DummyManager()
    sys.modules[module_name] = module
    source = file_path.read_text(encoding="utf-8")
    exec(compile(source, str(file_path), "exec"), module.__dict__)
    return module, state


@pytest.fixture()
def kb_app(monkeypatch, tmp_path):
    return _load_kb_app(monkeypatch, tmp_path)
