import importlib.util
import json as stdlib_json
import sys
from datetime import UTC, datetime
from pathlib import Path
from types import ModuleType, SimpleNamespace
from typing import ClassVar

import pytest


class _DynamicValuesMeta(type):
    def __getattr__(cls, name):
        return name.lower()


class _DynamicValues(metaclass=_DynamicValuesMeta):
    pass


class _DummyConnector:
    pass


class _ConnectorService:
    pass


class _DocumentService:
    @staticmethod
    def list_doc_headers_by_kb_and_source_type(*_args, **_kwargs):
        return []


class _KnowledgebaseService:
    @staticmethod
    def get_by_id(*_args, **_kwargs):
        return True, object()


class _SyncLogsService:
    starts: ClassVar[list] = []
    updates: ClassVar[list] = []
    schedules: ClassVar[list] = []
    done_calls: ClassVar[list] = []
    sink_error: ClassVar[Exception | None] = None
    parse_errors: ClassVar[list[str]] = []

    @classmethod
    def reset(cls):
        cls.starts.clear()
        cls.updates.clear()
        cls.schedules.clear()
        cls.done_calls.clear()
        cls.sink_error = None
        cls.parse_errors.clear()

    @classmethod
    def start(cls, *args):
        cls.starts.append(args)

    @classmethod
    def update_by_id(cls, *args):
        cls.updates.append(args)

    @classmethod
    def schedule(cls, *args, **kwargs):
        cls.schedules.append((args, kwargs))

    @classmethod
    def done(cls, *args):
        cls.done_calls.append(args)

    @classmethod
    def duplicate_and_parse(cls, *_args, **_kwargs):
        if cls.sink_error is not None:
            raise cls.sink_error
        return cls.parse_errors, ["stored-doc-id"]

    @staticmethod
    def increase_docs(*_args, **_kwargs):
        return None


def _module(name, **attributes):
    module = ModuleType(name)
    for key, value in attributes.items():
        setattr(module, key, value)
    return module


def _load_sync_module():
    connector_names = [
        "BlobStorageConnector",
        "RSSConnector",
        "SitemapConnector",
        "NotionConnector",
        "DiscordConnector",
        "GoogleDriveConnector",
        "MoodleConnector",
        "JiraConnector",
        "DropboxConnector",
        "AirtableConnector",
        "AsanaConnector",
        "ImapConnector",
        "ZendeskConnector",
        "SeaFileConnector",
        "RDBMSConnector",
        "BigQueryConnector",
        "DingTalkAITableConnector",
        "RestAPIConnector",
        "XquikConnector",
        "OneDriveConnector",
        "OutlookConnector",
        "AzureBlobConnector",
        "SalesforceConnector",
        "TeamsConnector",
        "SlackConnector",
        "SharePointConnector",
    ]
    data_source_module = _module(
        "common.data_source",
        **{name: _DummyConnector for name in connector_names},
    )

    stubs = {
        "flask": _module("flask", json=stdlib_json),
        "api": _module("api"),
        "api.db": _module("api.db"),
        "api.db.services": _module("api.db.services"),
        "api.db.services.connector_service": _module(
            "api.db.services.connector_service",
            ConnectorService=_ConnectorService,
            SyncLogsService=_SyncLogsService,
            resolve_connector_doc_id=lambda _kb, _connector, external_id, _existing: external_id,
        ),
        "api.db.services.document_service": _module(
            "api.db.services.document_service",
            DocumentService=_DocumentService,
        ),
        "api.db.services.knowledgebase_service": _module(
            "api.db.services.knowledgebase_service",
            KnowledgebaseService=_KnowledgebaseService,
        ),
        "common": _module("common", settings=SimpleNamespace()),
        "common.constants": _module(
            "common.constants",
            ConnectorTaskType=_DynamicValues,
            FileSource=_DynamicValues,
            TaskStatus=_DynamicValues,
        ),
        "common.config_utils": _module("common.config_utils", show_configs=lambda: None),
        "common.data_source": data_source_module,
        "common.data_source.config": _module("common.data_source.config", INDEX_BATCH_SIZE=2),
        "common.data_source.models": _module(
            "common.data_source.models",
            ConnectorFailure=type("ConnectorFailure", (), {}),
            SeafileSyncScope=_DynamicValues,
        ),
        "common.data_source.webdav_connector": _module("common.data_source.webdav_connector", WebDAVConnector=_DummyConnector),
        "common.data_source.confluence_connector": _module("common.data_source.confluence_connector", ConfluenceConnector=_DummyConnector),
        "common.data_source.gmail_connector": _module("common.data_source.gmail_connector", GmailConnector=_DummyConnector),
        "common.data_source.box_connector": _module("common.data_source.box_connector", BoxConnector=_DummyConnector),
        "common.data_source.github": _module("common.data_source.github"),
        "common.data_source.github.connector": _module("common.data_source.github.connector", GithubConnector=_DummyConnector),
        "common.data_source.gitlab_connector": _module("common.data_source.gitlab_connector", GitlabConnector=_DummyConnector),
        "common.data_source.bitbucket": _module("common.data_source.bitbucket"),
        "common.data_source.bitbucket.connector": _module("common.data_source.bitbucket.connector", BitbucketConnector=_DummyConnector),
        "common.data_source.azure_devops": _module("common.data_source.azure_devops"),
        "common.data_source.azure_devops.connector": _module("common.data_source.azure_devops.connector", AzureDevOpsConnector=_DummyConnector),
        "common.data_source.interfaces": _module(
            "common.data_source.interfaces",
            CheckpointOutputWrapper=type("CheckpointOutputWrapper", (), {}),
        ),
        "common.data_source.sitemap_connector": _module(
            "common.data_source.sitemap_connector",
            iter_in_worker_thread=lambda value, **_kwargs: value,
            validate_connector_in_thread=lambda *_args, **_kwargs: None,
        ),
        "common.data_source.exceptions": _module(
            "common.data_source.exceptions",
            ConnectorValidationError=type("ConnectorValidationError", (Exception,), {}),
        ),
        "common.log_utils": _module("common.log_utils", init_root_logger=lambda *_args: None),
        "common.signal_utils": _module(
            "common.signal_utils",
            start_tracemalloc_and_snapshot=lambda *_args: None,
            stop_tracemalloc=lambda *_args: None,
        ),
        "common.versions": _module("common.versions", get_ragflow_version=lambda: "test"),
        "rag.svr.feishu_wiki_sync": _module(
            "rag.svr.feishu_wiki_sync",
            build_feishu_wiki_generator=lambda *_args, **_kwargs: (_DummyConnector(), iter(())),
        ),
        "box_sdk_gen": _module(
            "box_sdk_gen",
            BoxOAuth=_DummyConnector,
            OAuthConfig=_DummyConnector,
            AccessToken=_DummyConnector,
        ),
    }
    saved_modules = {name: sys.modules.get(name) for name in stubs}
    sys.modules.update(stubs)
    try:
        path = Path(__file__).resolve().parents[3] / "rag" / "svr" / "sync_data_source.py"
        spec = importlib.util.spec_from_file_location("_feishu_sync_owner_under_test", path)
        module = importlib.util.module_from_spec(spec)
        assert spec.loader is not None
        spec.loader.exec_module(module)
        return module
    finally:
        for name, saved in saved_modules.items():
            if saved is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = saved


sync_data_source = _load_sync_module()


class _FakeSync(sync_data_source.SyncBase):
    SOURCE_NAME = "other"

    def __init__(self, batches):
        super().__init__({})
        self.batches = batches

    async def _generate(self, task):
        del task
        return iter(self.batches)


class _FakeFeishuSync(sync_data_source.FeishuWiki):
    def __init__(self, batches, *, window_end=None):
        super().__init__({})
        self.batches = batches
        self._poll_window_end = window_end

    async def _generate(self, task):
        del task
        return iter(self.batches)


def _doc(updated_at):
    return SimpleNamespace(
        id="external-doc-id",
        semantic_identifier="guide.pdf",
        extension=".pdf",
        size_bytes=4,
        doc_updated_at=updated_at,
        blob=b"data",
        metadata=None,
        fingerprint="fingerprint",
    )


def _task(*, poll_range_start=None):
    return {
        "id": "task-1",
        "connector_id": "connector-1",
        "kb_id": "kb-1",
        "tenant_id": "tenant-1",
        "poll_range_start": poll_range_start,
        "auto_parse": False,
        "reindex": "0",
        "timeout_secs": 10,
        "task_type": sync_data_source.ConnectorTaskType.SYNC,
    }


@pytest.fixture(autouse=True)
def _reset_sync_log_service():
    _SyncLogsService.reset()


@pytest.mark.asyncio
async def test_feishu_sink_failure_marks_task_failed_and_does_not_schedule():
    previous_cursor = datetime(2026, 1, 1, tzinfo=UTC)
    task = _task(poll_range_start=previous_cursor)
    _SyncLogsService.sink_error = RuntimeError("sink unavailable")

    await _FakeFeishuSync([[_doc(datetime(2026, 1, 2, tzinfo=UTC))]])(task)

    assert _SyncLogsService.done_calls == []
    assert _SyncLogsService.schedules == []
    assert _SyncLogsService.updates[-1][1]["status"] == sync_data_source.TaskStatus.FAIL


@pytest.mark.asyncio
async def test_feishu_reported_parse_error_marks_task_failed_and_does_not_schedule():
    task = _task(poll_range_start=datetime(2026, 1, 1, tzinfo=UTC))
    _SyncLogsService.parse_errors.append("parser rejected file")

    await _FakeFeishuSync([[_doc(datetime(2026, 1, 2, tzinfo=UTC))]])(task)

    assert _SyncLogsService.done_calls == []
    assert _SyncLogsService.schedules == []
    assert _SyncLogsService.updates[-1][1]["status"] == sync_data_source.TaskStatus.FAIL


@pytest.mark.asyncio
async def test_feishu_success_completes_and_schedules_at_document_cursor():
    updated_at = datetime(2026, 1, 2, tzinfo=UTC)
    task = _task(poll_range_start=datetime(2026, 1, 1, tzinfo=UTC))

    await _FakeFeishuSync([[_doc(updated_at)]])(task)

    assert _SyncLogsService.done_calls == [("task-1", "connector-1")]
    assert _SyncLogsService.updates == []
    assert _SyncLogsService.schedules[0][0][2] == updated_at


@pytest.mark.asyncio
async def test_default_source_preserves_skip_and_continue_behavior_on_sink_failure():
    task = _task(poll_range_start=datetime(2026, 1, 1, tzinfo=UTC))
    _SyncLogsService.sink_error = RuntimeError("sink unavailable")

    await _FakeSync([[_doc(datetime(2026, 1, 2, tzinfo=UTC))]])(task)

    assert _SyncLogsService.updates == []
    assert _SyncLogsService.done_calls == [("task-1", "connector-1")]
    assert len(_SyncLogsService.schedules) == 1


@pytest.mark.asyncio
async def test_empty_feishu_poll_schedules_the_completed_window_end():
    previous_cursor = datetime(2026, 1, 1, tzinfo=UTC)
    window_end = datetime(2026, 1, 2, tzinfo=UTC)
    task = _task(poll_range_start=previous_cursor)

    await _FakeFeishuSync([], window_end=window_end)(task)

    assert _SyncLogsService.done_calls == [("task-1", "connector-1")]
    assert _SyncLogsService.schedules[0][0][2] == window_end
