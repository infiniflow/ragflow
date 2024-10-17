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
#  limitations under the License
#
import json
from datetime import datetime

from flask_login import login_required, current_user

from api.db.db_models import APIToken
from api.db.services.api_service import APITokenService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import UserTenantService
from api.settings import DATABASE_TYPE
from api.utils import current_timestamp, datetime_format
from api.utils.api_utils import get_json_result, get_data_error_result, server_error_response, \
    generate_confirmation_token, request, validate_request
from api.versions import get_rag_version
from rag.utils.es_conn import ELASTICSEARCH
from rag.utils.storage_factory import STORAGE_IMPL, STORAGE_IMPL_TYPE
from timeit import default_timer as timer

from rag.utils.redis_conn import REDIS_CONN


@manager.route('/version', methods=['GET'])
@login_required
def version():
    return get_json_result(data=get_rag_version())


@manager.route('/status', methods=['GET'])
@login_required
def status():
    res = {}
    st = timer()
    try:
        res["es"] = ELASTICSEARCH.health()
        res["es"]["elapsed"] = "{:.1f}".format((timer() - st)*1000.)
    except Exception as e:
        res["es"] = {"status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    st = timer()
    try:
        STORAGE_IMPL.health()
        res["storage"] = {"storage": STORAGE_IMPL_TYPE.lower(), "status": "green", "elapsed": "{:.1f}".format((timer() - st)*1000.)}
    except Exception as e:
        res["storage"] = {"storage": STORAGE_IMPL_TYPE.lower(), "status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    st = timer()
    try:
        KnowledgebaseService.get_by_id("x")
        res["database"] = {"database": DATABASE_TYPE.lower(), "status": "green", "elapsed": "{:.1f}".format((timer() - st)*1000.)}
    except Exception as e:
        res["database"] = {"database": DATABASE_TYPE.lower(), "status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    st = timer()
    try:
        if not REDIS_CONN.health():
            raise Exception("Lost connection!")
        res["redis"] = {"status": "green", "elapsed": "{:.1f}".format((timer() - st)*1000.)}
    except Exception as e:
        res["redis"] = {"status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    try:
        v = REDIS_CONN.get("TASKEXE")
        if not v:
            raise Exception("No task executor running!")
        obj = json.loads(v)
        color = "green"
        for id in obj.keys():
            arr = obj[id]
            if len(arr) == 1:
                obj[id] = [0]
            else:
                obj[id] = [arr[i+1]-arr[i] for i in range(len(arr)-1)]
            elapsed = max(obj[id])
            if elapsed > 50: color = "yellow"
            if elapsed > 120: color = "red"
        res["task_executor"] = {"status": color, "elapsed": obj}
    except Exception as e:
        res["task_executor"] = {"status": "red", "error": str(e)}

    return get_json_result(data=res)


@manager.route('/new_token', methods=['POST'])
@login_required
def new_token():
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(retmsg="Tenant not found!")

        tenant_id = tenants[0].tenant_id
        obj = {"tenant_id": tenant_id, "token": generate_confirmation_token(tenant_id),
               "create_time": current_timestamp(),
               "create_date": datetime_format(datetime.now()),
               "update_time": None,
               "update_date": None
               }

        if not APITokenService.save(**obj):
            return get_data_error_result(retmsg="Fail to new a dialog!")

        return get_json_result(data=obj)
    except Exception as e:
        return server_error_response(e)


@manager.route('/token_list', methods=['GET'])
@login_required
def token_list():
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(retmsg="Tenant not found!")

        objs = APITokenService.query(tenant_id=tenants[0].tenant_id)
        return get_json_result(data=[o.to_dict() for o in objs])
    except Exception as e:
        return server_error_response(e)


@manager.route('/token/<token>', methods=['DELETE'])
@login_required
def rm(token):
    APITokenService.filter_delete(
                [APIToken.tenant_id == current_user.id, APIToken.token == token])
    return get_json_result(data=True)