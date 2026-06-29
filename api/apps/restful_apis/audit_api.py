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
"""Audit-log query API (issue #14598): paginated, filterable history of mutating
operations for the caller's tenant."""
from quart import request

from api.apps import current_user, login_required
from api.db.services.audit_log_service import VALID_AUDIT_OPERATIONS, AuditLogService
from api.db.services.user_service import UserTenantService
from api.utils.api_utils import get_data_error_result, get_json_result, server_error_response


@manager.route('/audit/list', methods=['GET'])  # noqa: F821
@login_required
def list_audit_logs():
    """List audit-log entries for the current user's tenant, newest first.

    Query params: ``operation`` (insert|update|delete), ``resource_type``,
    ``resource_id``, ``page`` (1-based), ``page_size``.
    """
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")
        tenant_id = tenants[0].tenant_id

        operation = request.args.get("operation")
        if operation and operation.lower() not in VALID_AUDIT_OPERATIONS:
            return get_data_error_result(
                message=f"invalid operation '{operation}'; expected one of {sorted(VALID_AUDIT_OPERATIONS)}"
            )
        try:
            page = int(request.args.get("page", 1))
            page_size = int(request.args.get("page_size", 30))
        except (TypeError, ValueError):
            return get_data_error_result(message="page and page_size must be integers")

        logs, total = AuditLogService.list_logs(
            tenant_id,
            operation=operation,
            resource_type=request.args.get("resource_type"),
            resource_id=request.args.get("resource_id"),
            page=page,
            page_size=page_size,
        )
        return get_json_result(data={"total": total, "logs": logs})
    except Exception as e:
        return server_error_response(e)
