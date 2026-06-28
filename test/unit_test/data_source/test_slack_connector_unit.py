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
from pathlib import Path
from types import ModuleType

import pytest


def _load_slack_connector_module():
    """Load slack_connector.py in isolation.

    Importing ``common.data_source`` directly would execute its ``__init__``
    and pull in every connector's (heavy) dependencies. We stub the package and
    exec only the Slack module, mirroring the Dropbox connector unit test.
    """
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
            "_slack_connector_under_test",
            repo_root / "common" / "data_source" / "slack_connector.py",
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


slack_connector = _load_slack_connector_module()
SlackConnector = slack_connector.SlackConnector
SlackTextCleaner = slack_connector.SlackTextCleaner


class _FakeResponse(dict):
    """Mimics slack_sdk SlackResponse: dict-like, plus .validate() and .data."""

    def validate(self):
        return self

    @property
    def data(self):
        return self


class _FakeSlackClient:
    def __init__(self):
        self.token = "xoxb-test"
        self.joined = []
        self._users = {
            "U1": {
                "real_name": "Alice",
                "profile": {"display_name": "alice", "first_name": "Alice", "last_name": "A"},
            },
            "U2": {"real_name": "Bob", "profile": {"display_name": "bob"}},
            "U3": {"real_name": "Carol", "profile": {"display_name": "carol"}},
        }

    def conversations_list(self, cursor=None, limit=None, **kwargs):
        return _FakeResponse(
            {
                "channels": [
                    {"id": "C1", "name": "general", "is_member": True, "is_private": False},
                ],
                "response_metadata": {"next_cursor": ""},
            }
        )

    def conversations_history(self, cursor=None, limit=None, channel=None, oldest=None, latest=None, **kwargs):
        return _FakeResponse(
            {
                "messages": [
                    {"ts": "1.0", "user": "U1", "text": "Hello world"},
                    {"ts": "2.0", "user": "U2", "text": "Question?", "thread_ts": "2.0"},
                ],
                "response_metadata": {"next_cursor": ""},
            }
        )

    def conversations_replies(self, cursor=None, limit=None, channel=None, ts=None, **kwargs):
        return _FakeResponse(
            {
                "messages": [
                    {"ts": "2.0", "user": "U2", "text": "Question?", "thread_ts": "2.0"},
                    {"ts": "3.0", "user": "U3", "text": "Answer!", "thread_ts": "2.0"},
                ],
                "response_metadata": {"next_cursor": ""},
            }
        )

    def users_info(self, user=None, **kwargs):
        return _FakeResponse({"ok": True, "user": self._users.get(user, {})})


def _connector_with_fake_client(client, batch_size=10):
    connector = SlackConnector(batch_size=batch_size)
    connector.client = client
    connector.text_cleaner = SlackTextCleaner(client=client)
    return connector


# --- credential loading -----------------------------------------------------


@pytest.mark.p2
def test_load_credentials_is_not_supported():
    connector = SlackConnector()
    with pytest.raises(NotImplementedError):
        connector.load_credentials({"slack_bot_token": "xoxb-abc"})


@pytest.mark.p2
def test_set_credentials_provider_initializes_clients():
    connector = SlackConnector()

    class _Provider:
        def get_credentials(self):
            return {"slack_bot_token": "xoxb-abc"}

    connector.set_credentials_provider(_Provider())

    assert connector.client is not None
    assert connector.fast_client is not None
    assert connector.text_cleaner is not None


@pytest.mark.p2
def test_fetch_without_credentials_raises():
    connector = SlackConnector()
    with pytest.raises(slack_connector.ConnectorMissingCredentialError):
        list(connector.load_from_state())


# --- document generation ----------------------------------------------------


@pytest.mark.p1
def test_load_from_state_generates_thread_documents():
    connector = _connector_with_fake_client(_FakeSlackClient())

    batches = list(connector.load_from_state())
    docs = [doc for batch in batches for doc in batch]

    # Standalone message + one thread (parent + reply collapsed into one doc).
    assert [doc.id for doc in docs] == ["C1__1.0", "C1__2.0"]

    standalone, thread_doc = docs
    assert standalone.source == "slack"
    assert standalone.extension == ".txt"
    assert standalone.blob == b"Hello world"
    assert standalone.size_bytes == len(b"Hello world")
    assert standalone.metadata == {"Channel": "general"}
    # get_semantic_name() prefers real_name ("Alice") over the display_name.
    assert standalone.semantic_identifier == "Alice in #general: Hello world"

    # Thread messages are flattened into a single blob, joined by blank lines.
    assert thread_doc.blob == "Question?\n\nAnswer!".encode("utf-8")
    assert thread_doc.size_bytes == len(thread_doc.blob)
    assert thread_doc.semantic_identifier == "Bob in #general: Question?"


@pytest.mark.p1
def test_poll_source_passes_time_window():
    client = _FakeSlackClient()
    captured = {}

    def _history(cursor=None, limit=None, channel=None, oldest=None, latest=None, **kwargs):
        captured["oldest"] = oldest
        captured["latest"] = latest
        return _FakeResponse({"messages": [], "response_metadata": {"next_cursor": ""}})

    client.conversations_history = _history
    connector = _connector_with_fake_client(client)

    list(connector.poll_source(100.0, 200.0))

    assert captured == {"oldest": "100.0", "latest": "200.0"}
