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

from api.db import StatusEnum
from api.db.db_models import TenantLLM
from api.db.services.dialog_service import DialogService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMService, TenantLLMService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import get_data_error_result, token_required
from api.utils.api_utils import get_json_result


@manager.route('/save', methods=['POST'])
@token_required
def save(tenant_id):
    req = request.json
    # dataset
    if req.get("knowledgebases") == []:
        return get_data_error_result(retmsg="knowledgebases can not be empty list")
    kb_list = []
    if req.get("knowledgebases"):
        for kb in req.get("knowledgebases"):
            if not kb["id"]:
                return get_data_error_result(retmsg="knowledgebase needs id")
            if not KnowledgebaseService.query(id=kb["id"], tenant_id=tenant_id):
                return get_data_error_result(retmsg="you do not own the knowledgebase")
            # if not DocumentService.query(kb_id=kb["id"]):
            #  return get_data_error_result(retmsg="There is a invalid knowledgebase")
            kb_list.append(kb["id"])
    req["kb_ids"] = kb_list
    # llm
    llm = req.get("llm")
    if llm:
        if "model_name" in llm:
            req["llm_id"] = llm.pop("model_name")
        req["llm_setting"] = req.pop("llm")
    e, tenant = TenantService.get_by_id(tenant_id)
    if not e:
        return get_data_error_result(retmsg="Tenant not found!")
    # prompt
    prompt = req.get("prompt")
    key_mapping = {"parameters": "variables",
                   "prologue": "opener",
                   "quote": "show_quote",
                   "system": "prompt",
                   "rerank_id": "rerank_model",
                   "vector_similarity_weight": "keywords_similarity_weight"}
    key_list = ["similarity_threshold", "vector_similarity_weight", "top_n", "rerank_id"]
    if prompt:
        for new_key, old_key in key_mapping.items():
            if old_key in prompt:
                prompt[new_key] = prompt.pop(old_key)
        for key in key_list:
            if key in prompt:
                req[key] = prompt.pop(key)
        req["prompt_config"] = req.pop("prompt")
    # create
    if "id" not in req:
        # dataset
        if not kb_list:
            return get_data_error_result(retmsg="knowledgebases are required!")
        # init
        req["id"] = get_uuid()
        req["description"] = req.get("description", "A helpful Assistant")
        req["icon"] = req.get("avatar", "")
        req["top_n"] = req.get("top_n", 6)
        req["top_k"] = req.get("top_k", 1024)
        req["rerank_id"] = req.get("rerank_id", "")
        if req.get("llm_id"):
            if not TenantLLMService.query(llm_name=req["llm_id"]):
                return get_data_error_result(retmsg="the model_name does not exist.")
        else:
            req["llm_id"] = tenant.llm_id
        if not req.get("name"):
            return get_data_error_result(retmsg="name is required.")
        if DialogService.query(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value):
            return get_data_error_result(retmsg="Duplicated assistant name in creating dataset.")
        # tenant_id
        if req.get("tenant_id"):
            return get_data_error_result(retmsg="tenant_id must not be provided.")
        req["tenant_id"] = tenant_id
        # prompt more parameter
        default_prompt = {
            "system": """你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。
                以下是知识库：
                {knowledge}
                以上是知识库。""",
            "prologue": "您好，我是您的助手小樱，长得可爱又善良，can I help you?",
            "parameters": [
                {"key": "knowledge", "optional": False}
            ],
            "empty_response": "Sorry! 知识库中未找到相关内容！"
        }
        key_list_2 = ["system", "prologue", "parameters", "empty_response"]
        if "prompt_config" not in req:
            req['prompt_config'] = {}
        for key in key_list_2:
            temp = req['prompt_config'].get(key)
            if not temp:
                req['prompt_config'][key] = default_prompt[key]
        for p in req['prompt_config']["parameters"]:
            if p["optional"]:
                continue
            if req['prompt_config']["system"].find("{%s}" % p["key"]) < 0:
                return get_data_error_result(
                    retmsg="Parameter '{}' is not used".format(p["key"]))
        # save
        if not DialogService.save(**req):
            return get_data_error_result(retmsg="Fail to new an assistant!")
        # response
        e, res = DialogService.get_by_id(req["id"])
        if not e:
            return get_data_error_result(retmsg="Fail to new an assistant!")
        res = res.to_json()
        renamed_dict = {}
        for key, value in res["prompt_config"].items():
            new_key = key_mapping.get(key, key)
            renamed_dict[new_key] = value
        res["prompt"] = renamed_dict
        del res["prompt_config"]
        new_dict = {"similarity_threshold": res["similarity_threshold"],
                    "keywords_similarity_weight": res["vector_similarity_weight"],
                    "top_n": res["top_n"],
                    "rerank_model": res['rerank_id']}
        res["prompt"].update(new_dict)
        for key in key_list:
            del res[key]
        res["llm"] = res.pop("llm_setting")
        res["llm"]["model_name"] = res.pop("llm_id")
        del res["kb_ids"]
        res["knowledgebases"] = req["knowledgebases"]
        res["avatar"] = res.pop("icon")
        return get_json_result(data=res)
    else:
        # authorization
        if not DialogService.query(tenant_id=tenant_id, id=req["id"], status=StatusEnum.VALID.value):
            return get_json_result(data=False, retmsg='You do not own the assistant', retcode=RetCode.OPERATING_ERROR)
        # prompt
        if not req["id"]:
            return get_data_error_result(retmsg="id can not be empty")
        e, res = DialogService.get_by_id(req["id"])
        res = res.to_json()
        if "llm_id" in req:
            if not TenantLLMService.query(llm_name=req["llm_id"]):
                return get_data_error_result(retmsg="the model_name does not exist.")
        if "name" in req:
            if not req.get("name"):
                return get_data_error_result(retmsg="name is not empty.")
            if req["name"].lower() != res["name"].lower() \
                    and len(
                DialogService.query(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value)) > 0:
                return get_data_error_result(retmsg="Duplicated assistant name in updating dataset.")
        if "prompt_config" in req:
            res["prompt_config"].update(req["prompt_config"])
            for p in res["prompt_config"]["parameters"]:
                if p["optional"]:
                    continue
                if res["prompt_config"]["system"].find("{%s}" % p["key"]) < 0:
                    return get_data_error_result(retmsg="Parameter '{}' is not used".format(p["key"]))
        if "llm_setting" in req:
            res["llm_setting"].update(req["llm_setting"])
        req["prompt_config"] = res["prompt_config"]
        req["llm_setting"] = res["llm_setting"]
        # avatar
        if "avatar" in req:
            req["icon"] = req.pop("avatar")
        assistant_id = req.pop("id")
        if "knowledgebases" in req:
            req.pop("knowledgebases")
        if not DialogService.update_by_id(assistant_id, req):
            return get_data_error_result(retmsg="Assistant not found!")
        return get_json_result(data=True)


@manager.route('/delete', methods=['DELETE'])
@token_required
def delete(tenant_id):
    req = request.args
    if "id" not in req:
        return get_data_error_result(retmsg="id is required")
    id = req['id']
    if not DialogService.query(tenant_id=tenant_id, id=id, status=StatusEnum.VALID.value):
        return get_json_result(data=False, retmsg='you do not own the assistant.', retcode=RetCode.OPERATING_ERROR)

    temp_dict = {"status": StatusEnum.INVALID.value}
    DialogService.update_by_id(req["id"], temp_dict)
    return get_json_result(data=True)


@manager.route('/get', methods=['GET'])
@token_required
def get(tenant_id):
    req = request.args
    if "id" in req:
        id = req["id"]
        ass = DialogService.query(tenant_id=tenant_id, id=id, status=StatusEnum.VALID.value)
        if not ass:
            return get_json_result(data=False, retmsg='You do not own the assistant.', retcode=RetCode.OPERATING_ERROR)
        if "name" in req:
            name = req["name"]
            if ass[0].name != name:
                return get_json_result(data=False, retmsg='name does not match id.', retcode=RetCode.OPERATING_ERROR)
        res = ass[0].to_json()
    else:
        if "name" in req:
            name = req["name"]
            ass = DialogService.query(name=name, tenant_id=tenant_id, status=StatusEnum.VALID.value)
            if not ass:
                return get_json_result(data=False, retmsg='You do not own the assistant.',
                                       retcode=RetCode.OPERATING_ERROR)
            res = ass[0].to_json()
        else:
            return get_data_error_result(retmsg="At least one of `id` or `name` must be provided.")
    renamed_dict = {}
    key_mapping = {"parameters": "variables",
                   "prologue": "opener",
                   "quote": "show_quote",
                   "system": "prompt",
                   "rerank_id": "rerank_model",
                   "vector_similarity_weight": "keywords_similarity_weight"}
    key_list = ["similarity_threshold", "vector_similarity_weight", "top_n", "rerank_id"]
    for key, value in res["prompt_config"].items():
        new_key = key_mapping.get(key, key)
        renamed_dict[new_key] = value
    res["prompt"] = renamed_dict
    del res["prompt_config"]
    new_dict = {"similarity_threshold": res["similarity_threshold"],
                "keywords_similarity_weight": res["vector_similarity_weight"],
                "top_n": res["top_n"],
                "rerank_model": res['rerank_id']}
    res["prompt"].update(new_dict)
    for key in key_list:
        del res[key]
    res["llm"] = res.pop("llm_setting")
    res["llm"]["model_name"] = res.pop("llm_id")
    kb_list = []
    for kb_id in res["kb_ids"]:
        kb = KnowledgebaseService.query(id=kb_id)
        kb_list.append(kb[0].to_json())
    del res["kb_ids"]
    res["knowledgebases"] = kb_list
    res["avatar"] = res.pop("icon")
    return get_json_result(data=res)


@manager.route('/list', methods=['GET'])
@token_required
def list_assistants(tenant_id):
    assts = DialogService.query(
        tenant_id=tenant_id,
        status=StatusEnum.VALID.value,
        reverse=True,
        order_by=DialogService.model.create_time)
    assts = [d.to_dict() for d in assts]
    list_assts = []
    renamed_dict = {}
    key_mapping = {"parameters": "variables",
                   "prologue": "opener",
                   "quote": "show_quote",
                   "system": "prompt",
                   "rerank_id": "rerank_model",
                   "vector_similarity_weight": "keywords_similarity_weight"}
    key_list = ["similarity_threshold", "vector_similarity_weight", "top_n", "rerank_id"]
    for res in assts:
        for key, value in res["prompt_config"].items():
            new_key = key_mapping.get(key, key)
            renamed_dict[new_key] = value
        res["prompt"] = renamed_dict
        del res["prompt_config"]
        new_dict = {"similarity_threshold": res["similarity_threshold"],
                    "keywords_similarity_weight": res["vector_similarity_weight"],
                    "top_n": res["top_n"],
                    "rerank_model": res['rerank_id']}
        res["prompt"].update(new_dict)
        for key in key_list:
            del res[key]
        res["llm"] = res.pop("llm_setting")
        res["llm"]["model_name"] = res.pop("llm_id")
        kb_list = []
        for kb_id in res["kb_ids"]:
            kb = KnowledgebaseService.query(id=kb_id)
            kb_list.append(kb[0].to_json())
        del res["kb_ids"]
        res["knowledgebases"] = kb_list
        res["avatar"] = res.pop("icon")
        list_assts.append(res)
    return get_json_result(data=list_assts)
