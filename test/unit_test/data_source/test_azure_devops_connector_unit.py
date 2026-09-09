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
import importlib.util
import sys
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from types import ModuleType
from typing import Any, Generic, TypeVar

import pytest
from pydantic import BaseModel

T = TypeVar("T")


@dataclass
class _BasicExpertInfo:
    display_name: str | None = None


@dataclass
class _Document:
    id: str
    blob: bytes
    source: str
    semantic_identifier: str
    extension: str
    doc_updated_at: datetime
    size_bytes: int
    primary_owners: list = field(default_factory=list)
    metadata: dict | None = None


@dataclass
class _SlimDocument:
    id: str


@dataclass
class _DocumentFailure:
    document_id: str
    document_link: str | None = None


@dataclass
class _ConnectorFailure:
    failed_document: Any = None
    failure_message: str = ""
    exception: Exception | None = None


class _ConnectorCheckpoint(BaseModel):
    has_more: bool = True


class _CheckpointedConnector(Generic[T]):
    pass


class _SlimConnectorWithPermSync:
    pass


class _DocumentSource:
    AZURE_DEVOPS = type("_Value", (), {"value": "azure_devops"})()


def _install_dependency_stubs():
    config_module = ModuleType("common.data_source.config")
    config_module.DocumentSource = _DocumentSource
    config_module.INDEX_BATCH_SIZE = 10
    config_module.REQUEST_TIMEOUT_SECONDS = 30

    exceptions_module = ModuleType("common.data_source.exceptions")
    for name in (
        "ConnectorMissingCredentialError",
        "ConnectorValidationError",
        "CredentialExpiredError",
        "InsufficientPermissionsError",
        "UnexpectedValidationError",
    ):
        setattr(exceptions_module, name, type(name, (Exception,), {}))

    interfaces_module = ModuleType("common.data_source.interfaces")
    interfaces_module.CheckpointedConnector = _CheckpointedConnector
    interfaces_module.CheckpointOutput = object
    interfaces_module.IndexingHeartbeatInterface = object
    interfaces_module.SecondsSinceUnixEpoch = float
    interfaces_module.SlimConnectorWithPermSync = _SlimConnectorWithPermSync

    models_module = ModuleType("common.data_source.models")
    models_module.BasicExpertInfo = _BasicExpertInfo
    models_module.ConnectorCheckpoint = _ConnectorCheckpoint
    models_module.ConnectorFailure = _ConnectorFailure
    models_module.Document = _Document
    models_module.DocumentFailure = _DocumentFailure
    models_module.SlimDocument = _SlimDocument

    utils_module = ModuleType("common.data_source.utils")
    utils_module.get_file_ext = lambda file_name: Path(file_name).suffix.lower()

    cross_module = ModuleType("common.data_source.cross_connector_utils")
    cross_module.__path__ = []
    retry_module = ModuleType("common.data_source.cross_connector_utils.retry_wrapper")
    retry_module.retry_builder = lambda **kwargs: lambda func: func
    rate_limit_module = ModuleType("common.data_source.cross_connector_utils.rate_limit_wrapper")
    rate_limit_module.rate_limit_builder = lambda **kwargs: lambda func: func

    sys.modules["common.data_source.config"] = config_module
    sys.modules["common.data_source.exceptions"] = exceptions_module
    sys.modules["common.data_source.interfaces"] = interfaces_module
    sys.modules["common.data_source.models"] = models_module
    sys.modules["common.data_source.utils"] = utils_module
    sys.modules["common.data_source.cross_connector_utils"] = cross_module
    sys.modules["common.data_source.cross_connector_utils.retry_wrapper"] = retry_module
    sys.modules["common.data_source.cross_connector_utils.rate_limit_wrapper"] = rate_limit_module


def _load_azure_devops_modules():
    """Load the Azure DevOps connector in isolation (avoid the package __init__)."""
    repo_root = Path(__file__).resolve().parents[3]
    package_name = "common.data_source"
    saved_modules = {name: module for name, module in sys.modules.items() if name == package_name or name.startswith(f"{package_name}.")}

    package_stub = ModuleType(package_name)
    package_stub.__path__ = [str(repo_root / "common" / "data_source")]
    sys.modules[package_name] = package_stub

    subpackage = ModuleType(f"{package_name}.azure_devops")
    subpackage.__path__ = [str(repo_root / "common" / "data_source" / "azure_devops")]
    sys.modules[f"{package_name}.azure_devops"] = subpackage

    _install_dependency_stubs()

    def _load(module_name: str, relative_path: str):
        spec = importlib.util.spec_from_file_location(module_name, repo_root / relative_path)
        module = importlib.util.module_from_spec(spec)
        assert spec.loader is not None
        sys.modules[module_name] = module
        spec.loader.exec_module(module)
        return module

    try:
        utils = _load(f"{package_name}.azure_devops.utils", "common/data_source/azure_devops/utils.py")
        connector = _load(f"{package_name}.azure_devops.connector", "common/data_source/azure_devops/connector.py")
        # The connector raises these exact stub classes, so they are captured
        # before sys.modules is restored.
        exceptions = sys.modules[f"{package_name}.exceptions"]
        return utils, connector, exceptions
    finally:
        for name in list(sys.modules):
            if name == package_name or name.startswith(f"{package_name}."):
                if name in saved_modules:
                    sys.modules[name] = saved_modules[name]
                else:
                    sys.modules.pop(name, None)


azure_utils, azure_connector, azure_exceptions = _load_azure_devops_modules()

ConnectorMissingCredentialError = azure_exceptions.ConnectorMissingCredentialError
CredentialExpiredError = azure_exceptions.CredentialExpiredError
InsufficientPermissionsError = azure_exceptions.InsufficientPermissionsError
UnexpectedValidationError = azure_exceptions.UnexpectedValidationError

AzureDevOpsConnector = azure_connector.AzureDevOpsConnector
INDEX_MODE_ORGANIZATION = azure_connector.INDEX_MODE_ORGANIZATION
INDEX_MODE_PROJECTS = azure_connector.INDEX_MODE_PROJECTS
INDEX_MODE_REPOSITORIES = azure_connector.INDEX_MODE_REPOSITORIES


class _FakeResponse:
    def __init__(self, payload: Any = None, status_code: int = 200, content_type: str = "application/json", text: str = ""):
        self._payload = payload if payload is not None else {}
        self.status_code = status_code
        self.headers = {"content-type": content_type}
        self.content = text.encode("utf-8") if text else b""

    def json(self) -> Any:
        return self._payload


class _FakeClient:
    """Matches request URLs by substring and returns canned payloads."""

    def __init__(self, routes: dict[str, Any]):
        self.routes = routes
        self.requests: list[str] = []

    def get(self, url: str, params: dict[str, Any] | None = None, timeout: Any = None) -> _FakeResponse:
        self.requests.append(url)
        for fragment, response in self.routes.items():
            if fragment in url:
                return response if isinstance(response, _FakeResponse) else _FakeResponse(response)
        return _FakeResponse({}, status_code=404)

    def __enter__(self) -> "_FakeClient":
        return self

    def __exit__(self, *args) -> None:
        return None


class _FakeStreamResponse:
    """Minimal stand-in for a streamed httpx response."""

    def __init__(self, chunks: list[bytes], status_code: int = 200, content_type: str = "text/plain"):
        self._chunks = chunks
        self.status_code = status_code
        self.headers = {"content-type": content_type}
        self.consumed_chunks = 0
        self.consumed_bytes = 0

    def iter_bytes(self):
        for chunk in self._chunks:
            self.consumed_chunks += 1
            self.consumed_bytes += len(chunk)
            yield chunk

    def __enter__(self) -> "_FakeStreamResponse":
        return self

    def __exit__(self, *args) -> None:
        return None


class _FakeStreamClient:
    def __init__(self, response: _FakeStreamResponse):
        self._response = response
        self.consumed = 0

    def stream(self, method: str, url: str, params=None, timeout=None) -> _FakeStreamResponse:
        return self._response


def _build_connector(**kwargs) -> AzureDevOpsConnector:
    connector = AzureDevOpsConnector(organization=kwargs.pop("organization", "contoso"), **kwargs)
    connector.load_credentials({"azure_devops_pat": "token"})
    return connector


@pytest.mark.p2
def test_organization_url_accepts_bare_name():
    assert azure_utils.organization_url("contoso") == "https://dev.azure.com/contoso"


@pytest.mark.p2
def test_organization_url_accepts_self_hosted_collection():
    """Azure DevOps Server lives behind a custom collection URL, not dev.azure.com."""
    assert azure_utils.organization_url("https://tfs.contoso.com/DefaultCollection/") == "https://tfs.contoso.com/DefaultCollection"


@pytest.mark.p2
def test_invalid_token_returns_203_sign_in_page_not_401():
    """Azure DevOps answers a bad PAT with 203 and an HTML sign-in page.

    Without this mapping the failure surfaces as an unrelated JSON parse error.
    """
    with pytest.raises(CredentialExpiredError) as excinfo:
        azure_utils.raise_for_auth(_FakeResponse(status_code=203, content_type="text/html"))
    assert "personal access token" in str(excinfo.value).lower()


@pytest.mark.p2
def test_html_response_with_200_is_still_treated_as_sign_in_page():
    with pytest.raises(CredentialExpiredError):
        azure_utils.raise_for_auth(_FakeResponse(status_code=200, content_type="text/html; charset=utf-8"))


@pytest.mark.p2
def test_forbidden_response_reports_missing_code_read_scope():
    with pytest.raises(InsufficientPermissionsError) as excinfo:
        azure_utils.raise_for_auth(_FakeResponse(status_code=403))
    assert "Code (Read)" in str(excinfo.value)


@pytest.mark.p2
def test_successful_response_passes_through():
    assert azure_utils.raise_for_auth(_FakeResponse(status_code=200)) is None


@pytest.mark.p2
def test_throttling_response_is_retriable():
    """Azure DevOps throttles on throughput units; 429 must not end the sync."""
    client = _FakeClient({"": _FakeResponse(status_code=429)})
    with pytest.raises(azure_utils.AzureDevOpsRetriableError):
        azure_utils.azure_devops_get(client, "https://dev.azure.com/contoso/_apis/projects")


@pytest.mark.p2
def test_retry_after_is_clamped(monkeypatch):
    """A large Retry-After must not park a sync worker for hours."""
    slept: list[float] = []
    monkeypatch.setattr(azure_utils.time, "sleep", lambda seconds: slept.append(seconds))

    azure_utils.sleep_for_retry_after("86400")
    azure_utils.sleep_for_retry_after("5")
    azure_utils.sleep_for_retry_after("not-a-number")
    azure_utils.sleep_for_retry_after(None)

    assert slept == [azure_utils.MAX_RETRY_AFTER_SECONDS, 5]


@pytest.mark.p2
def test_server_error_is_retriable():
    client = _FakeClient({"": _FakeResponse(status_code=503)})
    with pytest.raises(azure_utils.AzureDevOpsRetriableError):
        azure_utils.azure_devops_get(client, "https://dev.azure.com/contoso/_apis/projects")


@pytest.mark.p2
def test_client_error_is_not_retriable():
    client = _FakeClient({"": _FakeResponse(status_code=404)})
    with pytest.raises(azure_utils.AzureDevOpsNonRetriableError):
        azure_utils.azure_devops_get(client, "https://dev.azure.com/contoso/_apis/projects")


@pytest.mark.p2
@pytest.mark.parametrize(
    "path,expected",
    [
        ("/src/Payments/RefundService.cs", False),
        ("/Dockerfile", False),
        ("/LICENSE", False),
        ("/.gitattributes", True),
        ("/.gitignore", True),
        ("/node_modules/lib/index.js", True),
        ("/Bayi-Portal.Api/bin/Debug/app.dll", True),
        ("/docs/logo.png", True),
    ],
)
def test_should_skip_path(path: str, expected: bool):
    assert azure_utils.should_skip_path(path) is expected


@pytest.mark.p2
def test_document_extension_falls_back_to_txt_for_extensionless_files():
    """RAGFlow dispatches parsing on the extension, so it must never be empty."""
    assert azure_utils.document_extension("Dockerfile") == ".txt"
    assert azure_utils.document_extension("src/App.cs") == ".cs"


@pytest.mark.p2
def test_oversized_file_is_abandoned_mid_download():
    """The limit is enforced while reading, not after the body is in memory."""
    chunk = b"x" * 64_000
    chunks = [chunk] * (azure_utils.MAX_FILE_BYTES // len(chunk) + 4)
    response = _FakeStreamResponse(chunks)
    client = _FakeStreamClient(response)

    content = azure_utils.fetch_file_content(client, "https://dev.azure.com/contoso/p/_apis/git/repositories/r", "/huge.bin", "master")

    assert content is None
    # Reading the whole body and checking the size afterwards would also return
    # None, so the download itself has to be shown to stop early.
    assert response.consumed_chunks < len(chunks)
    assert response.consumed_bytes <= azure_utils.MAX_FILE_BYTES + len(chunk)


@pytest.mark.p2
def test_file_within_limit_is_returned():
    client = _FakeStreamClient(_FakeStreamResponse([b"public class ", b"App {}"]))
    content = azure_utils.fetch_file_content(client, "https://dev.azure.com/contoso/p/_apis/git/repositories/r", "/src/App.cs", "master")
    assert content == b"public class App {}"


@pytest.mark.p2
def test_default_branch_strips_ref_prefix():
    assert azure_utils.default_branch_of({"defaultBranch": "refs/heads/master"}) == "master"
    assert azure_utils.default_branch_of({}) == "main"


@pytest.mark.p2
def test_cleartext_collection_url_is_rejected():
    """The PAT travels in the Authorization header, so HTTP is refused."""
    with pytest.raises(UnexpectedValidationError) as excinfo:
        azure_utils.organization_url("http://tfs.contoso.com/DefaultCollection")
    assert "HTTPS" in str(excinfo.value)


@pytest.mark.p2
def test_html_body_is_not_an_auth_failure_for_raw_content():
    """A repository may legitimately contain .html files."""
    response = _FakeResponse(status_code=200, content_type="text/html", text="<html>page</html>")
    assert azure_utils.raise_for_auth(response, expect_json=False) is None


@pytest.mark.p2
@pytest.mark.parametrize(
    "overrides",
    [
        {"index_mode": "everything"},
        {"content_types": "everything"},
        {"index_mode": INDEX_MODE_REPOSITORIES},
    ],
)
def test_unusable_settings_are_rejected_before_any_request(overrides):
    connector = _build_connector(**overrides)
    with pytest.raises(UnexpectedValidationError, match="(?i)unsupported|required"):
        connector._validate_settings()


@pytest.mark.p2
def test_active_pull_request_is_always_reindexed():
    """Azure DevOps has no dependable "updated" timestamp for pull requests."""
    window_start = datetime(2026, 1, 1, tzinfo=timezone.utc)
    window_end = datetime(2026, 2, 1, tzinfo=timezone.utc)

    active = {"status": "active", "creationDate": "2024-05-01T00:00:00Z"}
    assert azure_utils.pull_request_in_window(active, window_start, window_end) is True

    closed_before = {"status": "completed", "closedDate": "2024-05-01T00:00:00Z"}
    assert azure_utils.pull_request_in_window(closed_before, window_start, window_end) is False

    closed_inside = {"status": "completed", "closedDate": "2026-01-15T00:00:00Z"}
    assert azure_utils.pull_request_in_window(closed_inside, window_start, window_end) is True


@pytest.mark.p2
def test_load_credentials_requires_personal_access_token():
    connector = AzureDevOpsConnector(organization="contoso")
    with pytest.raises(ConnectorMissingCredentialError):
        connector.load_credentials({})


@pytest.mark.p2
def test_repository_filter_accepts_qualified_and_bare_names():
    connector = _build_connector(index_mode=INDEX_MODE_REPOSITORIES, repositories="iddaa/Bayi-Portal, sportsbook")
    assert connector._matches_repository_filter("iddaa", "Bayi-Portal") is True
    assert connector._matches_repository_filter("other", "sportsbook") is True
    assert connector._matches_repository_filter("iddaa", "unrelated") is False


@pytest.mark.p2
def test_repository_filter_is_inactive_for_organization_scope():
    connector = _build_connector(index_mode=INDEX_MODE_ORGANIZATION, repositories="ignored")
    assert connector._matches_repository_filter("any", "repository") is True


@pytest.mark.p2
def test_discover_repositories_skips_disabled_and_sorts():
    client = _FakeClient(
        {
            "_apis/git/repositories": {
                "value": [
                    {"name": "zeta", "project": {"name": "iddaa"}, "defaultBranch": "refs/heads/master"},
                    {"name": "alpha", "project": {"name": "iddaa"}, "defaultBranch": "refs/heads/develop"},
                    {"name": "retired", "project": {"name": "iddaa"}, "isDisabled": True},
                ]
            }
        }
    )
    connector = _build_connector()
    repos = connector._discover_repositories(client)

    assert [repo["name"] for repo in repos] == ["alpha", "zeta"]
    assert repos[0]["branch"] == "develop"


@pytest.mark.p2
def test_list_items_filters_folders_and_noise():
    client = _FakeClient(
        {
            "/items": {
                "value": [
                    {"path": "/src", "gitObjectType": "tree", "isFolder": True},
                    {"path": "/src/App.cs", "gitObjectType": "blob"},
                    {"path": "/.gitignore", "gitObjectType": "blob"},
                    {"path": "/node_modules/a.js", "gitObjectType": "blob"},
                ]
            }
        }
    )
    items = azure_utils.list_items(client, "https://dev.azure.com/contoso/iddaa/_apis/git/repositories/repo", "master")
    assert [item["path"] for item in items] == ["/src/App.cs"]


@pytest.mark.p2
def test_map_item_to_document_carries_path_commit_and_web_url():
    item = {
        "path": "/src/Payments/RefundService.cs",
        "latestProcessedChange": {
            "commitId": "abc123",
            "committer": {"name": "Ada Lovelace", "date": "2026-01-13T21:53:06Z"},
        },
    }
    document = azure_utils.map_item_to_document(
        item,
        b"public class RefundService {}",
        "contoso",
        "https://dev.azure.com/contoso",
        "iddaa",
        "Bayi-Portal",
        "master",
    )

    assert document.id == "azure_devops:contoso:iddaa:Bayi-Portal:file:src/Payments/RefundService.cs"
    assert document.extension == ".cs"
    assert document.size_bytes == len(b"public class RefundService {}")
    assert document.doc_updated_at == datetime(2026, 1, 13, 21, 53, 6, tzinfo=timezone.utc)
    assert document.metadata["commit_id"] == "abc123"
    assert document.metadata["repository"] == "Bayi-Portal"
    assert "path=/src/Payments/RefundService.cs" in document.metadata["web_url"]
    assert document.primary_owners[0].display_name == "Ada Lovelace"


@pytest.mark.p2
def test_map_item_to_document_survives_missing_commit_metadata():
    document = azure_utils.map_item_to_document({"path": "/README.md"}, b"# readme", "contoso", "https://dev.azure.com/contoso", "iddaa", "repo", "master")
    assert document.doc_updated_at is not None
    assert document.primary_owners == []


@pytest.mark.p2
def test_map_pull_request_to_document_summarises_review_metadata():
    pull_request = {
        "pullRequestId": 4225,
        "title": "BYS-1517 - PDF olusturma islemi",
        "description": "Backend tarafina tasindi.",
        "status": "completed",
        "createdBy": {"displayName": "Ada Lovelace"},
        "reviewers": [{"displayName": "Grace Hopper"}],
        "sourceRefName": "refs/heads/feature/BYS-1517",
        "targetRefName": "refs/heads/master",
        "creationDate": "2025-11-18T06:20:07.836003Z",
        "closedDate": "2025-11-20T06:20:07.836003Z",
    }
    document = azure_utils.map_pull_request_to_document(pull_request, "contoso", "https://dev.azure.com/contoso", "iddaa", "Bayi-Portal")

    assert document.id == "azure_devops:contoso:iddaa:Bayi-Portal:pr:4225"
    assert document.semantic_identifier.startswith("PR #4225:")
    assert document.extension == ".txt"
    assert document.metadata["status"] == "completed"
    assert document.metadata["source_branch"] == "feature/BYS-1517"
    assert document.doc_updated_at == datetime(2025, 11, 20, 6, 20, 7, 836003, tzinfo=timezone.utc)

    body = document.blob.decode("utf-8")
    assert "Grace Hopper" in body
    assert "Backend tarafina tasindi." in body


@pytest.mark.p2
def test_short_pull_request_description_needs_no_detail_fetch():
    assert azure_utils.pull_request_may_be_truncated({"description": "kisa aciklama"}) is False
    assert azure_utils.pull_request_may_be_truncated({}) is False


@pytest.mark.p2
def test_long_pull_request_description_is_refetched_in_full(monkeypatch):
    """The list endpoint truncates descriptions at 400 characters."""
    truncated = "x" * azure_utils.PULL_REQUEST_DESCRIPTION_LIMIT
    full = truncated + " ...and the rest of the description"

    assert azure_utils.pull_request_may_be_truncated({"description": truncated}) is True

    client = _FakeClient(
        {
            "/items": {"value": []},
            "/pullrequests/77": {"pullRequestId": 77, "title": "long", "description": full, "status": "completed"},
            "/pullrequests": {"value": [{"pullRequestId": 77, "title": "long", "description": truncated, "status": "completed"}]},
            "_apis/git/repositories": {"value": [{"name": "repo-a", "project": {"name": "iddaa"}, "defaultBranch": "refs/heads/master"}]},
        }
    )
    connector = _build_connector(content_types="pull_requests")
    monkeypatch.setattr(connector, "_client", lambda: client)

    checkpoint = connector.build_dummy_checkpoint()
    documents = []
    while checkpoint.has_more:
        generator = connector.load_from_checkpoint(start=0.0, end=datetime.now(timezone.utc).timestamp(), checkpoint=checkpoint)
        while True:
            try:
                documents.append(next(generator))
            except StopIteration as stop:
                checkpoint = stop.value
                break

    assert len(documents) == 1
    assert full in documents[0].blob.decode("utf-8")
    assert any("/pullrequests/77" in request for request in client.requests)


@pytest.mark.p2
def test_pull_request_updated_at_prefers_closed_date():
    assert azure_utils.pull_request_updated_at({"creationDate": "2025-01-01T00:00:00Z", "closedDate": "2025-02-01T00:00:00Z"}) == datetime(2025, 2, 1, tzinfo=timezone.utc)
    assert azure_utils.pull_request_updated_at({}) is None


@pytest.mark.p2
def test_checkpoint_walks_files_then_pull_requests_then_next_repo(monkeypatch):
    """One stage per invocation; the checkpoint records the exact resume position."""
    # Routes are matched in order, so the specific paths must precede the
    # repository collection URL they are nested under.
    client = _FakeClient(
        {
            "/items": {"value": [{"path": "/src/App.cs", "gitObjectType": "blob"}]},
            "/pullrequests": {"value": []},
            "_apis/git/repositories": {"value": [{"name": "repo-a", "project": {"name": "iddaa"}, "defaultBranch": "refs/heads/master"}]},
        }
    )
    connector = _build_connector()
    monkeypatch.setattr(connector, "_client", lambda: client)
    monkeypatch.setattr(azure_utils, "fetch_file_content", lambda *args, **kwargs: b"code")
    monkeypatch.setattr(azure_connector, "fetch_file_content", lambda *args, **kwargs: b"code")

    checkpoint = connector.build_dummy_checkpoint()
    start, end = 0.0, datetime.now(timezone.utc).timestamp()

    generator = connector.load_from_checkpoint(start=start, end=end, checkpoint=checkpoint)
    documents = []
    while True:
        try:
            documents.append(next(generator))
        except StopIteration as stop:
            checkpoint = stop.value
            break

    assert [document.id for document in documents] == ["azure_devops:contoso:iddaa:repo-a:file:src/App.cs"]
    assert checkpoint.stage == "pull_requests"
    assert checkpoint.has_more is True

    generator = connector.load_from_checkpoint(start=start, end=end, checkpoint=checkpoint)
    while True:
        try:
            next(generator)
        except StopIteration as stop:
            checkpoint = stop.value
            break

    assert checkpoint.current_repo_index == 1
    assert checkpoint.has_more is False


def _drain(connector, checkpoint):
    """Run the connector to completion, collecting documents and failures."""
    produced = []
    while checkpoint.has_more:
        generator = connector.load_from_checkpoint(start=0.0, end=datetime.now(timezone.utc).timestamp(), checkpoint=checkpoint)
        while True:
            try:
                produced.append(next(generator))
            except StopIteration as stop:
                checkpoint = stop.value
                break
    return produced


@pytest.mark.p2
def test_resume_follows_the_anchor_when_the_listing_shifts(monkeypatch):
    """An offset points into a listing that shifts; the anchor does not."""
    client = _FakeClient(
        {
            # "added.cs" did not exist when the checkpoint was written, so every
            # index after it has moved by one.
            "/items": {
                "value": [
                    {"path": "/added.cs", "gitObjectType": "blob"},
                    {"path": "/a.cs", "gitObjectType": "blob"},
                    {"path": "/b.cs", "gitObjectType": "blob"},
                ]
            },
            "/pullrequests": {"value": []},
            "_apis/git/repositories": {"value": [{"name": "repo-a", "project": {"name": "iddaa"}, "defaultBranch": "refs/heads/master"}]},
        }
    )
    connector = _build_connector(content_types="code")
    monkeypatch.setattr(connector, "_client", lambda: client)
    monkeypatch.setattr(azure_connector, "fetch_file_content", lambda *args, **kwargs: b"code")

    checkpoint = connector.build_dummy_checkpoint()
    checkpoint.repos_queue = [{"project": "iddaa", "name": "repo-a", "branch": "master"}]
    checkpoint.file_offset = 1
    checkpoint.last_source_id = "azure_devops:contoso:iddaa:repo-a:file:a.cs"

    produced = _drain(connector, checkpoint)

    # Offset 1 now points at a.cs, which was already committed. Following the
    # anchor continues at b.cs instead.
    assert [document.id for document in produced] == ["azure_devops:contoso:iddaa:repo-a:file:b.cs"]


@pytest.mark.p2
def test_failed_file_is_retried_once_before_the_stage_ends(monkeypatch):
    """Advancing past a failure without a retry would drop the file silently."""
    attempts: list[str] = []

    def flaky_fetch(client, repo_api_url, path, branch):
        attempts.append(path)
        if path == "/b.cs" and attempts.count("/b.cs") == 1:
            raise RuntimeError("transient failure")
        return b"code"

    client = _FakeClient(
        {
            "/items": {
                "value": [
                    {"path": "/a.cs", "gitObjectType": "blob"},
                    {"path": "/b.cs", "gitObjectType": "blob"},
                ]
            },
            "/pullrequests": {"value": []},
            "_apis/git/repositories": {"value": [{"name": "repo-a", "project": {"name": "iddaa"}, "defaultBranch": "refs/heads/master"}]},
        }
    )
    connector = _build_connector(content_types="code")
    monkeypatch.setattr(connector, "_client", lambda: client)
    monkeypatch.setattr(azure_connector, "fetch_file_content", flaky_fetch)

    produced = _drain(connector, connector.build_dummy_checkpoint())

    failures = [item for item in produced if hasattr(item, "failure_message")]
    documents = [item for item in produced if hasattr(item, "id")]

    assert len(failures) == 1
    assert attempts.count("/b.cs") == 2, "the failed file must be attempted exactly twice"
    assert "azure_devops:contoso:iddaa:repo-a:file:b.cs" in [document.id for document in documents]


@pytest.mark.p2
def test_checkpoint_round_trips_through_json():
    connector = _build_connector()
    checkpoint = connector.build_dummy_checkpoint()
    checkpoint.repos_queue = [{"project": "iddaa", "name": "repo-a", "branch": "master"}]
    checkpoint.file_offset = 500

    restored = connector.validate_checkpoint_json(checkpoint.model_dump_json())
    assert restored.file_offset == 500
    assert restored.repos_queue[0]["name"] == "repo-a"


@pytest.mark.p2
def test_content_types_control_which_families_are_indexed():
    assert _build_connector(content_types="code")._indexes_pull_requests() is False
    assert _build_connector(content_types="pull_requests")._indexes_code() is False
    both = _build_connector(content_types="both")
    assert both._indexes_code() and both._indexes_pull_requests()
