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
import importlib.util
import sys
from datetime import datetime, timezone
from pathlib import Path
from types import ModuleType, SimpleNamespace


def _load_dropbox_connector_module():
    repo_root = Path(__file__).resolve().parents[3]
    package_name = "common.data_source"
    saved_modules = {name: module for name, module in sys.modules.items() if name == package_name or name.startswith(f"{package_name}.")}
    package_stub = ModuleType(package_name)
    package_stub.__path__ = [str(repo_root / "common" / "data_source")]
    sys.modules[package_name] = package_stub

    try:
        spec = importlib.util.spec_from_file_location(
            "_dropbox_connector_under_test",
            repo_root / "common" / "data_source" / "dropbox_connector.py",
        )
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module
    finally:
        for name in list(sys.modules):
            if name == package_name or name.startswith(f"{package_name}."):
                if name in saved_modules:
                    sys.modules[name] = saved_modules[name]
                else:
                    sys.modules.pop(name, None)


dropbox_connector = _load_dropbox_connector_module()
DropboxConnector = dropbox_connector.DropboxConnector


class _FakeFileMetadata:
    def __init__(self, file_id: str, name: str, path: str, client_modified: datetime, size: int = 10) -> None:
        self.id = file_id
        self.name = name
        self.path_display = path
        self.path_lower = path.lower()
        self.client_modified = client_modified
        self.size = size


class _FakeFolderMetadata:
    def __init__(self, name: str, path: str) -> None:
        self.name = name
        self.path_display = path
        self.path_lower = path.lower()


class _FakeListResult:
    def __init__(self, entries: list, cursor: str = "", has_more: bool = False) -> None:
        self.entries = entries
        self.cursor = cursor
        self.has_more = has_more


class _FakeDropboxClient:
    def __init__(self) -> None:
        self.downloaded_paths: list[str] = []
        self.root_file = _FakeFileMetadata(
            "id-root",
            "same.txt",
            "/same.txt",
            datetime(2026, 1, 1, 12, tzinfo=timezone.utc),
        )
        self.nested_file = _FakeFileMetadata(
            "id-nested",
            "same.txt",
            "/folder/same.txt",
            datetime(2026, 1, 1, 13, tzinfo=timezone.utc),
        )
        self.paged_file = _FakeFileMetadata(
            "id-paged",
            "unique.pdf",
            "/unique.pdf",
            datetime(2026, 1, 1, 14, tzinfo=timezone.utc),
        )

    def files_list_folder(self, path: str, **_kwargs):
        if path == "":
            return _FakeListResult(
                [self.root_file, _FakeFolderMetadata("folder", "/folder")],
                cursor="cursor-1",
                has_more=True,
            )
        if path == "/folder":
            return _FakeListResult([self.nested_file])
        raise AssertionError(f"unexpected Dropbox folder path: {path}")

    def files_list_folder_continue(self, cursor: str):
        assert cursor == "cursor-1"
        return _FakeListResult([self.paged_file])

    def files_download(self, path: str):
        self.downloaded_paths.append(path)
        return None, SimpleNamespace(content=f"content:{path}".encode())


def test_retrieve_all_slim_docs_perm_sync_lists_current_file_ids_without_downloads(monkeypatch):
    monkeypatch.setattr(dropbox_connector, "FileMetadata", _FakeFileMetadata)
    monkeypatch.setattr(dropbox_connector, "FolderMetadata", _FakeFolderMetadata)
    connector = DropboxConnector(batch_size=2)
    fake_client = _FakeDropboxClient()
    connector.dropbox_client = fake_client

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [[doc.id for doc in batch] for batch in batches] == [
        ["dropbox:id-root", "dropbox:id-nested"],
        ["dropbox:id-paged"],
    ]
    assert fake_client.downloaded_paths == []


def test_load_from_state_keeps_duplicate_filename_semantic_paths(monkeypatch):
    monkeypatch.setattr(dropbox_connector, "FileMetadata", _FakeFileMetadata)
    monkeypatch.setattr(dropbox_connector, "FolderMetadata", _FakeFolderMetadata)
    connector = DropboxConnector(batch_size=10)
    fake_client = _FakeDropboxClient()
    connector.dropbox_client = fake_client

    docs = list(next(connector.load_from_state()))

    assert [doc.id for doc in docs] == [
        "dropbox:id-root",
        "dropbox:id-nested",
        "dropbox:id-paged",
    ]
    assert [doc.semantic_identifier for doc in docs] == [
        "same.txt",
        "folder / same.txt",
        "unique.pdf",
    ]
    assert fake_client.downloaded_paths == [
        "/same.txt",
        "/folder/same.txt",
        "/unique.pdf",
    ]
