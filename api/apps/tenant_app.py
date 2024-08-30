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
from flask_login import current_user, login_required

from api.utils.api_utils import server_error_response
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import get_json_result


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
