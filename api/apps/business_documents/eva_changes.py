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

"""Safe draft-and-publish workflow for changes to existing EVA documents."""

from __future__ import annotations

import hashlib
import re
from datetime import datetime
from difflib import SequenceMatcher
from enum import StrEnum
from typing import Any
from urllib.parse import unquote, urlparse

from bs4 import BeautifulSoup
from markdown import markdown as render_markdown
from markdownify import markdownify
from peewee import fn

from api.apps.business_documents.errors import BusinessDocumentError, ConflictError, ValidationError
from api.db.db_models import BusinessDocumentEvaChange, BusinessDocumentEvaChangeEvent, Connector
from api.db.services.business_document_settings_service import DocumentsEvaConnection, get_business_documents_eva_connector_id, get_documents_eva_connection
from api.db.services.connector_service import ConnectorService
from api.db.services.user_external_credential_service import (
    ExternalCredentialDecryptionError,
    ExternalCredentialError,
    ExternalCredentialMissingError,
    UserExternalCredentialService,
)
from common.data_source.config import DocumentSource, INDEX_BATCH_SIZE
from common.data_source.eva_wiki_connector import EvaWikiConnector, EvaWikiMutationClient
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError, InsufficientPermissionsError
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp


class EvaChangeState(StrEnum):
    EDITING = "EDITING"
    APPROVED = "APPROVED"
    PREPARING_EVA_DRAFT = "PREPARING_EVA_DRAFT"
    EVA_DRAFT_READY = "EVA_DRAFT_READY"
    PUBLISHING = "PUBLISHING"
    PUBLISHED = "PUBLISHED"


_HEADING_PATTERN = re.compile(r"^(#{1,6})\s+(.+?)\s*$")
_UNSAFE_TAGS = frozenset({"script", "style", "iframe", "object", "embed", "form", "input", "button"})
_SAFE_URL_SCHEMES = frozenset({"", "http", "https", "mailto"})
_MAX_DRAFT_SIZE = 1_000_000
_EXTERNAL_OPERATION_TIMEOUT_MS = 120_000


def _timestamps() -> dict[str, Any]:
    now = datetime.now()
    timestamp = current_timestamp()
    return {"create_time": timestamp, "create_date": now, "update_time": timestamp, "update_date": now}


def _canonical_markdown(value: str) -> str:
    lines = [line.rstrip() for line in str(value or "").replace("\r\n", "\n").replace("\r", "\n").split("\n")]
    while lines and not lines[0].strip():
        lines.pop(0)
    while lines and not lines[-1].strip():
        lines.pop()
    return "\n".join(lines)


def _content_hash(markdown: str) -> str:
    return f"sha256:{hashlib.sha256(_canonical_markdown(markdown).encode('utf-8')).hexdigest()}"


def _sanitize_html(value: str) -> str:
    soup = BeautifulSoup(str(value or ""), "html.parser")
    for tag in soup.find_all(_UNSAFE_TAGS):
        tag.decompose()
    for tag in soup.find_all(True):
        for attribute in list(tag.attrs):
            lowered = attribute.casefold()
            if lowered.startswith("on") or lowered in {"style", "srcdoc"}:
                del tag.attrs[attribute]
                continue
            if lowered not in {"href", "src"}:
                continue
            raw_url = tag.attrs.get(attribute)
            candidate = raw_url[0] if isinstance(raw_url, list) and raw_url else raw_url
            if urlparse(str(candidate or "").strip()).scheme.casefold() not in _SAFE_URL_SCHEMES:
                del tag.attrs[attribute]
    return str(soup)


def _html_to_markdown(value: str) -> str:
    safe_html = _sanitize_html(value)
    converted = markdownify(safe_html, heading_style="ATX", bullets="-")
    return _canonical_markdown(converted)


def _markdown_to_html(value: str) -> str:
    rendered = render_markdown(
        _canonical_markdown(value),
        extensions=["extra", "sane_lists"],
        output_format="html5",
    )
    return _sanitize_html(rendered)


def _split_sections(markdown: str) -> list[dict[str, Any]]:
    sections: list[dict[str, Any]] = []
    occurrences: dict[str, int] = {}
    current = {"key": "__document__", "title": "Документ", "lines": []}
    for line in _canonical_markdown(markdown).splitlines():
        heading = _HEADING_PATTERN.match(line)
        if heading:
            if current["lines"]:
                sections.append(current)
            title = heading.group(2).strip()
            normalized = " ".join(title.casefold().split()) or "section"
            occurrence = occurrences.get(normalized, 0) + 1
            occurrences[normalized] = occurrence
            current = {"key": f"{normalized}:{occurrence}", "title": title, "lines": [line]}
        else:
            current["lines"].append(line)
    if current["lines"] or not sections:
        sections.append(current)
    return sections


def _section_diff(base_markdown: str, draft_markdown: str) -> dict[str, Any]:
    base_sections = _split_sections(base_markdown)
    draft_sections = _split_sections(draft_markdown)
    base_by_key = {section["key"]: section for section in base_sections}
    draft_by_key = {section["key"]: section for section in draft_sections}
    ordered_keys = [section["key"] for section in base_sections]
    ordered_keys.extend(section["key"] for section in draft_sections if section["key"] not in base_by_key)

    added = 0
    removed = 0
    changed_sections: list[dict[str, Any]] = []
    for key in ordered_keys:
        base_section = base_by_key.get(key)
        draft_section = draft_by_key.get(key)
        base_lines = base_section["lines"] if base_section else []
        draft_lines = draft_section["lines"] if draft_section else []
        matcher = SequenceMatcher(a=base_lines, b=draft_lines, autojunk=False)
        lines: list[dict[str, str]] = []
        section_changed = False
        for operation, old_start, old_end, new_start, new_end in matcher.get_opcodes():
            if operation == "equal":
                lines.extend({"type": "context", "content": line} for line in base_lines[old_start:old_end])
                continue
            section_changed = True
            if operation in {"delete", "replace"}:
                deleted = base_lines[old_start:old_end]
                removed += len(deleted)
                lines.extend({"type": "removed", "content": line} for line in deleted)
            if operation in {"insert", "replace"}:
                inserted = draft_lines[new_start:new_end]
                added += len(inserted)
                lines.extend({"type": "added", "content": line} for line in inserted)
        if section_changed:
            changed_sections.append(
                {
                    "key": key,
                    "title": (draft_section or base_section or {"title": "Документ"})["title"],
                    "lines": lines,
                }
            )
    return {
        "changed": bool(changed_sections),
        "added_lines": added,
        "removed_lines": removed,
        "changed_sections": len(changed_sections),
        "sections": changed_sections,
    }


class EvaDocumentChangeService:
    @staticmethod
    def _configured_connector_id() -> str:
        connector_id = get_business_documents_eva_connector_id()
        if not connector_id:
            raise BusinessDocumentError(
                "EVA_SPACE_NOT_CONFIGURED",
                "EVA Wiki space is not configured for the Documents section",
                409,
            )
        return connector_id

    @classmethod
    def _require_configured_connector(cls, connector_id: object, actor_id: str | None = None) -> str:
        normalized = str(connector_id or "").strip()
        configured = cls._configured_connector_id()
        if normalized != configured:
            raise BusinessDocumentError(
                "EVA_SPACE_SCOPE_VIOLATION",
                "The EVA document is outside the space configured for the Documents section",
                403,
            )
        connection = get_documents_eva_connection()
        if actor_id is not None and not (connection and connection.id == configured) and not ConnectorService.accessible(configured, actor_id):
            raise BusinessDocumentError(
                "EVA_SPACE_FORBIDDEN",
                "The EVA Wiki space configured for the Documents section is not accessible",
                403,
            )
        return configured

    @classmethod
    def _connector(cls, connector_id: str, actor_id: str) -> tuple[Connector | DocumentsEvaConnection, EvaWikiConnector]:
        """Build the EVA reader used by interactive document management.

        The Documents connection's shared token is the primary read credential;
        a missing shared token allows the current user's personal token.
        """

        configured_connector_id = cls._require_configured_connector(connector_id, actor_id)
        try:
            connector = get_documents_eva_connection(with_token=True)
        except ConnectorValidationError as error:
            raise cls._map_external_error(error) from error
        if connector is not None and connector.id != configured_connector_id:
            raise BusinessDocumentError("EVA_SPACE_SCOPE_VIOLATION", "The Documents EVA connection changed; retry the operation", 409)
        if connector is None:
            exists, connector = ConnectorService.get_by_id(configured_connector_id)
            if not exists or connector.source != DocumentSource.EVA_WIKI.value:
                raise BusinessDocumentError("EVA_CONNECTOR_NOT_FOUND", "EVA Wiki connector not found", 404)
        config = dict(connector.config or {})
        client = EvaWikiConnector(
            api_base_url=config.get("api_base_url", ""),
            web_base_url=config.get("web_base_url") or None,
            project_id=config.get("project_id") or None,
            include_attachments=False,
            include_archived=config.get("include_archived", False),
            verify_ssl=config.get("verify_ssl", True),
            batch_size=config.get("batch_size") or INDEX_BATCH_SIZE,
            page_size_limit=config.get("page_size_limit") or EvaWikiConnector.DEFAULT_PAGE_SIZE_LIMIT,
            retry_count=config.get("retry_count", EvaWikiConnector.DEFAULT_RETRY_COUNT),
        )
        credentials = dict(config.get("credentials") or {})
        if not str(credentials.get("eva_api_token") or "").strip():
            try:
                personal_credential = UserExternalCredentialService.get_eva_wiki_token(actor_id, config.get("api_base_url", ""))
            except ExternalCredentialMissingError:
                pass
            except ExternalCredentialError as error:
                raise cls._map_user_token_error(error) from error
            else:
                credentials["eva_api_token"] = personal_credential.secret
        client.load_credentials(credentials)
        return connector, client

    @classmethod
    def _mutation_client(cls, connector: Connector | DocumentsEvaConnection, actor_id: str) -> tuple[EvaWikiMutationClient, int]:
        config = dict(connector.config or {})
        credential = UserExternalCredentialService.get_eva_wiki_token(actor_id, config.get("api_base_url", ""))
        client = EvaWikiMutationClient(
            api_base_url=config.get("api_base_url", ""),
            project_id=config.get("project_id", ""),
            verify_ssl=config.get("verify_ssl", True),
            retry_count=config.get("retry_count", EvaWikiConnector.DEFAULT_RETRY_COUNT),
        )
        client.load_credentials({"eva_api_token": credential.secret})
        return client, credential.credential_version

    @staticmethod
    def _map_external_error(error: Exception) -> BusinessDocumentError:
        if isinstance(error, BusinessDocumentError):
            return error
        if isinstance(error, InsufficientPermissionsError):
            return BusinessDocumentError("EVA_PERMISSION_DENIED", "EVA Wiki rejected the configured credentials", 403)
        if isinstance(error, ConnectorMissingCredentialError):
            return BusinessDocumentError("EVA_CREDENTIALS_MISSING", "EVA Wiki connector credentials are missing", 422)
        if isinstance(error, ConnectorValidationError):
            return BusinessDocumentError("EVA_UNAVAILABLE", str(error), 502)
        return BusinessDocumentError("EVA_UNAVAILABLE", "EVA Wiki request failed", 502)

    @staticmethod
    def _map_user_token_error(error: Exception) -> BusinessDocumentError:
        if isinstance(error, BusinessDocumentError):
            return error
        if isinstance(error, ExternalCredentialMissingError):
            return BusinessDocumentError("EVA_USER_TOKEN_MISSING", "Add your personal EVA API token in Profile before changing EVA", 422)
        if isinstance(error, ExternalCredentialDecryptionError):
            return BusinessDocumentError("EVA_USER_TOKEN_UNAVAILABLE", "Your personal EVA API token could not be loaded", 503)
        if isinstance(error, ExternalCredentialError):
            return BusinessDocumentError("EVA_USER_TOKEN_INVALID", str(error), 422)
        if isinstance(error, InsufficientPermissionsError):
            return BusinessDocumentError("EVA_USER_TOKEN_REJECTED", "EVA rejected your personal API token for this change", 403)
        if isinstance(error, ConnectorMissingCredentialError):
            return BusinessDocumentError("EVA_USER_TOKEN_MISSING", "Add your personal EVA API token in Profile before changing EVA", 422)
        if isinstance(error, ConnectorValidationError):
            return BusinessDocumentError("EVA_USER_CHANGE_FAILED", str(error), 502)
        return BusinessDocumentError("EVA_USER_CHANGE_FAILED", "EVA Wiki change failed", 502)

    @staticmethod
    def _validate_text(value: Any, field: str, *, maximum: int, required: bool = True) -> str:
        if not isinstance(value, str):
            raise ValidationError("INVALID_EVA_CHANGE", f"{field} must be a string", {"field": field})
        normalized = value.strip()
        if required and not normalized:
            raise ValidationError("INVALID_EVA_CHANGE", f"{field} is required", {"field": field})
        if len(value) > maximum:
            raise ValidationError("INVALID_EVA_CHANGE", f"{field} is too long", {"field": field, "maximum": maximum})
        return normalized if required else value

    @classmethod
    def search_sources(cls, actor_id: str, query: str = "", limit: int = 20) -> dict[str, Any]:
        normalized_query = str(query or "").strip()
        if len(normalized_query) > 500:
            raise ValidationError("INVALID_EVA_SEARCH", "query is too long", {"maximum": 500})
        safe_limit = min(max(int(limit), 1), 100)
        connector_id = cls._configured_connector_id()
        connector, client = cls._connector(connector_id, actor_id)
        try:
            documents = client.search_documents(normalized_query, safe_limit)
        except Exception as error:
            raise cls._map_external_error(error) from error
        return {
            "items": [{**document, "connector_id": connector.id, "connector_name": connector.name} for document in documents],
            "connectors": [{"connector_id": connector.id, "connector_name": connector.name}],
        }

    @classmethod
    def resolve_page_url(cls, actor_id: str, page_url: object) -> dict[str, Any]:
        """Resolve a user-facing EVA URL without ever requesting that URL directly.

        The URL is matched against the Documents connection. An unmatched URL
        remains a read-only link; capabilities require verification through
        EVA's authenticated API.
        """

        normalized_url = cls._validate_text(page_url, "eva_page_url", maximum=2048)
        try:
            parsed = urlparse(normalized_url)
            hostname = parsed.hostname
        except ValueError as error:
            raise ValidationError("INVALID_EVA_PAGE_URL", "eva_page_url must be an absolute HTTP(S) URL") from error
        if parsed.scheme.casefold() not in {"http", "https"} or not hostname or parsed.username or parsed.password:
            raise ValidationError("INVALID_EVA_PAGE_URL", "eva_page_url must be an absolute HTTP(S) URL")
        canonical_url = parsed._replace(query="", fragment="").geturl().rstrip("/")
        marker = "/project/Document/"
        marker_index = parsed.path.casefold().find(marker.casefold())
        code = unquote(parsed.path[marker_index + len(marker) :].strip("/")) if marker_index >= 0 else ""

        binding: dict[str, Any] = {
            "page_url": canonical_url,
            "status": "LINK_ONLY",
            "capabilities": ["OPEN"],
            "connector_id": None,
            "project_id": None,
            "document_id": None,
            "document_code": code or None,
            "document_name": None,
            "remote_version": None,
            "remote_content_hash": None,
            "last_pulled_content_hash": None,
        }
        try:
            connector_id = cls._configured_connector_id()
            connector, client = cls._connector(connector_id, actor_id)
            page_origin = EvaWikiConnector._url_origin(normalized_url)
            configured_origins = {EvaWikiConnector._url_origin(base_url) for base_url in (client.web_base_url, client.api_base_url) if str(base_url or "").strip()}
            if page_origin not in configured_origins:
                return binding
            candidates = client.search_documents(code, 100) if code else []
            source = next(
                (
                    candidate
                    for candidate in candidates
                    if str(candidate.get("web_url") or "").rstrip("/").casefold() == canonical_url.casefold() or (code and str(candidate.get("code") or "").casefold() == code.casefold())
                ),
                None,
            )
            if source is None:
                return binding
            remote = client.get_document_for_edit(str(source["id"]))
            remote_markdown = _html_to_markdown(str(remote.get("html") or ""))
            return {
                **binding,
                "page_url": str(remote.get("web_url") or canonical_url),
                "status": "CONNECTED",
                "capabilities": ["OPEN", "PULL_FROM_EVA", "CREATE_EVA_CHANGE"],
                "connector_id": connector.id,
                "project_id": str(remote.get("project_id") or "") or None,
                "document_id": str(remote.get("id") or "") or None,
                "document_code": str(remote.get("code") or "") or None,
                "document_name": str(remote.get("name") or "") or None,
                "remote_version": str(remote.get("version") or "") or None,
                "remote_content_hash": _content_hash(remote_markdown) if remote_markdown else None,
            }
        except Exception:
            # Optional linking must not make document creation depend on EVA
            # availability; the URL remains an explicit read-only link.
            return binding

    @classmethod
    def create_change(cls, tenant_id: str, actor_id: str, raw: object) -> dict[str, Any]:
        if not isinstance(raw, dict):
            raise ValidationError("INVALID_EVA_CHANGE", "Request body must be an object")
        connector_id = cls._validate_text(raw.get("connector_id"), "connector_id", maximum=32)
        document_id = cls._validate_text(raw.get("document_id"), "document_id", maximum=128)
        change_summary = cls._validate_text(raw.get("change_summary"), "change_summary", maximum=50_000)
        _, client = cls._connector(connector_id, actor_id)
        try:
            source = client.get_document_for_edit(document_id)
        except Exception as error:
            raise cls._map_external_error(error) from error
        base_html = str(source.get("html") or "")
        base_markdown = _html_to_markdown(base_html)
        requested_draft = raw.get("draft_markdown")
        if not base_markdown and requested_draft is None:
            raise ValidationError("EVA_DOCUMENT_EMPTY", "The selected EVA document has no published content")
        document_name = (
            cls._validate_text(raw.get("document_name"), "document_name", maximum=255)
            if raw.get("document_name") is not None
            else cls._validate_text(str(source.get("name") or ""), "document_name", maximum=255)
        )
        draft_markdown = base_markdown if requested_draft is None else cls._validate_text(requested_draft, "draft_markdown", maximum=_MAX_DRAFT_SIZE)
        draft_html = _markdown_to_html(draft_markdown)
        change_id = get_uuid()
        with BusinessDocumentEvaChange._meta.database.atomic():
            change = BusinessDocumentEvaChange.create(
                id=change_id,
                tenant_id=tenant_id,
                owner_id=actor_id,
                connector_id=connector_id,
                eva_project_id=source["project_id"],
                eva_document_id=source["id"],
                eva_document_code=source.get("code") or None,
                eva_document_name=document_name,
                eva_web_url=source.get("web_url") or None,
                change_summary=change_summary,
                base_version=source["version"],
                base_content_hash=_content_hash(base_markdown),
                base_html=base_html,
                base_markdown=base_markdown,
                draft_markdown=draft_markdown,
                draft_html=draft_html,
                draft_content_hash=_content_hash(draft_markdown),
                workflow_state=EvaChangeState.EDITING.value,
                state_version=1,
                last_error=None,
                **_timestamps(),
            )
            cls._record_event(change, actor_id, "CHANGE_REQUEST_CREATED", {"base_version": change.base_version, "base_content_hash": change.base_content_hash})
        return cls._projection(change)

    @classmethod
    def list_changes(cls, tenant_id: str, actor_id: str, page: int = 1, page_size: int = 20) -> dict[str, Any]:
        if page < 1 or page_size < 1 or page_size > 100:
            raise ValidationError("INVALID_PAGINATION", "page must be positive and page_size must be between 1 and 100")
        connector_id = cls._require_configured_connector(cls._configured_connector_id(), actor_id)
        query = BusinessDocumentEvaChange.select().where(
            BusinessDocumentEvaChange.tenant_id == tenant_id,
            BusinessDocumentEvaChange.owner_id == actor_id,
            BusinessDocumentEvaChange.connector_id == connector_id,
        )
        total = query.count()
        rows = query.order_by(BusinessDocumentEvaChange.update_time.desc()).paginate(page, page_size)
        return {
            "items": [
                {
                    "change_id": row.id,
                    "document_name": row.eva_document_name,
                    "document_code": row.eva_document_code,
                    "change_summary": row.change_summary,
                    "workflow_state": row.workflow_state,
                    "state_version": row.state_version,
                    "update_time": row.update_time,
                    "web_url": row.eva_web_url,
                }
                for row in rows
            ],
            "total": total,
            "page": page,
            "page_size": page_size,
        }

    @classmethod
    def get_change(cls, tenant_id: str, actor_id: str, change_id: str) -> dict[str, Any]:
        return cls._projection(cls._get_change(tenant_id, actor_id, change_id))

    @classmethod
    def save_draft(cls, tenant_id: str, actor_id: str, change_id: str, raw: object) -> dict[str, Any]:
        if not isinstance(raw, dict):
            raise ValidationError("INVALID_EVA_DRAFT", "Request body must be an object")
        draft_markdown = cls._validate_text(raw.get("draft_markdown"), "draft_markdown", maximum=_MAX_DRAFT_SIZE, required=False)
        expected = cls._expected_version(raw)
        draft_html = _markdown_to_html(draft_markdown)
        draft_hash = _content_hash(draft_markdown)
        with BusinessDocumentEvaChange._meta.database.atomic():
            change = cls._get_change(tenant_id, actor_id, change_id)
            cls._require_version(change, expected)
            if change.workflow_state not in {EvaChangeState.EDITING.value, EvaChangeState.APPROVED.value}:
                raise ConflictError("EVA_CHANGE_NOT_EDITABLE", "The EVA change request can no longer be edited", {"state": change.workflow_state})
            updated = (
                BusinessDocumentEvaChange.update(
                    draft_markdown=_canonical_markdown(draft_markdown),
                    draft_html=draft_html,
                    draft_content_hash=draft_hash,
                    workflow_state=EvaChangeState.EDITING.value,
                    state_version=change.state_version + 1,
                    approved_at=None,
                    last_error=None,
                )
                .where(BusinessDocumentEvaChange.id == change.id, BusinessDocumentEvaChange.state_version == change.state_version)
                .execute()
            )
            if updated != 1:
                raise ConflictError("STATE_VERSION_CONFLICT", "The EVA change request changed concurrently")
            change = cls._get_change(tenant_id, actor_id, change_id)
            cls._record_event(change, actor_id, "DRAFT_UPDATED", {"draft_content_hash": draft_hash})
        return cls._projection(change)

    @classmethod
    def approve(cls, tenant_id: str, actor_id: str, change_id: str, raw: object) -> dict[str, Any]:
        expected = cls._expected_version(raw)
        with BusinessDocumentEvaChange._meta.database.atomic():
            change = cls._get_change(tenant_id, actor_id, change_id)
            cls._require_version(change, expected)
            if change.workflow_state != EvaChangeState.EDITING.value:
                raise ConflictError("EVA_CHANGE_NOT_EDITING", "Only an editing draft can be approved", {"state": change.workflow_state})
            diff = _section_diff(change.base_markdown, change.draft_markdown)
            if not diff["changed"]:
                raise ValidationError("EVA_DRAFT_UNCHANGED", "Make at least one change before approval")
            approved_at = current_timestamp()
            cls._transition(change, EvaChangeState.APPROVED, approved_at=approved_at, last_error=None)
            change = cls._get_change(tenant_id, actor_id, change_id)
            cls._record_event(change, actor_id, "DRAFT_APPROVED", {"draft_content_hash": change.draft_content_hash})
        return cls._projection(change)

    @classmethod
    def prepare_eva_draft(cls, tenant_id: str, actor_id: str, change_id: str, raw: object) -> dict[str, Any]:
        expected = cls._expected_version(raw)
        force_overwrite = cls._force_overwrite(raw)
        change = cls._reserve(tenant_id, actor_id, change_id, expected, EvaChangeState.APPROVED, EvaChangeState.PREPARING_EVA_DRAFT)
        reservation_version = change.state_version
        reader: EvaWikiConnector | None = None
        user_token_operation = False
        credential_version: int | None = None
        try:
            connector, reader = cls._connector(change.connector_id, actor_id)
            remote = reader.get_document_for_edit(change.eva_document_id)
            if not force_overwrite:
                try:
                    cls._ensure_source_unchanged(change, remote)
                except ConflictError:
                    if cls._remote_published_hash(remote) != cls._serialized_draft_hash(change):
                        raise
            user_token_operation = True
            mutation_client, credential_version = cls._mutation_client(connector, actor_id)
            mutation_client.update_document_draft(change.eva_document_id, change.draft_html, change.eva_document_name)
            user_token_operation = False
            verified = reader.get_document_for_edit(change.eva_document_id)
            if str(verified.get("name") or "").strip() != change.eva_document_name:
                raise ConflictError(
                    "EVA_NAME_VERIFICATION_FAILED",
                    "EVA returned a different document name after saving",
                    {"expected": change.eva_document_name, "actual": str(verified.get("name") or "").strip()},
                )
        except Exception as error:
            mapped = cls._map_user_token_error(error) if user_token_operation else cls._map_external_error(error)
            cls._restore_after_external_failure(change.id, reservation_version, EvaChangeState.PREPARING_EVA_DRAFT, EvaChangeState.APPROVED, mapped)
            raise mapped from error
        with BusinessDocumentEvaChange._meta.database.atomic():
            change = cls._get_change(tenant_id, actor_id, change_id)
            if change.workflow_state != EvaChangeState.PREPARING_EVA_DRAFT.value or change.state_version != reservation_version:
                raise ConflictError("STATE_VERSION_CONFLICT", "The EVA change request changed concurrently")
            cls._transition(change, EvaChangeState.EVA_DRAFT_READY, eva_draft_at=current_timestamp(), last_error=None)
            change = cls._get_change(tenant_id, actor_id, change_id)
            event_payload = {"draft_content_hash": change.draft_content_hash}
            if credential_version is not None:
                event_payload["user_credential_version"] = credential_version
            cls._record_event(change, actor_id, "EVA_DRAFT_SAVED", event_payload)
        return cls._projection(change)

    @classmethod
    def publish(cls, tenant_id: str, actor_id: str, change_id: str, raw: object) -> dict[str, Any]:
        expected = cls._expected_version(raw)
        force_overwrite = cls._force_overwrite(raw)
        change = cls._reserve(tenant_id, actor_id, change_id, expected, EvaChangeState.EVA_DRAFT_READY, EvaChangeState.PUBLISHING)
        reservation_version = change.state_version
        reader: EvaWikiConnector | None = None
        published: dict[str, Any] | None = None
        user_token_operation = False
        credential_version: int | None = None
        try:
            connector, reader = cls._connector(change.connector_id, actor_id)
            remote = reader.get_document_for_edit(change.eva_document_id)
            serialized_draft_hash = cls._serialized_draft_hash(change)
            published_hash = cls._remote_published_hash(remote)
            name_matches = cls._remote_name_matches(remote, change)
            if published_hash == serialized_draft_hash and name_matches:
                published = remote
            else:
                if force_overwrite:
                    user_token_operation = True
                    mutation_client, credential_version = cls._mutation_client(connector, actor_id)
                    mutation_client.update_document_draft(change.eva_document_id, change.draft_html, change.eva_document_name)
                else:
                    if published_hash == serialized_draft_hash and not name_matches:
                        cls._raise_source_conflict(change, remote, published_hash)
                    cls._ensure_source_unchanged(change, remote)
                    remote_draft_hash = _content_hash(_html_to_markdown(str(remote.get("draft_html") or "")))
                    if remote_draft_hash != serialized_draft_hash:
                        raise ConflictError(
                            "EVA_DRAFT_CONFLICT",
                            "The EVA draft changed after it was prepared",
                            {"expected": serialized_draft_hash, "actual": remote_draft_hash},
                        )
                    user_token_operation = True
                    mutation_client, credential_version = cls._mutation_client(connector, actor_id)
                mutation_client.publish_document(change.eva_document_id)
                user_token_operation = False
                published = reader.get_document_for_edit(change.eva_document_id)
                published_hash = cls._remote_published_hash(published)
                if published_hash != serialized_draft_hash:
                    raise ConflictError(
                        "EVA_PUBLISH_VERIFICATION_FAILED",
                        "EVA published content does not match the approved draft",
                        {"expected": serialized_draft_hash, "actual": published_hash},
                    )
                if str(published.get("name") or "").strip() != change.eva_document_name:
                    raise ConflictError(
                        "EVA_NAME_VERIFICATION_FAILED",
                        "EVA returned a different document name after publishing",
                        {"expected": change.eva_document_name, "actual": str(published.get("name") or "").strip()},
                    )
        except Exception as error:
            recovered = cls._published_remote_if_matching(reader, change)
            if recovered is not None:
                published = recovered
            else:
                mapped = cls._map_user_token_error(error) if user_token_operation else cls._map_external_error(error)
                cls._restore_after_external_failure(change.id, reservation_version, EvaChangeState.PUBLISHING, EvaChangeState.EVA_DRAFT_READY, mapped)
                raise mapped from error
        assert published is not None
        with BusinessDocumentEvaChange._meta.database.atomic():
            change = cls._get_change(tenant_id, actor_id, change_id)
            if change.workflow_state != EvaChangeState.PUBLISHING.value or change.state_version != reservation_version:
                raise ConflictError("STATE_VERSION_CONFLICT", "The EVA change request changed concurrently")
            cls._transition(
                change,
                EvaChangeState.PUBLISHED,
                published_at=current_timestamp(),
                published_version=str(published.get("version") or ""),
                last_error=None,
            )
            change = cls._get_change(tenant_id, actor_id, change_id)
            event_payload = {"published_version": change.published_version}
            if credential_version is not None:
                event_payload["user_credential_version"] = credential_version
            cls._record_event(change, actor_id, "EVA_DOCUMENT_PUBLISHED", event_payload)
        return cls._projection(change)

    @classmethod
    def _reserve(
        cls,
        tenant_id: str,
        actor_id: str,
        change_id: str,
        expected_version: int,
        required_state: EvaChangeState,
        reserved_state: EvaChangeState,
    ) -> BusinessDocumentEvaChange:
        with BusinessDocumentEvaChange._meta.database.atomic():
            change = cls._get_change(tenant_id, actor_id, change_id)
            cls._require_version(change, expected_version)
            if change.workflow_state == reserved_state.value:
                age_ms = current_timestamp() - int(change.update_time or change.create_time or 0)
                if age_ms < _EXTERNAL_OPERATION_TIMEOUT_MS:
                    raise ConflictError(
                        "EVA_CHANGE_BUSY",
                        "The EVA change request is still processing",
                        {"state": change.workflow_state, "retry_after_ms": _EXTERNAL_OPERATION_TIMEOUT_MS - max(age_ms, 0)},
                    )
                cls._transition(change, reserved_state, last_error=None)
                change = cls._get_change(tenant_id, actor_id, change_id)
                cls._record_event(change, actor_id, "EXTERNAL_OPERATION_RETRIED", {"state": change.workflow_state})
            elif change.workflow_state == required_state.value:
                cls._transition(change, reserved_state, last_error=None)
            else:
                raise ConflictError("EVA_CHANGE_STATE_CONFLICT", "The EVA change request is not ready for this action", {"state": change.workflow_state})
        return cls._get_change(tenant_id, actor_id, change_id)

    @classmethod
    def _restore_after_external_failure(
        cls,
        change_id: str,
        reservation_version: int,
        reserved_state: EvaChangeState,
        stable_state: EvaChangeState,
        error: BusinessDocumentError,
    ) -> None:
        with BusinessDocumentEvaChange._meta.database.atomic():
            change = BusinessDocumentEvaChange.get_or_none(BusinessDocumentEvaChange.id == change_id)
            if change is None or change.state_version != reservation_version or change.workflow_state != reserved_state.value:
                return
            cls._transition(change, stable_state, last_error={"code": error.code, "message": error.message, "details": error.details})

    @classmethod
    def _ensure_source_unchanged(cls, change: BusinessDocumentEvaChange, remote: dict[str, Any]) -> None:
        actual_hash = cls._remote_published_hash(remote)
        if actual_hash != change.base_content_hash:
            cls._raise_source_conflict(change, remote, actual_hash)

    @staticmethod
    def _raise_source_conflict(change: BusinessDocumentEvaChange, remote: dict[str, Any], actual_hash: str) -> None:
        raise ConflictError(
            "EVA_SOURCE_VERSION_CONFLICT",
            "The published EVA document changed after this change request was created",
            {
                "expected_version": change.base_version,
                "actual_version": remote.get("version"),
                "expected_hash": change.base_content_hash,
                "actual_hash": actual_hash,
                "expected_name": change.eva_document_name,
                "actual_name": str(remote.get("name") or "").strip(),
                "confirmation_required": True,
                "confirmation_action": "OVERWRITE_EVA_DOCUMENT",
            },
        )

    @classmethod
    def _published_remote_if_matching(cls, client: EvaWikiConnector | None, change: BusinessDocumentEvaChange) -> dict[str, Any] | None:
        if client is None:
            return None
        try:
            remote = client.get_document_for_edit(change.eva_document_id)
        except Exception:
            return None
        return remote if cls._remote_published_hash(remote) == cls._serialized_draft_hash(change) and cls._remote_name_matches(remote, change) else None

    @staticmethod
    def _remote_name_matches(remote: dict[str, Any], change: BusinessDocumentEvaChange) -> bool:
        return str(remote.get("name") or "").strip() == change.eva_document_name

    @staticmethod
    def _serialized_draft_hash(change: BusinessDocumentEvaChange) -> str:
        """Hash the representation that is actually sent to and returned by EVA."""

        return _content_hash(_html_to_markdown(change.draft_html))

    @staticmethod
    def _remote_published_hash(remote: dict[str, Any]) -> str:
        return _content_hash(_html_to_markdown(str(remote.get("html") or "")))

    @staticmethod
    def _expected_version(raw: object) -> int:
        if not isinstance(raw, dict) or isinstance(raw.get("expected_state_version"), bool) or not isinstance(raw.get("expected_state_version"), int):
            raise ValidationError("INVALID_EVA_CHANGE", "expected_state_version must be an integer")
        return raw["expected_state_version"]

    @staticmethod
    def _force_overwrite(raw: object) -> bool:
        if not isinstance(raw, dict):
            raise ValidationError("INVALID_EVA_CHANGE", "Request body must be an object")
        value = raw.get("force_overwrite", False)
        if not isinstance(value, bool):
            raise ValidationError("INVALID_EVA_CHANGE", "force_overwrite must be a boolean", {"field": "force_overwrite"})
        return value

    @staticmethod
    def _require_version(change: BusinessDocumentEvaChange, expected: int) -> None:
        if change.state_version != expected:
            raise ConflictError(
                "STATE_VERSION_CONFLICT",
                "The EVA change request changed concurrently",
                {"expected": expected, "actual": change.state_version},
            )

    @staticmethod
    def _transition(change: BusinessDocumentEvaChange, state: EvaChangeState, **values: Any) -> None:
        updated = (
            BusinessDocumentEvaChange.update(workflow_state=state.value, state_version=change.state_version + 1, **values)
            .where(BusinessDocumentEvaChange.id == change.id, BusinessDocumentEvaChange.state_version == change.state_version)
            .execute()
        )
        if updated != 1:
            raise ConflictError("STATE_VERSION_CONFLICT", "The EVA change request changed concurrently")

    @classmethod
    def _get_change(cls, tenant_id: str, actor_id: str, change_id: str) -> BusinessDocumentEvaChange:
        change = BusinessDocumentEvaChange.get_or_none(
            BusinessDocumentEvaChange.id == str(change_id or "").strip(),
            BusinessDocumentEvaChange.tenant_id == tenant_id,
            BusinessDocumentEvaChange.owner_id == actor_id,
        )
        if change is None:
            raise BusinessDocumentError("EVA_CHANGE_NOT_FOUND", "EVA document change request not found", 404)
        cls._require_configured_connector(change.connector_id, actor_id)
        return change

    @staticmethod
    def _record_event(change: BusinessDocumentEvaChange, actor_id: str, event_type: str, payload: dict[str, Any]) -> None:
        sequence = (BusinessDocumentEvaChangeEvent.select(fn.MAX(BusinessDocumentEvaChangeEvent.sequence)).where(BusinessDocumentEvaChangeEvent.change_id == change.id).scalar() or 0) + 1
        BusinessDocumentEvaChangeEvent.create(
            id=get_uuid(),
            change_id=change.id,
            sequence=sequence,
            event_type=event_type,
            actor_id=actor_id,
            payload=payload,
            **_timestamps(),
        )

    @classmethod
    def _projection(cls, change: BusinessDocumentEvaChange) -> dict[str, Any]:
        diff = _section_diff(change.base_markdown, change.draft_markdown)
        allowed_actions: list[str] = []
        if change.workflow_state in {EvaChangeState.EDITING.value, EvaChangeState.APPROVED.value}:
            allowed_actions.append("SAVE_DRAFT")
        if change.workflow_state == EvaChangeState.EDITING.value and diff["changed"]:
            allowed_actions.append("APPROVE")
        if change.workflow_state == EvaChangeState.APPROVED.value:
            allowed_actions.append("PREPARE_EVA_DRAFT")
        if change.workflow_state == EvaChangeState.EVA_DRAFT_READY.value:
            allowed_actions.append("PUBLISH_EVA")
        operation_retry_after_ms: int | None = None
        if change.workflow_state in {EvaChangeState.PREPARING_EVA_DRAFT.value, EvaChangeState.PUBLISHING.value}:
            operation_age_ms = max(current_timestamp() - int(change.update_time or change.create_time or 0), 0)
            operation_retry_after_ms = max(_EXTERNAL_OPERATION_TIMEOUT_MS - operation_age_ms, 0)
            if operation_retry_after_ms == 0:
                allowed_actions.append("PREPARE_EVA_DRAFT" if change.workflow_state == EvaChangeState.PREPARING_EVA_DRAFT.value else "PUBLISH_EVA")
        events = BusinessDocumentEvaChangeEvent.select().where(BusinessDocumentEvaChangeEvent.change_id == change.id).order_by(BusinessDocumentEvaChangeEvent.sequence.desc()).limit(20)
        return {
            "change_id": change.id,
            "state_version": change.state_version,
            "workflow_state": change.workflow_state,
            "change_summary": change.change_summary,
            "source": {
                "connector_id": change.connector_id,
                "project_id": change.eva_project_id,
                "document_id": change.eva_document_id,
                "document_code": change.eva_document_code,
                "document_name": change.eva_document_name,
                "web_url": change.eva_web_url,
                "base_version": change.base_version,
                "base_content_hash": change.base_content_hash,
            },
            "base_markdown": change.base_markdown,
            "draft_markdown": change.draft_markdown,
            "draft_content_hash": change.draft_content_hash,
            "diff": diff,
            "allowed_actions": allowed_actions,
            "approved_at": change.approved_at,
            "eva_draft_at": change.eva_draft_at,
            "published_at": change.published_at,
            "published_version": change.published_version,
            "last_error": change.last_error or None,
            "operation_retry_after_ms": operation_retry_after_ms,
            "events": [
                {
                    "event_id": event.id,
                    "sequence": event.sequence,
                    "event_type": event.event_type,
                    "actor_id": event.actor_id,
                    "payload": event.payload,
                    "create_time": event.create_time,
                }
                for event in reversed(list(events))
            ],
        }
