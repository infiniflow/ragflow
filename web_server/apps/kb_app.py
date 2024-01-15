#
#  Copyright 2019 The FATE Authors. All Rights Reserved.
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

from web_server.db.services import duplicate_name
from web_server.db.services.user_service import TenantService, UserTenantService
from web_server.utils.api_utils import server_error_response, get_data_error_result, validate_request
from web_server.utils import get_uuid, get_format_time
from web_server.db import StatusEnum, UserTenantRole
from web_server.db.services.kb_service import KnowledgebaseService
from web_server.db.db_models import Knowledgebase
from web_server.settings import stat_logger, RetCode
from web_server.utils.api_utils import get_json_result


@manager.route('/create', methods=['post'])
@login_required
@validate_request("name", "description", "permission", "embd_id", "parser_id")
def create():
    req = request.json
    req["name"] = req["name"].strip()
    req["name"] = duplicate_name(KnowledgebaseService.query, name=req["name"], tenant_id=current_user.id, status=StatusEnum.VALID.value)
    try:
        req["id"] = get_uuid()
        req["tenant_id"] = current_user.id
        req["created_by"] = current_user.id
        if not KnowledgebaseService.save(**req): return get_data_error_result()
        return get_json_result(data={"kb_id": req["id"]})
    except Exception as e:
        return server_error_response(e)


@manager.route('/update', methods=['post'])
@login_required
@validate_request("kb_id", "name", "description", "permission", "embd_id", "parser_id")
def update():
    req = request.json
    req["name"] = req["name"].strip()
    try:
        if not KnowledgebaseService.query(created_by=current_user.id, id=req["kb_id"]):
            return get_json_result(data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.', retcode=RetCode.OPERATING_ERROR)

        e, kb = KnowledgebaseService.get_by_id(req["kb_id"])
        if not e: return get_data_error_result(retmsg="Can't find this knowledgebase!")

        if req["name"].lower() != kb.name.lower() \
            and len(KnowledgebaseService.query(name=req["name"], tenant_id=current_user.id, status=StatusEnum.VALID.value))>1:
            return get_data_error_result(retmsg="Duplicated knowledgebase name.")

        del req["kb_id"]
        if not KnowledgebaseService.update_by_id(kb.id, req): return get_data_error_result()

        e, kb = KnowledgebaseService.get_by_id(kb.id)
        if not e: return get_data_error_result(retmsg="Database error (Knowledgebase rename)!")

        return get_json_result(data=kb.to_json())
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['GET'])
@login_required
def list():
    page_number = request.args.get("page", 1)
    items_per_page = request.args.get("page_size", 15)
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)
    try:
        tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
        kbs = KnowledgebaseService.get_by_tenant_ids([m["tenant_id"] for m in tenants], current_user.id, page_number, items_per_page, orderby, desc)
        return get_json_result(data=kbs)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['post'])
@login_required
@validate_request("kb_id")
def rm():
    req = request.json
    try:
        if not KnowledgebaseService.query(created_by=current_user.id, id=req["kb_id"]):
            return get_json_result(data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.', retcode=RetCode.OPERATING_ERROR)

        if not KnowledgebaseService.update_by_id(req["kb_id"], {"status": StatusEnum.IN_VALID.value}): return get_data_error_result(retmsg="Database error (Knowledgebase removal)!")
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)