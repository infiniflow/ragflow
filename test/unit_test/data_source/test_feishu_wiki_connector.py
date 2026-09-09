import hashlib
import sys
from datetime import UTC, datetime
from types import ModuleType

import pytest

from common.constants import FileSource


def _install_unrelated_provider_stubs() -> None:
    """Isolate optional providers while executing the real data-source registry."""
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
        sys.modules[module_name] = module

        if "." in relative_name:
            package_name = module_name.rsplit(".", 1)[0]
            package = ModuleType(package_name)
            package.__path__ = []
            sys.modules[package_name] = package


_install_unrelated_provider_stubs()

from common.data_source import CONNECTOR_BY_SOURCE, FeishuWikiConnector
from common.data_source.config import DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import Document
from common.data_source.utils import get_file_ext


class FakeResponse:
    def __init__(self, *, payload=None, content=b"", chunks=None, headers=None, status_code=200):
        self._payload = payload
        self.content = content
        self.chunks = chunks
        self.headers = headers or {}
        self.status_code = status_code
        self.closed = 0
        self.iterated = 0

    def json(self):
        return self._payload

    def iter_content(self, chunk_size):
        self.iterated += 1
        del chunk_size
        if self.chunks is not None:
            yield from self.chunks
        elif self.content:
            yield self.content

    def close(self):
        self.closed += 1


class FakeSession:
    def __init__(self, responses):
        self.responses = list(responses)
        self.requests = []
        self.downloaded_tokens = []

    def request(self, method, url, **kwargs):
        self.requests.append((method, url, kwargs))
        if "/drive/v1/files/" in url:
            self.downloaded_tokens.append(url.split("/files/", 1)[1].split("/", 1)[0])
        if not self.responses:
            raise AssertionError(f"unexpected request: {method} {url}")
        response = self.responses.pop(0)
        if isinstance(response, Exception):
            raise response
        return response


def _success(payload):
    return FakeResponse(payload={"code": 0, "msg": "success", "data": payload})


def _auth_success():
    return FakeResponse(
        payload={
            "code": 0,
            "msg": "success",
            "tenant_access_token": "token",
            "expire": 7200,
        }
    )


def _build_connector(session, **overrides):
    config = {
        "space_id": "space-1",
        "root_node_token": "root-node",
        "batch_size": 2,
        "include_extensions": [],
        "include_keywords": [],
        "exclude_keywords": [],
        "credentials": {"app_id": "cli_test", "app_secret": "secret"},
        **overrides,
    }
    connector = FeishuWikiConnector.build_connector(config)
    connector.session = session
    return connector


def test_real_registry_and_connector_contracts_are_wired():
    assert CONNECTOR_BY_SOURCE[FileSource.FEISHU_WIKI] is FeishuWikiConnector
    assert issubclass(FeishuWikiConnector, (LoadConnector, PollConnector))
    assert DocumentSource.FEISHU_WIKI.value == FileSource.FEISHU_WIKI.value
    assert get_file_ext("guide.PDF") == ".pdf"
    assert "fingerprint" in Document.model_fields


def test_build_connector_requires_app_credentials():
    with pytest.raises(ConnectorMissingCredentialError):
        FeishuWikiConnector.build_connector({"space_id": "space-1", "root_node_token": "root-node"})


def test_screening_happens_before_download_and_nested_files_are_discovered():
    session = FakeSession(
        [
            _auth_success(),
            _success(
                {
                    "items": [
                        {
                            "node_token": "folder-node",
                            "obj_token": "folder-object",
                            "obj_type": "docx",
                            "title": "Folder",
                            "has_child": True,
                            "obj_edit_time": "1767225600",
                        },
                        {
                            "node_token": "obsolete-node",
                            "obj_token": "obsolete-token",
                            "obj_type": "file",
                            "title": "obsolete-file.pdf",
                            "has_child": False,
                            "obj_edit_time": "1767225600",
                        },
                    ],
                    "has_more": False,
                }
            ),
            _success(
                {
                    "items": [
                        {
                            "node_token": "current-node",
                            "parent_node_token": "folder-node",
                            "obj_token": "current-file-token",
                            "obj_type": "file",
                            "title": "project-overview.pdf",
                            "has_child": False,
                            "obj_edit_time": "1767225600",
                        },
                        {
                            "node_token": "image-node",
                            "parent_node_token": "folder-node",
                            "obj_token": "image-token",
                            "obj_type": "file",
                            "title": "project-overview.png",
                            "has_child": False,
                            "obj_edit_time": "1767225600",
                        },
                    ],
                    "has_more": False,
                }
            ),
            FakeResponse(content=b"current pdf"),
        ]
    )
    connector = _build_connector(
        session,
        include_extensions=["pdf"],
        exclude_keywords=["obsolete"],
    )

    documents = [document for batch in connector.load_from_state() for document in batch]

    assert [document.semantic_identifier for document in documents] == ["project-overview.pdf"]
    assert session.downloaded_tokens == ["current-file-token"]


def test_pagination_incremental_window_metadata_and_fingerprint_are_preserved():
    session = FakeSession(
        [
            _auth_success(),
            _success(
                {
                    "items": [
                        {
                            "node_token": "old-node",
                            "obj_token": "old-token",
                            "obj_type": "file",
                            "title": "old.pdf",
                            "has_child": False,
                            "obj_edit_time": "1767225600",
                        }
                    ],
                    "has_more": True,
                    "page_token": "page-2",
                }
            ),
            _success(
                {
                    "items": [
                        {
                            "node_token": "current-node",
                            "parent_node_token": "root-node",
                            "obj_token": "current-token",
                            "obj_type": "file",
                            "title": "project-plan.docx",
                            "has_child": False,
                            "obj_edit_time": "1767398400",
                        }
                    ],
                    "has_more": False,
                }
            ),
            FakeResponse(content=b"current body"),
        ]
    )
    connector = _build_connector(session, include_keywords=["project"])

    documents = [document for batch in connector.poll_source(1767312000, 1767484800) for document in batch]

    assert len(documents) == 1
    document = documents[0]
    assert document.id == "feishu_wiki:space-1:current-node"
    assert document.doc_updated_at == datetime(2026, 1, 3, tzinfo=UTC)
    assert document.extension == ".docx"
    assert document.fingerprint == hashlib.sha256(b"current body").hexdigest()[:32]
    assert len(document.fingerprint) == 32
    assert document.metadata == {
        "source": "feishu_wiki",
        "wiki_space_id": "space-1",
        "wiki_node_token": "current-node",
        "wiki_object_token": "current-token",
        "wiki_object_type": "file",
        "wiki_parent_node_token": "root-node",
        "wiki_url": "https://feishu.cn/wiki/current-node",
        "source_updated_at": "2026-01-03T00:00:00+00:00",
        "content_sha256": hashlib.sha256(b"current body").hexdigest(),
    }
    assert session.requests[2][2]["params"]["page_token"] == "page-2"


def test_validate_surfaces_feishu_api_errors_without_secrets():
    session = FakeSession(
        [
            _auth_success(),
            FakeResponse(payload={"code": 99991663, "msg": "permission denied"}),
        ]
    )
    connector = _build_connector(session)

    with pytest.raises(ConnectorValidationError, match="permission denied") as error:
        connector.validate_connector_settings()

    assert "secret" not in str(error.value)
    assert "token" not in str(error.value)


def test_validate_surfaces_feishu_error_body_for_http_400_without_secrets():
    session = FakeSession(
        [
            _auth_success(),
            FakeResponse(
                status_code=400,
                payload={
                    "code": 131006,
                    "msg": "the parent node does not belong to the space",
                    "data": {"app_secret": "must-not-leak"},
                },
            ),
        ]
    )
    connector = _build_connector(session)

    with pytest.raises(
        ConnectorValidationError,
        match=(
            r"Feishu API error 131006 \(HTTP 400\) while attempting to "
            r"list Wiki nodes: the parent node does not belong to the space"
        ),
    ) as error:
        connector.validate_connector_settings()

    assert "must-not-leak" not in str(error.value)
    assert "app_secret" not in str(error.value)


def _prepare_download(connector):
    connector._access_token = "cached-access-token"
    connector._access_token_expires_at = float("inf")


def test_download_rejects_declared_oversize_before_reading_body():
    response = FakeResponse(content=b"large", headers={"Content-Length": "5"})
    connector = _build_connector(FakeSession([response]), max_file_size_bytes=4)
    _prepare_download(connector)

    with pytest.raises(ConnectorValidationError, match=r"size.*limit|limit.*size"):
        connector._download_file("file-1")

    assert response.iterated == 0
    assert response.closed == 1


def test_download_rejects_body_exceeding_limit_without_content_length():
    response = FakeResponse(chunks=[b"abc", b"de"])
    connector = _build_connector(FakeSession([response]), max_file_size_bytes=4)
    _prepare_download(connector)

    with pytest.raises(ConnectorValidationError, match=r"size.*limit|limit.*size"):
        connector._download_file("file-1")

    assert response.closed == 1


def test_download_accepts_body_at_exact_limit_and_escapes_object_token():
    response = FakeResponse(chunks=[b"ab", b"cd"], headers={"Content-Length": "4"})
    session = FakeSession([response])
    connector = _build_connector(session, max_file_size_bytes=4)
    _prepare_download(connector)

    assert connector._download_file("folder/file ?") == b"abcd"
    assert session.requests[0][1].endswith("/folder%2Ffile%20%3F/download")
    assert session.requests[0][2]["stream"] is True
    assert response.closed == 1


def test_download_closes_response_and_wraps_stream_read_failure():
    def failing_chunks():
        yield b"ab"
        raise OSError("socket reset")

    response = FakeResponse(chunks=failing_chunks())
    connector = _build_connector(FakeSession([response]))
    _prepare_download(connector)

    with pytest.raises(ConnectorValidationError, match="download") as error:
        connector._download_file("file-1")

    assert "socket reset" not in str(error.value)
    assert response.closed == 1


def test_download_permission_failure_uses_connector_validation_contract():
    response = FakeResponse(status_code=403, payload={"code": 99991663, "msg": "forbidden"})
    connector = _build_connector(FakeSession([response]))
    _prepare_download(connector)

    with pytest.raises(ConnectorValidationError, match="permission"):
        connector._download_file("file-1")

    assert response.closed == 1


def test_validate_wraps_transport_failure_without_credentials():
    import requests

    connector = _build_connector(FakeSession([requests.ConnectionError("secret transport detail")]))

    with pytest.raises(ConnectorValidationError, match="connect|authenticate") as error:
        connector.validate_connector_settings()

    assert "secret" not in str(error.value)


def test_http_permission_failure_is_a_connector_validation_error():
    response = FakeResponse(status_code=403, payload={"code": 99991663, "msg": "forbidden"})

    with pytest.raises(ConnectorValidationError, match="permission"):
        FeishuWikiConnector._decode_api_payload(response, "list Wiki nodes")


@pytest.mark.parametrize("batch_size", [0, 11])
def test_batch_size_must_be_between_one_and_ten(batch_size):
    with pytest.raises(ConnectorValidationError, match="batch_size"):
        _build_connector(FakeSession([]), batch_size=batch_size)


def test_default_limits_are_bounded():
    connector = _build_connector(FakeSession([]))

    assert connector.batch_size == 2
    assert connector.max_file_size_bytes == 50 * 1024 * 1024


def test_supported_extensions_match_the_document_and_image_upload_subset():
    connector = _build_connector(FakeSession([]))

    assert connector._matches_filters("guide.pdf")
    assert connector._matches_filters("diagram.tif")
    assert not connector._matches_filters("scan.tiff")
    assert not connector._matches_filters("archive.zip")
