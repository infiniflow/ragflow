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

from __future__ import annotations

import hashlib
import json
import logging
from datetime import datetime
from typing import Any
import unicodedata

from peewee import IntegrityError, fn

from api.apps.business_documents.assets import (
    apply_change_plan,
    prompt_descriptor,
    published_template,
    process_policy,
    render_document_ast,
    render_section_text,
    section_hash,
    validate_contract,
    validate_document_ast,
)
from api.apps.business_documents.authorization import BusinessDocumentAccess, BusinessDocumentRole
from api.apps.business_documents.contracts import CommandEnvelope, CommandType, LifecycleState, OperationState
from api.apps.business_documents.errors import BusinessDocumentError, ConflictError, NotFoundError, PermissionDeniedError, ValidationError
from api.apps.business_documents.evidence import ensure_dataset_access, ensure_dataset_embedding_compatibility, related_file_search_enabled
from api.db.db_models import (
    BusinessDocument,
    BusinessDocumentAnswer,
    BusinessDocumentCommand,
    BusinessDocumentComment,
    BusinessDocumentEvent,
    BusinessDocumentEvaBinding,
    BusinessDocumentEvidenceSnapshot,
    BusinessDocumentExportArtifact,
    BusinessDocumentJob,
    BusinessDocumentProposal,
    BusinessDocumentProposalDecision,
    BusinessDocumentQuestion,
    BusinessDocumentRevision,
    User,
)
from business_documents.domain.names import normalize_title, title_key
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp


_MODEL_TABLES = (
    BusinessDocument,
    BusinessDocumentRevision,
    BusinessDocumentQuestion,
    BusinessDocumentAnswer,
    BusinessDocumentProposal,
    BusinessDocumentProposalDecision,
    BusinessDocumentComment,
    BusinessDocumentEvent,
    BusinessDocumentEvaBinding,
    BusinessDocumentCommand,
    BusinessDocumentJob,
    BusinessDocumentExportArtifact,
    BusinessDocumentEvidenceSnapshot,
)

_MAX_EVA_SYNC_MARKDOWN_SIZE = 100_000


def _timestamps() -> dict[str, Any]:
    now = datetime.now()
    timestamp = current_timestamp()
    return {"create_time": timestamp, "create_date": now, "update_time": timestamp, "update_date": now}


def _sha256_text(value: str) -> str:
    return f"sha256:{hashlib.sha256(value.encode('utf-8')).hexdigest()}"


def _stable_hash(value: object) -> str:
    raw = json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    return _sha256_text(raw)


def _fixed_hash(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _binding_keys(binding: dict[str, Any]) -> tuple[str, str | None]:
    page_url = str(binding.get("page_url") or "").strip().rstrip("/").casefold()
    page_url_key = _fixed_hash(page_url)
    eva_origin = str(binding.get("eva_origin") or "").strip().rstrip("/").casefold()
    project_id = str(binding.get("project_id") or "").strip()
    document_id = str(binding.get("document_id") or binding.get("id") or "").strip()
    identity_key = _fixed_hash("\x1f".join((eva_origin, project_id, document_id))) if eva_origin and project_id and document_id else None
    return page_url_key, identity_key


def _canonical_semantic_text(value: str) -> str:
    return " ".join(unicodedata.normalize("NFKC", value).casefold().split())


def _canonical_semantic_tag(value: str) -> str:
    return unicodedata.normalize("NFKC", value).strip().casefold()


def _utf16_length(value: str) -> int:
    return len(value.encode("utf-16-le")) // 2


def _utf16_slice(value: str, start: int, end: int) -> str:
    try:
        return value.encode("utf-16-le")[start * 2 : end * 2].decode("utf-16-le")
    except UnicodeDecodeError as exc:
        raise ValidationError("INVALID_COMMENT_ANCHOR", "Anchor offsets split a Unicode character") from exc


def _utf16_context_slice(value: str, start: int, end: int, *, trim_start: bool) -> str:
    try:
        return _utf16_slice(value, start, end)
    except ValidationError:
        # A 64-unit context boundary may land between a surrogate pair. Keep
        # the selection boundary fixed and shrink the context inward by one.
        return _utf16_slice(value, start + 1, end) if trim_start else _utf16_slice(value, start, end - 1)


class BusinessDocumentService:
    """Application boundary for business-document commands and projections.

    LLM and export work is represented by ``BusinessDocumentJob`` rows. Workers
    can only commit results through ``complete_job`` so optimistic version and
    workflow guards are checked again after generation.
    """

    @classmethod
    def model_tables(cls):
        """Return all owned models for database bootstrap and isolated tests."""

        return _MODEL_TABLES

    @classmethod
    def create_document(
        cls,
        tenant_id: str,
        actor_id: str,
        raw: object,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
    ) -> dict[str, Any]:
        access = BusinessDocumentAccess(actor_id, access_role, is_admin)
        access.require_create()
        if not isinstance(raw, dict):
            raise ValidationError("INVALID_DOCUMENT", "Request body must be a JSON object")
        if "chat_id" in raw:
            raise ValidationError("CHAT_ID_NOT_ALLOWED", "chat_id is assigned by the business document channel")
        schema_version = raw.get("schema_version")
        if schema_version != "2" and raw.get("eva_page_url"):
            raise ValidationError(
                "EVA_BINDING_REQUIRES_SCHEMA_V2",
                "EVA page binding requires create-document schema version 2 and explicit replacement confirmation",
            )
        if raw.get("eva_page_url"):
            requested_decision = raw.get("eva_decision")
            if (
                not isinstance(requested_decision, dict)
                or requested_decision.get("mode") != "BIND"
                or requested_decision.get("confirm_replace") is not True
            ):
                raise ValidationError(
                    "EVA_REPLACE_CONFIRMATION_REQUIRED",
                    "Confirm that publishing this document will replace the current EVA page content",
                )
        validate_contract("create_document_v2" if schema_version == "2" else "create_document", raw)
        title = raw.get("title")
        idea = raw.get("idea")
        dataset_ids = raw.get("dataset_ids", [])
        if not isinstance(title, str) or not title.strip():
            raise ValidationError("INVALID_TITLE", "title must be a non-empty string")
        if not isinstance(idea, str) or not idea.strip():
            raise ValidationError("INVALID_IDEA", "idea must be a non-empty string")
        display_title = " ".join(unicodedata.normalize("NFKC", title).strip().split())
        normalized_title_key = title_key(display_title)
        ensure_dataset_access(actor_id, dataset_ids)
        ensure_dataset_embedding_compatibility(dataset_ids)
        if BusinessDocument.select().where(BusinessDocument.title_key == normalized_title_key).exists():
            raise ConflictError("DOCUMENT_TITLE_ALREADY_EXISTS", "A business document with this title already exists")
        document_id = get_uuid()
        chat_id = f"business-document:{document_id}"
        document_type = raw.get("document_type", "business_requirements")
        if document_type != "business_requirements":
            raise ValidationError("UNSUPPORTED_DOCUMENT_TYPE", "Only business_requirements is supported")
        template_version = raw.get("template_version", published_template()["template_version"])
        policy_version = raw.get("policy_version", process_policy()["policy_version"])
        if not all(isinstance(value, str) and value.strip() for value in (template_version, policy_version)):
            raise ValidationError("INVALID_CONFIGURATION_VERSION", "template_version and policy_version are required")
        if template_version != published_template()["template_version"]:
            raise ValidationError("TEMPLATE_NOT_PUBLISHED", "Requested business requirements template version is not published")
        if policy_version != process_policy()["policy_version"]:
            raise ValidationError("POLICY_NOT_PUBLISHED", "Requested business requirements policy version is not published")
        eva_binding = cls._resolve_create_eva_binding(actor_id, raw, schema_version, display_title)

        database = BusinessDocument._meta.database
        try:
            with database.atomic():
                if BusinessDocument.select().where(BusinessDocument.title_key == normalized_title_key).exists():
                    raise ConflictError("DOCUMENT_TITLE_ALREADY_EXISTS", "A business document with this title already exists")
                if BusinessDocument.select().where(BusinessDocument.chat_id == chat_id.strip()).exists():
                    raise ConflictError("CHAT_ALREADY_BOUND", "The RAGFlow chat is already bound to a business document")
                BusinessDocument.create(
                    id=document_id,
                    tenant_id=tenant_id,
                    owner_id=actor_id,
                    chat_id=chat_id.strip(),
                    document_type=document_type,
                    title=display_title,
                    title_key=normalized_title_key,
                    idea=idea.strip(),
                    dataset_ids=dataset_ids,
                    template_version=template_version.strip(),
                    policy_version=policy_version.strip(),
                    lifecycle_state=LifecycleState.INTAKE.value,
                    operation_state=OperationState.IDLE.value,
                    state_version=1,
                    active_review_cycle=0,
                    **_timestamps(),
                )
                if eva_binding:
                    cls._store_eva_binding(document_id, eva_binding)
                cls._create_event(
                    document_id=document_id,
                    sequence=1,
                    event_type="DocumentCreated",
                    actor_type="USER",
                    actor_id=actor_id,
                    payload={
                        "chat_id": chat_id.strip(),
                        "document_type": document_type,
                        "idea": idea.strip(),
                        **({"eva_binding": eva_binding} if eva_binding else {}),
                    },
                    correlation_id=document_id,
                )
        except IntegrityError as error:
            if BusinessDocument.select().where(BusinessDocument.title_key == normalized_title_key).exists():
                raise ConflictError("DOCUMENT_TITLE_ALREADY_EXISTS", "A business document with this title already exists") from error
            if eva_binding:
                raise ConflictError("EVA_PAGE_ALREADY_LINKED", "The selected EVA page is already linked to another document") from error
            raise
        return cls.get_document(tenant_id, document_id, actor_id, is_admin, access_role)

    @classmethod
    def get_document(
        cls,
        tenant_id: str,
        document_id: str,
        actor_id: str,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
    ) -> dict[str, Any]:
        access = BusinessDocumentAccess(actor_id, access_role, is_admin)
        document = cls._get_accessible_document(document_id)
        return cls._project(document, access)

    @classmethod
    def list_documents(
        cls,
        tenant_id: str,
        actor_id: str,
        page: int = 1,
        page_size: int = 20,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
        scope: str = "all",
    ) -> dict[str, Any]:
        if not isinstance(page, int) or not isinstance(page_size, int) or page < 1 or not 1 <= page_size <= 100:
            raise ValidationError("INVALID_PAGINATION", "page must be positive and page_size must be between 1 and 100")
        if scope not in {"mine", "all"}:
            raise ValidationError("INVALID_DOCUMENT_SCOPE", "scope must be mine or all")
        access = BusinessDocumentAccess(actor_id, access_role, is_admin)
        query = BusinessDocument.select()
        if scope == "mine":
            query = query.where(BusinessDocument.owner_id == actor_id)
        total = query.count()
        rows = list(query.order_by(BusinessDocument.update_time.desc()).paginate(page, page_size))
        owner_ids = {row.owner_id for row in rows if row.owner_id}
        owner_names = {owner_id: cls._user_display_name(owner_id) for owner_id in owner_ids}
        return {
            "items": [
                {
                    "document_id": row.id,
                    "owner_id": row.owner_id,
                    "owner_name": owner_names.get(row.owner_id),
                    "title": row.title,
                    "lifecycle_state": row.lifecycle_state,
                    "operation_state": row.operation_state,
                    "state_version": row.state_version,
                    "current_revision_number": (BusinessDocumentRevision.get_by_id(row.current_revision_id).revision_number if row.current_revision_id else None),
                    "eva_page_url": (cls._eva_binding(row) or {}).get("page_url"),
                    "latest_job": cls._latest_job(row.id),
                    "update_time": row.update_time,
                    "access_role": access.role.value,
                    "permissions": access.permissions(row.owner_id),
                }
                for row in rows
            ],
            "page": page,
            "page_size": page_size,
            "total": total,
            "scope": scope,
            "access_role": access.role.value,
            "capabilities": access.capabilities(),
        }

    @classmethod
    def list_access_users(
        cls,
        actor_id: str,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
    ) -> dict[str, Any]:
        access = BusinessDocumentAccess(actor_id, access_role, is_admin)
        access.require_assign()
        users = User.select(User.id, User.nickname, User.email, User.business_document_role, User.is_superuser).where((User.status == "1") & (User.is_active == "1"))
        return {
            "items": [
                {
                    "user_id": user.id,
                    "nickname": user.nickname,
                    "email": user.email,
                    "role": (BusinessDocumentRole.ADMIN.value if user.is_superuser else cls._normalized_access_role(user.business_document_role).value),
                }
                for user in users.order_by(User.nickname.asc(), User.id.asc())
            ]
        }

    @classmethod
    def update_user_access_role(cls, actor_id: str, user_id: str, raw: object, is_admin: bool = False) -> dict[str, Any]:
        if not is_admin:
            raise PermissionDeniedError("Only an administrator can change business document roles")
        if not isinstance(raw, dict):
            raise ValidationError("INVALID_ACCESS_ROLE", "Request body must be a JSON object")
        try:
            role = BusinessDocumentRole(raw.get("role"))
        except (TypeError, ValueError) as exc:
            raise ValidationError("INVALID_ACCESS_ROLE", "Unknown business document role") from exc
        if role == BusinessDocumentRole.ADMIN:
            raise ValidationError("INVALID_ACCESS_ROLE", "Administrator access is controlled by the superuser flag")
        user = User.get_or_none(User.id == user_id)
        if user is None:
            raise BusinessDocumentError("USER_NOT_FOUND", "User not found", 404)
        if user.is_superuser:
            raise ConflictError("ADMIN_ROLE_MANAGED_SEPARATELY", "A superuser always has administrator access")
        User.update(business_document_role=role.value, update_time=current_timestamp(), update_date=datetime.now()).where(User.id == user_id).execute()
        return {"user_id": user_id, "nickname": user.nickname, "role": role.value}

    @classmethod
    def assign_document(
        cls,
        actor_id: str,
        document_id: str,
        raw: object,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
    ) -> dict[str, Any]:
        access = BusinessDocumentAccess(actor_id, access_role, is_admin)
        access.require_assign()
        if not isinstance(raw, dict):
            raise ValidationError("INVALID_DOCUMENT_ASSIGNMENT", "Request body must be a JSON object")
        owner_id = raw.get("owner_id")
        expected_state_version = raw.get("expected_state_version")
        if not isinstance(owner_id, str) or not owner_id.strip():
            raise ValidationError("INVALID_DOCUMENT_ASSIGNMENT", "owner_id must be a non-empty string")
        if isinstance(expected_state_version, bool) or not isinstance(expected_state_version, int):
            raise ValidationError("INVALID_DOCUMENT_ASSIGNMENT", "expected_state_version must be an integer")
        new_owner = User.get_or_none((User.id == owner_id.strip()) & (User.status == "1") & (User.is_active == "1"))
        if new_owner is None:
            raise BusinessDocumentError("USER_NOT_FOUND", "Assigned user not found", 404)

        database = BusinessDocument._meta.database
        with database.atomic():
            document = cls._get_accessible_document(document_id)
            if document.state_version != expected_state_version:
                raise ConflictError(
                    "STATE_VERSION_CONFLICT",
                    "The document changed since it was loaded",
                    {"expected": expected_state_version, "actual": document.state_version},
                )
            if document.owner_id == new_owner.id:
                return cls._project(document, access)
            previous_owner_id = document.owner_id
            new_version = document.state_version + 1
            cls._optimistic_update(document, {"owner_id": new_owner.id, "state_version": new_version})
            cls._create_event(
                document.id,
                new_version,
                "DocumentAssigned",
                "USER",
                actor_id,
                {"previous_owner_id": previous_owner_id, "owner_id": new_owner.id},
                get_uuid(),
            )
        return cls._project(cls._get_accessible_document(document_id), access)

    @classmethod
    def list_revisions(cls, tenant_id: str, document_id: str, actor_id: str, is_admin: bool = False) -> list[dict[str, Any]]:
        cls._get_accessible_document(document_id)
        rows = BusinessDocumentRevision.select().where(BusinessDocumentRevision.document_id == document_id).order_by(BusinessDocumentRevision.revision_number.asc())
        return [cls._revision_dict(row) for row in rows]

    @classmethod
    def get_revision(cls, tenant_id: str, document_id: str, revision_id: str, actor_id: str, is_admin: bool = False) -> dict[str, Any]:
        cls._get_accessible_document(document_id)
        row = BusinessDocumentRevision.get_or_none((BusinessDocumentRevision.id == revision_id) & (BusinessDocumentRevision.document_id == document_id))
        if row is None:
            raise BusinessDocumentError("REVISION_NOT_FOUND", "Business document revision not found", 404)
        return cls._revision_dict(row)

    @classmethod
    def pull_from_eva(
        cls,
        tenant_id: str,
        actor_id: str,
        document_id: str,
        raw: object,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
    ) -> dict[str, Any]:
        """Bring a newer EVA page into the governed review protocol.

        The remote content is recorded as an immutable review input. It never
        overwrites a local revision directly; the existing review assessment and
        change-plan gates remain responsible for producing the next revision.
        """

        if not isinstance(raw, dict) or isinstance(raw.get("expected_state_version"), bool) or not isinstance(raw.get("expected_state_version"), int):
            raise ValidationError("INVALID_EVA_SYNC", "expected_state_version must be an integer")
        expected = raw["expected_state_version"]
        access = BusinessDocumentAccess(actor_id, access_role, is_admin)
        document = cls._get_editable_document(document_id, access)
        document_tenant_id = document.tenant_id
        if document.state_version != expected:
            raise ConflictError("STATE_VERSION_CONFLICT", "The document changed since it was loaded", {"expected": expected, "actual": document.state_version})
        cls._require_idle(document)
        if document.lifecycle_state not in {LifecycleState.AGREED.value, LifecycleState.REVIEW.value} or not document.current_revision_id:
            raise ConflictError("EVA_SYNC_REVIEW_REQUIRED", "EVA content can be pulled only after the first local revision exists")
        binding = cls._eva_binding(document)
        if not binding or "PULL_FROM_EVA" not in binding.get("capabilities", []):
            raise ConflictError("EVA_SYNC_UNAVAILABLE", "The linked EVA page is not connected to an accessible connector")

        from api.apps.business_documents.eva_changes import EvaDocumentChangeService, _content_hash, _html_to_markdown

        try:
            _, client = EvaDocumentChangeService._connector(binding["connector_id"], actor_id)
            remote = client.get_document_for_edit(binding["document_id"])
        except Exception as error:
            raise EvaDocumentChangeService._map_external_error(error) from error
        remote_markdown = _html_to_markdown(str(remote.get("html") or ""))
        if not remote_markdown:
            raise ValidationError("EVA_DOCUMENT_EMPTY", "The linked EVA document has no published content")
        if len(remote_markdown) > _MAX_EVA_SYNC_MARKDOWN_SIZE:
            raise ValidationError(
                "EVA_DOCUMENT_TOO_LARGE",
                "The linked EVA document is too large for governed synchronization",
                {"maximum": _MAX_EVA_SYNC_MARKDOWN_SIZE},
            )
        remote_hash = _content_hash(remote_markdown)
        current_revision = BusinessDocumentRevision.get_by_id(document.current_revision_id)
        pull_is_pending = document.lifecycle_state == LifecycleState.REVIEW.value and binding.get("last_pull_review_cycle") == document.active_review_cycle
        pull_is_consumed = binding.get("last_pull_event_id") in (current_revision.source_event_ids or [])
        if binding.get("last_pulled_content_hash") == remote_hash and (pull_is_pending or pull_is_consumed):
            return {
                "document": cls._project(document, access),
                "sync": {"changed": False, "direction": "FROM_EVA", "remote_version": str(remote.get("version") or "") or None},
            }

        database = BusinessDocument._meta.database
        with database.atomic():
            document = cls._get_editable_document(document_id, access)
            if document.state_version != expected:
                raise ConflictError("STATE_VERSION_CONFLICT", "The document changed during EVA synchronization", {"expected": expected, "actual": document.state_version})
            current_binding = cls._eva_binding(document)
            if not current_binding or current_binding.get("connector_id") != binding.get("connector_id") or current_binding.get("document_id") != binding.get("document_id"):
                raise ConflictError("EVA_BINDING_CONFLICT", "The EVA link changed during synchronization")
            review_cycle = document.active_review_cycle + 1 if document.lifecycle_state == LifecycleState.AGREED.value else document.active_review_cycle
            new_version = document.state_version + 1
            cls._optimistic_update(
                document,
                {
                    "lifecycle_state": LifecycleState.REVIEW.value,
                    "active_review_cycle": review_cycle,
                    "state_version": new_version,
                    "last_error": None,
                },
            )
            event_id = cls._create_event(
                document.id,
                new_version,
                "EvaDocumentPulled",
                "USER",
                actor_id,
                {
                    "review_cycle": review_cycle,
                    "revision_id": document.current_revision_id,
                    "page_url": binding["page_url"],
                    "connector_id": binding["connector_id"],
                    "document_id": binding["document_id"],
                    "remote_version": str(remote.get("version") or "") or None,
                    "remote_content_hash": remote_hash,
                    "remote_markdown": remote_markdown,
                },
                get_uuid(),
            )
            updated_binding = dict(current_binding)
            updated_binding.update(
                {
                    "remote_version": str(remote.get("version") or "") or None,
                    "remote_content_hash": remote_hash,
                    "last_pulled_content_hash": remote_hash,
                    "last_pulled_at": current_timestamp(),
                    "last_pull_event_id": event_id,
                    "last_pull_review_cycle": review_cycle,
                }
            )
            cls._store_eva_binding(document.id, updated_binding, replace=True)
        projection = cls._project(cls._get_document(document_tenant_id, document_id), access)
        return {
            "document": projection,
            "sync": {"changed": True, "direction": "FROM_EVA", "event_id": event_id, "remote_version": str(remote.get("version") or "") or None},
        }

    @classmethod
    def rebind_eva(
        cls,
        tenant_id: str,
        actor_id: str,
        document_id: str,
        raw: object,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
    ) -> dict[str, Any]:
        """Re-resolve an existing EVA link without rewriting its creation event."""

        if not isinstance(raw, dict) or isinstance(raw.get("expected_state_version"), bool) or not isinstance(raw.get("expected_state_version"), int):
            raise ValidationError("INVALID_EVA_BINDING", "expected_state_version must be an integer")
        expected = raw["expected_state_version"]
        access = BusinessDocumentAccess(actor_id, access_role, is_admin)
        document = cls._get_editable_document(document_id, access)
        document_tenant_id = document.tenant_id
        if document.state_version != expected:
            raise ConflictError("STATE_VERSION_CONFLICT", "The document changed since it was loaded", {"expected": expected, "actual": document.state_version})
        cls._require_idle(document)
        if document.lifecycle_state == LifecycleState.ARCHIVED.value:
            raise ConflictError("LIFECYCLE_STATE_CONFLICT", "Archived documents cannot be reconnected to EVA")
        current_binding = cls._eva_binding(document)
        if not current_binding:
            raise ConflictError("EVA_BINDING_MISSING", "The document has no EVA page link")

        from api.apps.business_documents.eva_changes import EvaDocumentChangeService

        resolved = EvaDocumentChangeService.resolve_page_url(actor_id, current_binding["page_url"])
        if resolved.get("status") != "CONNECTED":
            raise ConflictError(
                "EVA_BINDING_UNAVAILABLE",
                "No accessible EVA connector matches the linked page",
                {"document_code": resolved.get("document_code")},
            )

        database = BusinessDocument._meta.database
        with database.atomic():
            document = cls._get_editable_document(document_id, access)
            if document.state_version != expected:
                raise ConflictError("STATE_VERSION_CONFLICT", "The document changed during EVA reconnection", {"expected": expected, "actual": document.state_version})
            latest_binding = cls._eva_binding(document)
            if not latest_binding or latest_binding.get("page_url") != current_binding.get("page_url"):
                raise ConflictError("EVA_BINDING_CONFLICT", "The EVA link changed during reconnection")
            new_version = document.state_version + 1
            cls._optimistic_update(document, {"state_version": new_version, "last_error": None})
            cls._create_event(
                document.id,
                new_version,
                "EvaBindingResolved",
                "USER",
                actor_id,
                {"eva_binding": resolved},
                get_uuid(),
            )
            cls._store_eva_binding(document.id, resolved, replace=True)
        return cls._project(cls._get_document(document_tenant_id, document_id), access)

    @classmethod
    def create_eva_change_from_revision(
        cls,
        tenant_id: str,
        actor_id: str,
        document_id: str,
        raw: object,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
    ) -> dict[str, Any]:
        """Open the existing EVA approval workflow with the agreed local revision."""

        if not isinstance(raw, dict) or isinstance(raw.get("expected_state_version"), bool) or not isinstance(raw.get("expected_state_version"), int):
            raise ValidationError("INVALID_EVA_SYNC", "expected_state_version must be an integer")
        access = BusinessDocumentAccess(actor_id, access_role, is_admin)
        document = cls._get_editable_document(document_id, access)
        expected = raw["expected_state_version"]
        if document.state_version != expected:
            raise ConflictError("STATE_VERSION_CONFLICT", "The document changed since it was loaded", {"expected": expected, "actual": document.state_version})
        cls._require_idle(document)
        cls._require_lifecycle(document, LifecycleState.AGREED)
        if not document.current_revision_id:
            raise ConflictError("AGREED_REVISION_REQUIRED", "An agreed local revision is required")
        binding = cls._eva_binding(document)
        if not binding or "CREATE_EVA_CHANGE" not in binding.get("capabilities", []):
            raise ConflictError("EVA_SYNC_UNAVAILABLE", "The linked EVA page is not connected to an accessible connector")
        revision = BusinessDocumentRevision.get_by_id(document.current_revision_id)
        basis = cls._revision_change_basis(revision)
        basis_summary = "; ".join(item["summary"] for item in basis[:3] if item.get("summary"))
        change_summary = f"Синхронизация ревизии {revision.revision_number} документа «{document.title}»"
        if basis_summary:
            change_summary = f"{change_summary}: {basis_summary}"

        from api.apps.business_documents.eva_changes import EvaDocumentChangeService

        return EvaDocumentChangeService.create_change(
            tenant_id,
            actor_id,
            {
                "connector_id": binding["connector_id"],
                "document_id": binding["document_id"],
                "document_name": document.title,
                "change_summary": change_summary[:50_000],
                "draft_markdown": revision.body_markdown,
            },
        )

    @classmethod
    def list_jobs(cls, tenant_id: str, actor_id: str, document_id: str, is_admin: bool = False) -> list[dict[str, Any]]:
        cls._get_accessible_document(document_id)
        rows = BusinessDocumentJob.select().where(BusinessDocumentJob.document_id == document_id).order_by(BusinessDocumentJob.create_time.desc())
        return [cls._job_dict(row) for row in rows]

    @classmethod
    def delete_document(
        cls,
        actor_id: str,
        document_id: str,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
        storage=None,
    ) -> dict[str, Any]:
        access = BusinessDocumentAccess(actor_id, access_role, is_admin)
        access.require_delete()
        document = BusinessDocument.get_or_none(BusinessDocument.id == document_id)
        if document is None:
            raise NotFoundError()

        active_job = BusinessDocumentJob.select().where((BusinessDocumentJob.document_id == document_id) & (BusinessDocumentJob.status.in_(("PENDING", "RUNNING", "RETRY"))))
        if document.operation_state not in {OperationState.IDLE.value, OperationState.FAILED.value} or active_job.exists():
            raise ConflictError("OPERATION_IN_PROGRESS", "A document with an active operation cannot be deleted")

        artifacts = list(BusinessDocumentExportArtifact.select().where(BusinessDocumentExportArtifact.document_id == document_id))
        database = BusinessDocument._meta.database
        with database.atomic():
            document = BusinessDocument.get_or_none(BusinessDocument.id == document_id)
            if document is None:
                raise NotFoundError()
            active_job = BusinessDocumentJob.select().where((BusinessDocumentJob.document_id == document_id) & (BusinessDocumentJob.status.in_(("PENDING", "RUNNING", "RETRY"))))
            if document.operation_state not in {OperationState.IDLE.value, OperationState.FAILED.value} or active_job.exists():
                raise ConflictError("OPERATION_IN_PROGRESS", "A document with an active operation cannot be deleted")
            for model in (
                BusinessDocumentEvaBinding,
                BusinessDocumentEvidenceSnapshot,
                BusinessDocumentExportArtifact,
                BusinessDocumentJob,
                BusinessDocumentCommand,
                BusinessDocumentAnswer,
                BusinessDocumentProposalDecision,
                BusinessDocumentQuestion,
                BusinessDocumentProposal,
                BusinessDocumentComment,
                BusinessDocumentRevision,
                BusinessDocumentEvent,
            ):
                model.delete().where(model.document_id == document_id).execute()
            deleted = BusinessDocument.delete().where(BusinessDocument.id == document_id).execute()
            if deleted != 1:
                raise ConflictError("DOCUMENT_DELETE_CONFLICT", "The document changed while it was being deleted")

        cleanup_failures = 0
        if artifacts:
            if storage is None:
                from common import settings

                storage = settings.STORAGE_IMPL
            for artifact in artifacts:
                try:
                    if storage is None:
                        raise RuntimeError("storage is not initialized")
                    storage.rm(artifact.storage_bucket, artifact.storage_key)
                except Exception:
                    cleanup_failures += 1
                    logging.exception("Unable to remove business document export artifact %s", artifact.id)
        return {
            "document_id": document_id,
            "deleted": True,
            "deleted_artifacts": len(artifacts) - cleanup_failures,
            "storage_cleanup_failures": cleanup_failures,
        }

    @classmethod
    def execute_command(
        cls,
        tenant_id: str,
        actor_id: str,
        document_id: str,
        raw: object,
        is_admin: bool = False,
        access_role: BusinessDocumentRole | str = BusinessDocumentRole.AUTHOR_CREATOR,
    ) -> dict[str, Any]:
        envelope = CommandEnvelope.parse(raw)
        request_hash = _stable_hash(
            {
                "schema_version": envelope.schema_version,
                "command_id": envelope.command_id,
                "expected_state_version": envelope.expected_state_version,
                "type": envelope.type.value,
                "payload": envelope.payload,
            }
        )
        database = BusinessDocument._meta.database
        captured_error: BusinessDocumentError | None = None
        response: dict[str, Any]
        document_tenant_id = tenant_id
        try:
            with database.atomic():
                access = BusinessDocumentAccess(actor_id, access_role, is_admin)
                document = cls._get_editable_document(document_id, access)
                document_tenant_id = document.tenant_id
                existing = BusinessDocumentCommand.get_or_none(
                    (BusinessDocumentCommand.tenant_id == document_tenant_id)
                    & (BusinessDocumentCommand.document_id == document_id)
                    & (BusinessDocumentCommand.idempotency_key == envelope.idempotency_key)
                )
                if existing is not None:
                    return cls._replay_command(existing, request_hash)

                try:
                    # The savepoint makes command-side append-only writes atomic with
                    # the optimistic document CAS. A rejected command is recorded by
                    # the outer transaction only after every child write is rolled back.
                    with database.atomic():
                        if document.state_version != envelope.expected_state_version:
                            raise ConflictError(
                                "STATE_VERSION_CONFLICT",
                                "The document changed since it was loaded",
                                {"expected": envelope.expected_state_version, "actual": document.state_version},
                            )
                        response = cls._dispatch(document, actor_id, envelope)
                except BusinessDocumentError as exc:
                    captured_error = exc
                    response = {
                        "accepted": False,
                        "document_id": document_id,
                        "state_version": document.state_version,
                        "error": {"code": exc.code, "message": exc.message, "status": exc.status, "details": exc.details},
                    }

                BusinessDocumentCommand.create(
                    id=get_uuid(),
                    document_id=document_id,
                    tenant_id=document_tenant_id,
                    idempotency_key=envelope.idempotency_key,
                    request_hash=request_hash,
                    response=response,
                    **_timestamps(),
                )
        except IntegrityError:
            # A concurrent request may win the idempotency-key insert after the
            # initial lookup. Only that exact ledger collision is replayable;
            # unrelated integrity failures keep their original traceback.
            existing = BusinessDocumentCommand.get_or_none(
                (BusinessDocumentCommand.tenant_id == document_tenant_id) & (BusinessDocumentCommand.document_id == document_id) & (BusinessDocumentCommand.idempotency_key == envelope.idempotency_key)
            )
            if existing is None:
                raise
            return cls._replay_command(existing, request_hash)
        if captured_error is not None:
            raise captured_error
        return response

    @staticmethod
    def _replay_command(existing: BusinessDocumentCommand, request_hash: str) -> dict[str, Any]:
        if existing.request_hash != request_hash:
            raise ConflictError("IDEMPOTENCY_CONFLICT", "idempotency_key was already used for a different command")
        response = dict(existing.response)
        response["idempotent_replay"] = True
        if not response.get("accepted", False):
            error = response["error"]
            raise BusinessDocumentError(error["code"], error["message"], error["status"], error.get("details"))
        return response

    @classmethod
    def complete_job(
        cls,
        tenant_id: str,
        actor_id: str,
        job_id: str,
        output: object,
        lease_token: str | None = None,
        execution_audit: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Commit a worker result after rechecking the immutable job snapshot."""

        if not isinstance(output, dict):
            raise ValidationError("INVALID_JOB_OUTPUT", "Worker output must be a JSON object")
        database = BusinessDocument._meta.database
        with database.atomic():
            job = BusinessDocumentJob.get_or_none((BusinessDocumentJob.id == job_id) & (BusinessDocumentJob.tenant_id == tenant_id))
            if job is None:
                raise BusinessDocumentError("JOB_NOT_FOUND", "Business document job not found", 404)
            document = cls._get_document(tenant_id, job.document_id)
            if job.status == "COMPLETED":
                return cls._project(document)
            cls._require_current_job_lease(job, actor_id, lease_token)
            execution = cls._validate_execution_audit(execution_audit, job)
            if document.state_version != job.source_state_version:
                raise ConflictError(
                    "STALE_AI_RESULT",
                    "Worker result targets an outdated document state",
                    {"source": job.source_state_version, "actual": document.state_version},
                )

            if job.job_type == "ASSESS_INTAKE":
                cls._complete_assessment(document, job, actor_id, output, execution)
            elif job.job_type == "ASSESS_REVIEW":
                cls._complete_review_assessment(document, job, actor_id, output, execution)
            elif job.job_type == "GENERATE_DRAFT":
                cls._complete_draft(document, job, actor_id, output, execution)
            elif job.job_type == "PLAN_CHANGES":
                cls._complete_changes(document, job, actor_id, output, execution)
            elif job.job_type == "GENERATE_EXPORT":
                cls._complete_export(document, job, actor_id, output, execution)
            else:
                raise ValidationError("UNKNOWN_JOB_TYPE", "Unsupported business document job type", {"job_type": job.job_type})
            prompt_audit = cls._job_prompt_audit(job)
            BusinessDocumentJob.update(
                status="COMPLETED",
                progress=1.0,
                progress_stage="COMPLETED",
                progress_message="Обработка завершена",
                lease_owner=None,
                lease_token=None,
                lease_expires_at=None,
                error=None,
                result={"output": output, **prompt_audit, **({"execution": execution} if execution else {})},
                update_time=current_timestamp(),
                update_date=datetime.now(),
            ).where(BusinessDocumentJob.id == job.id).execute()
        return cls._project(cls._get_document(tenant_id, document.id))

    @classmethod
    def fail_job(
        cls,
        tenant_id: str,
        actor_id: str,
        job_id: str,
        error: dict[str, Any],
        lease_token: str | None = None,
    ) -> dict[str, Any]:
        database = BusinessDocument._meta.database
        with database.atomic():
            job = BusinessDocumentJob.get_or_none((BusinessDocumentJob.id == job_id) & (BusinessDocumentJob.tenant_id == tenant_id))
            if job is None:
                raise BusinessDocumentError("JOB_NOT_FOUND", "Business document job not found", 404)
            document = cls._get_document(tenant_id, job.document_id)
            if job.status == "DEAD":
                return cls._project(document)
            cls._require_current_job_lease(job, actor_id, lease_token)
            new_version = document.state_version + 1
            cls._optimistic_update(
                document,
                {"operation_state": OperationState.FAILED.value, "last_error": error, "state_version": new_version},
            )
            cls._create_event(document.id, new_version, "BusinessDocumentJobFailed", "SYSTEM", actor_id, {"job_id": job.id, "error": error}, job.correlation_id)
            BusinessDocumentJob.update(
                status="DEAD",
                progress_stage="FAILED",
                progress_message="Не удалось завершить обработку",
                lease_owner=None,
                lease_token=None,
                lease_expires_at=None,
                error=error,
                update_time=current_timestamp(),
                update_date=datetime.now(),
            ).where(BusinessDocumentJob.id == job.id).execute()
        return cls._project(cls._get_document(tenant_id, document.id))

    @classmethod
    def _dispatch(cls, document: BusinessDocument, actor_id: str, envelope: CommandEnvelope) -> dict[str, Any]:
        if document.lifecycle_state == LifecycleState.ARCHIVED.value:
            raise ConflictError("DOCUMENT_ARCHIVED", "Archived documents cannot be changed")
        if envelope.type == CommandType.REQUEST_INTAKE_ASSESSMENT:
            if cls._open_questions(document, "INTAKE"):
                raise ConflictError("OPEN_INTAKE_QUESTIONS", "Answer open intake questions before reassessment")
            return cls._request_job(document, actor_id, envelope, "ASSESS_INTAKE", OperationState.ANALYZING)
        if envelope.type == CommandType.REQUEST_REVIEW_ASSESSMENT:
            cls._require_lifecycle(document, LifecycleState.REVIEW)
            if cls._open_questions(document, "REVIEW") and not cls._has_unassessed_review_feedback(document.id):
                raise ConflictError("OPEN_REVIEW_QUESTIONS", "Add new review feedback before reassessment while questions remain open")
            return cls._request_job(document, actor_id, envelope, "ASSESS_REVIEW", OperationState.ANALYZING_REVIEW)
        if envelope.type == CommandType.REQUEST_DRAFT:
            cls._require_lifecycle(document, LifecycleState.INTAKE)
            if document.current_revision_id:
                raise ConflictError("DRAFT_ALREADY_EXISTS", "The document already has a draft")
            open_questions = cls._open_questions(document, "INTAKE")
            if open_questions:
                raise ConflictError("OPEN_INTAKE_QUESTIONS", "Draft cannot be created while intake questions are open", {"question_ids": open_questions})
            if not cls._assessment_is_current(document.id, "IntakeAssessed", {"QuestionAnswered"}, review_cycle=0):
                raise ConflictError("INTAKE_ASSESSMENT_REQUIRED", "Draft requires a current COMPLETE intake assessment")
            return cls._request_job(document, actor_id, envelope, "GENERATE_DRAFT", OperationState.GENERATING_DRAFT)
        if envelope.type == CommandType.ANSWER_QUESTION:
            return cls._answer_question(document, actor_id, envelope)
        if envelope.type == CommandType.DECIDE_PROPOSAL:
            return cls._decide_proposal(document, actor_id, envelope)
        if envelope.type == CommandType.ADD_COMMENT:
            return cls._add_comment(document, actor_id, envelope)
        if envelope.type == CommandType.APPLY_CHANGES:
            cls._require_lifecycle(document, LifecycleState.REVIEW)
            open_questions = cls._open_questions(document, "REVIEW")
            if open_questions:
                raise ConflictError("OPEN_REVIEW_QUESTIONS", "Changes cannot be applied while review questions are open", {"question_ids": open_questions})
            if not cls._assessment_is_current(
                document.id,
                "ReviewAssessed",
                {"QuestionAnswered", "ProposalDecided", "AuthorCommentAdded"},
                review_cycle=document.active_review_cycle,
            ):
                raise ConflictError("REVIEW_ASSESSMENT_REQUIRED", "Changes require a current COMPLETE review assessment")
            base_revision_id = envelope.payload.get("base_revision_id")
            if base_revision_id != document.current_revision_id:
                raise ConflictError(
                    "BASE_REVISION_CONFLICT",
                    "The change request does not target the current revision",
                    {"expected": document.current_revision_id, "actual": base_revision_id},
                )
            return cls._request_job(document, actor_id, envelope, "PLAN_CHANGES", OperationState.APPLYING_CHANGES)
        if envelope.type == CommandType.START_REVIEW:
            cls._require_idle(document)
            cls._require_lifecycle(document, LifecycleState.AGREED)
            return cls._transition(
                document,
                actor_id,
                envelope,
                "ReviewCycleStarted",
                lifecycle_state=LifecycleState.REVIEW.value,
                active_review_cycle=document.active_review_cycle + 1,
            )
        if envelope.type == CommandType.REQUEST_EXPORT:
            if document.lifecycle_state != LifecycleState.AGREED.value:
                raise ConflictError("AGREED_REVISION_REQUIRED", "Export requires an agreed document revision")
            revision_id = envelope.payload.get("revision_id")
            if revision_id != document.current_revision_id:
                raise ConflictError("REVISION_NOT_AGREED", "Only the current agreed revision can be exported")
            export_format = envelope.payload.get("format")
            if export_format not in {"MARKDOWN", "DOCX", "EVA_WIKI"}:
                raise ValidationError("INVALID_EXPORT_FORMAT", "Unsupported export format")
            return cls._request_job(document, actor_id, envelope, "GENERATE_EXPORT", OperationState.EXPORTING)
        if envelope.type == CommandType.ARCHIVE:
            cls._require_idle(document)
            return cls._transition(document, actor_id, envelope, "DocumentArchived", lifecycle_state=LifecycleState.ARCHIVED.value)
        raise ValidationError("UNKNOWN_COMMAND", "Unsupported command type")

    @classmethod
    def _request_job(
        cls,
        document: BusinessDocument,
        actor_id: str,
        envelope: CommandEnvelope,
        job_type: str,
        operation_state: OperationState,
    ) -> dict[str, Any]:
        cls._require_idle(document)
        if job_type == "ASSESS_INTAKE":
            cls._require_lifecycle(document, LifecycleState.INTAKE)
        new_version = document.state_version + 1
        job_id = get_uuid()
        snapshot = cls._job_snapshot(document, envelope.payload)
        snapshot["task_type"] = job_type
        snapshot["requested_by_actor_id"] = actor_id
        prompt = prompt_descriptor(job_type)
        if prompt is not None:
            snapshot["prompt"] = prompt
        snapshot["expected_contracts"] = {
            "ASSESS_INTAKE": ["question_batch.v1"],
            "ASSESS_REVIEW": ["review_plan.v1"],
            "GENERATE_DRAFT": ["document_draft.v1", "question_batch.v1", "review_plan.v1"],
            "PLAN_CHANGES": ["change_plan.v1"],
            "GENERATE_EXPORT": [],
        }[job_type]
        dedupe_key = _stable_hash({"document_id": document.id, "job_type": job_type, "source_state_version": new_version})
        now = current_timestamp()
        # Claim the document version before inserting the unique job. Competing
        # commands then fail with a version conflict, and the surrounding
        # savepoint rolls back this update if job creation fails.
        cls._optimistic_update(document, {"operation_state": operation_state.value, "last_error": None, "state_version": new_version})
        BusinessDocumentJob.create(
            id=job_id,
            document_id=document.id,
            tenant_id=document.tenant_id,
            job_type=job_type,
            status="PENDING",
            progress=0.02,
            progress_stage="QUEUED",
            progress_message="Ожидает запуска",
            dedupe_key=dedupe_key,
            source_state_version=new_version,
            base_revision_id=document.current_revision_id,
            payload=snapshot,
            result=None,
            attempt=0,
            max_attempts=3,
            available_at=now,
            lease_owner=None,
            lease_token=None,
            lease_expires_at=None,
            error=None,
            correlation_id=envelope.command_id,
            **_timestamps(),
        )
        event_id = cls._create_event(
            document.id,
            new_version,
            "BusinessDocumentJobRequested",
            "USER",
            actor_id,
            {
                "job_id": job_id,
                "job_type": job_type,
                "source_state_version": new_version,
                **(
                    {
                        "prompt_name": prompt["name"],
                        "prompt_version": prompt["version"],
                        "prompt_hash": prompt["content_hash"],
                    }
                    if prompt is not None
                    else {}
                ),
            },
            envelope.command_id,
        )
        return {
            "accepted": True,
            "document_id": document.id,
            "state_version": new_version,
            "lifecycle_state": document.lifecycle_state,
            "operation_state": operation_state.value,
            "job_id": job_id,
            "event_id": event_id,
            "allowed_commands": [],
        }

    @classmethod
    def _answer_question(cls, document: BusinessDocument, actor_id: str, envelope: CommandEnvelope) -> dict[str, Any]:
        cls._require_idle(document)
        question_id = envelope.payload.get("question_id")
        question = BusinessDocumentQuestion.get_or_none((BusinessDocumentQuestion.id == question_id) & (BusinessDocumentQuestion.document_id == document.id))
        if question is None:
            raise ValidationError("QUESTION_NOT_FOUND", "Question does not belong to this document")
        expected_stage = "INTAKE" if document.lifecycle_state == LifecycleState.INTAKE.value else "REVIEW"
        if question.stage != expected_stage or question.review_cycle != document.active_review_cycle:
            raise ConflictError("QUESTION_NOT_ACTIVE", "Question does not belong to the active workflow stage")
        if BusinessDocumentAnswer.select().where((BusinessDocumentAnswer.document_id == document.id) & (BusinessDocumentAnswer.question_id == question.id)).exists():
            raise ConflictError("QUESTION_ALREADY_CLOSED", "Published question answers are immutable")
        selected_option_id = envelope.payload.get("selected_option_id")
        custom_answer = envelope.payload.get("custom_answer")
        if bool(selected_option_id) == bool(isinstance(custom_answer, str) and custom_answer.strip()):
            raise ValidationError("INVALID_ANSWER", "Provide exactly one selected option or a custom answer")
        if selected_option_id:
            option_ids = {option.get("option_id") for option in question.options if isinstance(option, dict)}
            if selected_option_id not in option_ids:
                raise ValidationError("INVALID_OPTION", "selected_option_id is not one of the question options")
        elif not question.allow_custom_answer:
            raise ValidationError("CUSTOM_ANSWER_NOT_ALLOWED", "This question does not allow a custom answer")
        new_version = document.state_version + 1
        answer_id = get_uuid()
        BusinessDocumentAnswer.create(
            id=answer_id,
            document_id=document.id,
            question_id=question.id,
            actor_id=actor_id,
            selected_option_id=selected_option_id,
            custom_answer=custom_answer.strip() if isinstance(custom_answer, str) else None,
            **_timestamps(),
        )
        cls._optimistic_update(document, {"state_version": new_version})
        event_id = cls._create_event(
            document.id,
            new_version,
            "QuestionAnswered",
            "USER",
            actor_id,
            {"question_id": question.id, "answer_id": answer_id, "selected_option_id": selected_option_id},
            envelope.command_id,
        )
        return cls._command_response(document, new_version, event_id)

    @classmethod
    def _decide_proposal(cls, document: BusinessDocument, actor_id: str, envelope: CommandEnvelope) -> dict[str, Any]:
        cls._require_idle(document)
        cls._require_lifecycle(document, LifecycleState.REVIEW)
        proposal_id = envelope.payload.get("proposal_id")
        proposal = BusinessDocumentProposal.get_or_none(
            (BusinessDocumentProposal.id == proposal_id) & (BusinessDocumentProposal.document_id == document.id) & (BusinessDocumentProposal.review_cycle == document.active_review_cycle)
        )
        if proposal is None:
            raise ValidationError("PROPOSAL_NOT_FOUND", "Proposal does not belong to the active review cycle")
        if BusinessDocumentProposalDecision.select().where((BusinessDocumentProposalDecision.document_id == document.id) & (BusinessDocumentProposalDecision.proposal_id == proposal.id)).exists():
            raise ConflictError("PROPOSAL_ALREADY_DECIDED", "Published proposal decisions are immutable")
        decision = envelope.payload.get("decision")
        if decision not in {"ACCEPTED", "REJECTED"}:
            raise ValidationError("INVALID_PROPOSAL_DECISION", "decision must be ACCEPTED or REJECTED")
        new_version = document.state_version + 1
        decision_id = get_uuid()
        BusinessDocumentProposalDecision.create(
            id=decision_id,
            document_id=document.id,
            proposal_id=proposal.id,
            actor_id=actor_id,
            decision=decision,
            **_timestamps(),
        )
        cls._optimistic_update(document, {"state_version": new_version})
        event_id = cls._create_event(
            document.id,
            new_version,
            "ProposalDecided",
            "USER",
            actor_id,
            {"proposal_id": proposal.id, "decision_id": decision_id, "decision": decision},
            envelope.command_id,
        )
        return cls._command_response(document, new_version, event_id)

    @classmethod
    def _add_comment(cls, document: BusinessDocument, actor_id: str, envelope: CommandEnvelope) -> dict[str, Any]:
        cls._require_idle(document)
        cls._require_lifecycle(document, LifecycleState.REVIEW)
        text = envelope.payload.get("text")
        if not isinstance(text, str) or not text.strip():
            raise ValidationError("INVALID_COMMENT", "Comment text must be non-empty")
        revision_id = envelope.payload.get("revision_id")
        if revision_id != document.current_revision_id:
            raise ConflictError("COMMENT_REVISION_CONFLICT", "Comment must target the current revision")
        revision = BusinessDocumentRevision.get_or_none((BusinessDocumentRevision.id == revision_id) & (BusinessDocumentRevision.document_id == document.id))
        if revision is None:
            raise ConflictError("COMMENT_REVISION_CONFLICT", "Comment must target the current revision")
        section_id = envelope.payload.get("section_id")
        revision_sections = {section.get("id"): section for section in revision.document_ast.get("sections", []) if isinstance(section, dict) and isinstance(section.get("id"), str)}
        if section_id is not None and section_id not in revision_sections:
            raise ValidationError("COMMENT_SECTION_NOT_FOUND", "Comment section does not exist in the target revision")
        anchor = envelope.payload.get("anchor")
        if anchor is not None and not isinstance(anchor, dict):
            raise ValidationError("INVALID_COMMENT_ANCHOR", "anchor must be a JSON object")
        if anchor:
            if anchor.get("revision_id") != revision_id:
                raise ValidationError("INVALID_COMMENT_ANCHOR", "Anchor revision_id must match the target revision")
            if anchor.get("section_id") != section_id or section_id is None:
                raise ValidationError("INVALID_COMMENT_ANCHOR", "Anchor section_id must match the comment section")
            selected_text = anchor.get("selected_text")
            if not isinstance(selected_text, str) or not selected_text:
                raise ValidationError("INVALID_COMMENT_ANCHOR", "selected_text must be non-empty")
            start_offset = anchor.get("start_offset")
            end_offset = anchor.get("end_offset")
            section_text = render_section_text(revision_sections[section_id])
            section_length = _utf16_length(section_text)
            if (
                isinstance(start_offset, bool)
                or isinstance(end_offset, bool)
                or not isinstance(start_offset, int)
                or not isinstance(end_offset, int)
                or start_offset < 0
                or end_offset <= start_offset
                or end_offset > section_length
                or _utf16_slice(section_text, start_offset, end_offset) != selected_text
            ):
                raise ValidationError("INVALID_COMMENT_ANCHOR", "Anchor offsets must exactly select text inside the target section")
            expected_prefix = _utf16_context_slice(
                section_text,
                max(0, start_offset - 64),
                start_offset,
                trim_start=True,
            )
            expected_suffix = _utf16_context_slice(
                section_text,
                end_offset,
                min(section_length, end_offset + 64),
                trim_start=False,
            )
            if anchor.get("prefix") != expected_prefix or anchor.get("suffix") != expected_suffix:
                raise ValidationError("INVALID_COMMENT_ANCHOR", "Anchor context does not match the target section")
        new_version = document.state_version + 1
        comment_id = get_uuid()
        BusinessDocumentComment.create(
            id=comment_id,
            document_id=document.id,
            review_cycle=document.active_review_cycle,
            revision_id=revision_id,
            actor_id=actor_id,
            section_id=section_id,
            text=text.strip(),
            anchor=anchor,
            **_timestamps(),
        )
        cls._optimistic_update(document, {"state_version": new_version})
        event_id = cls._create_event(
            document.id,
            new_version,
            "AuthorCommentAdded",
            "USER",
            actor_id,
            {"comment_id": comment_id, "revision_id": revision_id},
            envelope.command_id,
        )
        return cls._command_response(document, new_version, event_id)

    @classmethod
    def _transition(cls, document, actor_id, envelope, event_type, **changes):
        new_version = document.state_version + 1
        changes["state_version"] = new_version
        cls._optimistic_update(document, changes)
        event_id = cls._create_event(document.id, new_version, event_type, "USER", actor_id, envelope.payload, envelope.command_id)
        lifecycle_state = changes.get("lifecycle_state", document.lifecycle_state)
        return {
            "accepted": True,
            "document_id": document.id,
            "state_version": new_version,
            "lifecycle_state": lifecycle_state,
            "operation_state": document.operation_state,
            "event_id": event_id,
            "allowed_commands": cls._allowed_commands_values(lifecycle_state, document.operation_state, document.id, changes.get("active_review_cycle", document.active_review_cycle)),
        }

    @classmethod
    def _complete_assessment(cls, document, job, actor_id, output, execution):
        if document.operation_state != OperationState.ANALYZING.value or document.lifecycle_state != LifecycleState.INTAKE.value:
            raise ConflictError("JOB_STATE_CONFLICT", "Intake assessment is not active")
        validate_contract("question_batch", output)
        questions = output["questions"]
        new_version = document.state_version + 1
        event_id = get_uuid()
        inserted_question_count = 0
        for question in questions:
            cls._validate_evidence_refs(execution, question.get("evidence_refs", []))
            _, inserted = cls._insert_question(document, question, "INTAKE", 0, event_id)
            inserted_question_count += int(inserted)
        cls._optimistic_update(document, {"operation_state": OperationState.IDLE.value, "state_version": new_version})
        cls._create_event(
            document.id,
            new_version,
            "IntakeAssessed",
            "AI",
            actor_id,
            {
                "job_id": job.id,
                "review_cycle": 0,
                "outcome": "NEEDS_INPUT" if cls._open_questions(document, "INTAKE") else "COMPLETE",
                "question_count": inserted_question_count,
                **cls._job_prompt_audit(job),
                **({"execution": execution} if execution else {}),
            },
            job.correlation_id,
            event_id=event_id,
        )

    @classmethod
    def _complete_review_assessment(cls, document, job, actor_id, output, execution):
        if document.operation_state != OperationState.ANALYZING_REVIEW.value or document.lifecycle_state != LifecycleState.REVIEW.value:
            raise ConflictError("JOB_STATE_CONFLICT", "Review assessment is not active")
        validate_contract("review_plan", output)
        questions = output["questions"]
        proposals = output["proposals"]
        dispositions = cls._validate_review_comment_dispositions(document, output)
        new_version = document.state_version + 1
        event_id = get_uuid()
        question_ids_by_tag: dict[str, str] = {}
        question_comment_sources: dict[str, list[str]] = {}
        for disposition in dispositions:
            if disposition["disposition"] == "NEEDS_QUESTION":
                question_comment_sources.setdefault(_canonical_semantic_tag(disposition["question_semantic_tag"]), []).append(disposition["comment_event_id"])
        inserted_question_count = 0
        for question in questions:
            cls._validate_evidence_refs(execution, question.get("evidence_refs", []))
            question_id, inserted = cls._insert_question(
                document,
                {**question, "stage": "REVIEW"},
                "REVIEW",
                document.active_review_cycle,
                event_id,
                question_comment_sources.get(_canonical_semantic_tag(question["semantic_tag"]), []),
            )
            question_ids_by_tag.setdefault(_canonical_semantic_tag(question["semantic_tag"]), question_id)
            inserted_question_count += int(inserted)
        normalized_dispositions = []
        for disposition in dispositions:
            normalized = dict(disposition)
            if disposition["disposition"] == "NEEDS_QUESTION":
                question_id = question_ids_by_tag[_canonical_semantic_tag(disposition["question_semantic_tag"])]
                if BusinessDocumentAnswer.select().where((BusinessDocumentAnswer.document_id == document.id) & (BusinessDocumentAnswer.question_id == question_id)).exists():
                    raise ValidationError(
                        "COMMENT_DISPOSITION_QUESTION_CLOSED",
                        "NEEDS_QUESTION must reference an open question",
                        {"comment_event_id": disposition["comment_event_id"], "question_id": question_id},
                    )
                normalized["question_id"] = question_id
            normalized_dispositions.append(normalized)
        inserted_proposal_count = 0
        for proposal in proposals:
            cls._validate_job_sources(job, proposal["source_event_ids"])
            cls._validate_evidence_refs(execution, proposal.get("evidence_refs", []))
            _, inserted = cls._insert_proposal(document, proposal, document.active_review_cycle, event_id)
            inserted_proposal_count += int(inserted)
        cls._optimistic_update(document, {"operation_state": OperationState.IDLE.value, "state_version": new_version})
        outcome = "NEEDS_INPUT" if cls._open_questions(document, "REVIEW") else "COMPLETE"
        cls._create_event(
            document.id,
            new_version,
            "ReviewAssessed",
            "AI",
            actor_id,
            {
                "job_id": job.id,
                "review_cycle": document.active_review_cycle,
                "outcome": outcome,
                "question_count": inserted_question_count,
                "proposal_count": inserted_proposal_count,
                "comment_dispositions": normalized_dispositions,
                **cls._job_prompt_audit(job),
                **({"execution": execution} if execution else {}),
            },
            job.correlation_id,
            event_id=event_id,
        )

    @classmethod
    def _complete_draft(cls, document, job, actor_id, output, execution):
        if document.operation_state != OperationState.GENERATING_DRAFT.value or document.lifecycle_state != LifecycleState.INTAKE.value:
            raise ConflictError("JOB_STATE_CONFLICT", "Draft generation is not active")
        if document.current_revision_id is not None or cls._open_questions(document, "INTAKE"):
            raise ConflictError("DRAFT_PRECONDITION_FAILED", "Draft preconditions changed while the worker was running")
        draft = validate_document_ast(output.get("draft"))
        for section in draft["sections"]:
            cls._validate_evidence_refs(execution, section.get("evidence_refs", []))
        if draft["template_version"] != document.template_version:
            raise ValidationError("TEMPLATE_VERSION_CONFLICT", "Draft does not use the version pinned to this document")
        body = render_document_ast(draft)
        review_questions = output.get("review_questions", {"schema_version": "1", "outcome": "COMPLETE", "questions": []})
        validate_contract("question_batch", review_questions)
        questions = review_questions["questions"]
        proposals = output.get("proposals", [])
        if not isinstance(proposals, list):
            raise ValidationError("INVALID_DRAFT_PROTOCOL", "proposals must be an array")
        validate_contract(
            "review_plan",
            {"schema_version": "1", "questions": [], "proposals": proposals, "comment_dispositions": []},
        )
        new_version = document.state_version + 1
        event_id = get_uuid()
        intake_answer_event_ids = [
            source_event["event_id"]
            for source_event in job.payload.get("source_events", [])
            if isinstance(source_event, dict) and source_event.get("event_type") == "QuestionAnswered" and isinstance(source_event.get("event_id"), str)
        ]
        revision_id = cls._insert_revision(
            document.id,
            1,
            draft,
            body,
            [event_id, *intake_answer_event_ids],
            cls._job_request_author(job, document.owner_id),
        )
        inserted_question_count = 0
        for question in questions:
            cls._validate_evidence_refs(execution, question.get("evidence_refs", []))
            _, inserted = cls._insert_question(document, question, "REVIEW", 1, event_id)
            inserted_question_count += int(inserted)
        inserted_proposal_count = 0
        for proposal in proposals:
            cls._validate_job_sources(job, proposal["source_event_ids"])
            cls._validate_evidence_refs(execution, proposal.get("evidence_refs", []))
            _, inserted = cls._insert_proposal(document, proposal, 1, event_id)
            inserted_proposal_count += int(inserted)
        cls._optimistic_update(
            document,
            {
                "lifecycle_state": LifecycleState.REVIEW.value,
                "operation_state": OperationState.IDLE.value,
                "current_revision_id": revision_id,
                "active_review_cycle": 1,
                "state_version": new_version,
            },
        )
        cls._create_event(
            document.id,
            new_version,
            "DraftCreated",
            "AI",
            actor_id,
            {
                "job_id": job.id,
                "revision_id": revision_id,
                "requested_by_actor_id": job.payload.get("requested_by_actor_id"),
                "question_count": inserted_question_count,
                "proposal_count": inserted_proposal_count,
                **cls._job_prompt_audit(job),
                **({"execution": execution} if execution else {}),
            },
            job.correlation_id,
            event_id=event_id,
        )

    @classmethod
    def _complete_changes(cls, document, job, actor_id, output, execution):
        if document.operation_state != OperationState.APPLYING_CHANGES.value or document.lifecycle_state != LifecycleState.REVIEW.value:
            raise ConflictError("JOB_STATE_CONFLICT", "Change application is not active")
        if job.base_revision_id != document.current_revision_id:
            raise ConflictError("STALE_AI_RESULT", "Change plan targets an outdated revision")
        if cls._open_questions(document, "REVIEW"):
            raise ConflictError("OPEN_REVIEW_QUESTIONS", "Changes cannot be applied while review questions are open")
        change_plan = output.get("change_plan")
        validate_contract("change_plan", change_plan)
        assert isinstance(change_plan, dict)
        if change_plan["base_revision_id"] != document.current_revision_id or change_plan["source_state_version"] != job.source_state_version:
            raise ConflictError("STALE_AI_RESULT", "Change plan does not match the immutable job snapshot")
        operations = change_plan["operations"]
        acknowledged_no_change_event_ids = change_plan.get("acknowledged_no_change_event_ids", [])
        source_event_ids: list[str] = []
        evidence_refs: list[str] = []
        for operation in operations:
            if not isinstance(operation, dict) or not isinstance(operation.get("source_event_ids"), list) or not operation["source_event_ids"]:
                raise ValidationError("UNSUPPORTED_CHANGE", "Every change operation requires source_event_ids")
            cls._validate_job_sources(job, operation["source_event_ids"])
            cls._validate_evidence_refs(execution, operation.get("evidence_refs", []))
            cls._validate_change_sources(document, operation)
            source_event_ids.extend(operation["source_event_ids"])
            evidence_refs.extend(operation.get("evidence_refs", []))
        active_inputs = cls._active_change_input_event_ids(document)
        acknowledged = set(acknowledged_no_change_event_ids)
        invalid_acknowledgements = sorted(acknowledged - active_inputs)
        if invalid_acknowledgements:
            raise ValidationError(
                "INVALID_NO_CHANGE_ACKNOWLEDGEMENT",
                "Only authorizing events from the active review cycle can be acknowledged as no-change",
                {"event_ids": invalid_acknowledgements},
            )
        used_sources = set(source_event_ids)
        duplicate_disposition = sorted(acknowledged & used_sources)
        if duplicate_disposition:
            raise ValidationError(
                "CHANGE_INPUT_DISPOSITION_CONFLICT",
                "A review input cannot both authorize a change and be acknowledged as no-change",
                {"event_ids": duplicate_disposition},
            )
        required_accepted = cls._accepted_proposal_event_ids(document)
        acknowledged_accepted = sorted(acknowledged & required_accepted)
        if acknowledged_accepted:
            raise ValidationError(
                "ACCEPTED_PROPOSAL_ACKNOWLEDGED_NO_CHANGE",
                "An accepted proposal must authorize a concrete change operation",
                {"event_ids": acknowledged_accepted},
            )
        current_comment_dispositions = cls._current_comment_dispositions(document)
        acknowledged_events = cls._validate_known_sources(document.id, list(acknowledged)) if acknowledged else {}
        invalid_comment_acknowledgements = sorted(
            event_id for event_id, event in acknowledged_events.items() if event.event_type == "AuthorCommentAdded" and current_comment_dispositions.get(event_id, {}).get("disposition") != "NO_CHANGE"
        )
        if invalid_comment_acknowledgements:
            raise ValidationError(
                "COMMENT_NO_CHANGE_NOT_CONFIRMED",
                "A comment can be acknowledged as no-change only when the current review assessment says NO_CHANGE",
                {"event_ids": invalid_comment_acknowledgements},
            )
        missing_inputs = active_inputs - used_sources - acknowledged
        missing_accepted = sorted(missing_inputs & required_accepted)
        if missing_accepted:
            raise ValidationError(
                "ACCEPTED_PROPOSAL_OMITTED",
                "Every accepted proposal in the active review cycle must be represented in the change plan",
                {"event_ids": missing_accepted},
            )
        if missing_inputs:
            raise ValidationError(
                "CHANGE_INPUT_OMITTED",
                "Every active review answer and comment must authorize a change or be explicitly acknowledged as no-change",
                {"event_ids": sorted(missing_inputs)},
            )
        if not operations:
            new_version = document.state_version + 1
            cls._optimistic_update(
                document,
                {
                    "lifecycle_state": LifecycleState.AGREED.value,
                    "operation_state": OperationState.IDLE.value,
                    "state_version": new_version,
                },
            )
            cls._create_event(
                document.id,
                new_version,
                "ReviewAgreedWithoutChanges",
                "AI",
                actor_id,
                {
                    "job_id": job.id,
                    "revision_id": document.current_revision_id,
                    "acknowledged_no_change_event_ids": acknowledged_no_change_event_ids,
                    **cls._job_prompt_audit(job),
                    **({"execution": execution} if execution else {}),
                },
                job.correlation_id,
            )
            return
        base_revision = BusinessDocumentRevision.get_by_id(document.current_revision_id)
        draft = apply_change_plan(base_revision.document_ast, change_plan)
        if draft["template_version"] != document.template_version:
            raise ValidationError("TEMPLATE_VERSION_CONFLICT", "Updated document does not use the pinned template")
        body = render_document_ast(draft)
        next_number = (BusinessDocumentRevision.select(fn.MAX(BusinessDocumentRevision.revision_number)).where(BusinessDocumentRevision.document_id == document.id).scalar() or 0) + 1
        new_version = document.state_version + 1
        event_id = get_uuid()
        revision_id = cls._insert_revision(
            document.id,
            next_number,
            draft,
            body,
            list(dict.fromkeys([event_id, *source_event_ids])),
            cls._job_request_author(job, document.owner_id),
        )
        cls._optimistic_update(
            document,
            {
                "lifecycle_state": LifecycleState.AGREED.value,
                "operation_state": OperationState.IDLE.value,
                "current_revision_id": revision_id,
                "state_version": new_version,
            },
        )
        cls._create_event(
            document.id,
            new_version,
            "ChangesApplied",
            "AI",
            actor_id,
            {
                "job_id": job.id,
                "base_revision_id": job.base_revision_id,
                "revision_id": revision_id,
                "requested_by_actor_id": cls._job_request_author(job, document.owner_id),
                "source_event_ids": source_event_ids,
                "acknowledged_no_change_event_ids": acknowledged_no_change_event_ids,
                "evidence_refs": list(dict.fromkeys(evidence_refs)),
                **cls._job_prompt_audit(job),
                **({"execution": execution} if execution else {}),
            },
            job.correlation_id,
            event_id=event_id,
        )

    @classmethod
    def _complete_export(cls, document, job, actor_id, output, execution):
        if document.operation_state != OperationState.EXPORTING.value or document.lifecycle_state != LifecycleState.AGREED.value:
            raise ConflictError("JOB_STATE_CONFLICT", "Export is not active")
        artifact_id = output.get("artifact_id")
        if not isinstance(artifact_id, str) or not artifact_id:
            raise ValidationError("INVALID_EXPORT_RESULT", "artifact_id is required")
        new_version = document.state_version + 1
        cls._optimistic_update(document, {"operation_state": OperationState.IDLE.value, "state_version": new_version})
        cls._create_event(
            document.id,
            new_version,
            "ExportGenerated",
            "SYSTEM",
            actor_id,
            {"job_id": job.id, "artifact_id": artifact_id, "format": job.payload.get("command_payload", {}).get("format")},
            job.correlation_id,
        )

    @classmethod
    def _insert_question(cls, document, raw, stage, review_cycle, source_event_id, additional_source_event_ids=None):
        if not isinstance(raw, dict):
            raise ValidationError("INVALID_QUESTION", "Question must be an object")
        text = raw.get("text")
        options = raw.get("options")
        if raw.get("stage") != stage:
            raise ValidationError("QUESTION_STAGE_CONFLICT", "Question stage does not match the active workflow stage")
        section_ids = {section["id"] for section in published_template()["sections"]}
        if raw.get("target_section_id") not in section_ids:
            raise ValidationError("QUESTION_SECTION_NOT_FOUND", "Question target_section_id is not in the published template")
        if not isinstance(text, str) or not text.strip() or not isinstance(options, list) or not 2 <= len(options) <= 4:
            raise ValidationError("INVALID_QUESTION", "Question requires text and 2 to 4 options")
        normalized_options = []
        seen_ids = set()
        for option in options:
            if not isinstance(option, dict) or not isinstance(option.get("option_id"), str) or not option["option_id"] or not isinstance(option.get("label"), str) or not option["label"].strip():
                raise ValidationError("INVALID_QUESTION_OPTION", "Each option requires a non-empty option_id and label")
            if option["option_id"] in seen_ids:
                raise ValidationError("DUPLICATE_QUESTION_OPTION", "Question option ids must be unique")
            seen_ids.add(option["option_id"])
            normalized_options.append({"option_id": option["option_id"], "label": option["label"].strip()})
        semantic_tag = _canonical_semantic_tag(raw["semantic_tag"])
        existing = BusinessDocumentQuestion.get_or_none(
            (BusinessDocumentQuestion.document_id == document.id)
            & (BusinessDocumentQuestion.stage == stage)
            & (BusinessDocumentQuestion.review_cycle == review_cycle)
            & (BusinessDocumentQuestion.semantic_tag == semantic_tag)
        )
        if existing is not None:
            return existing.id, False
        question_id = raw.get("id") or get_uuid()
        database = BusinessDocumentQuestion._meta.database
        try:
            with database.atomic():
                BusinessDocumentQuestion.create(
                    id=question_id,
                    document_id=document.id,
                    review_cycle=review_cycle,
                    stage=stage,
                    target_section_id=raw.get("target_section_id"),
                    semantic_tag=semantic_tag,
                    text=text.strip(),
                    options=normalized_options,
                    allow_custom_answer=raw.get("allow_custom_answer", True) is True,
                    source_event_ids=list(dict.fromkeys([source_event_id, *(additional_source_event_ids or [])])),
                    evidence_refs=raw.get("evidence_refs", []),
                    **_timestamps(),
                )
        except IntegrityError:
            existing = BusinessDocumentQuestion.get_or_none(
                (BusinessDocumentQuestion.document_id == document.id)
                & (BusinessDocumentQuestion.stage == stage)
                & (BusinessDocumentQuestion.review_cycle == review_cycle)
                & (BusinessDocumentQuestion.semantic_tag == semantic_tag)
            )
            if existing is None:
                raise
            return existing.id, False
        return question_id, True

    @classmethod
    def _insert_proposal(cls, document, raw, review_cycle, source_event_id):
        if not isinstance(raw, dict) or not isinstance(raw.get("text"), str) or not raw["text"].strip():
            raise ValidationError("INVALID_PROPOSAL", "Proposal text must be non-empty")
        section_ids = {section["id"] for section in published_template()["sections"]}
        if raw.get("target_section_id") not in section_ids:
            raise ValidationError("PROPOSAL_SECTION_NOT_FOUND", "Proposal target_section_id is not in the published template")
        fingerprint = _stable_hash(
            {
                "target_section_id": raw.get("target_section_id"),
                "text": _canonical_semantic_text(raw["text"]),
            }
        )
        source_scope_hash = _stable_hash(
            {
                "source_event_ids": sorted(set(raw.get("source_event_ids", []))),
                "evidence_refs": sorted(set(raw.get("evidence_refs", []))),
            }
        )
        identity = (
            (BusinessDocumentProposal.document_id == document.id)
            & (BusinessDocumentProposal.review_cycle == review_cycle)
            & (BusinessDocumentProposal.fingerprint == fingerprint)
            & (BusinessDocumentProposal.source_scope_hash == source_scope_hash)
        )
        existing = BusinessDocumentProposal.get_or_none(identity)
        if existing is not None:
            return existing.id, False
        proposal_id = raw.get("id") or get_uuid()
        database = BusinessDocumentProposal._meta.database
        try:
            with database.atomic():
                BusinessDocumentProposal.create(
                    id=proposal_id,
                    document_id=document.id,
                    review_cycle=review_cycle,
                    target_section_id=raw.get("target_section_id"),
                    text=raw["text"].strip(),
                    rationale=raw.get("rationale"),
                    source_event_ids=list(dict.fromkeys([source_event_id, *raw.get("source_event_ids", [])])),
                    evidence_refs=raw.get("evidence_refs", []),
                    fingerprint=fingerprint,
                    source_scope_hash=source_scope_hash,
                    **_timestamps(),
                )
        except IntegrityError:
            existing = BusinessDocumentProposal.get_or_none(identity)
            if existing is None:
                raise
            return existing.id, False
        return proposal_id, True

    @classmethod
    def _insert_revision(cls, document_id, revision_number, document_ast, body, source_event_ids, author_id=None):
        revision_id = get_uuid()
        BusinessDocumentRevision.create(
            id=revision_id,
            document_id=document_id,
            author_id=author_id,
            revision_number=revision_number,
            document_ast=document_ast,
            body_markdown=body,
            content_hash=_sha256_text(body),
            source_event_ids=source_event_ids,
            **_timestamps(),
        )
        return revision_id

    @classmethod
    def _validate_change_sources(cls, document, operation):
        source_event_ids = operation["source_event_ids"]
        events = cls._validate_known_sources(document.id, source_event_ids)
        allowed_event_types = {"QuestionAnswered", "ProposalDecided", "AuthorCommentAdded", "EvaDocumentPulled"}
        for event in events.values():
            if event.event_type not in allowed_event_types:
                raise ValidationError("INVALID_CHANGE_SOURCE_TYPE", "Event type cannot authorize a document change", {"event_id": event.id})
            if event.event_type == "ProposalDecided" and event.payload.get("decision") != "ACCEPTED":
                raise ValidationError("REJECTED_PROPOSAL_SOURCE", "Rejected proposals cannot support a change", {"event_id": event.id})
            if event.event_type == "AuthorCommentAdded" and cls._current_comment_dispositions(document).get(event.id, {}).get("disposition") != "CONFIRMED_CHANGE":
                raise ValidationError(
                    "COMMENT_CHANGE_NOT_CONFIRMED",
                    "A comment can authorize a change only when the current review assessment says CONFIRMED_CHANGE",
                    {"event_id": event.id},
                )
            source_cycle, source_section = cls._event_scope(event)
            if source_cycle != document.active_review_cycle:
                raise ValidationError("CHANGE_SOURCE_REVIEW_CYCLE_CONFLICT", "Change source is outside the active review cycle", {"event_id": event.id})
            # An anchor identifies the text the author commented on, not the
            # complete scope of the requested change.  A confirmed comment may
            # explicitly ask for a coordinated update in another section.
            # Questions and proposals retain their strict target-section
            # boundary because their contracts carry an explicit target.
            if event.event_type != "AuthorCommentAdded" and source_section is not None and source_section != operation["section_id"]:
                raise ValidationError(
                    "CHANGE_SOURCE_SECTION_CONFLICT",
                    "Change source targets a different template section",
                    {"event_id": event.id, "source_section_id": source_section, "operation_section_id": operation["section_id"]},
                )

    @classmethod
    def _validate_review_comment_dispositions(cls, document, output):
        active_events = cls._active_comment_events(document)
        dispositions = output["comment_dispositions"]
        disposition_ids = [item["comment_event_id"] for item in dispositions]
        if len(disposition_ids) != len(set(disposition_ids)):
            raise ValidationError("DUPLICATE_COMMENT_DISPOSITION", "Each active comment must have exactly one disposition")
        missing = sorted(set(active_events) - set(disposition_ids))
        unknown = sorted(set(disposition_ids) - set(active_events))
        if missing or unknown:
            raise ValidationError(
                "COMMENT_DISPOSITION_INCOMPLETE",
                "Review assessment must classify every and only active-cycle comments",
                {"missing_event_ids": missing, "unknown_event_ids": unknown},
            )
        question_tags = {
            _canonical_semantic_tag(question["semantic_tag"])
            for question in output["questions"]
            if isinstance(question, dict) and isinstance(question.get("semantic_tag"), str)
        }
        for disposition in dispositions:
            if disposition["disposition"] != "NEEDS_QUESTION":
                continue
            semantic_tag = _canonical_semantic_tag(disposition.get("question_semantic_tag", ""))
            if semantic_tag not in question_tags:
                raise ValidationError(
                    "COMMENT_DISPOSITION_QUESTION_NOT_FOUND",
                    "NEEDS_QUESTION must reference a concrete question from the same review plan",
                    {"comment_event_id": disposition["comment_event_id"]},
                )
            existing_question = BusinessDocumentQuestion.get_or_none(
                (BusinessDocumentQuestion.document_id == document.id)
                & (BusinessDocumentQuestion.stage == "REVIEW")
                & (BusinessDocumentQuestion.review_cycle == document.active_review_cycle)
                & (BusinessDocumentQuestion.semantic_tag == semantic_tag)
            )
            existing_answer = (
                BusinessDocumentAnswer.get_or_none(
                    (BusinessDocumentAnswer.document_id == document.id) & (BusinessDocumentAnswer.question_id == existing_question.id)
                )
                if existing_question is not None
                else None
            )
            if existing_answer is not None:
                raise ValidationError(
                    "COMMENT_DISPOSITION_QUESTION_CLOSED",
                    "NEEDS_QUESTION must reference an open question",
                    {
                        "comment_event_id": disposition["comment_event_id"],
                        "question_id": existing_question.id,
                        "question_semantic_tag": semantic_tag,
                        "required_action": (
                            "Use CONFIRMED_CHANGE or NO_CHANGE if the existing answer resolves the comment; "
                            "otherwise emit a different question with a new semantic_tag"
                        ),
                    },
                )
        return dispositions

    @staticmethod
    def _active_comment_events(document):
        comment_ids = {
            row.id
            for row in BusinessDocumentComment.select(BusinessDocumentComment.id).where(
                (BusinessDocumentComment.document_id == document.id) & (BusinessDocumentComment.review_cycle == document.active_review_cycle)
            )
        }
        return {
            event.id: event
            for event in BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document.id) & (BusinessDocumentEvent.event_type == "AuthorCommentAdded"))
            if event.payload.get("comment_id") in comment_ids
        }

    @staticmethod
    def _current_comment_dispositions(document):
        assessment = (
            BusinessDocumentEvent.select()
            .where((BusinessDocumentEvent.document_id == document.id) & (BusinessDocumentEvent.event_type == "ReviewAssessed"))
            .order_by(BusinessDocumentEvent.sequence.desc())
            .first()
        )
        if assessment is None or assessment.payload.get("review_cycle") != document.active_review_cycle:
            return {}
        dispositions = assessment.payload.get("comment_dispositions", [])
        return {item["comment_event_id"]: item for item in dispositions if isinstance(item, dict) and isinstance(item.get("comment_event_id"), str)}

    @staticmethod
    def _event_scope(event):
        if event.event_type == "QuestionAnswered":
            row = BusinessDocumentQuestion.get_by_id(event.payload["question_id"])
            return row.review_cycle, row.target_section_id
        if event.event_type == "ProposalDecided":
            row = BusinessDocumentProposal.get_by_id(event.payload["proposal_id"])
            return row.review_cycle, row.target_section_id
        if event.event_type == "AuthorCommentAdded":
            row = BusinessDocumentComment.get_by_id(event.payload["comment_id"])
            return row.review_cycle, row.section_id
        if event.event_type == "EvaDocumentPulled":
            return event.payload.get("review_cycle"), None
        return None, None

    @staticmethod
    def _accepted_proposal_event_ids(document):
        proposal_ids = {
            row.id
            for row in BusinessDocumentProposal.select(BusinessDocumentProposal.id).where(
                (BusinessDocumentProposal.document_id == document.id) & (BusinessDocumentProposal.review_cycle == document.active_review_cycle)
            )
        }
        return {
            event.id
            for event in BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document.id) & (BusinessDocumentEvent.event_type == "ProposalDecided"))
            if event.payload.get("proposal_id") in proposal_ids and event.payload.get("decision") == "ACCEPTED"
        }

    @classmethod
    def _active_change_input_event_ids(cls, document):
        accepted = cls._accepted_proposal_event_ids(document)
        question_ids = {
            row.id
            for row in BusinessDocumentQuestion.select(BusinessDocumentQuestion.id).where(
                (BusinessDocumentQuestion.document_id == document.id) & (BusinessDocumentQuestion.review_cycle == document.active_review_cycle) & (BusinessDocumentQuestion.stage == "REVIEW")
            )
        }
        comment_ids = {
            row.id
            for row in BusinessDocumentComment.select(BusinessDocumentComment.id).where(
                (BusinessDocumentComment.document_id == document.id) & (BusinessDocumentComment.review_cycle == document.active_review_cycle)
            )
        }
        related = {
            event.id
            for event in BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document.id)
            if (event.event_type == "QuestionAnswered" and event.payload.get("question_id") in question_ids)
            or (event.event_type == "AuthorCommentAdded" and event.payload.get("comment_id") in comment_ids)
        }
        latest_eva_pull = (
            BusinessDocumentEvent.select()
            .where((BusinessDocumentEvent.document_id == document.id) & (BusinessDocumentEvent.event_type == "EvaDocumentPulled"))
            .order_by(BusinessDocumentEvent.sequence.desc())
            .first()
        )
        eva_inputs = {latest_eva_pull.id} if latest_eva_pull is not None and latest_eva_pull.payload.get("review_cycle") == document.active_review_cycle else set()
        return accepted | related | eva_inputs

    @staticmethod
    def _validate_known_sources(document_id, source_event_ids):
        events = {event.id: event for event in BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == document_id) & (BusinessDocumentEvent.id.in_(source_event_ids)))}
        missing = [event_id for event_id in source_event_ids if event_id not in events]
        if missing:
            raise ValidationError("UNKNOWN_CHANGE_SOURCE", "Result references unknown document events", {"event_ids": missing})
        return events

    @staticmethod
    def _validate_job_sources(job, source_event_ids):
        snapshot_events = job.payload.get("source_events", []) if isinstance(job.payload, dict) else []
        allowed = {event.get("event_id") for event in snapshot_events if isinstance(event, dict) and isinstance(event.get("event_id"), str)}
        unknown = [event_id for event_id in source_event_ids if event_id not in allowed]
        if unknown:
            raise ValidationError(
                "SOURCE_NOT_IN_JOB_SNAPSHOT",
                "AI output references an event that was not present in its immutable job snapshot",
                {"event_ids": unknown},
            )

    @staticmethod
    def _validate_evidence_refs(execution, evidence_refs):
        if not evidence_refs:
            return
        if not isinstance(evidence_refs, list) or not all(isinstance(item, str) for item in evidence_refs):
            raise ValidationError("INVALID_EVIDENCE_REFS", "evidence_refs must be an array of source references")
        retrieval = execution.get("retrieval") if isinstance(execution, dict) else None
        allowed = set(retrieval.get("source_refs", [])) if isinstance(retrieval, dict) else set()
        unknown = [source_ref for source_ref in evidence_refs if source_ref not in allowed]
        if unknown:
            raise ValidationError(
                "EVIDENCE_REF_NOT_IN_SNAPSHOT",
                "AI output cites evidence outside the pinned retrieval snapshot",
                {"source_refs": unknown},
            )

    @classmethod
    def _job_snapshot(cls, document, command_payload):
        revision = None
        if document.current_revision_id:
            row = BusinessDocumentRevision.get_by_id(document.current_revision_id)
            revision = cls._revision_dict(row)
            revision["section_hashes"] = {section["id"]: section_hash(section) for section in row.document_ast.get("sections", []) if isinstance(section, dict) and isinstance(section.get("id"), str)}
        source_events = list(BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == document.id).order_by(BusinessDocumentEvent.sequence.asc()))
        created = next((event for event in source_events if event.event_type == "DocumentCreated"), None)
        return {
            "schema_version": "1",
            "document_id": document.id,
            "owner_id": document.owner_id,
            "document_type": document.document_type,
            "title": document.title,
            "idea": document.idea,
            "dataset_ids": document.dataset_ids,
            "idea_source_event_id": created.id if created else None,
            "source_events": [
                {
                    "event_id": event.id,
                    "sequence": event.sequence,
                    "event_type": event.event_type,
                    "actor_type": event.actor_type,
                    "payload": event.payload,
                }
                for event in source_events
            ],
            "active_change_input_event_ids": sorted(cls._active_change_input_event_ids(document)),
            "lifecycle_state": document.lifecycle_state,
            "state_version": document.state_version + 1,
            "template_version": document.template_version,
            "policy_version": document.policy_version,
            "active_review_cycle": document.active_review_cycle,
            "current_revision": revision,
            "protocol": cls._protocol(document),
            "command_payload": command_payload,
        }

    @classmethod
    def _project(cls, document, access: BusinessDocumentAccess | None = None):
        access = access or BusinessDocumentAccess(document.owner_id)
        permissions = access.permissions(document.owner_id)
        revision = None
        if document.current_revision_id:
            revision = cls._revision_dict(BusinessDocumentRevision.get_by_id(document.current_revision_id))
        return {
            "document_id": document.id,
            "tenant_id": document.tenant_id,
            "owner_id": document.owner_id,
            "access_role": access.role.value,
            "owner_name": cls._user_display_name(document.owner_id),
            "permissions": permissions,
            "chat_id": document.chat_id,
            "document_type": document.document_type,
            "title": document.title,
            "idea": document.idea,
            "dataset_ids": document.dataset_ids,
            "template_version": document.template_version,
            "policy_version": document.policy_version,
            "lifecycle_state": document.lifecycle_state,
            "operation_state": document.operation_state,
            "state_version": document.state_version,
            "current_revision": revision,
            "active_review_cycle": document.active_review_cycle,
            "protocol": cls._protocol(document),
            "allowed_commands": cls._allowed_commands(document) if permissions["edit"] else [],
            "last_error": document.last_error,
            "latest_job": cls._latest_job(document.id),
            "latest_exports": cls._latest_exports(document.id),
            "eva_binding": cls._eva_binding(document),
        }

    @classmethod
    def _protocol(cls, document):
        protocol_events = cls._protocol_event_ids(document.id)
        current_comment_dispositions = cls._current_comment_dispositions(document)
        questions = list(
            BusinessDocumentQuestion.select()
            .where((BusinessDocumentQuestion.document_id == document.id) & (BusinessDocumentQuestion.review_cycle == document.active_review_cycle))
            .order_by(BusinessDocumentQuestion.create_time.asc())
        )
        answers = cls._latest_by(
            BusinessDocumentAnswer.select().where(BusinessDocumentAnswer.document_id == document.id),
            "question_id",
        )
        proposals = list(
            BusinessDocumentProposal.select()
            .where((BusinessDocumentProposal.document_id == document.id) & (BusinessDocumentProposal.review_cycle == document.active_review_cycle))
            .order_by(BusinessDocumentProposal.create_time.asc())
        )
        decisions = cls._latest_by(
            BusinessDocumentProposalDecision.select().where(BusinessDocumentProposalDecision.document_id == document.id),
            "proposal_id",
        )
        comments = list(
            BusinessDocumentComment.select()
            .where((BusinessDocumentComment.document_id == document.id) & (BusinessDocumentComment.review_cycle == document.active_review_cycle))
            .order_by(BusinessDocumentComment.create_time.asc())
        )
        return {
            "questions": [
                {
                    "question_id": row.id,
                    "stage": row.stage,
                    "review_cycle": row.review_cycle,
                    "target_section_id": row.target_section_id,
                    "semantic_tag": row.semantic_tag,
                    "text": row.text,
                    "options": row.options,
                    "allow_custom_answer": row.allow_custom_answer,
                    "evidence_refs": row.evidence_refs,
                    "source_event_ids": row.source_event_ids,
                    "answer": cls._answer_dict(answers.get(row.id), protocol_events["answers"]),
                    "status": "ANSWERED" if row.id in answers else "OPEN",
                }
                for row in questions
            ],
            "proposals": [
                {
                    "proposal_id": row.id,
                    "review_cycle": row.review_cycle,
                    "target_section_id": row.target_section_id,
                    "text": row.text,
                    "rationale": row.rationale,
                    "source_event_ids": row.source_event_ids,
                    "evidence_refs": row.evidence_refs,
                    "decision": decisions[row.id].decision if row.id in decisions else "PENDING",
                    "decision_event_id": protocol_events["decisions"].get(row.id),
                }
                for row in proposals
            ],
            "comments": [
                {
                    "comment_id": row.id,
                    "review_cycle": row.review_cycle,
                    "revision_id": row.revision_id,
                    "section_id": row.section_id,
                    "text": row.text,
                    "anchor": row.anchor,
                    "anchor_status": ("GENERAL" if row.anchor is None else "ANCHORED" if row.revision_id == document.current_revision_id else "ORPHANED"),
                    "source_event_id": protocol_events["comments"].get(row.id),
                    "disposition": current_comment_dispositions.get(protocol_events["comments"].get(row.id)),
                }
                for row in comments
            ],
        }

    @classmethod
    def _allowed_commands(cls, document):
        return cls._allowed_commands_values(document.lifecycle_state, document.operation_state, document.id, document.active_review_cycle)

    @classmethod
    def _allowed_commands_values(cls, lifecycle_state, operation_state, document_id, review_cycle):
        if operation_state not in {OperationState.IDLE.value, OperationState.FAILED.value} or lifecycle_state == LifecycleState.ARCHIVED.value:
            return []
        if lifecycle_state == LifecycleState.INTAKE.value:
            commands = [CommandType.ARCHIVE.value]
            if cls._open_questions_by_values(document_id, "INTAKE", review_cycle):
                commands.append(CommandType.ANSWER_QUESTION.value)
            elif cls._assessment_is_current(document_id, "IntakeAssessed", {"QuestionAnswered"}, review_cycle=0):
                commands.append(CommandType.REQUEST_DRAFT.value)
            else:
                commands.append(CommandType.REQUEST_INTAKE_ASSESSMENT.value)
            return commands
        if lifecycle_state == LifecycleState.REVIEW.value:
            commands = [CommandType.DECIDE_PROPOSAL.value, CommandType.ADD_COMMENT.value, CommandType.ARCHIVE.value]
            if cls._open_questions_by_values(document_id, "REVIEW", review_cycle):
                commands.append(CommandType.ANSWER_QUESTION.value)
                if cls._has_unassessed_review_feedback(document_id):
                    commands.append(CommandType.REQUEST_REVIEW_ASSESSMENT.value)
            elif cls._assessment_is_current(
                document_id,
                "ReviewAssessed",
                {"QuestionAnswered", "ProposalDecided", "AuthorCommentAdded", "EvaDocumentPulled"},
                review_cycle=review_cycle,
            ):
                commands.append(CommandType.APPLY_CHANGES.value)
            else:
                commands.append(CommandType.REQUEST_REVIEW_ASSESSMENT.value)
            return commands
        if lifecycle_state == LifecycleState.AGREED.value:
            return [CommandType.START_REVIEW.value, CommandType.REQUEST_EXPORT.value, CommandType.ARCHIVE.value]
        return []

    @classmethod
    def _open_questions(cls, document, stage):
        return cls._open_questions_by_values(document.id, stage, document.active_review_cycle)

    @classmethod
    def _open_questions_by_values(cls, document_id, stage, review_cycle):
        question_ids = [
            row.id
            for row in BusinessDocumentQuestion.select(BusinessDocumentQuestion.id).where(
                (BusinessDocumentQuestion.document_id == document_id) & (BusinessDocumentQuestion.stage == stage) & (BusinessDocumentQuestion.review_cycle == review_cycle)
            )
        ]
        if not question_ids:
            return []
        answered = {
            row.question_id
            for row in BusinessDocumentAnswer.select(BusinessDocumentAnswer.question_id).where(
                (BusinessDocumentAnswer.document_id == document_id) & (BusinessDocumentAnswer.question_id.in_(question_ids))
            )
        }
        return [question_id for question_id in question_ids if question_id not in answered]

    @staticmethod
    def _has_unassessed_review_feedback(document_id):
        feedback_types = {"QuestionAnswered", "ProposalDecided", "AuthorCommentAdded", "EvaDocumentPulled"}
        boundaries = {"DraftCreated", "ReviewCycleStarted", "ReviewAssessed"}
        latest = (
            BusinessDocumentEvent.select(BusinessDocumentEvent.event_type)
            .where((BusinessDocumentEvent.document_id == document_id) & (BusinessDocumentEvent.event_type.in_(feedback_types | boundaries)))
            .order_by(BusinessDocumentEvent.sequence.desc())
            .first()
        )
        return latest is not None and latest.event_type in feedback_types

    @staticmethod
    def _assessment_is_current(document_id, assessment_event_type, invalidating_event_types, review_cycle):
        events = list(
            BusinessDocumentEvent.select(BusinessDocumentEvent.sequence, BusinessDocumentEvent.event_type, BusinessDocumentEvent.payload)
            .where(BusinessDocumentEvent.document_id == document_id)
            .order_by(BusinessDocumentEvent.sequence.desc())
        )
        assessment = next(
            (event for event in events if event.event_type == assessment_event_type and event.payload.get("review_cycle") == review_cycle),
            None,
        )
        if assessment is None or assessment.payload.get("outcome") != "COMPLETE":
            return False
        latest_mutation = max((event.sequence for event in events if event.event_type in invalidating_event_types), default=0)
        return assessment.sequence > latest_mutation

    @staticmethod
    def _latest_by(rows, key):
        latest = {}
        for row in rows:
            entity_id = getattr(row, key)
            current = latest.get(entity_id)
            if current is None or (row.create_time or 0, row.id) > (current.create_time or 0, current.id):
                latest[entity_id] = row
        return latest

    @staticmethod
    def _answer_dict(row, event_ids=None):
        if row is None:
            return None
        return {
            "answer_id": row.id,
            "selected_option_id": row.selected_option_id,
            "custom_answer": row.custom_answer,
            "actor_id": row.actor_id,
            "source_event_id": (event_ids or {}).get(row.id),
        }

    @staticmethod
    def _protocol_event_ids(document_id):
        result = {"answers": {}, "decisions": {}, "comments": {}}
        events = BusinessDocumentEvent.select(BusinessDocumentEvent.id, BusinessDocumentEvent.event_type, BusinessDocumentEvent.payload).where(
            (BusinessDocumentEvent.document_id == document_id) & (BusinessDocumentEvent.event_type.in_(("QuestionAnswered", "ProposalDecided", "AuthorCommentAdded")))
        )
        for event in events:
            if event.event_type == "QuestionAnswered" and event.payload.get("answer_id"):
                result["answers"][event.payload["answer_id"]] = event.id
            elif event.event_type == "ProposalDecided" and event.payload.get("proposal_id"):
                result["decisions"][event.payload["proposal_id"]] = event.id
            elif event.event_type == "AuthorCommentAdded" and event.payload.get("comment_id"):
                result["comments"][event.payload["comment_id"]] = event.id
        return result

    @classmethod
    def _revision_dict(cls, row):
        sections = row.document_ast.get("sections", []) if isinstance(row.document_ast, dict) else []
        author_id = row.author_id or cls._legacy_revision_author(row)
        return {
            "revision_id": row.id,
            "revision_number": row.revision_number,
            "author_id": author_id,
            "author_name": cls._user_display_name(author_id),
            "document_ast": row.document_ast,
            "body_markdown": row.body_markdown,
            "section_texts": {
                section["id"]: render_section_text(section) for section in sections if isinstance(section, dict) and isinstance(section.get("id"), str) and isinstance(section.get("blocks"), list)
            },
            "content_hash": row.content_hash,
            "source_event_ids": row.source_event_ids,
            "created_at": row.create_time,
            "change_basis": cls._revision_change_basis(row),
        }

    @staticmethod
    def _job_request_author(job: BusinessDocumentJob, fallback: str) -> str:
        requested_by = job.payload.get("requested_by_actor_id") if isinstance(job.payload, dict) else None
        return requested_by if isinstance(requested_by, str) and requested_by else fallback

    @staticmethod
    def _normalized_access_role(value: object) -> BusinessDocumentRole:
        try:
            role = BusinessDocumentRole(value)
        except (TypeError, ValueError):
            return BusinessDocumentRole.AUTHOR_EDITOR
        return BusinessDocumentRole.AUTHOR_EDITOR if role == BusinessDocumentRole.ADMIN else role

    @staticmethod
    def _legacy_revision_author(row: BusinessDocumentRevision) -> str | None:
        events = BusinessDocumentEvent.select().where(BusinessDocumentEvent.document_id == row.document_id).order_by(BusinessDocumentEvent.sequence.asc())
        for event in events:
            if event.event_type not in {"DraftCreated", "ChangesApplied"} or not isinstance(event.payload, dict):
                continue
            if event.payload.get("revision_id") != row.id:
                continue
            requested_by = event.payload.get("requested_by_actor_id")
            if isinstance(requested_by, str) and requested_by:
                return requested_by
            job_id = event.payload.get("job_id")
            if isinstance(job_id, str) and job_id:
                job = BusinessDocumentJob.get_or_none(BusinessDocumentJob.id == job_id)
                if job and isinstance(job.payload, dict):
                    requested_by = job.payload.get("requested_by_actor_id")
                    if isinstance(requested_by, str) and requested_by:
                        return requested_by
            if event.actor_type == "USER":
                return event.actor_id
        return None

    @staticmethod
    def _user_display_name(user_id: str | None) -> str | None:
        if not user_id:
            return None
        if User._meta.database is not BusinessDocumentRevision._meta.database:
            return user_id
        try:
            user = User.get_or_none(User.id == user_id)
        except Exception:
            return user_id
        return user.nickname if user and user.nickname else user_id

    @classmethod
    def _revision_change_basis(cls, row: BusinessDocumentRevision) -> list[dict[str, Any]]:
        event_ids = [event_id for event_id in (row.source_event_ids or []) if isinstance(event_id, str)]
        if not event_ids:
            return []
        events = {event.id: event for event in BusinessDocumentEvent.select().where((BusinessDocumentEvent.document_id == row.document_id) & (BusinessDocumentEvent.id.in_(event_ids)))}
        basis: list[dict[str, Any]] = []
        for event_id in event_ids:
            event = events.get(event_id)
            if event is None or event.event_type in {"ChangesApplied", "ReviewAgreedWithoutChanges"}:
                continue
            common = {
                "event_id": event.id,
                "actor_id": event.actor_id,
                "actor_type": event.actor_type,
                "created_at": event.create_time,
            }
            requested_by_actor_id = event.payload.get("requested_by_actor_id") if isinstance(event.payload, dict) else None
            if isinstance(requested_by_actor_id, str) and requested_by_actor_id:
                common["initiated_by_actor_id"] = requested_by_actor_id
            if event.event_type == "DraftCreated":
                document = BusinessDocument.get_by_id(row.document_id)
                basis.append(
                    {
                        **common,
                        "type": "INITIAL_DRAFT",
                        "title": "Первичный черновик",
                        "summary": document.idea,
                        "section_id": None,
                    }
                )
                continue
            if event.event_type == "QuestionAnswered":
                question = BusinessDocumentQuestion.get_or_none((BusinessDocumentQuestion.id == event.payload.get("question_id")) & (BusinessDocumentQuestion.document_id == row.document_id))
                answer = BusinessDocumentAnswer.get_or_none((BusinessDocumentAnswer.id == event.payload.get("answer_id")) & (BusinessDocumentAnswer.document_id == row.document_id))
                if question is None or answer is None:
                    continue
                option_label = next(
                    (str(option.get("label") or option.get("option_id")) for option in question.options if isinstance(option, dict) and option.get("option_id") == answer.selected_option_id),
                    None,
                )
                basis.append(
                    {
                        **common,
                        "type": "QUESTION",
                        "title": "Ответ на вопрос",
                        "summary": question.text,
                        "details": answer.custom_answer or option_label,
                        "section_id": question.target_section_id,
                    }
                )
                continue
            if event.event_type == "ProposalDecided" and event.payload.get("decision") == "ACCEPTED":
                proposal = BusinessDocumentProposal.get_or_none((BusinessDocumentProposal.id == event.payload.get("proposal_id")) & (BusinessDocumentProposal.document_id == row.document_id))
                if proposal is None:
                    continue
                basis.append(
                    {
                        **common,
                        "type": "PROPOSAL",
                        "title": "Принятое предложение ИИ",
                        "summary": proposal.text,
                        "details": proposal.rationale,
                        "section_id": proposal.target_section_id,
                    }
                )
                continue
            if event.event_type == "AuthorCommentAdded":
                comment = BusinessDocumentComment.get_or_none((BusinessDocumentComment.id == event.payload.get("comment_id")) & (BusinessDocumentComment.document_id == row.document_id))
                if comment is None:
                    continue
                basis.append(
                    {
                        **common,
                        "type": "COMMENT",
                        "title": "Комментарий автора",
                        "summary": comment.text,
                        "details": comment.anchor.get("selected_text") if isinstance(comment.anchor, dict) else None,
                        "section_id": comment.section_id,
                    }
                )
                continue
            if event.event_type == "EvaDocumentPulled":
                basis.append(
                    {
                        **common,
                        "type": "EVA_SYNC",
                        "title": "Изменения из EVA",
                        "summary": str(event.payload.get("page_url") or "Связанная страница EVA"),
                        "details": str(event.payload.get("remote_version") or "") or None,
                        "section_id": None,
                    }
                )
        return basis

    @classmethod
    def _eva_matches_with_occupancy(cls, matches: list[dict[str, Any]]) -> list[dict[str, Any]]:
        keys: set[str] = set()
        for match in matches:
            page_url_key, identity_key = _binding_keys(match)
            keys.add(page_url_key)
            if identity_key:
                keys.add(identity_key)
        occupied = list(
            BusinessDocumentEvaBinding.select().where(
                (BusinessDocumentEvaBinding.page_url_key.in_(keys)) | (BusinessDocumentEvaBinding.eva_identity_key.in_(keys))
            )
        ) if keys else []
        occupied_document_ids = [binding.document_id for binding in occupied]
        documents = (
            {row.id: row for row in BusinessDocument.select().where(BusinessDocument.id.in_(occupied_document_ids))}
            if occupied_document_ids
            else {}
        )
        occupied_by_key: dict[str, BusinessDocumentEvaBinding] = {}
        for binding in occupied:
            occupied_by_key[binding.page_url_key] = binding
            if binding.eva_identity_key:
                occupied_by_key[binding.eva_identity_key] = binding
        result = []
        for match in matches:
            page_url_key, identity_key = _binding_keys(match)
            existing = occupied_by_key.get(identity_key or "") or occupied_by_key.get(page_url_key)
            item = dict(match)
            item["binding_available"] = existing is None
            if existing is not None:
                document = documents.get(existing.document_id)
                item["linked_document"] = {
                    "document_id": existing.document_id,
                    "title": document.title if document else None,
                }
            result.append(item)
        return result

    @classmethod
    def _resolve_create_eva_binding(
        cls,
        actor_id: str,
        raw: dict[str, Any],
        schema_version: object,
        display_title: str,
    ) -> dict[str, Any] | None:
        page_url = raw.get("eva_page_url")
        if schema_version != "2":
            return None

        decision = raw.get("eva_decision")
        decision_mode = str(decision.get("mode") or "") if isinstance(decision, dict) else ""
        from api.apps.business_documents.eva_changes import EvaDocumentChangeService

        if page_url:
            binding = EvaDocumentChangeService.resolve_page_url(actor_id, page_url)
            if binding.get("status") not in {"CONNECTED", "LINK_ONLY"}:
                raise ConflictError("EVA_BINDING_UNAVAILABLE", "The selected EVA page could not be verified")
            remote_title = str(binding.get("document_name") or "").strip()
            if remote_title and normalize_title(remote_title) != normalize_title(display_title):
                raise ConflictError(
                    "EVA_PAGE_TITLE_CHANGED",
                    "The selected EVA page no longer has the document title",
                    {"actual_title": binding.get("document_name")},
                )
            return binding
        if decision_mode == "SKIP":
            return None
        matches = EvaDocumentChangeService.find_title_matches(actor_id, display_title)
        if matches:
            raise ConflictError(
                "EVA_BINDING_DECISION_REQUIRED",
                "EVA Wiki contains pages with this title. Choose a page or continue without linking.",
                {"matches": cls._eva_matches_with_occupancy(matches)},
            )
        return None

    @staticmethod
    def _store_eva_binding(document_id: str, binding: dict[str, Any], *, replace: bool = False) -> None:
        page_url_key, identity_key = _binding_keys(binding)
        values = {
            "status": str(binding.get("status") or "LINK_ONLY"),
            "page_url_key": page_url_key,
            "eva_identity_key": identity_key,
            "binding": dict(binding),
            **_timestamps(),
        }
        try:
            if replace:
                existing = BusinessDocumentEvaBinding.get_or_none(BusinessDocumentEvaBinding.document_id == document_id)
                if existing is not None:
                    values.pop("create_time", None)
                    values.pop("create_date", None)
                    BusinessDocumentEvaBinding.update(**values).where(BusinessDocumentEvaBinding.document_id == document_id).execute()
                    return
            BusinessDocumentEvaBinding.create(document_id=document_id, **values)
        except IntegrityError as error:
            raise ConflictError("EVA_PAGE_ALREADY_LINKED", "The selected EVA page is already linked to another document") from error

    @staticmethod
    def _eva_binding(document: BusinessDocument) -> dict[str, Any] | None:
        projection = BusinessDocumentEvaBinding.get_or_none(BusinessDocumentEvaBinding.document_id == document.id)
        if projection is not None and isinstance(projection.binding, dict):
            return dict(projection.binding)
        created = (
            BusinessDocumentEvent.select()
            .where((BusinessDocumentEvent.document_id == document.id) & (BusinessDocumentEvent.event_type == "DocumentCreated"))
            .order_by(BusinessDocumentEvent.sequence.asc())
            .first()
        )
        raw_binding = created.payload.get("eva_binding") if created is not None and isinstance(created.payload, dict) else None
        if not isinstance(raw_binding, dict) or not raw_binding.get("page_url"):
            return None
        binding = dict(raw_binding)
        latest_resolution = (
            BusinessDocumentEvent.select()
            .where((BusinessDocumentEvent.document_id == document.id) & (BusinessDocumentEvent.event_type == "EvaBindingResolved"))
            .order_by(BusinessDocumentEvent.sequence.desc())
            .first()
        )
        if latest_resolution is not None and isinstance(latest_resolution.payload, dict):
            resolved_binding = latest_resolution.payload.get("eva_binding")
            if isinstance(resolved_binding, dict) and resolved_binding.get("page_url"):
                binding = dict(resolved_binding)
        latest_pull = (
            BusinessDocumentEvent.select()
            .where((BusinessDocumentEvent.document_id == document.id) & (BusinessDocumentEvent.event_type == "EvaDocumentPulled"))
            .order_by(BusinessDocumentEvent.sequence.desc())
            .first()
        )
        if latest_pull is not None and (latest_resolution is None or latest_pull.sequence > latest_resolution.sequence):
            binding.update(
                {
                    "remote_version": latest_pull.payload.get("remote_version"),
                    "remote_content_hash": latest_pull.payload.get("remote_content_hash"),
                    "last_pulled_content_hash": latest_pull.payload.get("remote_content_hash"),
                    "last_pulled_at": latest_pull.create_time,
                    "last_pull_event_id": latest_pull.id,
                    "last_pull_review_cycle": latest_pull.payload.get("review_cycle"),
                }
            )
        return binding

    @staticmethod
    def _job_dict(row):
        return {
            "job_id": row.id,
            "job_type": row.job_type,
            "status": row.status,
            "progress": row.progress,
            "progress_stage": row.progress_stage,
            "progress_message": row.progress_message,
            "attempt": row.attempt,
            "max_attempts": row.max_attempts,
            "available_at": row.available_at,
            "lease_expires_at": row.lease_expires_at,
            "error": row.error,
            "create_time": row.create_time,
            "update_time": row.update_time,
        }

    @staticmethod
    def _job_prompt_audit(row):
        prompt = row.payload.get("prompt") if isinstance(row.payload, dict) else None
        if not isinstance(prompt, dict):
            return {}
        return {
            "prompt_name": prompt.get("name"),
            "prompt_version": prompt.get("version"),
            "prompt_hash": prompt.get("content_hash"),
        }

    @staticmethod
    def _validate_execution_audit(value, job):
        if value is None:
            if job.job_type != "GENERATE_EXPORT" and job.payload.get("dataset_ids") and related_file_search_enabled():
                raise ValidationError("EVIDENCE_AUDIT_REQUIRED", "AI jobs with datasets require a pinned evidence audit")
            return {}
        if not isinstance(value, dict) or set(value) != {"retrieval"} or not isinstance(value["retrieval"], dict):
            raise ValidationError("INVALID_EXECUTION_AUDIT", "Worker execution audit is invalid")
        retrieval = value["retrieval"]
        expected = {
            "attempt",
            "retrieved_at",
            "dataset_ids",
            "query_hash",
            "evidence_hash",
            "source_refs",
            "chunk_count",
            "total_chars",
        }
        if set(retrieval) != expected:
            raise ValidationError("INVALID_EXECUTION_AUDIT", "Worker retrieval audit fields are invalid")
        if (
            not isinstance(retrieval["attempt"], int)
            or retrieval["attempt"] < 1
            or not isinstance(retrieval["retrieved_at"], str)
            or not isinstance(retrieval["dataset_ids"], list)
            or len(retrieval["dataset_ids"]) > 20
            or not all(isinstance(item, str) for item in retrieval["dataset_ids"])
            or not isinstance(retrieval["source_refs"], list)
            or not all(isinstance(item, str) for item in retrieval["source_refs"])
            or not isinstance(retrieval["chunk_count"], int)
            or retrieval["chunk_count"] != len(retrieval["source_refs"])
            or not isinstance(retrieval["total_chars"], int)
            or retrieval["total_chars"] < 0
            or not all(isinstance(retrieval[field], str) and len(retrieval[field]) == 71 and retrieval[field].startswith("sha256:") for field in ("query_hash", "evidence_hash"))
        ):
            raise ValidationError("INVALID_EXECUTION_AUDIT", "Worker retrieval audit values are invalid")
        snapshot = BusinessDocumentEvidenceSnapshot.get_or_none(BusinessDocumentEvidenceSnapshot.job_id == job.id)
        if snapshot is None:
            raise ValidationError("EVIDENCE_SNAPSHOT_REQUIRED", "Retrieval audit has no pinned evidence snapshot")
        snapshot_chunks = snapshot.snapshot.get("chunks", []) if isinstance(snapshot.snapshot, dict) else []
        snapshot_refs = [chunk.get("source_ref") for chunk in snapshot_chunks if isinstance(chunk, dict)]
        if (
            retrieval["evidence_hash"] != snapshot.evidence_hash
            or retrieval["dataset_ids"] != snapshot.dataset_ids
            or retrieval["source_refs"] != snapshot_refs
            or retrieval["chunk_count"] != len(snapshot_chunks)
        ):
            raise ValidationError("EVIDENCE_AUDIT_MISMATCH", "Retrieval audit does not match the pinned evidence snapshot")
        return {"retrieval": {key: retrieval[key] for key in sorted(expected)}}

    @classmethod
    def _latest_job(cls, document_id):
        row = BusinessDocumentJob.select().where(BusinessDocumentJob.document_id == document_id).order_by(BusinessDocumentJob.create_time.desc()).first()
        return cls._job_dict(row) if row else None

    @staticmethod
    def _latest_exports(document_id):
        rows = BusinessDocumentExportArtifact.select().where(BusinessDocumentExportArtifact.document_id == document_id).order_by(BusinessDocumentExportArtifact.create_time.desc()).limit(10)
        return [
            {
                "artifact_id": row.id,
                "revision_id": row.revision_id,
                "revision_number": BusinessDocumentRevision.get_by_id(row.revision_id).revision_number,
                "format": row.export_format,
                "filename": row.filename,
                "mime_type": row.mime_type,
                "size": row.size,
                "content_hash": row.content_hash,
                "create_time": row.create_time,
            }
            for row in rows
        ]

    @classmethod
    def _command_response(cls, document, new_version, event_id):
        return {
            "accepted": True,
            "document_id": document.id,
            "state_version": new_version,
            "lifecycle_state": document.lifecycle_state,
            "operation_state": document.operation_state,
            "event_id": event_id,
            "allowed_commands": cls._allowed_commands_values(document.lifecycle_state, document.operation_state, document.id, document.active_review_cycle),
        }

    @staticmethod
    def _get_document(tenant_id, document_id):
        document = BusinessDocument.get_or_none((BusinessDocument.id == document_id) & (BusinessDocument.tenant_id == tenant_id))
        if document is None:
            raise NotFoundError()
        return document

    @staticmethod
    def _get_editable_document(document_id, access: BusinessDocumentAccess):
        document = BusinessDocument.get_or_none(BusinessDocument.id == document_id)
        if document is None:
            raise NotFoundError()
        access.require_edit(document.owner_id)
        return document

    @staticmethod
    def _get_accessible_document(document_id):
        document = BusinessDocument.get_or_none(BusinessDocument.id == document_id)
        if document is None:
            raise NotFoundError()
        return document

    @staticmethod
    def _require_lifecycle(document, lifecycle):
        if document.lifecycle_state != lifecycle.value:
            raise ConflictError(
                "LIFECYCLE_STATE_CONFLICT",
                f"Command requires {lifecycle.value} lifecycle state",
                {"actual": document.lifecycle_state},
            )

    @staticmethod
    def _require_idle(document):
        if document.operation_state not in {OperationState.IDLE.value, OperationState.FAILED.value}:
            raise ConflictError("OPERATION_IN_PROGRESS", "Another operation is already in progress", {"operation_state": document.operation_state})

    @staticmethod
    def _require_current_job_lease(job, worker_id, lease_token):
        if job.status != "RUNNING" or not lease_token or job.lease_owner != worker_id or job.lease_token != lease_token or job.lease_expires_at is None or job.lease_expires_at <= current_timestamp():
            raise ConflictError("JOB_LEASE_LOST", "Worker no longer owns a current lease for this job")

    @staticmethod
    def _optimistic_update(document, changes):
        old_version = document.state_version
        changes.update(update_time=current_timestamp(), update_date=datetime.now())
        changed = BusinessDocument.update(**changes).where((BusinessDocument.id == document.id) & (BusinessDocument.state_version == old_version)).execute()
        if changed != 1:
            raise ConflictError("STATE_VERSION_CONFLICT", "The document changed concurrently")
        for key, value in changes.items():
            setattr(document, key, value)

    @staticmethod
    def _create_event(
        document_id,
        sequence,
        event_type,
        actor_type,
        actor_id,
        payload,
        correlation_id,
        causation_id=None,
        event_id=None,
    ):
        event_id = event_id or get_uuid()
        BusinessDocumentEvent.create(
            id=event_id,
            document_id=document_id,
            sequence=sequence,
            event_type=event_type,
            actor_type=actor_type,
            actor_id=actor_id,
            payload=payload,
            correlation_id=correlation_id,
            causation_id=causation_id,
            **_timestamps(),
        )
        return event_id
