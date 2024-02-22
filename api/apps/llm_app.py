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

from api.db.services import duplicate_name
from api.db.services.llm_service import LLMFactoriesService, TenantLLMService, LLMService
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.utils import get_uuid, get_format_time
from api.db import StatusEnum, UserTenantRole, LLMType
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.db_models import Knowledgebase, TenantLLM
from api.settings import stat_logger, RetCode
from api.utils.api_utils import get_json_result
from rag.llm import EmbeddingModel, CvModel, ChatModel


@manager.route('/factories', methods=['GET'])
@login_required
def factories():
    try:
        fac = LLMFactoriesService.get_all()
        return get_json_result(data=[f.to_dict() for f in fac])
    except Exception as e:
        return server_error_response(e)


@manager.route('/set_api_key', methods=['POST'])
@login_required
@validate_request("llm_factory", "api_key")
def set_api_key():
    req = request.json
    # test if api key works
    msg = ""
    for llm in LLMService.query(fid=req["llm_factory"]):
        if llm.model_type == LLMType.EMBEDDING.value:
            mdl = EmbeddingModel[req["llm_factory"]](
                req["api_key"], llm.llm_name)
            try:
                arr, tc = mdl.encode(["Test if the api key is available"])
                if len(arr[0]) == 0 or tc ==0: raise Exception("Fail")
            except Exception as e:
                msg += f"\nFail to access embedding model({llm.llm_name}) using this api key."
        elif llm.model_type == LLMType.CHAT.value:
            mdl = ChatModel[req["llm_factory"]](
                req["api_key"], llm.llm_name)
            try:
                m, tc = mdl.chat(None, [{"role": "user", "content": "Hello! How are you doing!"}], {"temperature": 0.9})
                if not tc: raise Exception(m)
            except Exception as e:
                msg += f"\nFail to access model({llm.llm_name}) using this api key." + str(e)

    if msg: return get_data_error_result(retmsg=msg)

    llm = {
        "tenant_id": current_user.id,
        "llm_factory": req["llm_factory"],
        "api_key": req["api_key"]
    }
    for n in ["model_type", "llm_name"]:
        if n in req: llm[n] = req[n]

    TenantLLMService.filter_update([TenantLLM.tenant_id==llm["tenant_id"], TenantLLM.llm_factory==llm["llm_factory"]], llm)
    return get_json_result(data=True)


@manager.route('/my_llms', methods=['GET'])
@login_required
def my_llms():
    try:
        objs = TenantLLMService.get_my_llms(current_user.id)
        return get_json_result(data=objs)
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['GET'])
@login_required
def list():
    model_type = request.args.get("model_type")
    try:
        objs = TenantLLMService.query(tenant_id=current_user.id)
        facts = set([o.to_dict()["llm_factory"] for o in objs if o.api_key])
        llms = LLMService.get_all()
        llms = [m.to_dict() for m in llms if m.status == StatusEnum.VALID.value]
        for m in llms:
            m["available"] = m["fid"] in facts

        res = {}
        for m in llms:
            if model_type and m["model_type"] != model_type: continue
            if m["fid"] not in res: res[m["fid"]] = []
            res[m["fid"]].append(m)

        return get_json_result(data=res)
    except Exception as e:
        return server_error_response(e)