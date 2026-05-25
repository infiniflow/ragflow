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

import pytest


def _load_teams_connector_module():
    """Load teams_connector.py in isolation (avoid the package __init__)."""
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
            "_teams_connector_under_test",
            repo_root / "common" / "data_source" / "teams_connector.py",
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


teams_connector = _load_teams_connector_module()
TeamsConnector = teams_connector.TeamsConnector

pytestmark = pytest.mark.p2


# --- fakes for the office365 fluent API ------------------------------------


class _Query:
    def __init__(self, value):
        self._value = value

    def execute_query(self):
        return self._value


class _Collection:
    def __init__(self, items):
        self._items = items

    def get(self):
        return _Query(self._items)

    def get_all(self):
        # The connector pages with get_all(); the fake returns every item at once.
        return _Query(self._items)


class _Body:
    def __init__(self, content, content_type="text"):
        self.content = content
        self.contentType = content_type


class _Message:
    def __init__(self, msg_id, content, content_type="text", modified=None, replies=None):
        self.id = msg_id
        self.body = _Body(content, content_type)
        self.web_url = f"https://teams.microsoft.com/{msg_id}"
        self._replies = replies or []
        self.properties = {}
        if modified is not None:
            self.properties["lastModifiedDateTime"] = modified
            self.properties["createdDateTime"] = modified

    @property
    def replies(self):
        return _Collection(self._replies)


class _Channel:
    def __init__(self, channel_id, display_name, messages):
        self.id = channel_id
        self._messages = messages
        self.properties = {"displayName": display_name}

    @property
    def messages(self):
        return _Collection(self._messages)


class _Team:
    def __init__(self, team_id, display_name, channels):
        self.id = team_id
        self._channels = channels
        self.properties = {"displayName": display_name}

    @property
    def channels(self):
        return _Collection(self._channels)


class _FakeGraphClient:
    def __init__(self, teams):
        self.teams = _Collection(teams)


def _build_connector():
    jan = "2026-01-01T12:00:00Z"
    feb = "2026-02-01T12:00:00Z"

    reply = _Message("m1-r1", "All set.", modified=jan)
    post1 = _Message("m1", "How do we deploy?", modified=jan, replies=[reply])
    post2 = _Message("m2", "<b>Release notes</b>", content_type="html", modified=feb)
    channel = _Channel("c1", "General", [post1, post2])
    team = _Team("t1", "Engineering", [channel])

    connector = TeamsConnector(batch_size=100)
    connector.graph_client = _FakeGraphClient([team])
    return connector


def _collect(generator):
    docs = []
    try:
        while True:
            docs.append(next(generator))
    except StopIteration as stop:
        return docs, stop.value


# --- credentials ------------------------------------------------------------


def test_load_credentials_incomplete_raises():
    connector = TeamsConnector()
    with pytest.raises(teams_connector.ConnectorMissingCredentialError):
        connector.load_credentials({"tenant_id": "t"})


def test_load_credentials_sets_graph_client(monkeypatch):
    class _FakeApp:
        def __init__(self, **kwargs):
            pass

        def acquire_token_for_client(self, scopes):
            return {"access_token": "tok"}

    monkeypatch.setattr(teams_connector.msal, "ConfidentialClientApplication", _FakeApp)
    monkeypatch.setattr(teams_connector, "GraphClient", lambda token_callback: SimpleNamespace(cb=token_callback))

    connector = TeamsConnector()
    result = connector.load_credentials(
        {"tenant_id": "tenant", "client_id": "client", "client_secret": "secret"}
    )

    assert result is None
    assert connector.graph_client is not None


def test_fetch_without_credentials_raises():
    connector = TeamsConnector()
    with pytest.raises(teams_connector.ConnectorMissingCredentialError):
        list(connector.load_from_checkpoint(0.0, 9e12, connector.build_dummy_checkpoint()))


def test_validate_without_client_raises():
    connector = TeamsConnector()
    with pytest.raises(teams_connector.ConnectorMissingCredentialError):
        connector.validate_connector_settings()


def test_validate_maps_permission_error():
    class _RaisingQuery:
        def execute_query(self):
            raise Exception("(403) Forbidden: insufficient privileges")

    class _RaisingCollection:
        def get(self):
            return _RaisingQuery()

    connector = TeamsConnector()
    connector.graph_client = SimpleNamespace(teams=_RaisingCollection())

    with pytest.raises(teams_connector.InsufficientPermissionsError):
        connector.validate_connector_settings()


# --- document generation ----------------------------------------------------


def test_load_from_checkpoint_flattens_posts_and_replies():
    connector = _build_connector()

    docs, checkpoint = _collect(
        connector.load_from_checkpoint(0.0, 9e12, connector.build_dummy_checkpoint())
    )

    assert checkpoint.has_more is False
    assert {doc.id for doc in docs} == {"t1__c1__m1", "t1__c1__m2"}

    by_id = {doc.id: doc for doc in docs}
    # Post + reply flattened into one blob.
    assert by_id["t1__c1__m1"].blob == b"How do we deploy?\n\nAll set."
    assert by_id["t1__c1__m1"].source == "teams"
    assert by_id["t1__c1__m1"].extension == ".txt"
    assert by_id["t1__c1__m1"].metadata == {
        "team": "Engineering",
        "channel": "General",
        "web_url": "https://teams.microsoft.com/m1",
    }
    # HTML post gets the .html extension.
    assert by_id["t1__c1__m2"].extension == ".html"
    assert by_id["t1__c1__m2"].blob == b"<b>Release notes</b>"


def test_load_from_checkpoint_filters_by_modified_window():
    connector = _build_connector()

    start = datetime(2026, 1, 15, tzinfo=timezone.utc).timestamp()
    end = datetime(2026, 3, 1, tzinfo=timezone.utc).timestamp()

    docs, _ = _collect(
        connector.load_from_checkpoint(start, end, connector.build_dummy_checkpoint())
    )

    assert [doc.id for doc in docs] == ["t1__c1__m2"]


def test_retrieve_all_slim_docs_lists_ids():
    connector = _build_connector()

    batches = list(connector.retrieve_all_slim_docs_perm_sync())
    ids = [doc.id for batch in batches for doc in batch]

    assert sorted(ids) == ["t1__c1__m1", "t1__c1__m2"]
