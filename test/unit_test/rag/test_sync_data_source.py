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
import faulthandler
import importlib
import importlib.util
import os
import runpy
import sys
import types
import warnings
from datetime import datetime, timezone
from pathlib import Path

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
_ROOT = Path(__file__).resolve().parents[3]


_EXPECTED_CONNECTOR_FACTORY = {
    "rss": "RSS",
    "s3": "S3",
    "r2": "R2",
    "oci_storage": "OCI_STORAGE",
    "google_cloud_storage": "GOOGLE_CLOUD_STORAGE",
    "notion": "Notion",
    "discord": "Discord",
    "confluence": "Confluence",
    "eva_wiki": "EvaWiki",
    "openmetadata": "OpenMetadata",
    "gmail": "Gmail",
    "google_drive": "GoogleDrive",
    "jira": "Jira",
    "sharepoint": "SharePoint",
    "onedrive": "OneDrive",
    "outlook": "Outlook",
    "azure_blob": "AzureBlob",
    "salesforce": "Salesforce",
    "slack": "Slack",
    "teams": "Teams",
    "moodle": "Moodle",
    "dropbox": "Dropbox",
    "webdav": "WebDAV",
    "box": "BOX",
    "airtable": "Airtable",
    "asana": "Asana",
    "imap": "IMAP",
    "zendesk": "Zendesk",
    "github": "Github",
    "gitlab": "Gitlab",
    "bitbucket": "Bitbucket",
    "seafile": "SeaFile",
    "mysql": "MySQL",
    "postgresql": "PostgreSQL",
    "bigquery": "BigQuery",
    "dingtalk_ai_table": "DingTalkAITable",
    "rest_api": "REST_API",
}


def _connector_factory_inventory(factory):
    return {source.value: handler.__name__ for source, handler in factory.items()}


def _assert_connector_factory(factory):
    assert _connector_factory_inventory(factory) == _EXPECTED_CONNECTOR_FACTORY


def test_connector_factory_inventory_is_exact():
    _assert_connector_factory(sync_data_source.func_factory)


def test_connector_factory_contract_rejects_missing_handler():
    mutated = dict(sync_data_source.func_factory)
    mutated.pop(sync_data_source.FileSource.OPENMETADATA)

    with pytest.raises(AssertionError):
        _assert_connector_factory(mutated)


@pytest.mark.asyncio
async def test_sync_worker_main_lifecycle_is_exact(monkeypatch):
    events = []

    class StopEvent:
        checks = 0

        def is_set(self):
            self.checks += 1
            return self.checks > 1

    async def dispatch_tasks():
        events.append(("dispatch",))

    fake_signal = types.SimpleNamespace(
        SIGINT="SIGINT",
        SIGTERM="SIGTERM",
        signal=lambda signal_name, handler: events.append(("signal", signal_name, handler.__name__)),
    )
    settings = types.SimpleNamespace(init_settings=lambda: events.append(("settings_init",)))
    monkeypatch.setattr(sync_data_source.sys, "platform", "win32")
    monkeypatch.setattr(sync_data_source, "signal", fake_signal)
    monkeypatch.setattr(sync_data_source, "settings", settings)
    monkeypatch.setattr(sync_data_source, "stop_event", StopEvent())
    monkeypatch.setattr(sync_data_source, "dispatch_tasks", dispatch_tasks)
    monkeypatch.setattr(sync_data_source, "get_ragflow_version", lambda: events.append(("version",)) or "test-version")
    monkeypatch.setattr(sync_data_source, "show_configs", lambda: events.append(("show_configs",)))

    await sync_data_source.main()

    assert events == [
        ("version",),
        ("show_configs",),
        ("settings_init",),
        ("signal", "SIGINT", "signal_handler"),
        ("signal", "SIGTERM", "signal_handler"),
        ("dispatch",),
    ]


def test_sync_worker_main_module_handoff_is_exact(monkeypatch):
    events = []

    def fake_asyncio_run(coroutine):
        events.append(("asyncio.run", coroutine.cr_code.co_name))
        coroutine.close()

    monkeypatch.setattr(sys, "argv", ["sync_data_source.py", "7"])
    monkeypatch.setattr(faulthandler, "enable", lambda: events.append(("faulthandler",)))
    monkeypatch.setattr(asyncio, "run", fake_asyncio_run)
    monkeypatch.setattr(sys.modules["common.log_utils"], "init_root_logger", lambda name: events.append(("root_logger", name)))

    namespace = runpy.run_path(str(_ROOT / "rag" / "svr" / "sync_data_source.py"), run_name="__main__")

    assert namespace["CONSUMER_NO"] == "7"
    assert namespace["CONSUMER_NAME"] == "data_sync_7"
    assert events == [
        ("faulthandler",),
        ("root_logger", "data_sync_7"),
        ("asyncio.run", "main"),
    ]


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


class _FakeOpenMetadataConnector:
    instance = None

    DEFAULT_MAX_ENTITIES = 5000
    DEFAULT_TIMEOUT_SECONDS = 12
    DEFAULT_RETRY_COUNT = 2

    def __init__(self, **kwargs):
        self.kwargs = kwargs
        self.batch_size = kwargs.get("batch_size", 2)
        self.credentials = None
        self.validated = False
        self.fetched = []
        _FakeOpenMetadataConnector.instance = self

    def load_credentials(self, credentials):
        self.credentials = credentials

    def validate_connector_settings(self):
        self.validated = True

    def list_keys(self):
        yield types.SimpleNamespace(key="unchanged", fingerprint="a" * 32, deleted=False)
        yield types.SimpleNamespace(key="changed", fingerprint="b" * 32, deleted=False)

    def get_value(self, key):
        self.fetched.append(key)
        return _make_fake_doc(key)

    def load_from_state(self):
        return iter(([_make_fake_doc("full")],))

    def retrieve_all_slim_docs_perm_sync(self, callback=None):
        del callback
        yield [types.SimpleNamespace(id="unchanged"), types.SimpleNamespace(id="changed")]


@pytest.mark.asyncio
@pytest.mark.p2
async def test_openmetadata_uses_fingerprint_bypass_and_requires_private_dataset(monkeypatch):
    monkeypatch.setattr(sync_data_source, "OpenMetadataConnector", _FakeOpenMetadataConnector)
    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: (True, types.SimpleNamespace(permission="me")),
    )
    monkeypatch.setattr(
        sync_data_source.DocumentService,
        "list_id_content_hash_map_by_kb_and_source_type",
        lambda *_args, **_kwargs: {
            sync_data_source.hash128("kb-1:connector-1:unchanged"): "a" * 32,
        },
    )
    task = {**_make_task(), "reindex": "0", "skip_connection_log": True}
    sync = sync_data_source.OpenMetadata(
        {
            "base_url": "http://omd:8585",
            "credentials": {"openmetadata_jwt_token": "secret"},
        }
    )

    batches = list(await sync._generate(task))

    assert [[doc.id for doc in batch] for batch in batches] == [["changed"]]
    assert _FakeOpenMetadataConnector.instance.fetched == ["changed"]
    assert _FakeOpenMetadataConnector.instance.validated is True

    monkeypatch.setattr(
        sync_data_source.KnowledgebaseService,
        "get_by_id",
        lambda *_args, **_kwargs: (True, types.SimpleNamespace(permission="team")),
    )
    with pytest.raises(sync_data_source.ConnectorValidationError, match="private knowledge base"):
        await sync_data_source.OpenMetadata({"base_url": "http://omd:8585"})._generate(task)


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
async def test_failed_sync_is_rescheduled_without_advancing_cursor(monkeypatch):
    original_cursor = datetime(2026, 1, 1, tzinfo=timezone.utc)
    task = {
        **_make_task(),
        "poll_range_start": original_cursor,
        "timeout_secs": 30,
        "reindex": "0",
        "total_docs_indexed": 7,
    }
    scheduled = []
    task_updates = []
    connector_updates = []

    monkeypatch.setattr(sync_data_source.SyncLogsService, "start", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(sync_data_source.SyncLogsService, "done", lambda *_args, **_kwargs: pytest.fail("failed task must not be marked done"))
    monkeypatch.setattr(sync_data_source.SyncLogsService, "update_by_id", lambda *args, **kwargs: task_updates.append((args, kwargs)))
    monkeypatch.setattr(sync_data_source.SyncLogsService, "schedule", lambda *args, **kwargs: scheduled.append((args, kwargs)))
    monkeypatch.setattr(sync_data_source.ConnectorService, "update_by_id", lambda *args, **kwargs: connector_updates.append((args, kwargs)))
    monkeypatch.setattr(sync_data_source.DocumentService, "list_doc_headers_by_kb_and_source_type", lambda *_args, **_kwargs: [])
    monkeypatch.setattr(sync_data_source.KnowledgebaseService, "get_by_id", lambda *_args, **_kwargs: (True, object()))
    monkeypatch.setattr(sync_data_source.SyncLogsService, "duplicate_and_parse", lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("batch failed")))

    await _FakeSync(iter(([_make_fake_doc(updated_at=datetime(2026, 2, 1, tzinfo=timezone.utc))],)))(task)

    assert task["poll_range_start"] == original_cursor
    assert task_updates[-1][0][1]["status"] == sync_data_source.TaskStatus.FAIL
    assert connector_updates[-1][0] == ("connector-1", {"status": sync_data_source.TaskStatus.SCHEDULE})
    assert scheduled == [
        (
            ("connector-1", "kb-1", original_cursor),
            {
                "reindex": False,
                "total_docs_indexed": 7,
                "task_type": sync_data_source.ConnectorTaskType.SYNC,
            },
        )
    ]


@pytest.mark.asyncio
@pytest.mark.p2
async def test_run_prune_task_logic_cleans_up_for_empty_snapshot(monkeypatch):
    cleanup_calls = []

    _patch_common_dependencies(monkeypatch)

    def _fake_cleanup(task_id, connector_id, kb_id, tenant_id, file_batches, **kwargs):
        file_list = [file for batch in file_batches for file in batch]
        cleanup_calls.append(((task_id, connector_id, kb_id, tenant_id, file_list), kwargs))
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

    def _fake_cleanup(task_id, connector_id, kb_id, tenant_id, file_batches, **kwargs):
        file_list = [file for batch in file_batches for file in batch]
        cleanup_calls.append(((task_id, connector_id, kb_id, tenant_id, file_list), kwargs))
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

    document_generator = await sync._generate(task)
    connector = _FakeRDBMSConnector.instance

    assert connector is not None
    assert connector.load_from_state_called is True
    assert connector.load_from_cursor_range_called is False
    file_batches = sync._collect_prune_snapshot(task)
    assert file_batches is not None
    assert [doc.id for batch in file_batches for doc in batch] == ["row-1"]
    assert connector.retrieve_all_slim_docs_perm_sync_called is True
    assert list(document_generator) == [["full-sync"]]


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

    with pytest.raises(RuntimeError, match="skipped=1"):
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

    with pytest.raises(RuntimeError, match="skipped=1"):
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

    with pytest.raises(RuntimeError, match="skipped=1"):
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

    with pytest.raises(RuntimeError, match="skipped=1"):
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
    file_batches = sync._collect_prune_snapshot(task)
    connector = _FakeBigQueryConnector.instance

    assert [doc.id for batch in file_batches for doc in batch] == ["bq-row-1"]
    assert connector.retrieve_all_slim_docs_perm_sync_called is True


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
    file_batches = sync._collect_prune_snapshot(task)
    assert [doc.id for batch in file_batches for doc in batch] == ["dropbox:id-1", "dropbox:id-2"]
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
    file_batches = sync._collect_prune_snapshot(task)
    assert [doc.id for batch in file_batches for doc in batch] == ["dropbox:id-1", "dropbox:id-2"]
    assert connector.retrieve_all_slim_docs_perm_sync_called is True
    assert connector.poll_source_called is False
