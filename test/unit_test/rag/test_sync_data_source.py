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
import importlib
import importlib.util
import os
import sys
import types
import warnings
from datetime import datetime, timezone

import pytest

warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    try:
        importlib.import_module("cv2")
        return
    except Exception:
        pass

    stub = types.ModuleType("cv2")
    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1

    def _missing(*_args, **_kwargs):
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    def _module_getattr(name):
        if name.isupper():
            return 0
        return _missing

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


def _install_xgboost_stub_if_unavailable():
    if "xgboost" in sys.modules:
        return
    if importlib.util.find_spec("xgboost") is not None:
        return
    sys.modules["xgboost"] = types.ModuleType("xgboost")


def _install_ollama_stub():
    stub = types.ModuleType("ollama")

    class _DummyClient:
        def __init__(self, *_args, **_kwargs):
            pass

    stub.Client = _DummyClient
    sys.modules["ollama"] = stub


for proxy_key in ("ALL_PROXY", "all_proxy", "HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"):
    os.environ.pop(proxy_key, None)

_install_cv2_stub_if_unavailable()
_install_xgboost_stub_if_unavailable()
_install_ollama_stub()

sync_data_source = importlib.import_module("rag.svr.sync_data_source")


class _FakeSync(sync_data_source.SyncBase):
    SOURCE_NAME = "fake"

    def __init__(self, generate_output):
        super().__init__({})
        self._generate_output = generate_output

    async def _generate(self, task: dict):
        return self._generate_output


def _make_task():
    return {
        "id": "task-1",
        "connector_id": "connector-1",
        "kb_id": "kb-1",
        "tenant_id": "tenant-1",
        "poll_range_start": None,
        "auto_parse": False,
    }


def _patch_common_dependencies(monkeypatch):
    monkeypatch.setattr(
        sync_data_source.DocumentService,
        "list_doc_headers_by_kb_and_source_type",
        lambda *_args, **_kwargs: [],
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "done",
        lambda *_args, **_kwargs: None,
    )


@pytest.mark.anyio
@pytest.mark.p2
async def test_run_task_logic_skips_cleanup_for_empty_snapshot(monkeypatch):
    cleanup_calls = []

    _patch_common_dependencies(monkeypatch)
    monkeypatch.setattr(
        sync_data_source.ConnectorService,
        "cleanup_stale_documents_for_task",
        lambda *_args, **_kwargs: cleanup_calls.append((_args, _kwargs)),
    )

    await _FakeSync((iter(()), []))._run_task_logic(_make_task())

    assert cleanup_calls == []


@pytest.mark.anyio
@pytest.mark.p2
async def test_run_task_logic_cleans_up_for_non_empty_snapshot(monkeypatch):
    cleanup_calls = []

    _patch_common_dependencies(monkeypatch)

    def _fake_cleanup(*args, **kwargs):
        cleanup_calls.append((args, kwargs))
        return 2, []

    monkeypatch.setattr(
        sync_data_source.ConnectorService,
        "cleanup_stale_documents_for_task",
        _fake_cleanup,
    )

    file_list = [types.SimpleNamespace(id="doc-1")]
    await _FakeSync((iter(()), file_list))._run_task_logic(_make_task())

    assert cleanup_calls == [
        (
            (
                "task-1",
                "connector-1",
                "kb-1",
                "tenant-1",
                file_list,
            ),
            {},
        )
    ]


class _FakeDropboxConnector:
    instance = None

    def __init__(self, batch_size):
        self.batch_size = batch_size
        self.credentials = None
        self.retrieve_all_slim_docs_perm_sync_called = False
        self.snapshot_called_before_poll = None
        self.poll_source_call = None
        self.load_from_state_called = False
        self.poll_source_called = False
        _FakeDropboxConnector.instance = self

    def load_credentials(self, credentials):
        self.credentials = credentials

    def retrieve_all_slim_docs_perm_sync(self):
        self.retrieve_all_slim_docs_perm_sync_called = True
        self.snapshot_called_before_poll = not self.poll_source_called
        yield [types.SimpleNamespace(id="dropbox:id-1")]
        yield [types.SimpleNamespace(id="dropbox:id-2")]

    def poll_source(self, start, end):
        self.poll_source_called = True
        self.poll_source_call = (start, end)
        return iter((["poll-sync"],))

    def load_from_state(self):
        self.load_from_state_called = True
        return iter((["full-sync"],))


@pytest.mark.anyio
@pytest.mark.p2
async def test_dropbox_generate_returns_snapshot_when_sync_deleted_enabled(monkeypatch):
    monkeypatch.setattr(sync_data_source, "DropboxConnector", _FakeDropboxConnector)
    poll_start = datetime(2026, 1, 1, tzinfo=timezone.utc)
    task = {
        **_make_task(),
        "reindex": "0",
        "poll_range_start": poll_start,
        "skip_connection_log": True,
    }
    sync = sync_data_source.Dropbox(
        {
            "batch_size": 2,
            "sync_deleted_files": True,
            "credentials": {"dropbox_access_token": "token-1"},
        }
    )

    document_generator, file_list = await sync._generate(task)
    connector = _FakeDropboxConnector.instance

    assert list(document_generator) == [["poll-sync"]]
    assert [doc.id for doc in file_list] == ["dropbox:id-1", "dropbox:id-2"]
    assert connector.credentials == {"dropbox_access_token": "token-1"}
    assert connector.retrieve_all_slim_docs_perm_sync_called is True
    assert connector.snapshot_called_before_poll is True
    assert connector.poll_source_call[0] == poll_start.timestamp()
    assert connector.poll_source_call[1] >= poll_start.timestamp()


@pytest.mark.anyio
@pytest.mark.p2
async def test_dropbox_generate_skips_snapshot_for_full_reindex(monkeypatch):
    monkeypatch.setattr(sync_data_source, "DropboxConnector", _FakeDropboxConnector)
    task = {
        **_make_task(),
        "reindex": "1",
        "poll_range_start": datetime(2026, 1, 1, tzinfo=timezone.utc),
        "skip_connection_log": True,
    }
    sync = sync_data_source.Dropbox(
        {
            "batch_size": 2,
            "sync_deleted_files": True,
            "credentials": {"dropbox_access_token": "token-1"},
        }
    )

    document_generator, file_list = await sync._generate(task)
    connector = _FakeDropboxConnector.instance

    assert list(document_generator) == [["full-sync"]]
    assert file_list is None
    assert connector.load_from_state_called is True
    assert connector.retrieve_all_slim_docs_perm_sync_called is False
    assert connector.poll_source_called is False
