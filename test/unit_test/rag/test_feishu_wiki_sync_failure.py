import importlib
import json as stdlib_json
import sys
from datetime import UTC, datetime
from types import ModuleType, SimpleNamespace
from typing import ClassVar

import pytest


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


def _install_unrelated_provider_stubs() -> None:
    """Mock optional providers while retaining the real data-source package."""
    providers = {
        "airtable_connector": "AirtableConnector",
        "asana_connector": "AsanaConnector",
        "azure_blob_connector": "AzureBlobConnector",
        "bigquery_connector": "BigQueryConnector",
        "blob_connector": "BlobStorageConnector",
        "box_connector": "BoxConnector",
        "confluence_connector": "ConfluenceConnector",
        "dingtalk_ai_table_connector": "DingTalkAITableConnector",
        "discord_connector": "DiscordConnector",
        "dropbox_connector": "DropboxConnector",
        "gitlab_connector": "GitlabConnector",
        "gmail_connector": "GmailConnector",
        "imap_connector": "ImapConnector",
        "moodle_connector": "MoodleConnector",
        "notion_connector": "NotionConnector",
        "onedrive_connector": "OneDriveConnector",
        "outlook_connector": "OutlookConnector",
        "rdbms_connector": "RDBMSConnector",
        "rest_api_connector": "RestAPIConnector",
        "rss_connector": "RSSConnector",
        "salesforce_connector": "SalesforceConnector",
        "seafile_connector": "SeaFileConnector",
        "sharepoint_connector": "SharePointConnector",
        "sitemap_connector": "SitemapConnector",
        "slack_connector": "SlackConnector",
        "teams_connector": "TeamsConnector",
        "webdav_connector": "WebDAVConnector",
        "xquik_connector": "XquikConnector",
        "zendesk_connector": "ZendeskConnector",
        "azure_devops.connector": "AzureDevOpsConnector",
        "bitbucket.connector": "BitbucketConnector",
        "github.connector": "GithubConnector",
        "google_drive.connector": "GoogleDriveConnector",
        "jira.connector": "JiraConnector",
    }
    for relative_name, class_name in providers.items():
        module_name = f"common.data_source.{relative_name}"
        module = ModuleType(module_name)
        setattr(module, class_name, type(class_name, (), {}))
        if relative_name == "sitemap_connector":
            module.iter_in_worker_thread = lambda value, **_kwargs: value
            module.validate_connector_in_thread = lambda *_args, **_kwargs: None
        sys.modules[module_name] = module

        if "." in relative_name:
            package_name = module_name.rsplit(".", 1)[0]
            package = ModuleType(package_name)
            package.__path__ = []
            sys.modules[package_name] = package


_install_unrelated_provider_stubs()

import common
from common.constants import ConnectorTaskType, FileSource, TaskStatus
from common.data_source import CONNECTOR_BY_SOURCE, FeishuWikiConnector
from common.data_source.models import Document


def _load_sync_module():
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
        "common.config_utils": _module("common.config_utils", show_configs=lambda: None),
        "box_sdk_gen": _module(
            "box_sdk_gen",
            BoxOAuth=_DummyConnector,
            OAuthConfig=_DummyConnector,
            AccessToken=_DummyConnector,
        ),
    }
    saved_modules = {name: sys.modules.get(name) for name in stubs}
    previous_settings = getattr(common, "settings", None)
    common.settings = SimpleNamespace()
    sys.modules.update(stubs)
    try:
        return importlib.import_module("rag.svr.sync_data_source")
    finally:
        for name, saved in saved_modules.items():
            if saved is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = saved
        if previous_settings is None:
            delattr(common, "settings")
        else:
            common.settings = previous_settings


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
    return Document(
        id="external-doc-id",
        source=FileSource.FEISHU_WIKI,
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


def test_real_registries_and_models_reach_the_sync_owner():
    assert CONNECTOR_BY_SOURCE[FileSource.FEISHU_WIKI] is FeishuWikiConnector
    assert sync_data_source.func_factory[FileSource.FEISHU_WIKI] is sync_data_source.FeishuWiki
    assert sync_data_source.FileSource is FileSource
    assert sync_data_source.ConnectorTaskType is ConnectorTaskType
    assert sync_data_source.TaskStatus is TaskStatus
    assert isinstance(_doc(datetime(2026, 1, 2, tzinfo=UTC)), Document)


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
