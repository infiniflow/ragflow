#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""Audit-log service (issue #14598).

Records mutating operations — who (tenant/user) did what (insert/update/delete)
to which resource (dataset/document/file/...), and when — and exposes a paginated,
filterable query over that history. Call sites use :meth:`AuditLogService.log_safe`
so that an audit failure can never break the operation being audited.
"""
import logging

from api.db.db_models import DB, AuditLog
from api.db.services.common_service import CommonService
from common.misc_utils import get_uuid

#: The only operation kinds an audit entry may record.
VALID_AUDIT_OPERATIONS = {"insert", "update", "delete"}

#: Upper bound on a single page of audit history (defensive against huge queries).
MAX_AUDIT_PAGE_SIZE = 200


class AuditLogService(CommonService):
    model = AuditLog

    @classmethod
    def log(
        cls,
        tenant_id: str,
        operation: str,
        resource_type: str,
        *,
        user_id: str | None = None,
        resource_id: str | None = None,
        resource_name: str = "",
        detail: dict | None = None,
    ):
        """Record one audit entry and return the created row.

        :param tenant_id: tenant the operation belongs to (required).
        :param operation: one of ``insert`` / ``update`` / ``delete`` (case-insensitive).
        :param resource_type: kind of resource affected, e.g. ``dataset`` (required).
        :param user_id: id of the acting user, if known.
        :param resource_id: id of the affected resource, if known.
        :param resource_name: human-readable name of the affected resource.
        :param detail: extra structured context to persist alongside the entry.
        :raises ValueError: if ``operation`` is unsupported or a required field is missing.
        """
        op = (operation or "").lower()
        if op not in VALID_AUDIT_OPERATIONS:
            raise ValueError(f"invalid audit operation {operation!r}; expected one of {sorted(VALID_AUDIT_OPERATIONS)}")
        if not tenant_id or not resource_type:
            raise ValueError("tenant_id and resource_type are required")
        return cls.insert(
            id=get_uuid(),
            tenant_id=tenant_id,
            user_id=user_id,
            operation=op,
            resource_type=resource_type,
            resource_id=resource_id,
            resource_name=resource_name or "",
            detail=detail or {},
        )

    @classmethod
    def log_safe(cls, *args, **kwargs):
        """Best-effort :meth:`log` — swallow and warn on any failure so the audited
        operation is never broken by the audit trail itself. Returns the row or None."""
        try:
            return cls.log(*args, **kwargs)
        except Exception as e:  # noqa: BLE001
            logging.warning(f"audit log write failed: {e}")
            return None

    @classmethod
    @DB.connection_context()
    def list_logs(
        cls,
        tenant_id: str,
        *,
        operation: str | None = None,
        resource_type: str | None = None,
        resource_id: str | None = None,
        page: int = 1,
        page_size: int = 30,
    ) -> tuple[list[dict], int]:
        """Return ``(rows, total)`` of audit entries for ``tenant_id``, newest first.

        ``operation`` / ``resource_type`` / ``resource_id`` are optional equality
        filters. ``page`` is 1-based; ``page_size`` is clamped to
        :data:`MAX_AUDIT_PAGE_SIZE`.
        """
        m = cls.model
        conds = [m.tenant_id == tenant_id]
        if operation:
            conds.append(m.operation == operation.lower())
        if resource_type:
            conds.append(m.resource_type == resource_type)
        if resource_id:
            conds.append(m.resource_id == resource_id)
        query = m.select().where(*conds).order_by(m.create_time.desc())
        total = query.count()
        page = max(1, int(page))
        page_size = max(1, min(int(page_size), MAX_AUDIT_PAGE_SIZE))
        rows = [row.to_dict() for row in query.paginate(page, page_size)]
        return rows, total
