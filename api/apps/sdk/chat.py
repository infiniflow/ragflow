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
from api.utils.api_utils import get_error_data_result, token_required
from api.utils.api_utils import get_result

@manager.route('/chat', methods=['POST'])
@token_required
def create(tenant_id):
    req=request.json
    if not req.get("knowledgebases"):
        return get_error_data_result(retmsg="knowledgebases are required")
    kb_list = []
    for kb in req.get("knowledgebases"):
        if not kb["id"]:
            return get_error_data_result(retmsg="knowledgebase needs id")
        if not KnowledgebaseService.query(id=kb["id"], tenant_id=tenant_id):
            return get_error_data_result(retmsg="you do not own the knowledgebase")
        # if not DocumentService.query(kb_id=kb["id"]):
        #  return get_error_data_result(retmsg="There is a invalid knowledgebase")
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
        return get_error_data_result(retmsg="Tenant not found!")
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
    # init
    req["id"] = get_uuid()
    req["description"] = req.get("description", "A helpful Assistant")
    req["icon"] = req.get("avatar", "")
    req["top_n"] = req.get("top_n", 6)
    req["top_k"] = req.get("top_k", 1024)
    req["rerank_id"] = req.get("rerank_id", "")
    if req.get("llm_id"):
        if not TenantLLMService.query(llm_name=req["llm_id"]):
            return get_error_data_result(retmsg="the model_name does not exist.")
    else:
        req["llm_id"] = tenant.llm_id
    if not req.get("name"):
        return get_error_data_result(retmsg="name is required.")
    if DialogService.query(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value):
        return get_error_data_result(retmsg="Duplicated chat name in creating dataset.")
    # tenant_id
    if req.get("tenant_id"):
        return get_error_data_result(retmsg="tenant_id must not be provided.")
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
            return get_error_data_result(
                retmsg="Parameter '{}' is not used".format(p["key"]))
    # save
    if not DialogService.save(**req):
        return get_error_data_result(retmsg="Fail to new a chat!")
    # response
    e, res = DialogService.get_by_id(req["id"])
    if not e:
        return get_error_data_result(retmsg="Fail to new a chat!")
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
    return get_result(data=res)

@manager.route('/chat/<chat_id>', methods=['PUT'])
@token_required
def update(tenant_id,chat_id):
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(retmsg='You do not own the chat')
    req =request.json
    if "knowledgebases" in req:
        if not req.get("knowledgebases"):
            return  get_error_data_result(retmsg="knowledgebases can't be empty value")
        kb_list = []
        for kb in req.get("knowledgebases"):
            if not kb["id"]:
                return get_error_data_result(retmsg="knowledgebase needs id")
            if not KnowledgebaseService.query(id=kb["id"], tenant_id=tenant_id):
                return get_error_data_result(retmsg="you do not own the knowledgebase")
            # if not DocumentService.query(kb_id=kb["id"]):
            #  return get_error_data_result(retmsg="There is a invalid knowledgebase")
            kb_list.append(kb["id"])
        req["kb_ids"] = kb_list
    llm = req.get("llm")
    if llm:
        if "model_name" in llm:
            req["llm_id"] = llm.pop("model_name")
        req["llm_setting"] = req.pop("llm")
    e, tenant = TenantService.get_by_id(tenant_id)
    if not e:
        return get_error_data_result(retmsg="Tenant not found!")
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
    e, res = DialogService.get_by_id(chat_id)
    res = res.to_json()
    if "llm_id" in req:
        if not TenantLLMService.query(llm_name=req["llm_id"]):
            return get_error_data_result(retmsg="the model_name does not exist.")
    if "name" in req:
        if not req.get("name"):
            return get_error_data_result(retmsg="name is not empty.")
        if req["name"].lower() != res["name"].lower() \
                and len(
            DialogService.query(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value)) > 0:
            return get_error_data_result(retmsg="Duplicated chat name in updating dataset.")
    if "prompt_config" in req:
        res["prompt_config"].update(req["prompt_config"])
        for p in res["prompt_config"]["parameters"]:
            if p["optional"]:
                continue
            if res["prompt_config"]["system"].find("{%s}" % p["key"]) < 0:
                return get_error_data_result(retmsg="Parameter '{}' is not used".format(p["key"]))
    if "llm_setting" in req:
        res["llm_setting"].update(req["llm_setting"])
    req["prompt_config"] = res["prompt_config"]
    req["llm_setting"] = res["llm_setting"]
    # avatar
    if "avatar" in req:
        req["icon"] = req.pop("avatar")
    if "knowledgebases" in req:
        req.pop("knowledgebases")
    if not DialogService.update_by_id(chat_id, req):
        return get_error_data_result(retmsg="Chat not found!")
    return get_result()


@manager.route('/chat', methods=['DELETE'])
@token_required
def delete(tenant_id):
    req = request.json
    ids = req.get("ids")
    if not ids:
        return get_error_data_result(retmsg="ids are required")
    for id in ids:
        if not DialogService.query(tenant_id=tenant_id, id=id, status=StatusEnum.VALID.value):
            return get_error_data_result(retmsg=f"You don't own the chat {id}")
        temp_dict = {"status": StatusEnum.INVALID.value}
        DialogService.update_by_id(id, temp_dict)
    return get_result()

@manager.route('/chat', methods=['GET'])
@token_required
def list(tenant_id):
    id = request.args.get("id")
    name = request.args.get("name")
    chat = DialogService.query(id=id,name=name,status=StatusEnum.VALID.value)
    if not chat:
        return get_error_data_result(retmsg="The chat doesn't exist")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 1024))
    orderby = request.args.get("orderby", "create_time")
    if request.args.get("desc") == "False" or request.args.get("desc") == "false":
        desc = False
    else:
        desc = True
    chats = DialogService.get_list(tenant_id,page_number,items_per_page,orderby,desc,id,name)
    if not chats:
        return get_result(data=[])
    list_assts = []
    renamed_dict = {}
    key_mapping = {"parameters": "variables",
                   "prologue": "opener",
                   "quote": "show_quote",
                   "system": "prompt",
                   "rerank_id": "rerank_model",
                   "vector_similarity_weight": "keywords_similarity_weight"}
    key_list = ["similarity_threshold", "vector_similarity_weight", "top_n", "rerank_id"]
    for res in chats:
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
            if not kb :
                return get_error_data_result(retmsg=f"Don't exist the kb {kb_id}")
            kb_list.append(kb[0].to_json())
        del res["kb_ids"]
        res["knowledgebases"] = kb_list
        res["avatar"] = res.pop("icon")
        list_assts.append(res)
    return get_result(data=list_assts)
