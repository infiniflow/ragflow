#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import asyncio
import functools
import importlib.util
import logging
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

LOGGER = logging.getLogger(__name__)


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func
        return decorator


def _run(coro):
    return asyncio.run(coro)


def _load_file_commit_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[3]

    # Stub: quart
    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(
        args={},
        content_type="application/json",
    )
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    # Stub: api.apps
    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    # Stub: api.db
    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    # Stub: api.utils.api_utils
    api_utils_mod = ModuleType("api.utils.api_utils")

    def get_json_result(data=None, message="success", code=0):
        return {"code": code, "data": data, "message": message}

    def get_data_error_result(message=""):
        return {"code": 102, "data": None, "message": message}

    async def get_request_json():
        return {}

    def server_error_response(err):
        return {"code": 500, "data": None, "message": str(err)}

    def validate_request(*required_keys):
        def _decorator(func):
            @functools.wraps(func)
            async def _wrapper(*args, **kwargs):
                payload = await get_request_json()
                missing = [k for k in required_keys if k not in payload]
                if missing:
                    return get_json_result(
                        code=101, data=None,
                        message="required argument are missing: " + ", ".join(missing)
                    )
                return await func(*args, **kwargs)
            return _wrapper
        return _decorator

    api_utils_mod.get_json_result = get_json_result
    api_utils_mod.get_data_error_result = get_data_error_result
    api_utils_mod.get_request_json = get_request_json
    api_utils_mod.server_error_response = server_error_response
    api_utils_mod.validate_request = validate_request
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    # Stub: common.misc_utils
    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "uuid"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    # Stub: common.settings
    common_mod = ModuleType("common")
    common_mod.__path__ = []
    common_mod.settings = SimpleNamespace(
        STORAGE_IMPL=SimpleNamespace(
            put=lambda *_args, **_kwargs: None,
            get=lambda *_args, **_kwargs: b"stub",
        ),
        DATABASE_TYPE="mysql",
    )
    monkeypatch.setitem(sys.modules, "common", common_mod)

    # Stub: common.time_utils
    time_utils_mod = ModuleType("common.time_utils")
    time_utils_mod.current_timestamp = lambda: 1718200000000
    time_utils_mod.datetime_format = lambda *_args, **__: "mock"
    monkeypatch.setitem(sys.modules, "common.time_utils", time_utils_mod)

    # Stub: api.db.db_models
    db_models_mod = ModuleType("api.db.db_models")

    class FakeField:
        def __init__(self, name):
            self.name = name

    class _FakeModelMeta(type):
        def __getattr__(cls, name):
            return FakeField(name)

    class _FakeModel(metaclass=_FakeModelMeta):
        id = "mock-id"
        folder_id = ""
        parent_id = None
        message = ""
        author_id = ""
        file_count = 0
        tree_state = None
        create_time = 1718200000000
        create_date = None
        update_time = None
        update_date = None

        def __init__(self, **kwargs):
            for k, v in kwargs.items():
                setattr(self, k, v)

        def save(self, force_insert=False):
            return self

        @classmethod
        def get_by_id(cls, pid):
            obj = cls()
            obj.id = pid
            obj.tree_state = '{"file-1": {"hash": "abc", "location": ".objects/abc", "name": "file.txt", "size": 10, "status": "1"}}'
            return obj

        @classmethod
        def select(cls):
            return _FakeQuery(cls)

        @classmethod
        def update(cls, data):
            return _FakeUpdate(cls, data)

        @classmethod
        def table_exists(cls):
            return True

    class _FakeQuery:
        def __init__(self, model_cls, where_conds=None):
            self._model_cls = model_cls
            self._where = where_conds or []
            self._order = None
            self._offset_val = None
            self._limit_val = None

        def where(self, *args, **kwargs):
            return _FakeQuery(self._model_cls, self._where + list(args))

        def order_by(self, *args, **kwargs):
            q = _FakeQuery(self._model_cls, self._where)
            q._order = args
            return q

        def desc(self):
            return self

        def asc(self):
            return self

        def offset(self, n):
            self._offset_val = n
            return self

        def limit(self, n):
            self._limit_val = n
            return self

        def count(self):
            return 2

        def first(self):
            obj = self._model_cls()
            obj.id = "commit-latest"
            obj.tree_state = '{"file-1": {"hash": "old", "location": ".objects/old", "name": "file.txt", "size": 5, "status": "1"}}'
            return obj

        def execute(self):
            return 1

    class _FakeUpdate:
        def __init__(self, model_cls, data):
            self._model_cls = model_cls
            self._data = data

        def where(self, *args, **kwargs):
            return self

        def execute(self):
            return 1

    class FakeFileCommit(_FakeModel):
        pass

    class FakeFileCommitItem(_FakeModel):
        pass

    class FakeFile(_FakeModel):
        location = ".objects/old"
        name = "file.txt"

    db_models_mod.FileCommit = FakeFileCommit
    db_models_mod.FileCommitItem = FakeFileCommitItem
    db_models_mod.File = FakeFile

    class _DB:
        @staticmethod
        def connection_context():
            def dec(func):
                return func
            return dec

        @staticmethod
        def atomic():
            class _Ctx:
                def __enter__(self):
                    return self
                def __exit__(self, *args):
                    pass
            return _Ctx()

    db_models_mod.DB = _DB
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    # Stub: api.db.services.file_service
    file_svc_mod = ModuleType("api.db.services.file_service")
    file_svc_mod.FileService = SimpleNamespace(
        update_by_id=lambda *_args, **_kwargs: 1,
        get_by_id=lambda fid: (True, FakeFile()),
        get_or_none=lambda **kw: None,
        model=SimpleNamespace(update=lambda d: SimpleNamespace(where=lambda *a: SimpleNamespace(execute=lambda: 1))),
    )
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_svc_mod)

    # Stub: api.db.services.FileCommitService (full module import)
    svc_pkg = ModuleType("api.db.services")
    svc_pkg.__path__ = [str(repo_root / "api" / "db" / "services")]
    monkeypatch.setitem(sys.modules, "api.db.services", svc_pkg)

    # Load the actual module
    module_name = "api.apps.restful_apis.file_commit_api"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "file_commit_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


# ── Tests ──────────────────────────────────────────────────────────────────


@pytest.mark.p2
def test_create_commit_success(monkeypatch):
    module = _load_file_commit_module(monkeypatch)
    req_payload = {
        "message": "initial commit",
        "files": [
            {"file_id": "file-1", "file_name": "readme.md", "operation": "add", "content": "# Hello"}
        ],
    }
    monkeypatch.setattr(module, "get_request_json", _mk_req_maker(req_payload))

    res = _run(module.create_commit("folder-1"))
    assert res["code"] == 0
    assert res["data"]["message"] == "initial commit"


@pytest.mark.p2
def test_create_commit_missing_fields(monkeypatch):
    module = _load_file_commit_module(monkeypatch)
    monkeypatch.setattr(module, "get_request_json", _mk_req_maker({"message": "no files"}))

    res = _run(module.create_commit("folder-1"))
    assert res["code"] == 101


@pytest.mark.p2
def test_list_commits_success(monkeypatch):
    module = _load_file_commit_module(monkeypatch)
    monkeypatch.setattr(module, "request", SimpleNamespace(
        args={"page": "1", "page_size": "10", "order_by": "create_time", "desc": "true"},
        content_type="application/json",
    ))

    res = _run(module.list_commits("folder-1"))
    assert res["code"] == 0
    assert res["data"]["total"] == 2
    assert len(res["data"]["commits"]) > 0


@pytest.mark.p2
def test_get_commit_found(monkeypatch):
    module = _load_file_commit_module(monkeypatch)

    res = _run(module.get_commit("folder-1", "commit-1"))
    assert res["code"] == 0
    assert res["data"]["id"] == "commit-1"


@pytest.mark.p2
def test_get_commit_not_found(monkeypatch):
    module = _load_file_commit_module(monkeypatch)
    # Force get_by_id to return False
    monkeypatch.setattr(module.FileCommitService, "get_commit", lambda cid: None)

    res = _run(module.get_commit("folder-1", "missing"))
    assert res["code"] == 102
    assert "not found" in res["message"]


@pytest.mark.p2
def test_list_commit_files_success(monkeypatch):
    module = _load_file_commit_module(monkeypatch)

    res = _run(module.list_commit_files("folder-1", "commit-1"))
    assert res["code"] == 0


@pytest.mark.p2
def test_diff_commits_success(monkeypatch):
    module = _load_file_commit_module(monkeypatch)
    monkeypatch.setattr(module, "request", SimpleNamespace(
        args={"from": "commit-1", "to": "commit-2"},
    ))

    res = _run(module.diff_commits("folder-1"))
    assert res["code"] == 0


@pytest.mark.p2
def test_diff_commits_missing_params(monkeypatch):
    module = _load_file_commit_module(monkeypatch)
    monkeypatch.setattr(module, "request", SimpleNamespace(args={}))

    res = _run(module.diff_commits("folder-1"))
    assert res["code"] == 102
    assert "from" in res["message"]


@pytest.mark.p2
def test_get_uncommitted_changes_success(monkeypatch):
    module = _load_file_commit_module(monkeypatch)

    res = _run(module.get_uncommitted_changes("folder-1"))
    assert res["code"] == 0


@pytest.mark.p2
def test_get_commit_tree_success(monkeypatch):
    module = _load_file_commit_module(monkeypatch)

    res = _run(module.get_commit_tree("folder-1", "commit-1"))
    assert res["code"] == 0


@pytest.mark.p2
def test_get_commit_file_content_success(monkeypatch):
    module = _load_file_commit_module(monkeypatch)

    res = _run(module.get_commit_file_content("folder-1", "commit-1", "file-1"))
    assert res["code"] == 0
    assert res["data"]["content"] is not None


@pytest.mark.p2
def test_file_version_history_success(monkeypatch):
    module = _load_file_commit_module(monkeypatch)

    res = _run(module.get_file_version_history("file-1"))
    assert res["code"] == 0


def _mk_req_maker(payload):
    async def _inner():
        return payload
    return _inner
