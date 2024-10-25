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

from flask import request
from flask_login import login_required, current_user

from api.db import UserTenantRole, StatusEnum
from api.db.db_models import UserTenant
from api.db.services.user_service import UserTenantService, UserService

from api.utils import get_uuid, delta_seconds
from api.utils.api_utils import get_json_result, validate_request, server_error_response, get_data_error_result


@manager.route("/<tenant_id>/user/list", methods=["GET"])
@login_required
def user_list(tenant_id):
    try:
        users = UserTenantService.get_by_tenant_id(tenant_id)
        for u in users:
            u["delta_seconds"] = delta_seconds(str(u["update_date"]))
        return get_json_result(data=users)
    except Exception as e:
        return server_error_response(e)


@manager.route('/<tenant_id>/user', methods=['POST'])
@login_required
@validate_request("email")
def create(tenant_id):
    req = request.json
    usrs = UserService.query(email=req["email"])
    if not usrs:
        return get_data_error_result(retmsg="User not found.")

    user_id = usrs[0].id
    user_tenants = UserTenantService.query(user_id=user_id, tenant_id=tenant_id)
    if user_tenants:
        if user_tenants[0].status == UserTenantRole.NORMAL.value:
            return get_data_error_result(retmsg="This user is in the team already.")
        return get_data_error_result(retmsg="Invitation notification is sent.")

    UserTenantService.save(
        id=get_uuid(),
        user_id=user_id,
        tenant_id=tenant_id,
        invited_by=current_user.id,
        role=UserTenantRole.INVITE,
        status=StatusEnum.VALID.value)

    usr = usrs[0].to_dict()
    usr = {k: v for k, v in usr.items() if k in ["id", "avatar", "email", "nickname"]}

    return get_json_result(data=usr)


@manager.route('/<tenant_id>/user/<user_id>', methods=['DELETE'])
@login_required
def rm(tenant_id, user_id):
    try:
        UserTenantService.filter_delete([UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id])
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route("/list", methods=["GET"])
@login_required
def tenant_list():
    try:
        users = UserTenantService.get_tenants_by_user_id(current_user.id)
        for u in users:
            u["delta_seconds"] = delta_seconds(str(u["update_date"]))
        return get_json_result(data=users)
    except Exception as e:
        return server_error_response(e)


@manager.route("/agree/<tenant_id>", methods=["PUT"])
@login_required
def agree(tenant_id):
    try:
        UserTenantService.filter_update([UserTenant.tenant_id == tenant_id, UserTenant.user_id == current_user.id], {"role": UserTenantRole.NORMAL})
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)
