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
import sys
from enum import Enum
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


class _DummyUploadFile:
    def __init__(self, filename, blob=b"blob"):
        self.filename = filename
        self._blob = blob

    def read(self):
        return self._blob


class _DummyFile:
    def __init__(
        self,
        file_id,
        file_type,
        *,
        tenant_id="tenant1",
        parent_id="pf1",
        location="loc1",
        name="doc.txt",
        source_type="user",
        size=1,
    ):
        self.id = file_id
        self.type = file_type
        self.tenant_id = tenant_id
        self.parent_id = parent_id
        self.location = location
        self.name = name
        self.source_type = source_type
        self.size = size

    def to_json(self):
        return {"id": self.id, "name": self.name, "type": self.type}


def _run(coro):
    return asyncio.run(coro)


def _load_file_api_service(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    common_pkg = ModuleType("api.common")
    common_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.common", common_pkg)

    permission_mod = ModuleType("api.common.check_team_permission")
    permission_mod.check_file_team_permission = lambda *_args, **_kwargs: True
    monkeypatch.setitem(sys.modules, "api.common.check_team_permission", permission_mod)
    common_pkg.check_team_permission = permission_mod

    db_pkg = ModuleType("api.db")
    db_pkg.__path__ = []

    class _FileType(Enum):
        FOLDER = "folder"
        VIRTUAL = "virtual"
        DOC = "doc"
        VISUAL = "visual"

    db_pkg.FileType = _FileType
    monkeypatch.setitem(sys.modules, "api.db", db_pkg)
    api_pkg.db = db_pkg

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    services_pkg.duplicate_name = lambda _query, **kwargs: kwargs.get("name", "")
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    document_service_mod = ModuleType("api.db.services.document_service")
    document_service_mod.DocumentService = SimpleNamespace(
        get_doc_count=lambda _uid: 0,
        get_by_id=lambda doc_id: (True, SimpleNamespace(id=doc_id)),
        get_tenant_id=lambda _doc_id: "tenant1",
        remove_document=lambda *_args, **_kwargs: True,
        update_by_id=lambda *_args, **_kwargs: True,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)
    services_pkg.document_service = document_service_mod

    file2doc_mod = ModuleType("api.db.services.file2document_service")
    file2doc_mod.File2DocumentService = SimpleNamespace(
        get_by_file_id=lambda _file_id: [],
        delete_by_file_id=lambda _file_id: None,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2doc_mod)
    services_pkg.file2document_service = file2doc_mod

    file_service_mod = ModuleType("api.db.services.file_service")
    file_service_mod.FileService = SimpleNamespace(
        get_root_folder=lambda _tenant_id: {"id": "root"},
        get_by_id=lambda file_id: (True, _DummyFile(file_id, _FileType.DOC.value)),
        get_id_list_by_id=lambda _pf_id, _names, _idx, ids: ids,
        create_folder=lambda _file, parent_id, _names, _len_id: SimpleNamespace(id=parent_id, name=str(parent_id)),
        query=lambda **_kwargs: [],
        insert=lambda data: SimpleNamespace(to_json=lambda: data, **data),
        is_parent_folder_exist=lambda _pf_id: True,
        get_by_pf_id=lambda *_args, **_kwargs: ([], 0),
        get_parent_folder=lambda _file_id: SimpleNamespace(to_json=lambda: {"id": "root"}),
        get_all_parent_folders=lambda _file_id: [],
        list_all_files_by_parent_id=lambda _parent_id: [],
        delete=lambda _file: True,
        delete_by_id=lambda _file_id: True,
        update_by_id=lambda *_args, **_kwargs: True,
        get_by_ids=lambda file_ids: [_DummyFile(file_id, _FileType.DOC.value) for file_id in file_ids],
    )
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)
    services_pkg.file_service = file_service_mod

    file_utils_mod = ModuleType("api.utils.file_utils")
    file_utils_mod.filename_type = lambda _filename: _FileType.DOC.value
    monkeypatch.setitem(sys.modules, "api.utils.file_utils", file_utils_mod)

    common_root_mod = ModuleType("common")
    common_root_mod.__path__ = [str(repo_root / "common")]
    common_root_mod.settings = SimpleNamespace(
        STORAGE_IMPL=SimpleNamespace(
            obj_exist=lambda *_args, **_kwargs: False,
            put=lambda *_args, **_kwargs: None,
            rm=lambda *_args, **_kwargs: None,
            move=lambda *_args, **_kwargs: None,
        )
    )
    monkeypatch.setitem(sys.modules, "common", common_root_mod)

    constants_mod = ModuleType("common.constants")

    class _FileSource:
        KNOWLEDGEBASE = "knowledgebase"

    constants_mod.FileSource = _FileSource
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "uuid-1"

    async def thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    misc_utils_mod.thread_pool_exec = thread_pool_exec
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    module_path = repo_root / "api" / "apps" / "services" / "file_api_service.py"
    spec = importlib.util.spec_from_file_location("api.apps.services.file_api_service", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "api.apps.services.file_api_service", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_upload_file_requires_existing_folder(monkeypatch):
    module = _load_file_api_service(monkeypatch)
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (False, None))

    ok, message = _run(module.upload_file("tenant1", "pf1", [_DummyUploadFile("a.txt")]))
    assert ok is False
    assert message == "Can't find this folder!"


@pytest.mark.p2
def test_upload_file_respects_user_limit(monkeypatch):
    module = _load_file_api_service(monkeypatch)
    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, SimpleNamespace(id="pf1", name="pf1")))
    monkeypatch.setattr(module.DocumentService, "get_doc_count", lambda _uid: 1)
    monkeypatch.setenv("MAX_FILE_NUM_PER_USER", "1")

    ok, message = _run(module.upload_file("tenant1", "pf1", [_DummyUploadFile("a.txt")]))
    assert ok is False
    assert message == "Exceed the maximum file number of a free user!"
    monkeypatch.delenv("MAX_FILE_NUM_PER_USER", raising=False)


@pytest.mark.p2
def test_upload_file_success_uses_new_service_layer(monkeypatch):
    module = _load_file_api_service(monkeypatch)
    storage_puts = []

    monkeypatch.setattr(module.FileService, "get_by_id", lambda _file_id: (True, SimpleNamespace(id="pf1", name="pf1")))
    monkeypatch.setattr(module.FileService, "get_id_list_by_id", lambda *_args, **_kwargs: ["pf1"])
    monkeypatch.setattr(
        module.FileService,
        "create_folder",
        lambda _file, parent_id, _names, _len_id: SimpleNamespace(id=parent_id),
    )
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", SimpleNamespace(
        obj_exist=lambda *_args, **_kwargs: False,
        put=lambda bucket, location, blob: storage_puts.append((bucket, location, blob)),
        rm=lambda *_args, **_kwargs: None,
        move=lambda *_args, **_kwargs: None,
    ))

    ok, data = _run(module.upload_file("tenant1", "pf1", [_DummyUploadFile("a.txt", b"hello")]))
    assert ok is True
    assert data[0]["name"] == "a.txt"
    assert storage_puts == [("pf1", "a.txt", b"hello")]


@pytest.mark.p2
def test_create_folder_rejects_duplicate_name(monkeypatch):
    module = _load_file_api_service(monkeypatch)
    monkeypatch.setattr(module.FileService, "query", lambda **_kwargs: [SimpleNamespace(id="existing")])

    ok, message = _run(module.create_folder("tenant1", "dup", "pf1", module.FileType.FOLDER.value))
    assert ok is False
    assert message == "Duplicated folder name in the same folder."


@pytest.mark.p2
def test_delete_files_checks_team_permission(monkeypatch):
    module = _load_file_api_service(monkeypatch)
    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda _file_id: (True, _DummyFile("file1", module.FileType.DOC.value)),
    )
    monkeypatch.setattr(module, "check_file_team_permission", lambda *_args, **_kwargs: False)

    ok, message = _run(module.delete_files("tenant1", ["file1"]))
    assert ok is False
    assert message == "No authorization."


@pytest.mark.p2
def test_move_files_rejects_extension_change_in_new_name(monkeypatch):
    module = _load_file_api_service(monkeypatch)
    monkeypatch.setattr(
        module.FileService,
        "get_by_ids",
        lambda _ids: [_DummyFile("file1", module.FileType.DOC.value, name="a.txt")],
    )

    ok, message = _run(module.move_files("tenant1", ["file1"], new_name="a.pdf"))
    assert ok is False
    assert message == "The extension of file can't be changed"


@pytest.mark.p2
def test_move_files_handles_dest_and_storage_move(monkeypatch):
    module = _load_file_api_service(monkeypatch)
    moved = []
    updated = []

    monkeypatch.setattr(
        module.FileService,
        "get_by_id",
        lambda file_id: (False, None) if file_id == "missing" else (True, _DummyFile(file_id, module.FileType.FOLDER.value, name="dest")),
    )
    monkeypatch.setattr(
        module.FileService,
        "get_by_ids",
        lambda _ids: [_DummyFile("file1", module.FileType.DOC.value, parent_id="src", location="old", name="a.txt")],
    )
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", SimpleNamespace(
        obj_exist=lambda *_args, **_kwargs: False,
        put=lambda *_args, **_kwargs: None,
        rm=lambda *_args, **_kwargs: None,
        move=lambda old_bucket, old_loc, new_bucket, new_loc: moved.append((old_bucket, old_loc, new_bucket, new_loc)),
    ))
    monkeypatch.setattr(module.FileService, "update_by_id", lambda file_id, data: updated.append((file_id, data)) or True)

    ok, message = _run(module.move_files("tenant1", ["file1"], "missing"))
    assert ok is False
    assert message == "Parent folder not found!"

    ok, data = _run(module.move_files("tenant1", ["file1"], "dest"))
    assert ok is True
    assert data is True
    assert moved == [("src", "old", "dest", "a.txt")]
    assert updated == [("file1", {"parent_id": "dest", "location": "a.txt"})]


@pytest.mark.p2
def test_move_files_renames_in_place_without_storage_move(monkeypatch):
    module = _load_file_api_service(monkeypatch)
    db_updates = []
    doc_updates = []

    monkeypatch.setattr(
        module.FileService,
        "get_by_ids",
        lambda _ids: [_DummyFile("file1", module.FileType.DOC.value, parent_id="pf1", name="a.txt")],
    )
    monkeypatch.setattr(module.FileService, "update_by_id", lambda file_id, data: db_updates.append((file_id, data)) or True)
    monkeypatch.setattr(
        module.File2DocumentService,
        "get_by_file_id",
        lambda _file_id: [SimpleNamespace(document_id="doc1")],
    )
    monkeypatch.setattr(module.DocumentService, "update_by_id", lambda doc_id, data: doc_updates.append((doc_id, data)) or True)

    ok, data = _run(module.move_files("tenant1", ["file1"], new_name="b.txt"))
    assert ok is True
    assert data is True
    assert db_updates == [("file1", {"name": "b.txt"})]
    assert doc_updates == [("doc1", {"name": "b.txt"})]


@pytest.mark.p2
def test_get_file_content_checks_permission(monkeypatch):
    module = _load_file_api_service(monkeypatch)
    monkeypatch.setattr(module, "check_file_team_permission", lambda *_args, **_kwargs: False)

    ok, message = module.get_file_content("tenant1", "file1")
    assert ok is False
    assert message == "No authorization."

    monkeypatch.setattr(module, "check_file_team_permission", lambda *_args, **_kwargs: True)
    ok, file = module.get_file_content("tenant1", "file1")
    assert ok is True
    assert file.id == "file1"
