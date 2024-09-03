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
from flask_login import current_user, login_required

from api.db import UserTenantRole, StatusEnum
from api.db.db_models import UserTenant
from api.db.services.user_service import TenantService, UserTenantService
from api.settings import RetCode

from api.utils import get_uuid
from api.utils.api_utils import get_json_result, validate_request, server_error_response


@manager.route("/list", methods=["GET"])
@login_required
def tenant_list():
    try:
        tenants = TenantService.get_by_user_id(current_user.id)
        return get_json_result(data=tenants)
    except Exception as e:
        return server_error_response(e)


@manager.route("/<tenant_id>/user/list", methods=["GET"])
@login_required
def user_list(tenant_id):
    try:
        users = UserTenantService.get_by_tenant_id(tenant_id)
        return get_json_result(data=users)
    except Exception as e:
        return server_error_response(e)


@manager.route('/<tenant_id>/user', methods=['POST'])
@login_required
@validate_request("user_id")
def create(tenant_id):
    user_id = request.json.get("user_id")
    if not user_id:
        return get_json_result(
            data=False, retmsg='Lack of "USER ID"', retcode=RetCode.ARGUMENT_ERROR)

    try:
        user_tenants = UserTenantService.query(user_id=user_id, tenant_id=tenant_id)
        if user_tenants:
            uuid = user_tenants[0].id
            return get_json_result(data={"id": uuid})

        uuid = get_uuid()
        UserTenantService.save(
            id = uuid,
            user_id = user_id,
            tenant_id = tenant_id,
            role = UserTenantRole.NORMAL.value,
            status = StatusEnum.VALID.value)

        return get_json_result(data={"id": uuid})
    except Exception as e:
        return server_error_response(e)


@manager.route('/<tenant_id>/user/<user_id>', methods=['DELETE'])
@login_required
def rm(tenant_id, user_id):
    try:
        UserTenantService.filter_delete([UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id])
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)
    