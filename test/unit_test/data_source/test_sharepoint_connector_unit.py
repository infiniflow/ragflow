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
from types import ModuleType

import pytest


def _load_sharepoint_connector_module():
    """Load sharepoint_connector.py in isolation (avoid the package __init__)."""
    repo_root = Path(__file__).resolve().parents[3]
    package_name = "common.data_source"
    saved_modules = {
        name: module
        for name, module in sys.modules.items()
        if name == package_name or name.startswith(f"{package_name}.")
    }
    package_stub = ModuleType(package_name)
    package_stub.__path__ = [str(repo_root / "common" / "data_source")]
    sys.modules[package_name] = package_stub

    try:
        spec = importlib.util.spec_from_file_location(
            "_sharepoint_connector_under_test",
            repo_root / "common" / "data_source" / "sharepoint_connector.py",
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


sharepoint_connector = _load_sharepoint_connector_module()
SharePointConnector = sharepoint_connector.SharePointConnector


# --- fakes for the office365 fluent API ------------------------------------


class _Query:
    """Mimics the `.get()` / `.get_by_url()` -> `.execute_query()` chain."""

    def __init__(self, value):
        self._value = value

    def execute_query(self):
        return self._value


class _Content:
    def __init__(self, value: bytes):
        self.value = value

    def execute_query(self):
        return self


class _FakeDriveItem:
    def __init__(self, item_id, name=None, content=None, modified=None, children=None, size=None):
        self.id = item_id
        self.name = name
        self.web_url = f"https://contoso.sharepoint.com/{item_id}"
        self.last_modified_datetime = modified
        self._content = content
        self._children = children or []
        self.properties = {}
        if children is not None:
            self.properties["folder"] = {"childCount": len(children)}
        else:
            self.properties["file"] = {"mimeType": "text/plain"}
        if size is not None:
            self.properties["size"] = size

    @property
    def children(self):
        return _FakeDrivesAccessor(self._children)

    def get_content(self):
        return _Content(self._content)


class _FakeDrive:
    def __init__(self, name, root, drive_id=None):
        self.name = name
        self.root = root
        self.id = drive_id or f"drive-{name}"
        self.properties = {"name": name, "id": self.id}


class _FakeDrivesAccessor:
    def __init__(self, drives):
        self._drives = drives

    def get(self):
        return _Query(self._drives)


class _FakeSite:
    def __init__(self, drives):
        self.drives = _FakeDrivesAccessor(drives)

    def __bool__(self):
        return True


class _FakeSitesAccessor:
    def __init__(self, site):
        self._site = site

    def get_by_url(self, url):
        return _Query(self._site)


class _FakeGraphClient:
    def __init__(self, site):
        self.sites = _FakeSitesAccessor(site)


def _build_connector_with_tree():
    jan = datetime(2026, 1, 1, 12, tzinfo=timezone.utc)
    feb = datetime(2026, 2, 1, 12, tzinfo=timezone.utc)

    readme = _FakeDriveItem("f1", "readme.txt", b"hello sharepoint", jan, size=16)
    nested = _FakeDriveItem("f2", "report.md", b"# Report", feb, size=8)
    subfolder = _FakeDriveItem("d2", "sub", children=[nested])
    root = _FakeDriveItem("d1", "root", children=[readme, subfolder])
    drive = _FakeDrive("Documents", root, drive_id="drv-A")
    site = _FakeSite([drive])

    connector = SharePointConnector(batch_size=10)
    connector.graph_client = _FakeGraphClient(site)
    connector._site_url = "https://contoso.sharepoint.com/sites/MySite"
    return connector, jan, feb


# --- credential loading -----------------------------------------------------


def test_load_credentials_incomplete_raises():
    connector = SharePointConnector()
    with pytest.raises(sharepoint_connector.ConnectorMissingCredentialError):
        connector.load_credentials({"tenant_id": "t", "client_id": "c"})


def test_load_credentials_sets_graph_client(monkeypatch):
    captured = {}

    class _FakeApp:
        def __init__(self, **kwargs):
            captured.update(kwargs)

        def acquire_token_for_client(self, scopes):
            return {"access_token": "tok"}

    monkeypatch.setattr(sharepoint_connector.msal, "ConfidentialClientApplication", _FakeApp)
    monkeypatch.setattr(sharepoint_connector, "GraphClient", lambda token_callback: ("client", token_callback))

    connector = SharePointConnector()
    result = connector.load_credentials(
        {
            "tenant_id": "tenant",
            "client_id": "client",
            "client_secret": "secret",
            "site_url": "https://contoso.sharepoint.com/sites/MySite",
        }
    )

    assert result is None
    assert connector._site_url == "https://contoso.sharepoint.com/sites/MySite"
    assert connector.graph_client is not None


def test_fetch_without_credentials_raises():
    connector = SharePointConnector()
    with pytest.raises(sharepoint_connector.ConnectorMissingCredentialError):
        list(connector.load_from_checkpoint(0.0, 9e12, connector.build_dummy_checkpoint()))


# --- document generation ----------------------------------------------------


def _collect(generator):
    """Drain a checkpoint generator, returning (documents, final_checkpoint)."""
    docs = []
    try:
        while True:
            docs.append(next(generator))
    except StopIteration as stop:
        return docs, stop.value


def test_load_from_checkpoint_walks_libraries_and_downloads():
    connector, _jan, _feb = _build_connector_with_tree()

    docs, checkpoint = _collect(
        connector.load_from_checkpoint(0.0, 9e12, connector.build_dummy_checkpoint())
    )

    assert checkpoint.has_more is False
    assert {doc.id for doc in docs} == {"drv-A:f1", "drv-A:f2"}

    by_id = {doc.id: doc for doc in docs}
    assert by_id["drv-A:f1"].blob == b"hello sharepoint"
    assert by_id["drv-A:f1"].extension == ".txt"
    assert by_id["drv-A:f1"].size_bytes == 16
    assert by_id["drv-A:f1"].source == "sharepoint"
    assert by_id["drv-A:f1"].metadata["drive"] == "Documents"
    assert by_id["drv-A:f1"].metadata["drive_id"] == "drv-A"
    assert by_id["drv-A:f1"].metadata["drive_item_id"] == "f1"
    assert by_id["drv-A:f2"].semantic_identifier == "report.md"
    assert by_id["drv-A:f2"].extension == ".md"


def test_load_from_checkpoint_filters_by_modified_window():
    connector, _jan, feb = _build_connector_with_tree()

    # Only include files modified strictly after mid-January -> just report.md (Feb).
    start = datetime(2026, 1, 15, tzinfo=timezone.utc).timestamp()
    end = datetime(2026, 3, 1, tzinfo=timezone.utc).timestamp()

    docs, _ = _collect(
        connector.load_from_checkpoint(start, end, connector.build_dummy_checkpoint())
    )

    assert [doc.id for doc in docs] == ["drv-A:f2"]


def test_retrieve_all_slim_docs_lists_ids_without_download():
    connector, _jan, _feb = _build_connector_with_tree()

    batches = list(connector.retrieve_all_slim_docs_perm_sync())
    ids = [doc.id for batch in batches for doc in batch]

    assert sorted(ids) == ["drv-A:f1", "drv-A:f2"]


def test_document_ids_are_unique_across_drives_with_colliding_item_ids():
    # Graph driveItem IDs are unique only within a single drive; two libraries
    # under the same site can legitimately yield items with identical IDs.
    jan = datetime(2026, 1, 1, 12, tzinfo=timezone.utc)

    file_a = _FakeDriveItem("same-id", "a.txt", b"A", jan, size=1)
    root_a = _FakeDriveItem("rootA", "root", children=[file_a])
    drive_a = _FakeDrive("LibraryA", root_a, drive_id="drv-A")

    file_b = _FakeDriveItem("same-id", "b.txt", b"B", jan, size=1)
    root_b = _FakeDriveItem("rootB", "root", children=[file_b])
    drive_b = _FakeDrive("LibraryB", root_b, drive_id="drv-B")

    site = _FakeSite([drive_a, drive_b])
    connector = SharePointConnector(batch_size=10)
    connector.graph_client = _FakeGraphClient(site)
    connector._site_url = "https://contoso.sharepoint.com/sites/MySite"

    docs, _ = _collect(
        connector.load_from_checkpoint(0.0, 9e12, connector.build_dummy_checkpoint())
    )
    ids = {doc.id for doc in docs}
    assert ids == {"drv-A:same-id", "drv-B:same-id"}

    slim_ids = [doc.id for batch in connector.retrieve_all_slim_docs_perm_sync() for doc in batch]
    assert sorted(slim_ids) == ["drv-A:same-id", "drv-B:same-id"]
