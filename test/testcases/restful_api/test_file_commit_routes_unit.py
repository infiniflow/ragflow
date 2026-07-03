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
"""API-level integration test for file commit endpoints.

Uses an in-memory SQLite database so the real FileCommitService and
FileCommit/FileCommitItem models execute against real SQL — only the
HTTP layer (quart.request, login_required, current_user) and storage
are mocked.
"""

import asyncio
import functools
import importlib.util
import logging
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest
from peewee import SqliteDatabase, Model, CharField, IntegerField, BigIntegerField, TextField

LOGGER = logging.getLogger(__name__)


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


def _run(coro):
    return asyncio.run(coro)


# Shared mutable payload used by both get_request_json (stub) and
# _setup_request so validate_request's closure always sees the current value.
_request_payload: list = [{}]


# ── SQLite in-memory models ───────────────────────────────────────────────
# We create minimal Peewee models that match the real table schemas so
# FileCommitService (which uses DB.atomic(), .select(), .where(), etc.)
# works against real SQL.

sqlite_db = SqliteDatabase(":memory:")


class BaseTestModel(Model):
    class Meta:
        database = sqlite_db


class FileCommitTestModel(BaseTestModel):
    id = CharField(max_length=32, primary_key=True)
    folder_id = CharField(max_length=32, index=True)
    parent_id = CharField(max_length=32, null=True, index=True)
    message = CharField(max_length=512, default="")
    author_id = CharField(max_length=32, index=True)
    file_count = IntegerField(default=0)
    tree_state = TextField(null=True)
    create_time = BigIntegerField(null=True, index=True)
    create_date = CharField(null=True, max_length=32, index=True)
    update_time = BigIntegerField(null=True, index=True)
    update_date = CharField(null=True, max_length=32, index=True)

    class Meta:
        db_table = "file_commit"


class FileCommitItemTestModel(BaseTestModel):
    id = CharField(max_length=32, primary_key=True)
    commit_id = CharField(max_length=32, index=True)
    file_id = CharField(max_length=32, index=True)
    operation = CharField(max_length=16, index=True)
    old_hash = CharField(max_length=64, null=True, index=True)
    new_hash = CharField(max_length=64, null=True, index=True)
    old_location = CharField(max_length=255, null=True)
    new_location = CharField(max_length=255, null=True)
    old_name = CharField(max_length=255, null=True)
    new_name = CharField(max_length=255, null=True)
    create_time = BigIntegerField(null=True, index=True)
    create_date = CharField(null=True, max_length=32, index=True)
    update_time = BigIntegerField(null=True, index=True)
    update_date = CharField(null=True, max_length=32, index=True)

    class Meta:
        db_table = "file_commit_item"


class FileTestModel(BaseTestModel):
    id = CharField(max_length=32, primary_key=True)
    parent_id = CharField(max_length=32, index=True)
    tenant_id = CharField(max_length=32, index=True)
    created_by = CharField(max_length=32, index=True)
    name = CharField(max_length=255, index=True)
    location = CharField(max_length=255, null=True, index=True)
    size = BigIntegerField(default=0, index=True)
    type = CharField(max_length=32, index=True)
    source_type = CharField(max_length=128, default="", index=True)
    status = CharField(max_length=1, null=True, default="1", index=True)
    create_time = BigIntegerField(null=True, index=True)
    create_date = CharField(null=True, max_length=32, index=True)
    update_time = BigIntegerField(null=True, index=True)
    update_date = CharField(null=True, max_length=32, index=True)

    class Meta:
        db_table = "file"


class UserTestModel(BaseTestModel):
    id = CharField(max_length=32, primary_key=True)
    nickname = CharField(max_length=100, null=False, index=True)
    email = CharField(max_length=255, null=False)

    class Meta:
        db_table = "user"


_TABLES = [FileCommitTestModel, FileCommitItemTestModel, FileTestModel, UserTestModel]
sqlite_db.create_tables(_TABLES)


def _clear_db():
    """Delete all rows from every test table so each test starts clean."""
    for model in _TABLES:
        model.delete().execute()


# ── Module loader ─────────────────────────────────────────────────────────


def _load_module(monkeypatch):
    """Load file_commit_api.py with SQLite in-memory DB and mocked HTTP layer."""
    repo_root = Path(__file__).resolve().parents[3]

    # Stub: quart.request
    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args={}, content_type="application/json")
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    # Stub: api.apps with login_required / current_user
    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="test-user")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)
    api_pkg.apps = apps_mod

    # Stub: api.utils.api_utils
    api_utils_mod = ModuleType("api.utils.api_utils")

    def get_json_result(data=None, message="success", code=0):
        return {"code": code, "data": data, "message": message}

    def get_data_error_result(message=""):
        return {"code": 102, "data": None, "message": message}

    async def get_request_json():
        return _request_payload[0]

    def server_error_response(err):
        return {"code": 500, "data": None, "message": str(err)}

    def validate_request(*required_keys):
        def _decorator(func):
            @functools.wraps(func)
            async def _wrapper(*args, **kwargs):
                payload = await get_request_json()
                missing = [k for k in required_keys if k not in payload]
                if missing:
                    return get_json_result(code=101, data=None, message="required argument are missing: " + ", ".join(missing))
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
    import uuid

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: uuid.uuid1().hex
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    # Stub: common.settings (STORAGE_IMPL is a no-op for testing)
    common_mod = ModuleType("common")
    common_mod.__path__ = []
    common_mod.settings = SimpleNamespace(
        STORAGE_IMPL=SimpleNamespace(
            put=lambda *_a, **_kw: None,
            get=lambda *_a, **_kw: b"stub-content",
        ),
        DATABASE_TYPE="sqlite",
    )
    monkeypatch.setitem(sys.modules, "common", common_mod)

    # Stub: common.time_utils (monotonically increasing timestamps)
    _ts_iter = iter(range(1718200000000, 1718200000100))
    time_utils_mod = ModuleType("common.time_utils")
    time_utils_mod.current_timestamp = lambda: next(_ts_iter)
    time_utils_mod.datetime_format = lambda *_a, **__: "mock"
    monkeypatch.setitem(sys.modules, "common.time_utils", time_utils_mod)

    # Stub: api.db.db_models — inject SQLite DB and our test models
    db_models_mod = ModuleType("api.db.db_models")

    class _DB:
        """Drop-in replacement that wraps our SQLite DB with the same
        methods (connection_context, atomic) that CommonService expects."""

        @staticmethod
        def connection_context():
            def dec(func):
                return func

            return dec

        @staticmethod
        def atomic():
            class Ctx:
                def __enter__(self2):
                    return self2

                def __exit__(self2, *args):
                    pass

            return Ctx()

    db_models_mod.DB = _DB
    db_models_mod.FileCommit = FileCommitTestModel
    db_models_mod.FileCommitItem = FileCommitItemTestModel
    db_models_mod.File = FileTestModel
    db_models_mod.User = UserTestModel
    db_models_mod.DataBaseModel = BaseTestModel
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    class _StubFileService:
        model = FileTestModel  # class attribute, not staticmethod — code accesses FileService.model.update(...)

        @staticmethod
        def update_by_id(pid, data):
            return FileTestModel.update(data).where(FileTestModel.id == pid).execute()

        @staticmethod
        def get_by_id(pid):
            try:
                obj = FileTestModel.get_by_id(pid)
                return True, obj
            except Exception:
                return False, None

        @staticmethod
        def get_or_none(**kwargs):
            try:
                return FileTestModel.get(**kwargs)
            except Exception:
                return None

    class CommonServiceBase:
        model = None

        @classmethod
        def get_by_id(cls, pid):
            try:
                obj = cls.model.get_or_none(cls.model.id == pid)
                if obj:
                    return True, obj
            except Exception:
                pass
            return False, None

        @classmethod
        def query(cls, cols=None, reverse=None, order_by=None, **kwargs):
            q = cls.model.select()
            for f_n, f_v in kwargs.items():
                if f_v is not None and hasattr(cls.model, f_n):
                    q = q.where(getattr(cls.model, f_n) == f_v)
            return q

        @classmethod
        def update_by_id(cls, pid, data):
            return cls.model.update(data).where(cls.model.id == pid).execute()

        @classmethod
        def filter_update(cls, filters, update_data):
            return cls.model.update(update_data).where(*filters).execute()

    # Stub: common.constants with FileSource for resolver
    constants_mod = ModuleType("common.constants")
    constants_mod.FileSource = type("FileSource", (), {"KNOWLEDGEBASE": "knowledgebase"})
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    # Stub: api.db with real filesystem path so sub-packages can be discovered.
    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = [str(repo_root / "api" / "db")]
    db_pkg.UserTenantRole = type("UserTenantRole", (), {k: k for k in ("OWNER", "ADMIN", "NORMAL", "INVITE")})
    db_pkg.TenantPermission = type("TenantPermission", (), {"ME": "me", "TEAM": "team"})
    db_pkg.FileType = type("FileType", (), {"FOLDER": "folder", "DOC": "doc", "VISUAL": "visual", "AURAL": "aural", "VIRTUAL": "virtual", "PDF": "pdf", "OTHER": "other"})
    db_pkg.KNOWLEDGEBASE_FOLDER_NAME = ".knowledgebase"
    db_pkg.SKILLS_FOLDER_NAME = "skills"
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    # Stub api.db.services — prevent real __init__ from loading (avoids
    # importing every real service module).  Keep the filesystem path so
    # file_commit_service can be discovered, but pre-stub file_service
    # (which has heavy deps that would cascade-fail).
    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = [str(repo_root / "api" / "db" / "services")]
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    # Pre-stub service modules that file_commit_api.py imports.
    # Each stub prevents the real .py file from loading (and cascading deps).
    file_svc_mod = ModuleType("api.db.services.file_service")
    file_svc_mod.FileService = _StubFileService
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_svc_mod)

    common_svc_mod = ModuleType("api.db.services.common_service")
    common_svc_mod.CommonService = CommonServiceBase
    monkeypatch.setitem(sys.modules, "api.db.services.common_service", common_svc_mod)

    kb_svc_mod = ModuleType("api.db.services.knowledgebase_service")

    # NB: The dataset resolver in the API calls KnowledgebaseService.get_by_id
    # then accesses .name and .tenant_id.  We return a simple object.
    class _StubKnowledgebaseService:
        @staticmethod
        def get_by_id(dataset_id):
            if dataset_id == "ds-1":
                return True, SimpleNamespace(name="test-ds", tenant_id="t1")
            return False, None

    kb_svc_mod.KnowledgebaseService = _StubKnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_svc_mod)

    # Remove cached file_commit_service so it reimports with our SQLite stubs.
    # Keep api.db.db_models in sys.modules — it's already patched above.
    for mod_name in list(sys.modules.keys()):
        if mod_name.startswith("api.db.services.file_commit"):
            del sys.modules[mod_name]

    # Load the module
    module_name = "api.apps.restful_apis.file_commit_api"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "file_commit_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)

    return module


# ── Helpers ───────────────────────────────────────────────────────────────


def _setup_request(module, json_payload=None, args=None):
    """Set up a request payload and query args for the next handler call."""
    if json_payload is not None:
        _request_payload[0] = json_payload
    if args is not None:
        module.request.args = args


# ── Fixtures ──────────────────────────────────────────────────────────────


@pytest.fixture(scope="session")
def auth():
    return "test-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


@pytest.fixture(autouse=True)
def reset_db():
    """Clear all rows before each test to prevent order-dependent failures."""
    _clear_db()


# ── Tests ─────────────────────────────────────────────────────────────────


@pytest.mark.p2
def test_create_commit_success(monkeypatch):
    module = _load_module(monkeypatch)
    # Seed a file
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")

    _setup_request(
        module,
        json_payload={
            "message": "initial commit",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "hello"}],
        },
    )

    res = _run(module.create_commit("root-folder"))
    assert res["code"] == 0, f"Expected 0, got {res}"
    data = res["data"]
    assert data["message"] == "initial commit"
    assert data["folder_id"] == "root-folder"
    assert data["author_id"] == "test-user"
    assert data["file_count"] == 1
    assert data["tree_state"] is not None
    assert data["id"] is not None


@pytest.mark.p2
def test_create_commit_missing_fields(monkeypatch):
    module = _load_module(monkeypatch)
    _setup_request(module, json_payload={"message": "no files"})

    res = _run(module.create_commit("root-folder"))
    assert res["code"] == 101, f"Expected validation error, got {res}"


@pytest.mark.p2
def test_create_commit_modify_and_add(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")
    FileTestModel.create(id="f2", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="b.txt", type="txt")

    # Commit 1: add f1
    _setup_request(
        module,
        json_payload={
            "message": "c1",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "v1"}],
        },
    )
    _run(module.create_commit("root-folder"))

    # Commit 2: modify f1, add f2
    _setup_request(
        module,
        json_payload={
            "message": "c2",
            "files": [
                {"file_id": "f1", "file_name": "a.txt", "operation": "modify", "content": "v2"},
                {"file_id": "f2", "file_name": "b.txt", "operation": "add", "content": "world"},
            ],
        },
    )
    res = _run(module.create_commit("root-folder"))
    assert res["code"] == 0
    assert res["data"]["file_count"] == 2


@pytest.mark.p2
def test_create_commit_delete(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")

    # Add then delete
    _setup_request(
        module,
        json_payload={
            "message": "add",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "hello"}],
        },
    )
    _run(module.create_commit("root-folder"))

    _setup_request(
        module,
        json_payload={
            "message": "delete",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "delete"}],
        },
    )
    res = _run(module.create_commit("root-folder"))
    assert res["code"] == 0


@pytest.mark.p2
def test_create_commit_rename(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="old.txt", type="txt")

    _setup_request(
        module,
        json_payload={
            "message": "add",
            "files": [{"file_id": "f1", "file_name": "old.txt", "operation": "add", "content": "data"}],
        },
    )
    _run(module.create_commit("root-folder"))

    # Rename
    _setup_request(
        module,
        json_payload={
            "message": "rename",
            "files": [{"file_id": "f1", "file_name": "old.txt", "operation": "rename", "old_name": "old.txt", "new_name": "new.txt"}],
        },
    )
    res = _run(module.create_commit("root-folder"))
    assert res["code"] == 0


@pytest.mark.p2
def test_list_commits_success(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")

    # Create 2 commits
    _setup_request(
        module,
        json_payload={
            "message": "c1",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "v1"}],
        },
    )
    _run(module.create_commit("root-folder"))

    _setup_request(
        module,
        json_payload={
            "message": "c2",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "modify", "content": "v2"}],
        },
    )
    _run(module.create_commit("root-folder"))

    # List
    module.request.args = {"page": "1", "page_size": "10"}
    res = _run(module.list_commits("root-folder"))
    assert res["code"] == 0
    assert res["data"]["total"] == 2
    assert len(res["data"]["commits"]) == 2


@pytest.mark.p2
def test_get_commit_detail(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")

    _setup_request(
        module,
        json_payload={
            "message": "detail test",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "data"}],
        },
    )
    create_res = _run(module.create_commit("root-folder"))
    commit_id = create_res["data"]["id"]

    res = _run(module.get_commit("root-folder", commit_id))
    assert res["code"] == 0
    assert res["data"]["id"] == commit_id
    assert res["data"]["message"] == "detail test"
    assert len(res["data"]["files"]) == 1


@pytest.mark.p2
def test_get_commit_not_found(monkeypatch):
    module = _load_module(monkeypatch)
    res = _run(module.get_commit("root-folder", "nonexistent"))
    assert res["code"] == 102
    assert "not found" in res["message"].lower()


@pytest.mark.p2
def test_diff_commits(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")
    FileTestModel.create(id="f2", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="b.txt", type="txt")

    # c1: add f1
    _setup_request(
        module,
        json_payload={
            "message": "c1",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "v1"}],
        },
    )
    c1 = _run(module.create_commit("root-folder"))["data"]["id"]

    # c2: add f2, modify f1
    _setup_request(
        module,
        json_payload={
            "message": "c2",
            "files": [
                {"file_id": "f2", "file_name": "b.txt", "operation": "add", "content": "world"},
                {"file_id": "f1", "file_name": "a.txt", "operation": "modify", "content": "v2"},
            ],
        },
    )
    c2 = _run(module.create_commit("root-folder"))["data"]["id"]
    assert c1 != c2, "c1 and c2 must have different IDs"

    module.request.args = {"from": c1, "to": c2}
    res = _run(module.diff_commits("root-folder"))
    assert res["code"] == 0, f"diff failed: {res}"
    assert len(res["data"]) == 2, f"Expected 2 diff entries, got {len(res['data'])}: {res['data']}"

    # Verify f2 was added (present in c2 but not in c1)
    f2_entries = [e for e in res["data"] if e["file_id"] == "f2"]
    assert len(f2_entries) == 1
    assert f2_entries[0]["operation"] == "add"

    # Verify f1 was modified (hash changed from v1 to v2)
    f1_entries = [e for e in res["data"] if e["file_id"] == "f1"]
    assert len(f1_entries) == 1
    assert f1_entries[0]["operation"] == "modify"
    assert f1_entries[0]["old_hash"] != f1_entries[0]["new_hash"]


@pytest.mark.p2
def test_diff_commits_missing_params(monkeypatch):
    module = _load_module(monkeypatch)
    module.request.args = {}
    res = _run(module.diff_commits("root-folder"))
    assert res["code"] == 102


@pytest.mark.p2
def test_get_uncommitted_changes(monkeypatch):
    module = _load_module(monkeypatch)
    # Seed a file that will be committed
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")
    # Seed a file that will NOT be committed (uncommitted add)
    FileTestModel.create(id="f2", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="b.txt", type="txt")

    # Commit only f1
    _setup_request(
        module,
        json_payload={
            "message": "add f1",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "hello"}],
        },
    )
    _run(module.create_commit("root-folder"))

    res = _run(module.get_uncommitted_changes("root-folder"))
    assert res["code"] == 0
    # f2 should appear as uncommitted "add"
    f2_changes = [c for c in res["data"] if c["file_id"] == "f2"]
    assert len(f2_changes) > 0, "Expected f2 to show as uncommitted change"
    assert f2_changes[0]["operation"] == "add"


@pytest.mark.p2
def test_get_commit_tree(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")

    _setup_request(
        module,
        json_payload={
            "message": "c1",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "data"}],
        },
    )
    create_res = _run(module.create_commit("root-folder"))
    commit_id = create_res["data"]["id"]

    res = _run(module.get_commit_tree("root-folder", commit_id))
    assert res["code"] == 0
    assert res["data"]["type"] == "folder"
    assert res["data"]["id"] == "root-folder"
    assert any(c["id"] == "f1" for c in res["data"].get("children", []))


@pytest.mark.p2
def test_get_commit_file_content(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")

    _setup_request(
        module,
        json_payload={
            "message": "c1",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "hello world"}],
        },
    )
    create_res = _run(module.create_commit("root-folder"))
    commit_id = create_res["data"]["id"]

    res = _run(module.get_commit_file_content("root-folder", commit_id, "f1"))
    assert res["code"] == 0
    assert "content" in res["data"]


@pytest.mark.p2
def test_get_file_version_history(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="root-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")

    # Two commits modifying f1
    _setup_request(
        module,
        json_payload={
            "message": "v1",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "v1"}],
        },
    )
    _run(module.create_commit("root-folder"))

    _setup_request(
        module,
        json_payload={
            "message": "v2",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "modify", "content": "v2"}],
        },
    )
    _run(module.create_commit("root-folder"))

    res = _run(module.get_file_version_history("f1"))
    assert res["code"] == 0
    assert len(res["data"]) == 2


@pytest.mark.p2
def test_workspace_alias(monkeypatch):
    """Verify /workspace/ alias routes work the same as /folders/."""
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="ws-folder", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")

    _setup_request(
        module,
        json_payload={
            "message": "workspace commit",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "data"}],
        },
    )
    res = _run(module.create_commit("ws-folder"))
    assert res["code"] == 0

    # List via workspace alias
    module.request.args = {"page": "1", "page_size": "10"}
    res = _run(module.list_commits("ws-folder"))
    assert res["code"] == 0
    assert res["data"]["total"] == 1


@pytest.mark.p2
def test_get_commit_wrong_folder_returns_not_found(monkeypatch):
    module = _load_module(monkeypatch)
    FileTestModel.create(id="f1", parent_id="folder-a", tenant_id="t1", created_by="test-user", name="a.txt", type="txt")

    _setup_request(
        module,
        json_payload={
            "message": "c1",
            "files": [{"file_id": "f1", "file_name": "a.txt", "operation": "add", "content": "data"}],
        },
    )
    create_res = _run(module.create_commit("folder-a"))
    commit_id = create_res["data"]["id"]

    # Attempt to read commit from a different folder
    res = _run(module.get_commit("folder-b", commit_id))
    assert res["code"] == 102
    assert "not found in workspace" in res["message"].lower()
