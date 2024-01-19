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
from api.db.services.dialog_service import DialogService
from api.db import StatusEnum
from api.db.services.kb_service import KnowledgebaseService
from api.db.services.user_service import TenantService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.utils import get_uuid
from api.utils.api_utils import get_json_result


@manager.route('/set', methods=['POST'])
@login_required
def set():
    req = request.json
    dialog_id = req.get("dialog_id")
    name = req.get("name", "New Dialog")
    description = req.get("description", "A helpful Dialog")
    language = req.get("language", "Chinese")
    llm_setting_type = req.get("llm_setting_type", "Precise")
    llm_setting = req.get("llm_setting", {
        "Creative": {
            "temperature": 0.9,
            "top_p": 0.9,
            "frequency_penalty": 0.2,
            "presence_penalty": 0.4,
            "max_tokens": 512
        },
        "Precise": {
            "temperature": 0.1,
            "top_p": 0.3,
            "frequency_penalty": 0.7,
            "presence_penalty": 0.4,
            "max_tokens": 215
        },
        "Evenly": {
            "temperature": 0.5,
            "top_p": 0.5,
            "frequency_penalty": 0.7,
            "presence_penalty": 0.4,
            "max_tokens": 215
        },
        "Custom": {
            "temperature": 0.2,
            "top_p": 0.3,
            "frequency_penalty": 0.6,
            "presence_penalty": 0.3,
            "max_tokens": 215
        },
    })
    prompt_config = req.get("prompt_config", {
        "system": """你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。
以下是知识库：
{knowledge}
以上是知识库。""",
        "prologue": "您好，我是您的助手小樱，长得可爱又善良，can I help you?",
        "parameters": [
            {"key": "knowledge", "optional": False}
        ],
        "empty_response": "Sorry! 知识库中未找到相关内容！"
    })

    if len(prompt_config["parameters"]) < 1:
        return get_data_error_result(retmsg="'knowledge' should be in parameters")

    for p in prompt_config["parameters"]:
        if prompt_config["system"].find("{%s}"%p["key"]) < 0:
            return get_data_error_result(retmsg="Parameter '{}' is not used".format(p["key"]))

    try:
        e, tenant = TenantService.get_by_id(current_user.id)
        if not e:return get_data_error_result(retmsg="Tenant not found!")
        llm_id = req.get("llm_id", tenant.llm_id)
        if not dialog_id:
            dia = {
                "id": get_uuid(),
                "tenant_id": current_user.id,
                "name": name,
                "description": description,
                "language": language,
                "llm_id": llm_id,
                "llm_setting_type": llm_setting_type,
                "llm_setting": llm_setting,
                "prompt_config": prompt_config
            }
            if not DialogService.save(**dia): return get_data_error_result(retmsg="Fail to new a dialog!")
            e, dia = DialogService.get_by_id(dia["id"])
            if not e: return get_data_error_result(retmsg="Fail to new a dialog!")
            return get_json_result(data=dia.to_json())
        else:
            del req["dialog_id"]
            if "kb_names" in req: del req["kb_names"]
            if not DialogService.update_by_id(dialog_id, req):
                return get_data_error_result(retmsg="Dialog not found!")
            e, dia = DialogService.get_by_id(dialog_id)
            if not e: return get_data_error_result(retmsg="Fail to update a dialog!")
            dia = dia.to_dict()
            dia["kb_ids"], dia["kb_names"] = get_kb_names(dia["kb_ids"])
            return get_json_result(data=dia)
    except Exception as e:
        return server_error_response(e)


@manager.route('/get', methods=['GET'])
@login_required
def get():
    dialog_id = request.args["dialog_id"]
    try:
        e,dia = DialogService.get_by_id(dialog_id)
        if not e: return get_data_error_result(retmsg="Dialog not found!")
        dia = dia.to_dict()
        dia["kb_ids"], dia["kb_names"] = get_kb_names(dia["kb_ids"])
        return get_json_result(data=dia)
    except Exception as e:
        return server_error_response(e)

def get_kb_names(kb_ids):
    ids, nms = [], []
    for kid in kb_ids:
        e, kb = KnowledgebaseService.get_by_id(kid)
        if not e or kb.status != StatusEnum.VALID.value:continue
        ids.append(kid)
        nms.append(kb.name)
    return ids, nms

@manager.route('/list', methods=['GET'])
@login_required
def list():
    try:
        diags = DialogService.query(tenant_id=current_user.id, status=StatusEnum.VALID.value)
        diags = [d.to_dict() for d in diags]
        for d in diags:
            d["kb_ids"], d["kb_names"] = get_kb_names(d["kb_ids"])
        return get_json_result(data=diags)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])
@login_required
@validate_request("dialog_id")
def rm():
    req = request.json
    try:
        if not DialogService.update_by_id(req["dialog_id"], {"status": StatusEnum.INVALID.value}):
            return get_data_error_result(retmsg="Dialog not found!")
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)