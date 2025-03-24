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


from flask import request
from flask_login import current_user, login_required
from langfuse import Langfuse

from api.db.db_models import DB
from api.db.services.langfuse_service import TenantLangfuseService
from api.utils.api_utils import get_error_data_result, get_json_result, server_error_response, validate_request


@manager.route("/set_api_key", methods=["POST"])  # noqa: F821
@login_required
@validate_request("secret_key", "public_key", "host")
def set_api_key():
    req = request.get_json()
    secret_key = req.get("secret_key", "")
    public_key = req.get("public_key", "")
    host = req.get("host", "")
    if not all([secret_key, public_key, host]):
        return get_error_data_result(message="Missing required fields")

    langfuse_keys = dict(
        tenant_id=current_user.id,
        secret_key=secret_key,
        public_key=public_key,
        host=host,
    )

    langfuse = Langfuse(public_key=langfuse_keys["public_key"], secret_key=langfuse_keys["secret_key"], host=langfuse_keys["host"])
    if not langfuse.auth_check():
        return get_error_data_result(message="Invalid Langfuse keys")

    langfuse_entry = TenantLangfuseService.filter_by_tenant(tenant_id=current_user.id)
    with DB.atomic():
        try:
            if not langfuse_entry:
                TenantLangfuseService.save(**langfuse_keys)
            else:
                TenantLangfuseService.update_by_tenant(tenant_id=current_user.id, langfuse_keys=langfuse_keys)
            return get_json_result(data=True)
        except Exception as e:
            server_error_response(e)
