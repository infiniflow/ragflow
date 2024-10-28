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
from api.db.services.dialog_service import DialogService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import  TenantLLMService
from api.db.services.user_service import TenantService
from api.utils import get_uuid
from api.utils.api_utils import get_error_data_result, token_required
from api.utils.api_utils import get_result



@manager.route('/chat', methods=['POST'])
@token_required
def create(tenant_id):
    req=request.json
    ids= req.get("dataset_ids")
    if not ids:
        return get_error_data_result(retmsg="`dataset_ids` is required")
    for kb_id in ids:
        kbs = KnowledgebaseService.query(id=kb_id,tenant_id=tenant_id)
        if not kbs:
            return get_error_data_result(f"You don't own the dataset {kb_id}")
        kb=kbs[0]
        if kb.chunk_num == 0:
            return get_error_data_result(f"The dataset {kb_id} doesn't own parsed file")
    req["kb_ids"] = ids
    # llm
    llm = req.get("llm")
    if llm:
        if "model_name" in llm:
            req["llm_id"] = llm.pop("model_name")
            if not TenantLLMService.query(tenant_id=tenant_id,llm_name=req["llm_id"],model_type="chat"):
                return get_error_data_result(f"`model_name` {req.get('llm_id')} doesn't exist")
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
    if req.get("rerank_id"):
        if not TenantLLMService.query(tenant_id=tenant_id,llm_name=req.get("rerank_id"),model_type="rerank"):
            return get_error_data_result(f"`rerank_model` {req.get('rerank_id')} doesn't exist")
    if not req.get("llm_id"):
        req["llm_id"] = tenant.llm_id
    if not req.get("name"):
        return get_error_data_result(retmsg="`name` is required.")
    if DialogService.query(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value):
        return get_error_data_result(retmsg="Duplicated chat name in creating chat.")
    # tenant_id
    if req.get("tenant_id"):
        return get_error_data_result(retmsg="`tenant_id` must not be provided.")
    req["tenant_id"] = tenant_id
    # prompt more parameter
    default_prompt = {
        "system": """You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.""",
        "prologue": "Hi! I'm your assistant, what can I do for you?",
        "parameters": [
            {"key": "knowledge", "optional": False}
        ],
        "empty_response": "Sorry! No relevant content was found in the knowledge base!"
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
    res["dataset_ids"] = req["dataset_ids"]
    res["avatar"] = res.pop("icon")
    return get_result(data=res)

@manager.route('/chat/<chat_id>', methods=['PUT'])
@token_required
def update(tenant_id,chat_id):
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(retmsg='You do not own the chat')
    req =request.json
    ids = req.get("dataset_ids")
    if "dataset_ids" in req:
        if not ids:
            return get_error_data_result("`datasets` can't be empty")
        if ids:
            for kb_id in ids:
                kbs = KnowledgebaseService.query(id=kb_id, tenant_id=tenant_id)
                if not kbs:
                    return get_error_data_result(f"You don't own the dataset {kb_id}")
                kb = kbs[0]
                if kb.chunk_num == 0:
                    return get_error_data_result(f"The dataset {kb_id} doesn't own parsed file")
            req["kb_ids"] = ids
    llm = req.get("llm")
    if llm:
        if "model_name" in llm:
            req["llm_id"] = llm.pop("model_name")
            if not TenantLLMService.query(tenant_id=tenant_id,llm_name=req["llm_id"],model_type="chat"):
                return get_error_data_result(f"`model_name` {req.get('llm_id')} doesn't exist")
        req["llm_setting"] = req.pop("llm")
    e, tenant = TenantService.get_by_id(tenant_id)
    if not e:
        return get_error_data_result(retmsg="Tenant not found!")
    if req.get("rerank_model"):
        if not TenantLLMService.query(tenant_id=tenant_id,llm_name=req.get("rerank_model"),model_type="rerank"):
            return get_error_data_result(f"`rerank_model` {req.get('rerank_model')} doesn't exist")
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
    if "name" in req:
        if not req.get("name"):
            return get_error_data_result(retmsg="`name` is not empty.")
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
    if "dataset_ids" in req:
        req.pop("dataset_ids")
    if not DialogService.update_by_id(chat_id, req):
        return get_error_data_result(retmsg="Chat not found!")
    return get_result()


@manager.route('/chat', methods=['DELETE'])
@token_required
def delete(tenant_id):
    req = request.json
    ids = req.get("ids")
    if not ids:
        return get_error_data_result(retmsg="`ids` are required")
    for id in ids:
        if not DialogService.query(tenant_id=tenant_id, id=id, status=StatusEnum.VALID.value):
            return get_error_data_result(retmsg=f"You don't own the chat {id}")
        temp_dict = {"status": StatusEnum.INVALID.value}
        DialogService.update_by_id(id, temp_dict)
    return get_result()

@manager.route('/chat', methods=['GET'])
@token_required
def list_chat(tenant_id):
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
        res["datasets"] = kb_list
        res["avatar"] = res.pop("icon")
        list_assts.append(res)
    return get_result(data=list_assts)
