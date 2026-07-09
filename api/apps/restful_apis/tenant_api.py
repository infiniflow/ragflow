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
import asyncio
import logging
from typing import Set

from api.apps import current_user, login_required
from api.db import UserTenantRole
from api.db.db_models import UserTenant
from api.db.services.user_service import UserService, UserTenantService
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
    validate_request,
)
from api.utils.web_utils import send_invite_email
from common import settings
from common.constants import RetCode, StatusEnum
from common.misc_utils import get_uuid
from common.time_utils import delta_seconds

# Keeps strong references to fire-and-forget tasks so they are not GC'd before completion.
_background_tasks: Set[asyncio.Task] = set()


@manager.route("/tenants/<tenant_id>/users", methods=["GET"])  # noqa: F821
@login_required
def user_list(tenant_id):
    if current_user.id != tenant_id:
        return get_json_result(
            data=False,
            message="No authorization.",
            code=RetCode.AUTHENTICATION_ERROR,
        )

    try:
        users = UserTenantService.get_by_tenant_id(tenant_id)
        for user in users:
            user["delta_seconds"] = delta_seconds(str(user["update_date"]))
        return get_json_result(data=users)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/tenants/<tenant_id>/users", methods=["POST"])  # noqa: F821
@login_required
@validate_request("email")
async def create(tenant_id):
    if current_user.id != tenant_id:
        return get_json_result(
            data=False,
            message="No authorization.",
            code=RetCode.AUTHENTICATION_ERROR,
        )

    req = await get_request_json()
    invite_user_email = req["email"]
    invite_users = UserService.query(email=invite_user_email)
    if not invite_users:
        return get_data_error_result(message="User not found.")

    user_id_to_invite = invite_users[0].id
    user_tenants = UserTenantService.query(user_id=user_id_to_invite, tenant_id=tenant_id)
    if user_tenants:
        user_tenant_role = user_tenants[0].role
        if user_tenant_role == UserTenantRole.NORMAL:
            return get_data_error_result(message=f"{invite_user_email} is already in the team.")
        if user_tenant_role == UserTenantRole.OWNER:
            return get_data_error_result(message=f"{invite_user_email} is the owner of the team.")
        return get_data_error_result(message=f"{invite_user_email} is in the team, but the role: {user_tenant_role} is invalid.")

    UserTenantService.save(
        id=get_uuid(),
        user_id=user_id_to_invite,
        tenant_id=tenant_id,
        invited_by=current_user.id,
        role=UserTenantRole.INVITE,
        status=StatusEnum.VALID.value,
    )

    try:
        user_name = ""
        _, user = UserService.get_by_id(current_user.id)
        if user:
            user_name = user.nickname

        def _on_invite_email_done(done_task: asyncio.Task) -> None:
            _background_tasks.discard(done_task)
            try:
                done_task.result()
            except asyncio.CancelledError:
                logging.warning("Invite email task cancelled: tenant_id=%s to=%s", tenant_id, invite_user_email)
            except Exception:
                logging.exception("Invite email task failed: tenant_id=%s to=%s", tenant_id, invite_user_email)

        task = asyncio.create_task(
            send_invite_email(
                to_email=invite_user_email,
                invite_url=settings.MAIL_FRONTEND_URL,
                tenant_id=tenant_id,
                inviter=user_name or current_user.email,
            )
        )
        if isinstance(task, asyncio.Task):
            _background_tasks.add(task)
            task.add_done_callback(_on_invite_email_done)
    except Exception as exc:
        logging.exception(f"Failed to send invite email to {invite_user_email}: {exc}")
        return get_json_result(
            data=False,
            message="Failed to send invite email.",
            code=RetCode.SERVER_ERROR,
        )

    user = invite_users[0].to_dict()
    user = {k: v for k, v in user.items() if k in ["id", "avatar", "email", "nickname"]}
    return get_json_result(data=user)


@manager.route("/tenants/<tenant_id>/users", methods=["DELETE"])  # noqa: F821
@login_required
@validate_request("user_id")
async def rm(tenant_id):
    req = await get_request_json()
    user_id = req["user_id"]
    if current_user.id != tenant_id and current_user.id != user_id:
        return get_json_result(
            data=False,
            message="No authorization.",
            code=RetCode.AUTHENTICATION_ERROR,
        )

    try:
        UserTenantService.filter_delete([UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id])
        return get_json_result(data=True)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/tenants", methods=["GET"])  # noqa: F821
@login_required
def tenant_list():
    try:
        users = UserTenantService.get_tenants_by_user_id(current_user.id)
        for user in users:
            user["delta_seconds"] = delta_seconds(str(user["update_date"]))
        return get_json_result(data=users)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/tenants/<tenant_id>", methods=["PATCH"])  # noqa: F821
@login_required
def agree(tenant_id):
    try:
        UserTenantService.filter_update(
            [UserTenant.tenant_id == tenant_id, UserTenant.user_id == current_user.id],
            {"role": UserTenantRole.NORMAL},
        )
        return get_json_result(data=True)
    except Exception as exc:
        return server_error_response(exc)
