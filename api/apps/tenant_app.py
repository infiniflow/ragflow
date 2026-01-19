#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import logging
import asyncio
from api.db import UserTenantRole
from api.db.db_models import UserTenant
from api.db.services.user_service import UserTenantService, UserService

from common.constants import RetCode, StatusEnum
from common.misc_utils import get_uuid
from common.time_utils import delta_seconds
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from api.utils.web_utils import send_invite_email
from common import settings
from api.apps import login_required, current_user


@manager.route("/<tenant_id>/user/list", methods=["GET"])  # noqa: F821
@login_required
def user_list(tenant_id):
    if current_user.id != tenant_id:
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR)

    try:
        users = UserTenantService.get_by_tenant_id(tenant_id)
        for u in users:
            u["delta_seconds"] = delta_seconds(str(u["update_date"]))
        return get_json_result(data=users)
    except Exception as e:
        return server_error_response(e)


@manager.route('/<tenant_id>/user', methods=['POST'])  # noqa: F821
@login_required
@validate_request("email")
async def create(tenant_id):
    if current_user.id != tenant_id:
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR)

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
        return get_data_error_result(
            message=f"{invite_user_email} is in the team, but the role: {user_tenant_role} is invalid.")

    UserTenantService.save(
        id=get_uuid(),
        user_id=user_id_to_invite,
        tenant_id=tenant_id,
        invited_by=current_user.id,
        role=UserTenantRole.INVITE,
        status=StatusEnum.VALID.value)

    try:

        user_name = ""
        _, user = UserService.get_by_id(current_user.id)
        if user:
            user_name = user.nickname

        asyncio.create_task(
            send_invite_email(
                to_email=invite_user_email,
                invite_url=settings.MAIL_FRONTEND_URL,
                tenant_id=tenant_id,
                inviter=user_name or current_user.email
            )
        )
    except Exception as e:
        logging.exception(f"Failed to send invite email to {invite_user_email}: {e}")
        return get_json_result(data=False, message="Failed to send invite email.", code=RetCode.SERVER_ERROR)
    usr = invite_users[0].to_dict()
    usr = {k: v for k, v in usr.items() if k in ["id", "avatar", "email", "nickname"]}

    return get_json_result(data=usr)


@manager.route('/<tenant_id>/user/<user_id>', methods=['DELETE'])  # noqa: F821
@login_required
def rm(tenant_id, user_id):
    if current_user.id != tenant_id and current_user.id != user_id:
        return get_json_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR)

    try:
        UserTenantService.filter_delete([UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id])
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
def tenant_list():
    try:
        users = UserTenantService.get_tenants_by_user_id(current_user.id)
        for u in users:
            u["delta_seconds"] = delta_seconds(str(u["update_date"]))
        return get_json_result(data=users)
    except Exception as e:
        return server_error_response(e)


@manager.route("/agree/<tenant_id>", methods=["PUT"])  # noqa: F821
@login_required
def agree(tenant_id):
    try:
        UserTenantService.filter_update([UserTenant.tenant_id == tenant_id, UserTenant.user_id == current_user.id],
                                        {"role": UserTenantRole.NORMAL})
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)
