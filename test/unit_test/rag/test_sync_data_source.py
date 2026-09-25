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
import importlib
import importlib.util
import os
import sys
import threading
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


@pytest.mark.asyncio
@pytest.mark.p2
async def test_run_task_logic_skips_empty_sync_batches(monkeypatch):
    _patch_common_dependencies(monkeypatch)
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "increase_docs",
        lambda *_args, **_kwargs: pytest.fail("increase_docs should not be called for empty batches"),
    )
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: pytest.fail("get_by_id should not be called for empty batches"),
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "duplicate_and_parse",
        lambda *_args, **_kwargs: pytest.fail("duplicate_and_parse should not be called for empty batches"),
    )

    await _FakeSync(iter(([],)))._run_task_logic(_make_task())


@pytest.mark.asyncio
@pytest.mark.p2
async def test_run_task_logic_skips_multiple_empty_sync_batches(monkeypatch):
    _patch_common_dependencies(monkeypatch)
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "increase_docs",
        lambda *_args, **_kwargs: pytest.fail("increase_docs should not be called for empty batches"),
    )
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: pytest.fail("get_by_id should not be called for empty batches"),
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "duplicate_and_parse",
        lambda *_args, **_kwargs: pytest.fail("duplicate_and_parse should not be called for empty batches"),
    )

    await _FakeSync(
        iter(
            (
                [],
                [],
            )
        )
    )._run_task_logic(_make_task())


@pytest.mark.asyncio
@pytest.mark.p2
async def test_run_task_logic_keeps_event_loop_responsive_during_parse(monkeypatch):
    _patch_common_dependencies(monkeypatch)
    started = threading.Event()
    release = threading.Event()

    def _slow_duplicate(*_args, **_kwargs):
        started.set()
        if not release.wait(timeout=5):
            pytest.fail("release was not signalled while parse was blocked")
        return [], ["doc-1"]

    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: (True, object()),
    )
    monkeypatch.setattr(sync_data_source.SyncLogsService, "duplicate_and_parse", _slow_duplicate)
    monkeypatch.setattr(sync_data_source.SyncLogsService, "increase_docs", lambda *_args, **_kwargs: None)

    probe_done = False

    async def _probe():
        nonlocal probe_done
        flagged = await asyncio.to_thread(started.wait, 5)
        assert flagged, "ingest did not start"
        probe_done = True
        release.set()

    await asyncio.wait_for(
        asyncio.gather(
            _FakeSync(iter(([_make_fake_doc()],)))._run_task_logic(_make_task()),
            _probe(),
        ),
        timeout=10,
    )
    assert probe_done


def _patch_successful_ingest(monkeypatch):
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: (True, object()),
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "duplicate_and_parse",
        lambda *_args, **_kwargs: ([], ["doc-1"]),
    )
    monkeypatch.setattr(sync_data_source.SyncLogsService, "increase_docs", lambda *_args, **_kwargs: None)


async def _collect_sync_sleeps(monkeypatch, generate_output):
    _patch_common_dependencies(monkeypatch)
    _patch_successful_ingest(monkeypatch)
    monkeypatch.setattr(sync_data_source, "SYNC_BATCH_PAUSE_SECONDS", 0.05)
    sleep_calls = []

    async def _track_sleep(delay, *_args, **_kwargs):
        sleep_calls.append(delay)

    monkeypatch.setattr(sync_data_source.asyncio, "sleep", _track_sleep)
    await _FakeSync(generate_output)._run_task_logic(_make_task())
    return sleep_calls


@pytest.mark.asyncio
@pytest.mark.p2
async def test_run_task_logic_pauses_only_between_nonempty_batches(monkeypatch):
    sleep_calls = await _collect_sync_sleeps(
        monkeypatch,
        iter(([_make_fake_doc("doc-1")], [], [_make_fake_doc("doc-2")])),
    )
    assert sleep_calls == [0.05]


@pytest.mark.asyncio
@pytest.mark.p2
async def test_run_task_logic_does_not_pause_after_last_batch(monkeypatch):
    sleep_calls = await _collect_sync_sleeps(monkeypatch, iter(([_make_fake_doc()],)))
    assert sleep_calls == []


def test_ingest_document_batch_skips_writes_when_already_cancelled(monkeypatch):
    _patch_common_dependencies(monkeypatch)
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: pytest.fail("get_by_id should not run after cancel"),
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "duplicate_and_parse",
        lambda *_args, **_kwargs: pytest.fail("duplicate_and_parse should not run after cancel"),
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "increase_docs",
        lambda *_args, **_kwargs: pytest.fail("increase_docs should not run after cancel"),
    )
    cancel_event = threading.Event()
    cancel_event.set()
    err, dids = _FakeSync(iter(()))._ingest_document_batch(
        _make_task(),
        [{"id": "doc-1"}],
        datetime(2026, 1, 1, tzinfo=timezone.utc),
        cancel_event,
    )
    assert err == []
    assert dids == []


def test_ingest_document_batch_skips_progress_when_cancelled_during_parse(monkeypatch):
    _patch_common_dependencies(monkeypatch)
    cancel_event = threading.Event()
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: (True, object()),
    )

    def _duplicate_then_cancel(*_args, **_kwargs):
        cancel_event.set()
        return [], ["doc-1"]

    monkeypatch.setattr(sync_data_source.SyncLogsService, "duplicate_and_parse", _duplicate_then_cancel)
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "increase_docs",
        lambda *_args, **_kwargs: pytest.fail("increase_docs should not advance after cancel"),
    )
    err, dids = _FakeSync(iter(()))._ingest_document_batch(
        _make_task(),
        [{"id": "doc-1"}],
        datetime(2026, 1, 1, tzinfo=timezone.utc),
        cancel_event,
    )
    assert err == []
    assert dids == ["doc-1"]


class _PendingCancelTask:
    def cancelled(self):
        return False

    def cancelling(self):
        return 1


class _DeferredCancelTask:
    def __init__(self):
        self._cancelling = 0

    def cancelled(self):
        return False

    def cancelling(self):
        return self._cancelling


def test_ingest_document_batch_skips_writes_when_parent_task_is_cancelling(monkeypatch):
    _patch_common_dependencies(monkeypatch)
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: pytest.fail("get_by_id should not run after cancel"),
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "duplicate_and_parse",
        lambda *_args, **_kwargs: pytest.fail("duplicate_and_parse should not run after cancel"),
    )
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "increase_docs",
        lambda *_args, **_kwargs: pytest.fail("increase_docs should not run after cancel"),
    )
    err, dids = _FakeSync(iter(()))._ingest_document_batch(
        _make_task(),
        [{"id": "doc-1"}],
        datetime(2026, 1, 1, tzinfo=timezone.utc),
        threading.Event(),
        _PendingCancelTask(),
    )
    assert err == []
    assert dids == []


def test_ingest_document_batch_skips_progress_when_parent_task_starts_cancelling(monkeypatch):
    _patch_common_dependencies(monkeypatch)
    parent_task = _DeferredCancelTask()
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: (True, object()),
    )

    def _duplicate_then_cancel(*_args, **_kwargs):
        parent_task._cancelling = 1
        return [], ["doc-1"]

    monkeypatch.setattr(sync_data_source.SyncLogsService, "duplicate_and_parse", _duplicate_then_cancel)
    monkeypatch.setattr(
        sync_data_source.SyncLogsService,
        "increase_docs",
        lambda *_args, **_kwargs: pytest.fail("increase_docs should not advance after cancel"),
    )
    err, dids = _FakeSync(iter(()))._ingest_document_batch(
        _make_task(),
        [{"id": "doc-1"}],
        datetime(2026, 1, 1, tzinfo=timezone.utc),
        threading.Event(),
        parent_task,
    )
    assert err == []
    assert dids == ["doc-1"]


def test_ingest_document_batch_forwards_cancel_callback(monkeypatch):
    _patch_common_dependencies(monkeypatch)
    captured = {}
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: (True, object()),
    )

    def _duplicate(*_args, **kwargs):
        captured["should_cancel"] = kwargs.get("should_cancel")
        return [], ["doc-1"]

    monkeypatch.setattr(sync_data_source.SyncLogsService, "duplicate_and_parse", _duplicate)
    monkeypatch.setattr(sync_data_source.SyncLogsService, "increase_docs", lambda *_args, **_kwargs: None)
    cancel_event = threading.Event()
    err, dids = _FakeSync(iter(()))._ingest_document_batch(
        _make_task(),
        [{"id": "doc-1"}],
        datetime(2026, 1, 1, tzinfo=timezone.utc),
        cancel_event,
    )
    assert err == []
    assert dids == ["doc-1"]
    assert captured["should_cancel"] is not None
    assert captured["should_cancel"]() is False
    cancel_event.set()
    assert captured["should_cancel"]() is True


@pytest.mark.asyncio
@pytest.mark.p2
async def test_run_prune_task_logic_cleans_up_for_empty_snapshot(monkeypatch):
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

    task = {**_make_task(), "task_type": sync_data_source.ConnectorTaskType.PRUNE}
    sync = _FakeSync(iter(()))
    sync.conf["sync_deleted_files"] = True
    sync.connector = types.SimpleNamespace(retrieve_all_slim_docs_perm_sync=lambda: iter(([],)))

    await sync._run_task_logic(task)

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


@pytest.mark.asyncio
@pytest.mark.p2
async def test_run_prune_task_logic_cleans_up_for_non_empty_snapshot(monkeypatch):
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
    task = {**_make_task(), "task_type": sync_data_source.ConnectorTaskType.PRUNE}
    sync = _FakeSync(iter(()))
    sync.conf["sync_deleted_files"] = True
    sync.connector = types.SimpleNamespace(retrieve_all_slim_docs_perm_sync=lambda: iter((file_list,)))

    await sync._run_task_logic(task)

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
        file_extension=None,
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
        self.file_extension = file_extension if file_extension else ".txt"
        self.load_from_state_called = False
        self.retrieve_all_slim_docs_perm_sync_called = False
        self.prepare_sync_state_called = False
        self.load_from_cursor_range_called = False
        self.persist_sync_state_called = False
        self.close_connection_called = False
        self.validate_thread_ident = None
        self.prepare_thread_ident = None
        self.close_thread_ident = None
        self._pending_sync_cursor_value = None
        _FakeRDBMSConnector.instance = self

    def load_credentials(self, credentials):
        self.credentials = credentials

    def validate_connector_settings(self):
        self.validate_thread_ident = threading.get_ident()

    def prepare_sync_state(self, connector_id, config):
        self.prepare_sync_state_called = True
        self.prepare_thread_ident = threading.get_ident()
        self.prepare_sync_state_args = (connector_id, config)

    def _close_connection(self):
        self.close_connection_called = True
        self.close_thread_ident = threading.get_ident()

    def get_saved_sync_cursor_value(self):
        return None

    def retrieve_all_slim_docs_perm_sync(self, callback=None):
        del callback
        self.retrieve_all_slim_docs_perm_sync_called = True
        yield [types.SimpleNamespace(id="row-1")]

    def load_from_state(self):
        self.load_from_state_called = True
        return iter((["full-sync"],))

    def load_from_cursor_range(self, start_value=None, start_id=None, end_value=None):
        self.load_from_cursor_range_called = True
        return iter(([_make_fake_doc("incremental-doc")],))

    def persist_sync_state(self):
        self.persist_sync_state_called = True


@pytest.mark.asyncio
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

    main_ident = threading.get_ident()
    document_generator = await sync._generate(task)
    connector = _FakeRDBMSConnector.instance

    assert connector is not None
    assert connector.load_from_state_called is True
    assert connector.load_from_cursor_range_called is False
    assert connector.prepare_sync_state_called is True
    assert connector.close_connection_called is True
    assert connector.validate_thread_ident != main_ident
    assert connector.prepare_thread_ident == connector.validate_thread_ident
    assert connector.close_thread_ident == connector.validate_thread_ident
    file_list = sync._collect_prune_snapshot(task)
    assert connector.retrieve_all_slim_docs_perm_sync_called is True
    assert file_list is not None
    assert [doc.id for doc in file_list] == ["row-1"]
    assert list(document_generator) == [["full-sync"]]


class _FakeXquikConnector:
    def __init__(self):
        self.poll_args = None
        self.loaded = False

    def poll_source(self, start, end):
        self.poll_args = (start, end)
        return iter((["incremental"],))

    def load_from_state(self):
        self.loaded = True
        return iter((["full"],))


@pytest.mark.asyncio
@pytest.mark.p2
async def test_xquik_generate_applies_incremental_window(monkeypatch):
    captured = {}
    fake_connector = _FakeXquikConnector()

    def _from_config(config, **kwargs):
        captured["config"] = config
        captured.update(kwargs)
        return fake_connector

    monkeypatch.setattr(sync_data_source.XquikConnector, "from_config", _from_config)
    poll_start = datetime(2026, 8, 24, 10, 0, tzinfo=timezone.utc)
    task = {
        **_make_task(),
        "reindex": "0",
        "poll_range_start": poll_start,
        "skip_connection_log": True,
    }
    config = {
        "query": "ragflow",
        "credentials": {"xquik_api_key": "xq_test_key"},
    }

    generator = await sync_data_source.Xquik(config)._generate(task)

    assert list(generator) == [["incremental"]]
    assert captured["config"] == config
    assert captured["since_time"] == poll_start
    assert captured["until_time"].tzinfo == timezone.utc
    assert fake_connector.poll_args[0] == poll_start.timestamp()
    assert fake_connector.poll_args[1] == captured["until_time"].timestamp()


@pytest.mark.asyncio
@pytest.mark.p2
async def test_xquik_generate_full_sync_omits_lower_bound(monkeypatch):
    captured = {}
    fake_connector = _FakeXquikConnector()

    def _from_config(config, **kwargs):
        captured.update(kwargs)
        return fake_connector

    monkeypatch.setattr(sync_data_source.XquikConnector, "from_config", _from_config)
    task = {
        **_make_task(),
        "reindex": "1",
        "skip_connection_log": True,
    }

    generator = await sync_data_source.Xquik({"query": "ragflow"})._generate(task)

    assert list(generator) == [["full"]]
    assert captured["since_time"] is None
    assert captured["until_time"].tzinfo == timezone.utc
    assert fake_connector.loaded is True


@pytest.mark.asyncio
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


@pytest.mark.asyncio
@pytest.mark.p2
async def test_rdbms_cursor_does_not_persist_when_parse_returns_errors(monkeypatch):
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
        lambda *_args, **_kwargs: (["parse error"], ["parsed-doc-id"]),
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


@pytest.mark.asyncio
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


class _FakeBigQueryConnector:
    instance = None

    def __init__(
        self,
        project_id,
        dataset_id=None,
        table_id=None,
        location=None,
        query="",
        content_columns="",
        metadata_columns=None,
        id_column=None,
        timestamp_column=None,
        batch_size=2,
        page_size=1000,
        maximum_bytes_billed=None,
        job_timeout_ms=None,
        use_query_cache=True,
    ):
        self.project_id = project_id
        self.dataset_id = dataset_id
        self.table_id = table_id
        self.query = query
        self.content_columns = content_columns
        self.timestamp_column = timestamp_column
        self.batch_size = batch_size
        self.load_from_state_called = False
        self.load_from_cursor_range_called = False
        self.retrieve_all_slim_docs_perm_sync_called = False
        self.prepare_sync_state_called = False
        self.persist_sync_state_called = False
        self._pending_sync_cursor_value = None
        _FakeBigQueryConnector.instance = self

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
        yield [types.SimpleNamespace(id="bq-row-1")]

    def load_from_state(self):
        self.load_from_state_called = True
        return iter((["full-sync"],))

    def load_from_cursor_range(self, start_value=None, start_id=None, end_value=None):
        self.load_from_cursor_range_called = True
        return iter(([_make_fake_doc("bq-incremental-doc")],))

    def persist_sync_state(self):
        self.persist_sync_state_called = True


def _bigquery_conf(**overrides):
    conf = {
        "project_id": "proj",
        "dataset_id": "ds",
        "table_id": "tbl",
        "content_columns": "name",
        "credentials": {"service_account_json": "{}"},
        "sync_deleted_files": False,
    }
    conf.update(overrides)
    return conf


@pytest.mark.asyncio
@pytest.mark.p2
async def test_bigquery_generate_full_sync_on_first_run(monkeypatch):
    monkeypatch.setattr(sync_data_source, "BigQueryConnector", _FakeBigQueryConnector)

    task = {
        **_make_task(),
        "reindex": "0",
        "poll_range_start": None,
        "skip_connection_log": True,
    }
    sync = sync_data_source.BigQuery(_bigquery_conf())

    document_generator = await sync._generate(task)
    connector = _FakeBigQueryConnector.instance

    assert connector is not None
    assert connector.prepare_sync_state_called is True
    assert connector.load_from_state_called is True
    assert connector.load_from_cursor_range_called is False
    assert list(document_generator) == [["full-sync"]]


@pytest.mark.asyncio
@pytest.mark.p2
async def test_bigquery_generate_incremental_cursor_path(monkeypatch):
    monkeypatch.setattr(sync_data_source, "BigQueryConnector", _FakeBigQueryConnector)

    task = {
        **_make_task(),
        "reindex": "0",
        "poll_range_start": datetime(2026, 1, 1, tzinfo=timezone.utc),
        "skip_connection_log": True,
    }
    sync = sync_data_source.BigQuery(_bigquery_conf(timestamp_column="updated_at"))

    document_generator = await sync._generate(task)
    connector = _FakeBigQueryConnector.instance

    assert connector is not None
    assert connector.load_from_cursor_range_called is True
    assert connector.load_from_state_called is False
    assert [doc.id for doc in list(document_generator)[0]] == ["bq-incremental-doc"]


@pytest.mark.asyncio
@pytest.mark.p2
async def test_bigquery_cursor_persists_only_after_success(monkeypatch):
    monkeypatch.setattr(sync_data_source, "BigQueryConnector", _FakeBigQueryConnector)
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
    sync = sync_data_source.BigQuery(_bigquery_conf(timestamp_column="updated_at"))

    await sync._run_task_logic(task)

    connector = _FakeBigQueryConnector.instance
    assert connector is not None
    assert connector.persist_sync_state_called is True


@pytest.mark.asyncio
@pytest.mark.p2
async def test_bigquery_cursor_does_not_persist_when_parse_returns_errors(monkeypatch):
    monkeypatch.setattr(sync_data_source, "BigQueryConnector", _FakeBigQueryConnector)
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
        lambda *_args, **_kwargs: (["parse error"], ["parsed-doc-id"]),
    )

    task = {
        **_make_task(),
        "reindex": "0",
        "poll_range_start": datetime(2026, 1, 1, tzinfo=timezone.utc),
        "skip_connection_log": True,
    }
    sync = sync_data_source.BigQuery(_bigquery_conf(timestamp_column="updated_at"))

    await sync._run_task_logic(task)

    connector = _FakeBigQueryConnector.instance
    assert connector is not None
    assert connector.persist_sync_state_called is False


@pytest.mark.asyncio
@pytest.mark.p2
async def test_bigquery_cursor_does_not_persist_when_batch_is_skipped(monkeypatch):
    monkeypatch.setattr(sync_data_source, "BigQueryConnector", _FakeBigQueryConnector)
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
    sync = sync_data_source.BigQuery(_bigquery_conf(timestamp_column="updated_at"))

    await sync._run_task_logic(task)

    connector = _FakeBigQueryConnector.instance
    assert connector is not None
    assert connector.persist_sync_state_called is False


@pytest.mark.asyncio
@pytest.mark.p2
async def test_bigquery_collect_prune_snapshot_when_enabled(monkeypatch):
    monkeypatch.setattr(sync_data_source, "BigQueryConnector", _FakeBigQueryConnector)

    task = {
        **_make_task(),
        "reindex": "0",
        "poll_range_start": None,
        "skip_connection_log": True,
    }
    sync = sync_data_source.BigQuery(_bigquery_conf(sync_deleted_files=True))

    await sync._generate(task)
    file_list = sync._collect_prune_snapshot(task)
    connector = _FakeBigQueryConnector.instance

    assert connector.retrieve_all_slim_docs_perm_sync_called is True
    assert [doc.id for doc in file_list] == ["bq-row-1"]


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


@pytest.mark.asyncio
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

    document_generator = await sync._generate(task)
    connector = _FakeDropboxConnector.instance

    assert list(document_generator) == [["poll-sync"]]
    file_list = sync._collect_prune_snapshot(task)
    assert [doc.id for doc in file_list] == ["dropbox:id-1", "dropbox:id-2"]
    assert connector.credentials == {"dropbox_access_token": "token-1"}
    assert connector.retrieve_all_slim_docs_perm_sync_called is True
    assert connector.snapshot_called_before_poll is False
    assert connector.poll_source_call[0] == poll_start.timestamp()
    assert connector.poll_source_call[1] >= poll_start.timestamp()


@pytest.mark.asyncio
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

    document_generator = await sync._generate(task)
    connector = _FakeDropboxConnector.instance

    assert list(document_generator) == [["full-sync"]]
    assert connector.load_from_state_called is True
    file_list = sync._collect_prune_snapshot(task)
    assert [doc.id for doc in file_list] == ["dropbox:id-1", "dropbox:id-2"]
    assert connector.retrieve_all_slim_docs_perm_sync_called is True
    assert connector.poll_source_called is False


def test_index_batch_size_env_helpers_use_defaults_for_invalid_values(monkeypatch):
    from common.data_source import config as data_source_config

    monkeypatch.delenv("RAGFLOW_TEST_INDEX_BATCH_SIZE", raising=False)
    assert data_source_config._env_int("RAGFLOW_TEST_INDEX_BATCH_SIZE", 2) == 2
    monkeypatch.setenv("RAGFLOW_TEST_INDEX_BATCH_SIZE", "32")
    assert data_source_config._env_int("RAGFLOW_TEST_INDEX_BATCH_SIZE", 2) == 32
    monkeypatch.setenv("RAGFLOW_TEST_INDEX_BATCH_SIZE", "0")
    assert data_source_config._env_int("RAGFLOW_TEST_INDEX_BATCH_SIZE", 2) == 2
    monkeypatch.setenv("RAGFLOW_TEST_INDEX_BATCH_SIZE", "nope")
    assert data_source_config._env_int("RAGFLOW_TEST_INDEX_BATCH_SIZE", 2) == 2

    monkeypatch.delenv("RAGFLOW_TEST_SYNC_PAUSE", raising=False)
    assert data_source_config._env_float("RAGFLOW_TEST_SYNC_PAUSE", 0.0) == 0.0
    monkeypatch.setenv("RAGFLOW_TEST_SYNC_PAUSE", "0.05")
    assert data_source_config._env_float("RAGFLOW_TEST_SYNC_PAUSE", 0.0) == 0.05
    monkeypatch.setenv("RAGFLOW_TEST_SYNC_PAUSE", "-1")
    assert data_source_config._env_float("RAGFLOW_TEST_SYNC_PAUSE", 0.0) == 0.0
    monkeypatch.setenv("RAGFLOW_TEST_SYNC_PAUSE", "inf")
    assert data_source_config._env_float("RAGFLOW_TEST_SYNC_PAUSE", 0.0) == 0.0
    monkeypatch.setenv("RAGFLOW_TEST_SYNC_PAUSE", "nan")
    assert data_source_config._env_float("RAGFLOW_TEST_SYNC_PAUSE", 0.0) == 0.0
    monkeypatch.setenv("RAGFLOW_TEST_SYNC_PAUSE", "-inf")
    assert data_source_config._env_float("RAGFLOW_TEST_SYNC_PAUSE", 0.0) == 0.0


def test_redact_url_strips_credentials_query_and_fragment():
    _redact_url = sync_data_source._redact_url

    assert _redact_url("https://user:pass@tfs.corp.local:8080/tfs/DefaultCollection?test=1#frag") == "https://tfs.corp.local:8080/tfs/DefaultCollection"
    assert _redact_url("http://user:pass@host/path") == "http://host/path"
    assert _redact_url(None) == ""
    assert _redact_url("") == ""
    assert _redact_url("organization(myorg)") == "organization(myorg)"
    assert _redact_url("http://[invalid") == "<invalid URL>"


# Bytes 0..31 and a value encrypted with them; the Go tests use the same pair.
_CONNECTOR_KEY = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
_ENCRYPTED_CREDENTIALS = "enc:v1:ZGVmZ2hpamtsbW5vMzm/FhC2IvFVBzHK4EVIiS2pKzu5X9Feh/PZO57Rh3K0yyGkLTDmruUEeZB6LtRoLedehJBJUg=="


class _CredentialsProbe(sync_data_source.SyncBase):
    SOURCE_NAME = "probe"

    def __init__(self, conf):
        super().__init__(conf)
        self.seen_credentials = None

    async def _run_task_logic(self, task: dict):
        self.seen_credentials = self.conf["credentials"]


def _record_sync_log_calls(monkeypatch):
    calls = {"update_by_id": [], "schedule": []}
    monkeypatch.setattr(sync_data_source.SyncLogsService, "start", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(sync_data_source.SyncLogsService, "update_by_id", lambda task_id, fields: calls["update_by_id"].append((task_id, fields)))
    monkeypatch.setattr(sync_data_source.SyncLogsService, "schedule", lambda *args, **_kwargs: calls["schedule"].append(args))
    return calls


@pytest.mark.asyncio
@pytest.mark.p2
async def test_call_decrypts_credentials_before_the_task_runs(monkeypatch):
    monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", _CONNECTOR_KEY)
    calls = _record_sync_log_calls(monkeypatch)
    sync = _CredentialsProbe({"credentials": _ENCRYPTED_CREDENTIALS})

    await sync({**_make_task(), "timeout_secs": 60})

    assert sync.seen_credentials == {"api_token": "tok-123", "user": "ada"}
    assert calls["update_by_id"] == []
    assert len(calls["schedule"]) == 1


@pytest.mark.asyncio
@pytest.mark.p2
async def test_call_fails_the_task_when_credentials_cannot_be_decrypted(monkeypatch):
    monkeypatch.delenv("RAGFLOW_CONNECTOR_KEY", raising=False)
    calls = _record_sync_log_calls(monkeypatch)
    sync = _CredentialsProbe({"credentials": _ENCRYPTED_CREDENTIALS})

    await sync({**_make_task(), "timeout_secs": 60})

    assert sync.seen_credentials is None
    [(task_id, fields)] = calls["update_by_id"]
    assert task_id == "task-1"
    assert fields["status"] == sync_data_source.TaskStatus.FAIL
    assert fields["error_msg"] == "connector credentials are encrypted but RAGFLOW_CONNECTOR_KEY is not set"
    assert calls["schedule"] == []
