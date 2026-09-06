import json
from datetime import datetime, timezone
from unittest.mock import MagicMock

import pytest
import requests

from common.data_source.config import REQUEST_TIMEOUT_SECONDS
from common.data_source.eva_wiki_connector import EvaWikiConnector, EvaWikiMutationClient
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError, InsufficientPermissionsError


PROJECT_ID = "CmfProject:project-1"


def _response(payload=None, status_code=200, content=b""):
    response = MagicMock(spec=requests.Response)
    response.status_code = status_code
    response.content = content
    response.headers = {}
    response.iter_content.return_value = [content] if content else []
    response.json.return_value = payload
    if status_code >= 400:
        response.raise_for_status.side_effect = requests.HTTPError(response=response)
    return response


def _connector(**overrides):
    kwargs = {
        "api_base_url": "http://eva.internal:8084",
        "web_base_url": "https://eva.example.com",
        "project_id": PROJECT_ID,
        "batch_size": 10,
    }
    kwargs.update(overrides)
    connector = EvaWikiConnector(**kwargs)
    connector.load_credentials({"eva_api_token": "secret-token"})
    return connector


def _mutation_client():
    client = EvaWikiMutationClient(
        api_base_url="http://eva.internal:8084",
        project_id=PROJECT_ID,
    )
    client.load_credentials({"eva_api_token": "personal-token"})
    return client


def _page(**overrides):
    page = {
        "id": "CmfDocument:doc-1",
        "name": "Runbook",
        "code": "DOC-42",
        "text": "<p>Hello <strong>EVA</strong></p>",
        "text_render": "<p>shorter</p>",
        "cmf_modified_at": "2026-08-24T10:00:00+03:00",
        "project_id": PROJECT_ID,
        "project": {"name": "Operations"},
        "parent_id": PROJECT_ID,
        "cmf_author_id": "CmfUser:author-1",
        "cmf_author": {"name": "Alice", "email": "alice@example.com"},
        "cmf_owner_id": "CmfUser:owner-1",
        "cmf_owner": {"name": "Owner", "email": "owner@example.com"},
        "perm_public": False,
        "perm_has_acl": True,
        "perm_inherit": False,
        "perm_effective_acl_id": "CmfAcl:acl-1",
        "tags": [{"name": "runbook"}],
    }
    page.update(overrides)
    return page


def _attachment(**overrides):
    attachment = {
        "id": "CmfAttachment:att-1",
        "name": "Guide.docx",
        "file_name": "Guide.docx",
        "file_type": ".docx",
        "st_size": 4,
        "cmf_modified_at": "2026-08-24T11:00:00+03:00",
        "project_id": PROJECT_ID,
        "project": {"name": "Operations"},
        "parent_id": "CmfDocument:doc-1",
        "url": "/files/guide.docx",
    }
    attachment.update(overrides)
    return attachment


def test_validation_requires_token():
    connector = EvaWikiConnector(api_base_url="https://eva.example.com", project_id=PROJECT_ID)

    with pytest.raises(ConnectorMissingCredentialError):
        connector.validate_connector_settings()


def test_validation_requires_project_scope():
    connector = EvaWikiConnector(api_base_url="https://eva.example.com")
    connector.load_credentials({"eva_api_token": "token"})

    with pytest.raises(ConnectorValidationError, match="project_id is required"):
        connector.validate_connector_settings()


def test_validation_uses_token_and_project_filter():
    connector = _connector()
    connector._session.post = MagicMock(return_value=_response({"result": [{"id": PROJECT_ID, "name": "Operations", "code": "ops"}]}))

    connector.validate_connector_settings()

    assert connector._session.headers["X-Eva-Token"] == "secret-token"
    request_payload = connector._session.post.call_args.kwargs["json"]
    assert request_payload["method"] == "CmfProject.list"
    assert json.loads(request_payload["kwargs"]["filter"]) == [
        ["id", "==", PROJECT_ID],
        ["cmf_deleted", "==", False],
        ["cmf_archived", "==", False],
    ]
    assert connector._session.post.call_args.kwargs["allow_redirects"] is False


def test_get_project_returns_display_metadata():
    connector = _connector()
    connector._session.post = MagicMock(return_value=_response({"result": [{"id": PROJECT_ID, "name": "Operations", "code": "ops"}]}))

    assert connector.get_project() == {
        "id": PROJECT_ID,
        "name": "Operations",
        "code": "ops",
    }


def test_list_projects_does_not_require_project_id_and_returns_stable_options():
    connector = _connector(project_id=None, batch_size=10)
    connector._session.post = MagicMock(
        return_value=_response(
            {
                "result": [
                    {"id": "CmfProject:1", "name": "Secret", "code": "secret"},
                    {"id": "CmfProject:2", "name": "Portal", "code": "portal"},
                ]
            }
        )
    )

    projects = connector.list_projects()

    assert projects == [
        {"id": "CmfProject:2", "name": "Portal", "code": "portal"},
        {"id": "CmfProject:1", "name": "Secret", "code": "secret"},
    ]
    request_payload = connector._session.post.call_args.kwargs["json"]
    assert request_payload["method"] == "CmfProject.list"
    assert json.loads(request_payload["kwargs"]["filter"]) == [
        ["cmf_deleted", "==", False],
        ["cmf_archived", "==", False],
    ]
    assert request_payload["kwargs"]["order_by"] == ["id"]


def test_search_documents_returns_editable_source_metadata():
    connector = _connector()
    connector._session.post = MagicMock(
        side_effect=[
            _response({"result": [_page(doc_version=3, cur_published_version_id="CmfVersion:v3")]}),
            _response({"result": []}),
        ]
    )

    documents = connector.search_documents("runbook", limit=5)

    assert documents == [
        {
            "id": "CmfDocument:doc-1",
            "name": "Runbook",
            "code": "DOC-42",
            "project_id": PROJECT_ID,
            "version": "3|CmfVersion:v3|2026-08-24T10:00:00+03:00",
            "modified_at": "2026-08-24T10:00:00+03:00",
            "web_url": "https://eva.example.com/project/Document/DOC-42",
            "excerpt": "Hello EVA",
        }
    ]


def test_find_documents_by_exact_title_returns_all_matches_with_page_hierarchy():
    connector = _connector()
    root = _page(id="CmfDocument:00-root", name="Products", code="PRODUCTS", parent_id=PROJECT_ID)
    first = _page(id="CmfDocument:01-first", name="Runbook", code="RUN-1", parent_id=root["id"])
    second = _page(id="CmfDocument:02-second", name=" runbook ", code="RUN-2", parent_id=root["id"])
    connector._session.post = MagicMock(return_value=_response({"result": [root, first, second]}))

    documents = connector.find_documents_by_exact_title("RUNBOOK")

    assert [document["id"] for document in documents] == ["CmfDocument:01-first", "CmfDocument:02-second"]
    assert documents[0]["hierarchy"] == "Products › Runbook"
    assert documents[0]["breadcrumbs"] == [
        {"id": "CmfDocument:00-root", "name": "Products", "web_url": "https://eva.example.com/project/Document/PRODUCTS"},
        {"id": "CmfDocument:01-first", "name": "Runbook", "web_url": "https://eva.example.com/project/Document/RUN-1"},
    ]


def test_editable_document_draft_and_publish_use_explicit_rpc_args():
    reader = _connector()
    assert not hasattr(reader, "update_document_draft")
    assert not hasattr(reader, "publish_document")

    connector = _mutation_client()
    transport = connector._EvaWikiMutationClient__transport
    transport._session.post = MagicMock(return_value=_response({"result": True}))

    connector.update_document_draft("CmfDocument:doc-1", "<p>Draft</p>", "Business requirements")
    update_call = transport._session.post.call_args
    update_payload = update_call.kwargs["json"]
    assert update_payload["method"] == "CmfDocument.update"
    assert update_payload["args"] == ["CmfDocument:doc-1"]
    assert update_payload["kwargs"] == {"text_draft": "<p>Draft</p>", "name": "Business requirements"}
    assert transport._session.headers["X-Eva-Token"] == "personal-token"

    connector.publish_document("CmfDocument:doc-1")
    publish_payload = transport._session.post.call_args.kwargs["json"]
    assert publish_payload["method"] == "CmfDocument.do_publish"
    assert publish_payload["args"] == ["CmfDocument:doc-1"]
    assert publish_payload["kwargs"] == {}


def test_rpc_reports_eva_abort_payload():
    connector = _mutation_client()
    transport = connector._EvaWikiMutationClient__transport
    transport._session.post = MagicMock(return_value=_response({"abort": {"message": "workflow denied"}}))

    with pytest.raises(ConnectorValidationError, match="workflow denied"):
        connector.publish_document("CmfDocument:doc-1")


def test_archived_projects_are_available_only_when_enabled():
    connector = _connector(project_id=None, include_archived=True)
    connector._session.post = MagicMock(return_value=_response({"result": []}))

    connector.list_projects()

    request_payload = connector._session.post.call_args.kwargs["json"]
    assert json.loads(request_payload["kwargs"]["filter"]) == [["cmf_deleted", "==", False]]


def test_archived_project_can_be_validated_only_when_enabled():
    connector = _connector(include_archived=True)
    connector._session.post = MagicMock(return_value=_response({"result": [{"id": PROJECT_ID, "cmf_archived": True}]}))

    connector.validate_connector_settings()

    request_payload = connector._session.post.call_args.kwargs["json"]
    assert json.loads(request_payload["kwargs"]["filter"]) == [
        ["id", "==", PROJECT_ID],
        ["cmf_deleted", "==", False],
    ]


def test_load_pages_comments_and_attachments():
    connector = _connector()
    page = _page()
    comment = {
        "id": "CmfComment:comment-1",
        "parent_id": page["id"],
        "text": "<p>Checked.</p>",
        "cmf_created_at": "2026-08-24T10:30:00+03:00",
        "cmf_modified_at": "2026-08-24T10:35:00+03:00",
        "cmf_author": {"name": "Bob", "email": "bob@example.com"},
    }
    attachment = _attachment()
    connector._session.post = MagicMock(
        side_effect=[
            _response({"result": [page]}),
            _response({"result": [comment]}),
            _response({"result": [page]}),
            _response({"result": [attachment]}),
        ]
    )
    connector._session.get = MagicMock(return_value=_response(content=b"PK\x03\x04"))

    batches = list(connector.load_from_state())

    assert len(batches) == 1
    page_document, attachment_document = batches[0]
    assert page_document.id == "eva-wiki:document:CmfDocument:doc-1"
    assert page_document.semantic_identifier == "Operations > Runbook"
    assert b"Hello EVA" in page_document.blob
    assert b"shorter" not in page_document.blob
    assert b"Bob <bob@example.com>: Checked." in page_document.blob
    assert page_document.metadata["comment_count"] == 1
    assert page_document.metadata["eva_perm_effective_acl_id"] == "CmfAcl:acl-1"
    assert page_document.metadata["tags"] == ["runbook"]
    assert [owner.email for owner in page_document.primary_owners] == ["owner@example.com", "alice@example.com"]
    assert attachment_document.id == "eva-wiki:attachment:CmfAttachment:att-1"
    assert attachment_document.semantic_identifier == "Operations > Runbook > Guide.docx"
    assert attachment_document.extension == ".docx"
    assert attachment_document.blob == b"PK\x03\x04"
    connector._session.get.assert_called_once_with(
        "http://eva.internal:8084/files/guide.docx",
        timeout=REQUEST_TIMEOUT_SECONDS,
        verify=True,
        stream=True,
        allow_redirects=False,
    )


def test_poll_adds_time_bounds_and_project_filter():
    connector = _connector(include_attachments=False)
    connector._session.post = MagicMock(
        side_effect=[
            _response({"result": []}),
            _response({"result": []}),
            _response({"result": []}),
            _response({"result": []}),
        ]
    )

    assert list(connector.poll_source(1_700_000_000, 1_700_000_100)) == []

    document_call = next(call for call in connector._session.post.call_args_list if call.kwargs["json"]["method"] == "CmfDocument.list" and "text" in call.kwargs["json"]["kwargs"]["fields"])
    filters = json.loads(document_call.kwargs["json"]["kwargs"]["filter"])
    assert ["project_id", "==", PROJECT_ID] in filters
    assert ["cmf_deleted", "==", False] in filters
    assert ["cmf_archived", "==", False] in filters
    assert any(item[0:2] == ["cmf_modified_at", ">"] for item in filters)
    assert any(item[0:2] == ["cmf_modified_at", "<="] for item in filters)


def test_keyset_pagination_is_monotonic():
    connector = _connector(batch_size=1)
    connector._session.post = MagicMock(
        side_effect=[
            _response({"result": [{"id": "CmfDocument:1"}]}),
            _response({"result": [{"id": "CmfDocument:2"}]}),
            _response({"result": []}),
        ]
    )

    assert [row["id"] for row in connector._iter_entities("CmfDocument", ["id"])] == ["CmfDocument:1", "CmfDocument:2"]

    second_filters = json.loads(connector._session.post.call_args_list[1].kwargs["json"]["kwargs"]["filter"])
    assert ["id", ">", "CmfDocument:1"] in second_filters
    assert connector._session.post.call_args_list[1].kwargs["json"]["kwargs"]["slice"] == [0, 1]


def test_non_monotonic_page_is_rejected():
    connector = _connector(batch_size=1)
    connector._session.post = MagicMock(
        side_effect=[
            _response({"result": [{"id": "CmfDocument:2"}]}),
            _response({"result": [{"id": "CmfDocument:1"}]}),
        ]
    )

    with pytest.raises(ConnectorValidationError, match="non-monotonic"):
        list(connector._iter_entities("CmfDocument", ["id"]))


def test_slim_snapshot_contains_only_indexable_entities():
    connector = _connector(batch_size=10)
    connector._session.post = MagicMock(
        side_effect=[
            _response({"result": [{"id": "CmfDocument:doc-1"}]}),
            _response(
                {
                    "result": [
                        {
                            "id": "CmfAttachment:att-1",
                            "file_name": "guide.docx",
                            "st_size": 1,
                            "parent_id": "CmfDocument:doc-1",
                            "url": "/files/guide.docx",
                        },
                        {
                            "id": "CmfAttachment:att-2",
                            "file_name": "archive.bin",
                            "st_size": 1,
                            "parent_id": "CmfDocument:doc-1",
                            "url": "/files/archive.bin",
                        },
                        {
                            "id": "CmfAttachment:att-3",
                            "file_name": "orphan.docx",
                            "st_size": 1,
                            "parent_id": "CmfDocument:missing",
                            "url": "/files/orphan.docx",
                        },
                        {
                            "id": "CmfAttachment:att-4",
                            "file_name": "missing-url.docx",
                            "st_size": 1,
                            "parent_id": "CmfDocument:doc-1",
                            "url": "",
                        },
                    ]
                }
            ),
        ]
    )

    batches = list(connector.retrieve_all_slim_docs_perm_sync())

    assert [document.id for batch in batches for document in batch] == [
        "eva-wiki:document:CmfDocument:doc-1",
        "eva-wiki:attachment:CmfAttachment:att-1",
    ]


def test_attachment_download_enforces_actual_size_limit():
    connector = _connector(attachment_size_limit=3)
    connector._page_index = {"CmfDocument:doc-1": _page()}
    connector._page_paths = {"CmfDocument:doc-1": "Operations > Runbook"}
    connector._session.get = MagicMock(return_value=_response(content=b"1234"))

    with pytest.raises(ConnectorValidationError, match="exceeds the configured size limit"):
        connector._build_attachment_document(_attachment(file_name="Guide.txt", file_type=".txt", url="/files/guide.txt"))


@pytest.mark.parametrize("url", ["https://evil.example/file.docx", "//evil.example/file.docx"])
def test_attachment_download_rejects_cross_origin_url(url):
    connector = _connector()
    connector._session.get = MagicMock()

    with pytest.raises(ConnectorValidationError, match="configured API origin"):
        connector._build_attachment_document(_attachment(url=url))

    connector._session.get.assert_not_called()


def test_attachment_download_rejects_redirect():
    connector = _connector()
    connector._session.get = MagicMock(return_value=_response(status_code=302))

    with pytest.raises(ConnectorValidationError, match="redirect was refused"):
        connector._build_attachment_document(_attachment())

    assert connector._session.get.call_args.kwargs["allow_redirects"] is False


def test_unsupported_attachment_is_skipped_before_download(caplog):
    connector = _connector()
    connector._page_index = {"CmfDocument:doc-1": _page()}
    connector._session.get = MagicMock()

    assert connector._attachment_to_document(_attachment(file_name="payload.bin", file_type=".bin")) is None
    assert "unsupported" in caplog.text.lower()
    connector._session.get.assert_not_called()


def test_page_content_is_bounded_and_marked():
    connector = _connector(page_size_limit=64)
    connector._page_paths = {"CmfDocument:doc-1": "Operations > Runbook"}

    document = connector._build_page_document(_page(text="<p>" + "x" * 500 + "</p>"), [])

    assert len(document.blob) <= 64
    assert document.metadata["content_truncated"] is True
    assert document.metadata["source_size_bytes"] > 64


def test_auth_failure_is_reported_without_exposing_token():
    connector = _connector()
    connector._session.post = MagicMock(return_value=_response(status_code=403))

    with pytest.raises(InsufficientPermissionsError, match="rejected credentials") as exc_info:
        connector.validate_connector_settings()

    assert "secret-token" not in str(exc_info.value)


def test_invalid_datetime_is_reported_as_connector_error():
    with pytest.raises(ConnectorValidationError, match="invalid datetime"):
        EvaWikiConnector._parse_datetime("not-a-date")


def test_comment_change_timestamp_can_advance_page_document():
    connector = _connector()
    connector._page_paths = {"CmfDocument:doc-1": "Operations > Runbook"}
    changed_at = datetime(2026, 8, 25, tzinfo=timezone.utc)

    document = connector._build_page_document(_page(), [], changed_at)

    assert document.doc_updated_at == changed_at
