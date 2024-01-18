#
#  Copyright 2019 The InfiniFlow Authors. All Rights Reserved.
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
import re

import tiktoken
from flask import request
from flask_login import login_required, current_user
from api.db.services.dialog_service import DialogService, ConversationService
from api.db import StatusEnum, LLMType
from api.db.services.kb_service import KnowledgebaseService
from api.db.services.llm_service import LLMService, TenantLLMService
from api.db.services.user_service import TenantService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.utils import get_uuid
from api.utils.api_utils import get_json_result
from rag.llm import ChatModel
from rag.nlp import retrievaler
from rag.nlp.query import EsQueryer
from rag.utils import num_tokens_from_string, encoder


@manager.route('/set', methods=['POST'])
@login_required
@validate_request("dialog_id")
def set():
    req = request.json
    conv_id = req.get("conversation_id")
    if conv_id:
        del req["conversation_id"]
        try:
            if not ConversationService.update_by_id(conv_id, req):
                return get_data_error_result(retmsg="Conversation not found!")
            e, conv = ConversationService.get_by_id(conv_id)
            if not e:
                return get_data_error_result(
                    retmsg="Fail to update a conversation!")
            conv = conv.to_dict()
            return get_json_result(data=conv)
        except Exception as e:
            return server_error_response(e)

    try:
        e, dia = DialogService.get_by_id(req["dialog_id"])
        if not e:
            return get_data_error_result(retmsg="Dialog not found")
        conv = {
            "id": get_uuid(),
            "dialog_id": req["dialog_id"],
            "name": "New conversation",
            "message": [{"role": "assistant", "content": dia.prompt_config["prologue"]}]
        }
        ConversationService.save(**conv)
        e, conv = ConversationService.get_by_id(conv["id"])
        if not e:
            return get_data_error_result(retmsg="Fail to new a conversation!")
        conv = conv.to_dict()
        return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route('/get', methods=['GET'])
@login_required
def get():
    conv_id = request.args["conversation_id"]
    try:
        e, conv = ConversationService.get_by_id(conv_id)
        if not e:
            return get_data_error_result(retmsg="Conversation not found!")
        conv = conv.to_dict()
        return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])
@login_required
def rm():
    conv_ids = request.json["conversation_ids"]
    try:
        for cid in conv_ids:
            ConversationService.delete_by_id(cid)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)

@manager.route('/list', methods=['GET'])
@login_required
def list():
    dialog_id = request.args["dialog_id"]
    try:
        convs = ConversationService.query(dialog_id=dialog_id)
        convs = [d.to_dict() for d in convs]
        return get_json_result(data=convs)
    except Exception as e:
        return server_error_response(e)


def message_fit_in(msg, max_length=4000):
    def count():
        nonlocal msg
        tks_cnts = []
        for m in msg:tks_cnts.append({"role": m["role"], "count": num_tokens_from_string(m["content"])})
        total = 0
        for m in tks_cnts: total += m["count"]
        return total

    c = count()
    if c < max_length: return c, msg
    msg = [m for m in msg if m.role in ["system", "user"]]
    c = count()
    if c < max_length:return c, msg
    msg_ = [m for m in msg[:-1] if m.role == "system"]
    msg_.append(msg[-1])
    msg = msg_
    c = count()
    if c < max_length:return c, msg
    ll = num_tokens_from_string(msg_[0].content)
    l = num_tokens_from_string(msg_[-1].content)
    if ll/(ll + l) > 0.8:
        m = msg_[0].content
        m = encoder.decode(encoder.encode(m)[:max_length-l])
        msg[0].content = m
        return max_length, msg

    m = msg_[1].content
    m = encoder.decode(encoder.encode(m)[:max_length-l])
    msg[1].content = m
    return max_length, msg


def chat(dialog, messages, **kwargs):
    assert messages[-1]["role"] == "user", "The last content of this conversation is not from user."
    llm = LLMService.query(llm_name=dialog.llm_id)
    if not llm:
        raise LookupError("LLM(%s) not found"%dialog.llm_id)
    llm = llm[0]
    prompt_config = dialog.prompt_config
    for p in prompt_config["parameters"]:
        if p["key"] == "knowledge":continue
        if p["key"] not in kwargs and not p["optional"]:raise KeyError("Miss parameter: " + p["key"])
        if p["key"] not in kwargs:
            prompt_config["system"] = prompt_config["system"].replace("{%s}"%p["key"], " ")

    model_config = TenantLLMService.get_api_key(dialog.tenant_id, LLMType.CHAT.value, dialog.llm_id)
    if not model_config: raise LookupError("LLM(%s) API key not found"%dialog.llm_id)

    question = messages[-1]["content"]
    embd_mdl = TenantLLMService.model_instance(
        dialog.tenant_id, LLMType.EMBEDDING.value)
    kbinfos = retrievaler.retrieval(question, embd_mdl, dialog.tenant_id, dialog.kb_ids, 1, dialog.top_n, dialog.similarity_threshold,
                        dialog.vector_similarity_weight, top=1024, aggs=False)
    knowledges = [ck["content_ltks"] for ck in kbinfos["chunks"]]

    if not knowledges and prompt_config["empty_response"]:
        return {"answer": prompt_config["empty_response"], "retrieval": kbinfos}

    kwargs["knowledge"] = "\n".join(knowledges)
    gen_conf = dialog.llm_setting[dialog.llm_setting_type]
    msg = [{"role": m["role"], "content": m["content"]} for m in messages if m["role"] != "system"]
    used_token_count = message_fit_in(msg, int(llm.max_tokens * 0.97))
    if "max_tokens" in gen_conf:
        gen_conf["max_tokens"] = min(gen_conf["max_tokens"], llm.max_tokens - used_token_count)
    mdl = ChatModel[model_config.llm_factory](model_config["api_key"], dialog.llm_id)
    answer = mdl.chat(prompt_config["system"].format(**kwargs), msg, gen_conf)

    answer = retrievaler.insert_citations(answer,
                                 [ck["content_ltks"] for ck in kbinfos["chunks"]],
                                 [ck["vector"] for ck in kbinfos["chunks"]],
                                 embd_mdl,
                                 tkweight=1-dialog.vector_similarity_weight,
                                 vtweight=dialog.vector_similarity_weight)
    return {"answer": answer, "retrieval": kbinfos}


@manager.route('/completion', methods=['POST'])
@login_required
@validate_request("dialog_id", "messages")
def completion():
    req = request.json
    msg = []
    for m in req["messages"]:
        if m["role"] == "system":continue
        if m["role"] == "assistant" and not msg:continue
        msg.append({"role": m["role"], "content": m["content"]})
    try:
        e, dia = DialogService.get_by_id(req["dialog_id"])
        if not e:
            return get_data_error_result(retmsg="Dialog not found!")
        del req["dialog_id"]
        del req["messages"]
        return get_json_result(data=chat(dia, msg, **req))
    except Exception as e:
        return server_error_response(e)
