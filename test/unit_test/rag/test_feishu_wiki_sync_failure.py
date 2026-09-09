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


import common
from common.constants import ConnectorTaskType, FileSource, TaskStatus

rag_svr = importlib.import_module("rag.svr")
_MISSING = object()


def _load_sync_stack() -> SimpleNamespace:
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
    exact_names = {*stubs, "rag.svr.feishu_wiki_sync", "rag.svr.sync_data_source"}
    saved_exact = {name: sys.modules[name] for name in exact_names if name in sys.modules}
    saved_data_source = {name: module for name, module in sys.modules.items() if name == "common.data_source" or name.startswith("common.data_source.")}
    saved_attrs = {
        (common, "config_utils"): getattr(common, "config_utils", _MISSING),
        (common, "data_source"): getattr(common, "data_source", _MISSING),
        (common, "settings"): getattr(common, "settings", _MISSING),
        (rag_svr, "feishu_wiki_sync"): getattr(rag_svr, "feishu_wiki_sync", _MISSING),
        (rag_svr, "sync_data_source"): getattr(rag_svr, "sync_data_source", _MISSING),
    }
    for name in list(sys.modules):
        if name in exact_names or name == "common.data_source" or name.startswith("common.data_source."):
            sys.modules.pop(name, None)
    for parent, attribute in saved_attrs:
        if hasattr(parent, attribute):
            delattr(parent, attribute)

    _install_unrelated_provider_stubs()
    common.settings = SimpleNamespace()
    sys.modules.update(stubs)
    try:
        registry = importlib.import_module("common.data_source")
        models = importlib.import_module("common.data_source.models")
        sync_owner = importlib.import_module("rag.svr.sync_data_source")
        return SimpleNamespace(registry=registry, Document=models.Document, sync_owner=sync_owner)
    finally:
        for name in list(sys.modules):
            if name in exact_names or name == "common.data_source" or name.startswith("common.data_source."):
                sys.modules.pop(name, None)
        sys.modules.update(saved_data_source)
        sys.modules.update(saved_exact)
        for (parent, attribute), value in saved_attrs.items():
            if value is _MISSING:
                if hasattr(parent, attribute):
                    delattr(parent, attribute)
            else:
                setattr(parent, attribute, value)


def _import_state() -> tuple[dict[str, ModuleType], tuple[object, ...]]:
    tracked_modules = {
        name: module
        for name, module in sys.modules.items()
        if name == "common.data_source" or name.startswith("common.data_source.") or name in {"rag.svr.feishu_wiki_sync", "rag.svr.sync_data_source"}
    }
    tracked_attrs = (
        getattr(common, "config_utils", _MISSING),
        getattr(common, "data_source", _MISSING),
        getattr(common, "settings", _MISSING),
        getattr(rag_svr, "feishu_wiki_sync", _MISSING),
        getattr(rag_svr, "sync_data_source", _MISSING),
    )
    return tracked_modules, tracked_attrs


def _restore_import_state(state: tuple[dict[str, ModuleType], tuple[object, ...]]) -> None:
    modules, attrs = state
    for name in list(sys.modules):
        if name == "common.data_source" or name.startswith("common.data_source.") or name in {"rag.svr.feishu_wiki_sync", "rag.svr.sync_data_source"}:
            sys.modules.pop(name, None)
    sys.modules.update(modules)
    for (parent, attribute), value in zip(
        (
            (common, "config_utils"),
            (common, "data_source"),
            (common, "settings"),
            (rag_svr, "feishu_wiki_sync"),
            (rag_svr, "sync_data_source"),
        ),
        attrs,
    ):
        if value is _MISSING:
            if hasattr(parent, attribute):
                delattr(parent, attribute)
        else:
            setattr(parent, attribute, value)


_IMPORT_STATE_BEFORE = _import_state()
_stack = _load_sync_stack()
_IMPORT_STATE_AFTER = _import_state()

CONNECTOR_BY_SOURCE = _stack.registry.CONNECTOR_BY_SOURCE
FeishuWikiConnector = _stack.registry.FeishuWikiConnector
Document = _stack.Document
sync_data_source = _stack.sync_owner


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
    assert _IMPORT_STATE_AFTER[0] == _IMPORT_STATE_BEFORE[0]
    assert all(after is before for after, before in zip(_IMPORT_STATE_AFTER[1], _IMPORT_STATE_BEFORE[1]))
    assert CONNECTOR_BY_SOURCE[FileSource.FEISHU_WIKI] is FeishuWikiConnector
    assert sync_data_source.func_factory[FileSource.FEISHU_WIKI] is sync_data_source.FeishuWiki
    assert sync_data_source.FileSource is FileSource
    assert sync_data_source.ConnectorTaskType is ConnectorTaskType
    assert sync_data_source.TaskStatus is TaskStatus
    assert isinstance(_doc(datetime(2026, 1, 2, tzinfo=UTC)), Document)

    entry_state = _import_state()
    late_child = ModuleType("common.data_source.imported_after_collection")
    sys.modules[late_child.__name__] = late_child
    try:
        execution_state = _import_state()
        sentinel_data_source = ModuleType("common.data_source")
        sentinel_sync_owner = ModuleType("rag.svr.sync_data_source")
        sys.modules["common.data_source"] = sentinel_data_source
        sys.modules["rag.svr.sync_data_source"] = sentinel_sync_owner
        common.data_source = sentinel_data_source
        rag_svr.sync_data_source = sentinel_sync_owner
        try:
            isolated = _load_sync_stack()
            assert isolated.registry.CONNECTOR_BY_SOURCE[FileSource.FEISHU_WIKI] is isolated.registry.FeishuWikiConnector
            assert isolated.sync_owner.func_factory[FileSource.FEISHU_WIKI] is isolated.sync_owner.FeishuWiki
            assert sys.modules["common.data_source"] is sentinel_data_source
            assert sys.modules["rag.svr.sync_data_source"] is sentinel_sync_owner
            assert common.data_source is sentinel_data_source
            assert rag_svr.sync_data_source is sentinel_sync_owner
        finally:
            _restore_import_state(execution_state)

        assert sys.modules[late_child.__name__] is late_child
    finally:
        _restore_import_state(entry_state)


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
