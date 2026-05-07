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


def _make_fake_doc(doc_id="doc-1", updated_at=None):
    return types.SimpleNamespace(
        id=doc_id,
        semantic_identifier=doc_id,
        extension=".txt",
        size_bytes=1,
        doc_updated_at=updated_at or datetime(2026, 1, 1, tzinfo=timezone.utc),
        blob=b"x",
        metadata=None,
    )


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
async def test_run_task_logic_cleans_up_for_empty_snapshot(monkeypatch):
    cleanup_calls = []

    _patch_common_dependencies(monkeypatch)

    def _fake_cleanup(*args, **kwargs):
        cleanup_calls.append((args, kwargs))
        return 1, []

    monkeypatch.setattr(
        sync_data_source.ConnectorService,
        "cleanup_stale_documents_for_task",
        _fake_cleanup,
    )

    await _FakeSync((iter(()), []))._run_task_logic(_make_task())

    assert cleanup_calls == [
        (
            (
                "task-1",
                "connector-1",
                "kb-1",
                "tenant-1",
                [],
            ),
            {},
        )
    ]


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


class _FakeRDBMSConnector:
    instance = None

    def __init__(
        self,
        db_type,
        host,
        port,
        database,
        query,
        content_columns,
        metadata_columns=None,
        id_column=None,
        timestamp_column=None,
        batch_size=2,
    ):
        self.db_type = db_type
        self.host = host
        self.port = port
        self.database = database
        self.query = query
        self.content_columns = content_columns
        self.metadata_columns = metadata_columns
        self.id_column = id_column
        self.timestamp_column = timestamp_column
        self.batch_size = batch_size
        self.load_from_state_called = False
        self.retrieve_all_slim_docs_perm_sync_called = False
        self.prepare_sync_state_called = False
        self.load_from_cursor_range_called = False
        self.persist_sync_state_called = False
        self._pending_sync_cursor_value = None
        _FakeRDBMSConnector.instance = self

    def load_credentials(self, credentials):
        self.credentials = credentials

    def validate_connector_settings(self):
        return None

    def prepare_sync_state(self, connector_id, config):
        self.prepare_sync_state_called = True
        self.prepare_sync_state_args = (connector_id, config)

    def get_saved_sync_cursor_value(self):
        return None

    def retrieve_all_slim_docs_perm_sync(self, callback=None):
        del callback
        self.retrieve_all_slim_docs_perm_sync_called = True
        yield [types.SimpleNamespace(id="row-1")]

    def load_from_state(self):
        self.load_from_state_called = True
        return iter((["full-sync"],))

    def load_from_cursor_range(self, start_value=None, end_value=None):
        self.load_from_cursor_range_called = True
        return iter(([ _make_fake_doc("incremental-doc") ],))

    def persist_sync_state(self):
        self.persist_sync_state_called = True


@pytest.mark.anyio
@pytest.mark.p2
async def test_rdbms_generate_keeps_deleted_file_snapshot_without_timestamp_column(monkeypatch):
    monkeypatch.setattr(sync_data_source, "RDBMSConnector", _FakeRDBMSConnector)

    task = {
        **_make_task(),
        "reindex": "0",
        "poll_range_start": datetime(2026, 1, 1, tzinfo=timezone.utc),
        "skip_connection_log": True,
    }
    sync = sync_data_source.MySQL(
        {
            "host": "localhost",
            "port": 3306,
            "database": "db",
            "query": "SELECT * FROM t",
            "content_columns": "name",
            "credentials": {"username": "u", "password": "p"},
            "sync_deleted_files": True,
        }
    )

    document_generator, file_list = await sync._generate(task)
    connector = _FakeRDBMSConnector.instance

    assert connector is not None
    assert connector.load_from_state_called is True
    assert connector.load_from_cursor_range_called is False
    assert connector.retrieve_all_slim_docs_perm_sync_called is True
    assert file_list is not None
    assert [doc.id for doc in file_list] == ["row-1"]
    assert list(document_generator) == [["full-sync"]]


@pytest.mark.anyio
@pytest.mark.p2
async def test_rdbms_cursor_persists_only_after_success(monkeypatch):
    monkeypatch.setattr(sync_data_source, "RDBMSConnector", _FakeRDBMSConnector)
    _patch_common_dependencies(monkeypatch)
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: (True, object()),
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "increase_docs",
        lambda *_args, **_kwargs: None,
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "duplicate_and_parse",
        lambda *_args, **_kwargs: ([], ["parsed-doc-id"]),
    )

    task = {
        **_make_task(),
        "reindex": "0",
        "poll_range_start": datetime(2026, 1, 1, tzinfo=timezone.utc),
        "skip_connection_log": True,
    }
    sync = sync_data_source.MySQL(
        {
            "host": "localhost",
            "port": 3306,
            "database": "db",
            "query": "SELECT * FROM t",
            "content_columns": "name",
            "timestamp_column": "ts",
            "credentials": {"username": "u", "password": "p"},
            "sync_deleted_files": False,
        }
    )

    await sync._run_task_logic(task)

    connector = _FakeRDBMSConnector.instance
    assert connector is not None
    assert connector.persist_sync_state_called is True


@pytest.mark.anyio
@pytest.mark.p2
async def test_rdbms_cursor_does_not_persist_when_batch_is_skipped(monkeypatch):
    monkeypatch.setattr(sync_data_source, "RDBMSConnector", _FakeRDBMSConnector)
    _patch_common_dependencies(monkeypatch)
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: (True, object()),
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "increase_docs",
        lambda *_args, **_kwargs: None,
    )

    def _raise_in_duplicate_and_parse(*_args, **_kwargs):
        raise RuntimeError("batch failed")

    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "duplicate_and_parse",
        _raise_in_duplicate_and_parse,
    )

    task = {
        **_make_task(),
        "reindex": "0",
        "poll_range_start": datetime(2026, 1, 1, tzinfo=timezone.utc),
        "skip_connection_log": True,
    }
    sync = sync_data_source.MySQL(
        {
            "host": "localhost",
            "port": 3306,
            "database": "db",
            "query": "SELECT * FROM t",
            "content_columns": "name",
            "timestamp_column": "ts",
            "credentials": {"username": "u", "password": "p"},
            "sync_deleted_files": False,
        }
    )

    await sync._run_task_logic(task)

    connector = _FakeRDBMSConnector.instance
    assert connector is not None
    assert connector.persist_sync_state_called is False


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

    def retrieve_all_slim_docs_perm_sync(self, callback=None):
        del callback
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
