import hashlib
import importlib
import json
import sys
from datetime import UTC, datetime
from types import ModuleType, SimpleNamespace

import pytest
from requests.structures import CaseInsensitiveDict

import common
from common.constants import FileSource
from test.unit_test.feishu_wiki_test_support import install_unrelated_provider_stubs

_MISSING = object()


def _data_source_modules() -> dict[str, ModuleType]:
    return {name: module for name, module in sys.modules.items() if name == "common.data_source" or name.startswith("common.data_source.")}


def _load_real_data_source_stack() -> SimpleNamespace:
    saved_modules = _data_source_modules()
    saved_parent_attr = getattr(common, "data_source", _MISSING)
    for name in list(_data_source_modules()):
        sys.modules.pop(name, None)
    if hasattr(common, "data_source"):
        del common.data_source

    install_unrelated_provider_stubs()
    try:
        registry = importlib.import_module("common.data_source")
        config = importlib.import_module("common.data_source.config")
        exceptions = importlib.import_module("common.data_source.exceptions")
        interfaces = importlib.import_module("common.data_source.interfaces")
        models = importlib.import_module("common.data_source.models")
        utils = importlib.import_module("common.data_source.utils")
        return SimpleNamespace(
            registry=registry,
            DocumentSource=config.DocumentSource,
            ConnectorMissingCredentialError=exceptions.ConnectorMissingCredentialError,
            ConnectorValidationError=exceptions.ConnectorValidationError,
            LoadConnector=interfaces.LoadConnector,
            PollConnector=interfaces.PollConnector,
            Document=models.Document,
            get_file_ext=utils.get_file_ext,
        )
    finally:
        for name in list(_data_source_modules()):
            sys.modules.pop(name, None)
        sys.modules.update(saved_modules)
        if saved_parent_attr is _MISSING:
            if hasattr(common, "data_source"):
                del common.data_source
        else:
            common.data_source = saved_parent_attr


_DATA_SOURCE_STATE_BEFORE = (_data_source_modules(), getattr(common, "data_source", _MISSING))
_real = _load_real_data_source_stack()
_DATA_SOURCE_STATE_AFTER = (_data_source_modules(), getattr(common, "data_source", _MISSING))

CONNECTOR_BY_SOURCE = _real.registry.CONNECTOR_BY_SOURCE
FeishuWikiConnector = _real.registry.FeishuWikiConnector
DocumentSource = _real.DocumentSource
ConnectorMissingCredentialError = _real.ConnectorMissingCredentialError
ConnectorValidationError = _real.ConnectorValidationError
LoadConnector = _real.LoadConnector
PollConnector = _real.PollConnector
Document = _real.Document
get_file_ext = _real.get_file_ext


class FakeResponse:
    def __init__(self, *, payload=None, content=b"", chunks=None, headers=None, status_code=200):
        self._payload = payload
        self.content = content
        self.chunks = chunks
        self.headers = CaseInsensitiveDict(headers or {})
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
    assert _DATA_SOURCE_STATE_AFTER[0] == _DATA_SOURCE_STATE_BEFORE[0]
    assert _DATA_SOURCE_STATE_AFTER[1] is _DATA_SOURCE_STATE_BEFORE[1]
    assert CONNECTOR_BY_SOURCE[FileSource.FEISHU_WIKI] is FeishuWikiConnector
    assert issubclass(FeishuWikiConnector, (LoadConnector, PollConnector))
    assert DocumentSource.FEISHU_WIKI.value == FileSource.FEISHU_WIKI.value
    assert get_file_ext("guide.PDF") == ".pdf"
    assert "fingerprint" in Document.model_fields

    saved_modules = _data_source_modules()
    saved_parent_attr = getattr(common, "data_source", _MISSING)
    sentinel = ModuleType("common.data_source")
    for name in list(saved_modules):
        sys.modules.pop(name, None)
    sys.modules["common.data_source"] = sentinel
    common.data_source = sentinel
    try:
        isolated = _load_real_data_source_stack()
        assert isolated.registry.CONNECTOR_BY_SOURCE[FileSource.FEISHU_WIKI] is isolated.registry.FeishuWikiConnector
        assert sys.modules["common.data_source"] is sentinel
        assert common.data_source is sentinel
    finally:
        for name in list(_data_source_modules()):
            sys.modules.pop(name, None)
        sys.modules.update(saved_modules)
        if saved_parent_attr is _MISSING:
            del common.data_source
        else:
            common.data_source = saved_parent_attr


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


@pytest.mark.parametrize("status_code", [300, 400, 403, 404, 500])
def test_download_surfaces_bounded_api_error_details(status_code):
    response = FakeResponse(
        status_code=status_code,
        content=json.dumps({"code": 99991672, "msg": "Access denied", "data": {"private": "must-not-leak"}}).encode(),
        headers={"X-Tt-Logid": "request-123"},
    )
    session = FakeSession([response])
    connector = _build_connector(session)
    _prepare_download(connector)

    with pytest.raises(ConnectorValidationError) as error:
        connector._download_file("file-1")

    detail = str(error.value)
    assert f"Feishu API error 99991672 (HTTP {status_code})" in detail
    assert "Access denied" in detail
    assert "logid=request-123" in detail
    assert "must-not-leak" not in detail
    assert len(session.requests) == 1
    assert response.closed == 1


@pytest.mark.parametrize("body", [b"<html>gateway failed</html>", b"[]", b"\xff", b"{}"])
def test_download_keeps_status_and_logid_for_invalid_error_bodies(body):
    response = FakeResponse(status_code=502, content=body, headers={"x-tt-logid": "gateway-123"})
    connector = _build_connector(FakeSession([response]))
    _prepare_download(connector)

    with pytest.raises(ConnectorValidationError) as error:
        connector._download_file("file-1")

    assert "HTTP 502" in str(error.value)
    assert "logid=gateway-123" in str(error.value)
    assert "gateway failed" not in str(error.value)
    assert response.closed == 1


def test_download_does_not_read_an_unbounded_error_body():
    def chunks():
        for _ in range(8):
            yield b"x" * 1024
        pytest.fail("error response was read beyond the 8 KiB limit")

    response = FakeResponse(status_code=503, chunks=chunks(), headers={"x-tt-logid": "large-error"})
    connector = _build_connector(FakeSession([response]))
    _prepare_download(connector)

    with pytest.raises(ConnectorValidationError) as error:
        connector._download_file("file-1")

    assert "HTTP 503" in str(error.value)
    assert "logid=large-error" in str(error.value)
    assert response.closed == 1


def test_download_redacts_credentials_and_bounds_error_fields():
    response = FakeResponse(
        status_code=400,
        content=json.dumps({"code": 123, "msg": "secret cached-access-token\n" + "x" * 2000}).encode(),
        headers={"x-tt-logid": "secret\n" + "y" * 2000},
    )
    connector = _build_connector(FakeSession([response]))
    _prepare_download(connector)

    with pytest.raises(ConnectorValidationError) as error:
        connector._download_file("file-1")

    detail = str(error.value)
    assert "secret" not in detail
    assert "cached-access-token" not in detail
    assert "\n" not in detail
    assert len(detail) < 700
    assert response.closed == 1


def test_download_preserves_successful_json_file_content():
    content = b'{"code":123,"msg":"this is an actual user file"}'
    response = FakeResponse(content=content, headers={"Content-Type": "application/json"})
    connector = _build_connector(FakeSession([response]))
    _prepare_download(connector)

    assert connector._download_file("file-1") == content
    assert response.closed == 1


def test_download_retries_429_after_reset_and_closes_before_waiting(monkeypatch):
    throttled = FakeResponse(status_code=429, headers={"X-Ogw-Ratelimit-Reset": "2"})
    success = FakeResponse(content=b"downloaded file")
    session = FakeSession([throttled, success])
    connector = _build_connector(session)
    _prepare_download(connector)
    sleeps = []

    def sleep(delay):
        assert throttled.closed == 1
        sleeps.append(delay)

    monkeypatch.setattr("time.sleep", sleep)

    assert connector._download_file("file-1") == b"downloaded file"
    assert sleeps == [2]
    assert session.downloaded_tokens == ["file-1", "file-1"]
    assert session.requests[0] == session.requests[1]
    assert throttled.iterated == 0
    assert success.closed == 1


@pytest.mark.parametrize("reset", [None, "invalid", "-10", "NaN", "Infinity"])
def test_download_uses_bounded_backoff_when_reset_is_missing_or_invalid(monkeypatch, reset):
    headers = {} if reset is None else {"x-ogw-ratelimit-reset": reset}
    throttled = [FakeResponse(status_code=429, headers=headers) for _ in range(3)]
    session = FakeSession([*throttled, FakeResponse(content=b"complete")])
    connector = _build_connector(session)
    _prepare_download(connector)
    sleeps = []
    monkeypatch.setattr("time.sleep", sleeps.append)

    assert connector._download_file("file-1") == b"complete"
    assert sleeps == [1, 2, 4]
    assert all(response.closed == 1 for response in throttled)


def test_download_retry_exhaustion_preserves_final_error(monkeypatch):
    responses = [
        FakeResponse(
            status_code=429,
            content=b'{"code":99991400,"msg":"request limited"}',
            headers={"x-tt-logid": f"attempt-{attempt}"},
        )
        for attempt in range(4)
    ]
    session = FakeSession(responses)
    connector = _build_connector(session)
    _prepare_download(connector)
    sleeps = []
    monkeypatch.setattr("time.sleep", sleeps.append)

    with pytest.raises(ConnectorValidationError) as error:
        connector._download_file("file-1")

    assert "99991400" in str(error.value)
    assert "HTTP 429" in str(error.value)
    assert "logid=attempt-3" in str(error.value)
    assert sleeps == [1, 2, 4]
    assert len(session.requests) == 4
    assert all(response.closed == 1 for response in responses)


def test_download_does_not_retry_before_an_excessive_server_reset(monkeypatch):
    response = FakeResponse(status_code=429, headers={"x-ogw-ratelimit-reset": "3600"})
    session = FakeSession([response])
    connector = _build_connector(session)
    _prepare_download(connector)
    sleeps = []
    monkeypatch.setattr("time.sleep", sleeps.append)

    with pytest.raises(ConnectorValidationError, match="HTTP 429"):
        connector._download_file("file-1")

    assert sleeps == []
    assert len(session.requests) == 1
    assert response.closed == 1


@pytest.mark.parametrize("reset,expected_delay", [("0", 1), ("60", 60)])
def test_download_retry_handles_reset_boundaries(monkeypatch, reset, expected_delay):
    response = FakeResponse(status_code=429, headers={"x-ogw-ratelimit-reset": reset})
    connector = _build_connector(FakeSession([response, FakeResponse(content=b"complete")]))
    _prepare_download(connector)
    sleeps = []
    monkeypatch.setattr("time.sleep", sleeps.append)

    assert connector._download_file("file-1") == b"complete"
    assert sleeps == [expected_delay]
    assert response.closed == 1


@pytest.mark.parametrize(
    "headers,payload,logid",
    [
        ({"X-Request-Id": "request-id"}, {}, "request-id"),
        ({}, {"error": {"logid": "body-log-id"}}, "body-log-id"),
        ({"X-Tt-Logid": "header-id"}, {"error": {"logid": "body-id"}}, "header-id"),
    ],
)
def test_download_preserves_available_request_identifiers(headers, payload, logid):
    response = FakeResponse(status_code=400, headers=headers, content=json.dumps(payload).encode())
    connector = _build_connector(FakeSession([response]))
    _prepare_download(connector)

    with pytest.raises(ConnectorValidationError) as error:
        connector._download_file("file-1")

    assert f"logid={logid}" in str(error.value)
    assert response.closed == 1


def test_full_sync_imports_file_after_a_throttled_download(monkeypatch):
    session = FakeSession(
        [
            _auth_success(),
            _success({"items": [{"node_token": "node-1", "obj_token": "file-1", "obj_type": "file", "title": "guide.pdf"}], "has_more": False}),
            FakeResponse(status_code=429),
            FakeResponse(content=b"file after retry"),
        ]
    )
    connector = _build_connector(session)
    sleeps = []
    monkeypatch.setattr("time.sleep", sleeps.append)

    documents = [document for batch in connector.load_from_state() for document in batch]

    assert len(documents) == 1
    assert documents[0].blob == b"file after retry"
    assert documents[0].semantic_identifier == "guide.pdf"
    assert sleeps == [1]


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
    connector = _build_connector(FakeSession([]), batch_size=None)

    assert connector.batch_size == 2
    assert connector.max_file_size_bytes == 50 * 1024 * 1024


def test_supported_extensions_match_the_document_and_image_upload_subset():
    connector = _build_connector(FakeSession([]))

    assert connector._matches_filters("guide.pdf")
    assert connector._matches_filters("diagram.tif")
    assert not connector._matches_filters("scan.tiff")
    assert not connector._matches_filters("archive.zip")
