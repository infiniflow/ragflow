import hashlib
import importlib.util
import os
import sys
from datetime import datetime, timezone
from enum import Enum
from pathlib import Path
from types import ModuleType
from typing import Any

import pytest
from pydantic import BaseModel


class ConnectorMissingCredentialError(ValueError):
    pass


class ConnectorValidationError(ValueError):
    pass


class InsufficientPermissionsError(ConnectorValidationError):
    pass


class DocumentSource(str, Enum):
    FEISHU_WIKI = "feishu_wiki"


class Document(BaseModel):
    id: str
    source: str
    semantic_identifier: str
    extension: str
    blob: bytes
    doc_updated_at: datetime
    size_bytes: int
    metadata: dict[str, Any] | None = None
    fingerprint: str | None = None


def _install_dependency_stubs() -> None:
    config_module = ModuleType("common.data_source.config")
    config_module.DocumentSource = DocumentSource
    config_module.INDEX_BATCH_SIZE = 2

    exceptions_module = ModuleType("common.data_source.exceptions")
    exceptions_module.ConnectorMissingCredentialError = ConnectorMissingCredentialError
    exceptions_module.ConnectorValidationError = ConnectorValidationError
    exceptions_module.InsufficientPermissionsError = InsufficientPermissionsError

    interfaces_module = ModuleType("common.data_source.interfaces")

    class LoadConnector:
        pass

    class PollConnector:
        pass

    interfaces_module.LoadConnector = LoadConnector
    interfaces_module.PollConnector = PollConnector
    interfaces_module.SecondsSinceUnixEpoch = float

    models_module = ModuleType("common.data_source.models")
    models_module.Document = Document
    models_module.GenerateDocumentsOutput = Any

    utils_module = ModuleType("common.data_source.utils")
    utils_module.get_file_ext = lambda name: os.path.splitext(name)[1]

    sys.modules["common.data_source.config"] = config_module
    sys.modules["common.data_source.exceptions"] = exceptions_module
    sys.modules["common.data_source.interfaces"] = interfaces_module
    sys.modules["common.data_source.models"] = models_module
    sys.modules["common.data_source.utils"] = utils_module


def _load_connector_module():
    repo_root = Path(__file__).resolve().parents[3]
    connector_path = repo_root / "common" / "data_source" / "feishu_wiki_connector.py"
    if not connector_path.exists():
        return None

    package_name = "common.data_source"
    saved_modules = {
        name: module
        for name, module in sys.modules.items()
        if name == package_name or name.startswith(f"{package_name}.")
    }
    package_stub = ModuleType(package_name)
    package_stub.__path__ = [str(repo_root / "common" / "data_source")]
    sys.modules[package_name] = package_stub
    _install_dependency_stubs()

    try:
        spec = importlib.util.spec_from_file_location(
            "_feishu_wiki_connector_under_test",
            connector_path,
        )
        module = importlib.util.module_from_spec(spec)
        assert spec.loader is not None
        spec.loader.exec_module(module)
        return module
    finally:
        for name in list(sys.modules):
            if name == package_name or name.startswith(f"{package_name}."):
                if name in saved_modules:
                    sys.modules[name] = saved_modules[name]
                else:
                    sys.modules.pop(name, None)


feishu_wiki_connector = _load_connector_module()
FeishuWikiConnector = (
    feishu_wiki_connector.FeishuWikiConnector if feishu_wiki_connector else None
)


def _connector_class():
    assert FeishuWikiConnector is not None, "Feishu Wiki connector is not implemented"
    return FeishuWikiConnector


class FakeResponse:
    def __init__(self, *, payload=None, content=b"", status_code=200):
        self._payload = payload
        self.content = content
        self.status_code = status_code

    def json(self):
        return self._payload


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
        return self.responses.pop(0)


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
    connector = _connector_class().build_connector(config)
    connector.session = session
    return connector


def test_build_connector_requires_app_credentials():
    connector_class = _connector_class()
    with pytest.raises(ConnectorMissingCredentialError):
        connector_class.build_connector(
            {"space_id": "space-1", "root_node_token": "root-node"}
        )


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
                            "title": "Approved",
                            "has_child": True,
                            "obj_edit_time": "1767225600",
                        },
                        {
                            "node_token": "obsolete-node",
                            "obj_token": "obsolete-token",
                            "obj_type": "file",
                            "title": "obsolete-WI.pdf",
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
                            "node_token": "approved-node",
                            "parent_node_token": "folder-node",
                            "obj_token": "approved-file-token",
                            "obj_type": "file",
                            "title": "WI-001.pdf",
                            "has_child": False,
                            "obj_edit_time": "1767225600",
                        },
                        {
                            "node_token": "image-node",
                            "parent_node_token": "folder-node",
                            "obj_token": "image-token",
                            "obj_type": "file",
                            "title": "WI-001.png",
                            "has_child": False,
                            "obj_edit_time": "1767225600",
                        },
                    ],
                    "has_more": False,
                }
            ),
            FakeResponse(content=b"approved pdf"),
        ]
    )
    connector = _build_connector(
        session,
        include_extensions=["pdf"],
        exclude_keywords=["obsolete"],
    )

    documents = [document for batch in connector.load_from_state() for document in batch]

    assert [document.semantic_identifier for document in documents] == ["WI-001.pdf"]
    assert session.downloaded_tokens == ["approved-file-token"]


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
                            "title": "OLD.pdf",
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
                            "title": "SOP-002.docx",
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
    connector = _build_connector(session, include_keywords=["sop"])

    documents = [
        document
        for batch in connector.poll_source(1767312000, 1767484800)
        for document in batch
    ]

    assert len(documents) == 1
    document = documents[0]
    assert document.id == "feishu_wiki:space-1:current-node"
    assert document.doc_updated_at == datetime(2026, 1, 3, tzinfo=timezone.utc)
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
    page_two_request = session.requests[2]
    assert page_two_request[2]["params"]["page_token"] == "page-2"


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
