#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

import json
import logging
import os
import re
import tempfile
from copy import deepcopy
from types import SimpleNamespace

from quart import Response, request

from api.apps import current_user, login_required
from api.db.joint_services.tenant_model_service import (
    get_model_config_by_type_and_name,
    get_tenant_default_model_by_type,
)
from api.db.services.chunk_feedback_service import ChunkFeedbackService
from api.db.services.conversation_service import ConversationService, structure_answer
from api.db.services.dialog_service import DialogService, async_chat, gen_mindmap
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.services.search_service import SearchService
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import (
    check_duplicate_ids,
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
    validate_request,
)
from api.utils.tenant_utils import ensure_tenant_model_id_for_params
from common.constants import LLMType, RetCode, StatusEnum
from common.misc_utils import get_uuid
from rag.prompts.generator import chunks_format
from rag.prompts.template import load_prompt

_DEFAULT_PROMPT_CONFIG = {
    "system": (
        'You are an intelligent assistant. Please summarize the content of the dataset to answer the question. '
        'Please list the data in the dataset and answer in detail. When all dataset content is irrelevant to the '
        'question, your answer must include the sentence "The answer you are looking for is not found in the dataset!" '
        "Answers need to consider chat history.\n"
        "      Here is the knowledge base:\n"
        "      {knowledge}\n"
        "      The above is the knowledge base."
    ),
    "prologue": "Hi! I'm your assistant. What can I do for you?",
    "parameters": [{"key": "knowledge", "optional": False}],
    "empty_response": "Sorry! No relevant content was found in the knowledge base!",
    "quote": True,
    "tts": False,
    "refine_multiturn": True,
}
_DEFAULT_DIRECT_CHAT_PROMPT_CONFIG = {
    "system": "",
    "prologue": "",
    "parameters": [],
    "empty_response": "",
    "quote": False,
    "tts": False,
    "refine_multiturn": True,
}
_DEFAULT_RERANK_MODELS = {"BAAI/bge-reranker-v2-m3", "maidalun1020/bce-reranker-base_v1"}
_READONLY_FIELDS = {"id", "tenant_id", "created_by", "create_time", "create_date", "update_time", "update_date"}
_PERSISTED_FIELDS = set(DialogService.model._meta.fields)


def _build_chat_response(chat):
    data = chat.to_dict() if hasattr(chat, "to_dict") else dict(chat)
    kb_ids, kb_names = _resolve_kb_names(data.get("kb_ids", []))
    data["dataset_ids"] = kb_ids
    data.pop("kb_ids", None)
    data["kb_names"] = kb_names
    return data


def _resolve_kb_names(kb_ids):
    ids, names = [], []
    for kb_id in kb_ids or []:
        ok, kb = KnowledgebaseService.get_by_id(kb_id)
        if not ok or kb.status != StatusEnum.VALID.value:
            continue
        ids.append(kb_id)
        names.append(kb.name)
    return ids, names


def _has_knowledge_placeholder(prompt_config):
    return "{knowledge}" in (prompt_config or {}).get("system", "")


def _validate_name(name, *, required=True):
    if name is None:
        if required:
            return None, "`name` is required."
        return None, None
    if not isinstance(name, str):
        return None, "Chat name must be a string."
    name = name.strip()
    if not name:
        return None, "`name` is required." if required else "`name` cannot be empty."
    if len(name.encode("utf-8")) > 255:
        return None, f"Chat name length is {len(name.encode('utf-8'))} which is larger than 255."
    return name, None


def _build_session_response(conv: dict) -> dict:
    conv = dict(conv)
    conv["chat_id"] = conv.pop("dialog_id", conv.get("chat_id"))
    conv["messages"] = conv.pop("message", conv.get("messages", []))
    return conv


def _ensure_owned_chat(chat_id):
    return DialogService.query(
        tenant_id=current_user.id, id=chat_id, status=StatusEnum.VALID.value
    )


def _build_default_completion_dialog():
    return SimpleNamespace(
        tenant_id=current_user.id,
        llm_id="",
        tenant_llm_id=None,
        llm_setting={},
        prompt_config=deepcopy(_DEFAULT_DIRECT_CHAT_PROMPT_CONFIG),
        kb_ids=[],
        top_n=6,
        top_k=1024,
        rerank_id="",
        similarity_threshold=0.1,
        vector_similarity_weight=0.3,
        meta_data_filter=None,
    )


def _create_session_for_completion(chat_id, dialog, user_id):
    conv = {
        "id": get_uuid(),
        "dialog_id": chat_id,
        "name": "New session",
        "message": [{"role": "assistant", "content": dialog.prompt_config.get("prologue", "")}],
        "user_id": user_id,
        "reference": [],
    }
    ConversationService.save(**conv)
    ok, conv_obj = ConversationService.get_by_id(conv["id"])
    if not ok:
        raise LookupError("Fail to create a session!")
    return conv_obj


def _validate_llm_id(llm_id, tenant_id, llm_setting=None):
    if not llm_id:
        return None

    llm_name, llm_factory = TenantLLMService.split_model_name_and_factory(llm_id)
    model_type = (llm_setting or {}).get("model_type")
    candidate_model_types = [model_type] if model_type in {"chat", "image2text"} else ["chat", "image2text"]

    for current_model_type in candidate_model_types:
        if TenantLLMService.query(
            tenant_id=tenant_id,
            llm_name=llm_name,
            llm_factory=llm_factory,
            model_type=current_model_type,
        ):
            return None

    return f"`llm_id` {llm_id} doesn't exist"


def _validate_rerank_id(rerank_id, tenant_id):
    if not rerank_id:
        return None
    llm_name, llm_factory = TenantLLMService.split_model_name_and_factory(rerank_id)
    if llm_name in _DEFAULT_RERANK_MODELS:
        return None
    if TenantLLMService.query(
        tenant_id=tenant_id,
        llm_name=llm_name,
        llm_factory=llm_factory,
        model_type="rerank",
    ):
        return None
    return f"`rerank_id` {rerank_id} doesn't exist"


# def _validate_prompt_config(prompt_config):
#     for parameter in prompt_config.get("parameters", []):
#         if parameter.get("optional"):
#             continue
#         if prompt_config.get("system", "").find("{%s}" % parameter["key"]) < 0:
#             return f"Parameter '{parameter['key']}' is not used"
#     return None


def _validate_dataset_ids(dataset_ids, tenant_id):
    if dataset_ids is None:
        return []
    if not isinstance(dataset_ids, list):
        return "`dataset_ids` should be a list."

    normalized_ids = [dataset_id for dataset_id in dataset_ids if dataset_id]
    kbs = []
    for dataset_id in normalized_ids:
        if not KnowledgebaseService.accessible(kb_id=dataset_id, user_id=tenant_id):
            return f"You don't own the dataset {dataset_id}"
        matches = KnowledgebaseService.query(id=dataset_id)
        if not matches:
            return f"You don't own the dataset {dataset_id}"
        kb = matches[0]
        if kb.chunk_num == 0:
            return f"The dataset {dataset_id} doesn't own parsed file"
        kbs.append(kb)

    embd_ids = [TenantLLMService.split_model_name_and_factory(kb.embd_id)[0] for kb in kbs]
    if len(set(embd_ids)) > 1:
        return f'Datasets use different embedding models: {[kb.embd_id for kb in kbs]}'

    return normalized_ids


def _apply_prompt_defaults(req):
    prompt_config = req.setdefault("prompt_config", {})
    for key, value in _DEFAULT_PROMPT_CONFIG.items():
        temp = prompt_config.get(key)
        if (key == "system" and not temp) or key not in prompt_config:
            prompt_config[key] = deepcopy(value)

    if req.get("kb_ids") and not prompt_config.get("parameters") and "{knowledge}" in prompt_config.get("system", ""):
        prompt_config["parameters"] = [{"key": "knowledge", "optional": False}]


@manager.route("/chats", methods=["POST"])  # noqa: F821
@login_required
async def create():
    try:
        req = await get_request_json()
        ok, tenant = TenantService.get_by_id(current_user.id)
        if not ok:
            return get_data_error_result(message="Tenant not found!")

        # Validate tenant_id should not be provided
        if req.get("tenant_id"):
            return get_data_error_result(message="`tenant_id` must not be provided.")

        # Validate name
        name, err = _validate_name(req.get("name"), required=True)
        if err:
            return get_data_error_result(message=err)
        req["name"] = name

        if "dataset_ids" in req:
            kb_ids = _validate_dataset_ids(req.get("dataset_ids"), current_user.id)
            if isinstance(kb_ids, str):
                return get_data_error_result(message=kb_ids)
            req["kb_ids"] = kb_ids
            req.pop("dataset_ids", None)

        if "llm_id" in req:
            err = _validate_llm_id(req.get("llm_id"), current_user.id, req.get("llm_setting"))
            if err:
                return get_data_error_result(message=err)

        if "rerank_id" in req:
            err = _validate_rerank_id(req.get("rerank_id"), current_user.id)
            if err:
                return get_data_error_result(message=err)

        if "prompt_config" in req:
            if not isinstance(req["prompt_config"], dict):
                return get_data_error_result(message="`prompt_config` should be an object.")
            # err = _validate_prompt_config(req["prompt_config"])
            # if err:
            #     return get_data_error_result(message=err)

        req.setdefault("kb_ids", [])
        req.setdefault("llm_id", tenant.llm_id)
        if req["llm_id"] is None:
            req["llm_id"] = tenant.llm_id
        req.setdefault("llm_setting", {})
        req.setdefault("description", "A helpful Assistant")
        req.setdefault("top_n", 6)
        req.setdefault("top_k", 1024)
        req.setdefault("rerank_id", "")
        req.setdefault("similarity_threshold", 0.1)
        req.setdefault("vector_similarity_weight", 0.3)
        req.setdefault("icon", "")
        _apply_prompt_defaults(req)
        # err = _validate_prompt_config(req["prompt_config"])
        # if err:
        #     return get_data_error_result(message=err)

        req = ensure_tenant_model_id_for_params(current_user.id, req)
        req = {field: value for field, value in req.items() if field in _PERSISTED_FIELDS}
        for field in _READONLY_FIELDS:
            req.pop(field, None)

        if DialogService.query(
            name=req["name"],
            tenant_id=current_user.id,
            status=StatusEnum.VALID.value,
        ):
            return get_data_error_result(message="Duplicated chat name in creating chat.")

        req["id"] = get_uuid()
        req["tenant_id"] = current_user.id
        if not DialogService.save(**req):
            return get_data_error_result(message="Failed to create chat.")

        ok, chat = DialogService.get_by_id(req["id"])
        if not ok:
            return get_data_error_result(message="Failed to retrieve created chat.")
        return get_json_result(data=_build_chat_response(chat))
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats", methods=["GET"])  # noqa: F821
@login_required
def list_chats():
    chat_id = request.args.get("id")
    name = request.args.get("name")
    keywords = request.args.get("keywords", "")
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", "true").lower() != "false"
    owner_ids = request.args.getlist("owner_ids")
    exact_filters = {"id": chat_id, "name": name}
    if chat_id or name:
        keywords = ""

    try:
        page_number = int(request.args.get("page", 0))
        items_per_page = int(request.args.get("page_size", 0))

        if owner_ids:
            chats, total = DialogService.get_by_tenant_ids(
                owner_ids, current_user.id, 0, 0, orderby, desc, keywords, **exact_filters
            )
            chats = [chat for chat in chats if chat["tenant_id"] in owner_ids]
            total = len(chats)
            if page_number and items_per_page:
                start = (page_number - 1) * items_per_page
                chats = chats[start : start + items_per_page]
        else:
            chats, total = DialogService.get_by_tenant_ids(
                [], current_user.id, page_number, items_per_page, orderby, desc, keywords, **exact_filters
            )

        return get_json_result(
            data={"chats": [_build_chat_response(chat) for chat in chats], "total": total}
        )
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>", methods=["GET"])  # noqa: F821
@login_required
def get_chat(chat_id):
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        for tenant in tenants:
            if DialogService.query(
                tenant_id=tenant.tenant_id, id=chat_id, status=StatusEnum.VALID.value
            ):
                break
        else:
            return get_json_result(
                data=False,
                message="No authorization.",
                code=RetCode.AUTHENTICATION_ERROR,
            )

        ok, chat = DialogService.get_by_id(chat_id)
        if not ok:
            return get_data_error_result(message="Chat not found!")
        return get_json_result(data=_build_chat_response(chat))
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>", methods=["PUT"])  # noqa: F821
@login_required
async def update_chat(chat_id):
    if not _ensure_owned_chat(chat_id):
        return get_json_result(
            data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR
        )

    try:
        req = await get_request_json()
        ok, tenant = TenantService.get_by_id(current_user.id)
        if not ok:
            return get_data_error_result(message="Tenant not found!")

        ok, current_chat = DialogService.get_by_id(chat_id)
        if not ok:
            return get_data_error_result(message="Chat not found!")
        current_chat = current_chat.to_dict()

        if req.get("tenant_id"):
            return get_data_error_result(message="`tenant_id` must not be provided.")

        if "name" in req:
            name, err = _validate_name(req.get("name"), required=True)
            if err:
                return get_data_error_result(message=err)
            req["name"] = name

        if "dataset_ids" in req:
            kb_ids = _validate_dataset_ids(req.get("dataset_ids"), current_user.id)
            if isinstance(kb_ids, str):
                return get_data_error_result(message=kb_ids)
            req["kb_ids"] = kb_ids
            req.pop("dataset_ids", None)

        if "llm_id" in req:
            err = _validate_llm_id(req.get("llm_id"), current_user.id, req.get("llm_setting"))
            if err:
                return get_data_error_result(message=err)

        if "rerank_id" in req:
            err = _validate_rerank_id(req.get("rerank_id"), current_user.id)
            if err:
                return get_data_error_result(message=err)

        if "prompt_config" in req:
            if not isinstance(req["prompt_config"], dict):
                return get_data_error_result(message="`prompt_config` should be an object.")
            # err = _validate_prompt_config(req["prompt_config"])
            # if err:
            #     return get_data_error_result(message=err)

        # prompt_config = req.get("prompt_config", {})
        # if not prompt_config:
        #     prompt_config = current_chat.get("prompt_config", {})
        # kb_ids = req.get("kb_ids", current_chat.get("kb_ids", []))
        # if not kb_ids and not prompt_config.get("tavily_api_key") and _has_knowledge_placeholder(prompt_config):
        #     return get_data_error_result(message="Please remove `{knowledge}` in system prompt since no dataset / Tavily used here.")

        req = ensure_tenant_model_id_for_params(current_user.id, req)
        req = {field: value for field, value in req.items() if field in _PERSISTED_FIELDS}
        for field in _READONLY_FIELDS:
            req.pop(field, None)

        if (
            "name" in req
            and req["name"].lower() != current_chat["name"].lower()
            and DialogService.query(
                name=req["name"],
                tenant_id=current_user.id,
                status=StatusEnum.VALID.value,
            )
        ):
            return get_data_error_result(message="Duplicated chat name.")

        if not DialogService.update_by_id(chat_id, req):
            return get_data_error_result(message="Chat not found!")

        ok, chat = DialogService.get_by_id(chat_id)
        if not ok:
            return get_data_error_result(message="Failed to retrieve updated chat.")
        return get_json_result(data=_build_chat_response(chat))
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>", methods=["PATCH"])  # noqa: F821
@login_required
async def patch_chat(chat_id):
    if not _ensure_owned_chat(chat_id):
        return get_json_result(
            data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR
        )

    try:
        req = await get_request_json()
        ok, tenant = TenantService.get_by_id(current_user.id)
        if not ok:
            return get_data_error_result(message="Tenant not found!")

        ok, current_chat = DialogService.get_by_id(chat_id)
        if not ok:
            return get_data_error_result(message="Chat not found!")
        current_chat = current_chat.to_dict()

        if "name" in req:
            name, err = _validate_name(req.get("name"), required=False)
            if err:
                return get_data_error_result(message=err)
            if name is not None:
                req["name"] = name

        if "dataset_ids" in req:
            kb_ids = _validate_dataset_ids(req.get("dataset_ids"), current_user.id)
            if isinstance(kb_ids, str):
                return get_data_error_result(message=kb_ids)
            req["kb_ids"] = kb_ids
            req.pop("dataset_ids", None)

        if "llm_id" in req:
            err = _validate_llm_id(req.get("llm_id"), current_user.id, req.get("llm_setting"))
            if err:
                return get_data_error_result(message=err)

        if "rerank_id" in req:
            err = _validate_rerank_id(req.get("rerank_id"), current_user.id)
            if err:
                return get_data_error_result(message=err)

        if "prompt_config" in req:
            if not isinstance(req["prompt_config"], dict):
                return get_data_error_result(message="`prompt_config` should be an object.")
            prompt_config = deepcopy(current_chat.get("prompt_config", {}))
            prompt_config.update(req["prompt_config"])
            req["prompt_config"] = prompt_config
            # err = _validate_prompt_config(prompt_config)
            # if err:
            #     return get_data_error_result(message=err)

        if "llm_setting" in req:
            llm_setting = deepcopy(current_chat.get("llm_setting", {}))
            llm_setting.update(req["llm_setting"])
            req["llm_setting"] = llm_setting

        # if "prompt_config" in req or "kb_ids" in req:
        #     prompt_config = req.get("prompt_config", current_chat.get("prompt_config", {}))
        #     kb_ids = req.get("kb_ids", current_chat.get("kb_ids", []))
        #     if not kb_ids and not prompt_config.get("tavily_api_key") and _has_knowledge_placeholder(prompt_config):
        #         return get_data_error_result(message="Please remove `{knowledge}` in system prompt since no dataset / Tavily used here.")

        req = ensure_tenant_model_id_for_params(current_user.id, req)
        req = {field: value for field, value in req.items() if field in _PERSISTED_FIELDS}
        for field in _READONLY_FIELDS:
            req.pop(field, None)

        if (
            "name" in req
            and req["name"].lower() != current_chat["name"].lower()
            and DialogService.query(
                name=req["name"],
                tenant_id=current_user.id,
                status=StatusEnum.VALID.value,
            )
        ):
            return get_data_error_result(message="Duplicated chat name.")

        if not DialogService.update_by_id(chat_id, req):
            return get_data_error_result(message="Failed to update chat.")

        ok, chat = DialogService.get_by_id(chat_id)
        if not ok:
            return get_data_error_result(message="Failed to retrieve updated chat.")
        return get_json_result(data=_build_chat_response(chat))
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>", methods=["DELETE"])  # noqa: F821
@login_required
def delete_chat(chat_id):
    if not _ensure_owned_chat(chat_id):
        return get_json_result(
            data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR
        )

    try:
        if not DialogService.update_by_id(chat_id, {"status": StatusEnum.INVALID.value}):
            return get_data_error_result(message=f"Failed to delete chat {chat_id}")
        return get_json_result(data=True)
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats", methods=["DELETE"])  # noqa: F821
@login_required
async def bulk_delete_chats():
    req = await get_request_json()
    if not req:
        return get_json_result(data={})

    ids = req.get("ids")
    if not ids:
        if req.get("delete_all") is True:
            ids = [
                chat.id
                for chat in DialogService.query(
                    tenant_id=current_user.id, status=StatusEnum.VALID.value
                )
            ]
            if not ids:
                return get_json_result(data={})
        else:
            return get_json_result(data={})

    errors = []
    success_count = 0
    unique_ids, duplicate_messages = check_duplicate_ids(ids, "chat")

    for chat_id in unique_ids:
        if not _ensure_owned_chat(chat_id):
            errors.append(f"Chat({chat_id}) not found.")
            continue
        success_count += DialogService.update_by_id(chat_id, {"status": StatusEnum.INVALID.value})

    all_errors = errors + duplicate_messages
    if all_errors:
        if success_count > 0:
            return get_json_result(
                data={"success_count": success_count, "errors": all_errors},
                message=f"Partially deleted {success_count} chats with {len(all_errors)} errors",
            )
        return get_data_error_result(message="; ".join(all_errors))

    return get_json_result(data={"success_count": success_count})


@manager.route("/chats/<chat_id>/sessions", methods=["POST"])  # noqa: F821
@login_required
async def create_session(chat_id):
    if not _ensure_owned_chat(chat_id):
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)
    try:
        req = await get_request_json()
        ok, dia = DialogService.get_by_id(chat_id)
        if not ok:
            return get_data_error_result(message="Chat not found!")
        name = req.get("name", "New session")
        if not isinstance(name, str) or not name.strip():
            return get_data_error_result(message="`name` can not be empty.")
        name = name.strip()[:255]
        conv = {
            "id": get_uuid(),
            "dialog_id": chat_id,
            "name": name,
            "message": [{"role": "assistant", "content": dia.prompt_config.get("prologue", "")}],
            "user_id": req.get("user_id", current_user.id),
            "reference": [],
        }
        ConversationService.save(**conv)
        ok, conv_obj = ConversationService.get_by_id(conv["id"])
        if not ok:
            return get_data_error_result(message="Fail to create a session!")
        return get_json_result(data=_build_session_response(conv_obj.to_dict()))
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>/sessions", methods=["GET"])  # noqa: F821
@login_required
def list_sessions(chat_id):
    try:
        if not _ensure_owned_chat(chat_id):
            return get_json_result(
                data=False,
                message="No authorization.",
                code=RetCode.AUTHENTICATION_ERROR,
            )
        page_number = int(request.args.get("page", 1))
        items_per_page = int(request.args.get("page_size", 30))
        orderby = request.args.get("orderby", "create_time")
        desc = request.args.get("desc", "true").lower() != "false"
        session_id = request.args.get("id")
        name = request.args.get("name")
        user_id = request.args.get("user_id")
        convs = ConversationService.get_list(
            chat_id, page_number, items_per_page, orderby, desc, session_id, name, user_id
        )
        if items_per_page == 0:
            convs = []
        return get_json_result(data=[_build_session_response(c) for c in convs])
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>/sessions/<session_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_session(chat_id, session_id):
    if not _ensure_owned_chat(chat_id):
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)
    try:
        ok, conv = ConversationService.get_by_id(session_id)
        if not ok:
            return get_data_error_result(message="Session not found!")
        if conv.dialog_id != chat_id:
            return get_data_error_result(message="Session does not belong to this chat!")
        dialog = _ensure_owned_chat(chat_id)
        avatar = dialog[0].icon if dialog else ""
        for ref in conv.reference:
            if isinstance(ref, list):
                continue
            ref["chunks"] = chunks_format(ref)
        result = _build_session_response(conv.to_dict())
        result["avatar"] = avatar
        return get_json_result(data=result)
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>/sessions/<session_id>", methods=["PATCH"])  # noqa: F821
@login_required
async def update_session(chat_id, session_id):
    if not _ensure_owned_chat(chat_id):
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)
    try:
        req = await get_request_json()
        if not ConversationService.query(id=session_id, dialog_id=chat_id):
            return get_data_error_result(message="Session not found!")
        if "message" in req or "messages" in req:
            return get_data_error_result(message="`messages` cannot be changed.")
        if "reference" in req:
            return get_data_error_result(message="`reference` cannot be changed.")
        name = req.get("name")
        if name is not None:
            if not isinstance(name, str) or not name.strip():
                return get_data_error_result(message="`name` can not be empty.")
            req["name"] = name.strip()[:255]
        update_fields = {k: v for k, v in req.items() if k not in {"id", "dialog_id", "chat_id", "user_id"}}
        if not ConversationService.update_by_id(session_id, update_fields):
            return get_data_error_result(message="Session not found!")
        ok, conv = ConversationService.get_by_id(session_id)
        if not ok:
            return get_data_error_result(message="Fail to update a session!")
        return get_json_result(data=_build_session_response(conv.to_dict()))
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>/sessions", methods=["DELETE"])  # noqa: F821
@login_required
async def delete_sessions(chat_id):
    if not _ensure_owned_chat(chat_id):
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)
    try:
        req = await get_request_json()
        if not req:
            return get_json_result(data={})

        session_ids = req.get("ids")
        if not session_ids:
            if req.get("delete_all") is True:
                session_ids = [conv.id for conv in ConversationService.query(dialog_id=chat_id)]
                if not session_ids:
                    return get_json_result(data={})
            else:
                return get_json_result(data={})
        unique_ids, duplicate_messages = check_duplicate_ids(session_ids, "session")
        errors = []
        success_count = 0
        for sid in unique_ids:
            if not ConversationService.query(id=sid, dialog_id=chat_id):
                errors.append(f"The chat doesn't own the session {sid}")
                continue
            ConversationService.delete_by_id(sid)
            success_count += 1
        all_errors = errors + duplicate_messages
        if all_errors:
            if success_count > 0:
                return get_json_result(
                    data={"success_count": success_count, "errors": all_errors},
                    message=f"Partially deleted {success_count} sessions with {len(all_errors)} errors",
                )
            return get_data_error_result(message="; ".join(all_errors))
        return get_json_result(data=True)
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>/sessions/<session_id>/messages/<msg_id>", methods=["DELETE"])  # noqa: F821
@login_required
async def delete_session_message(chat_id, session_id, msg_id):
    if not _ensure_owned_chat(chat_id):
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)
    try:
        ok, conv = ConversationService.get_by_id(session_id)
        if not ok or conv.dialog_id != chat_id:
            return get_data_error_result(message="Session not found!")
        conv = conv.to_dict()
        for i, msg in enumerate(conv["message"]):
            if msg_id != msg.get("id", ""):
                continue
            assert conv["message"][i + 1]["id"] == msg_id
            conv["message"].pop(i)
            conv["message"].pop(i)
            conv["reference"].pop(max(0, i // 2 - 1))
            break
        ConversationService.update_by_id(conv["id"], conv)
        return get_json_result(data=_build_session_response(conv))
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chats/<chat_id>/sessions/<session_id>/messages/<msg_id>/feedback", methods=["PUT"])  # noqa: F821
@login_required
async def update_message_feedback(chat_id, session_id, msg_id):
    owned = _ensure_owned_chat(chat_id)
    if not owned:
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)
    try:
        req = await get_request_json()
        ok, conv = ConversationService.get_by_id(session_id)
        if not ok or conv.dialog_id != chat_id:
            return get_data_error_result(message="Session not found!")
        thumb_raw = req.get("thumbup")
        if not isinstance(thumb_raw, bool):
            return get_data_error_result(message="thumbup must be a boolean")
        feedback = req.get("feedback", "")
        conv_dict = conv.to_dict()
        message_index = None
        apply_chunk_feedback = False
        prior_thumb = None
        for i, msg in enumerate(conv_dict["message"]):
            if msg_id == msg.get("id", "") and msg.get("role", "") == "assistant":
                prior_thumb = msg.get("thumbup")
                if thumb_raw is True:
                    msg["thumbup"] = True
                    msg.pop("feedback", None)
                    apply_chunk_feedback = prior_thumb is not True
                else:
                    msg["thumbup"] = False
                    if feedback:
                        msg["feedback"] = feedback
                    apply_chunk_feedback = prior_thumb is not False
                message_index = i
                break

        if message_index is not None and apply_chunk_feedback:
            try:
                ref_index = (message_index - 1) // 2
                if 0 <= ref_index < len(conv_dict.get("reference", [])):
                    reference = conv_dict["reference"][ref_index]
                    if reference:
                        if isinstance(prior_thumb, bool) and prior_thumb != thumb_raw:
                            ChunkFeedbackService.apply_feedback(
                                tenant_id=current_user.id,
                                reference=reference,
                                is_positive=not prior_thumb,
                            )
                        feedback_result = ChunkFeedbackService.apply_feedback(
                            tenant_id=current_user.id,
                            reference=reference,
                            is_positive=thumb_raw is True,
                        )
                        logging.debug(
                            "Chunk feedback applied: %s succeeded, %s failed",
                            feedback_result["success_count"],
                            feedback_result["fail_count"],
                        )
            except Exception as e:
                logging.warning("Failed to apply chunk feedback: %s", e)

        ConversationService.update_by_id(conv_dict["id"], conv_dict)
        return get_json_result(data=_build_session_response(conv_dict))
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/chat/audio/speech", methods=["POST"])  # noqa: F821
@login_required
async def tts():
    req = await get_request_json()
    text = req["text"]

    try:
        default_tts_model_config = get_tenant_default_model_by_type(current_user.id, LLMType.TTS)
    except Exception as e:
        return get_data_error_result(message=str(e))

    tts_mdl = LLMBundle(current_user.id, default_tts_model_config)

    def stream_audio():
        try:
            for txt in re.split(r"[，。/《》？；：！\n\r:;]+", text):
                for chunk in tts_mdl.tts(txt):
                    yield chunk
        except Exception as e:
            yield ("data:" + json.dumps({"code": 500, "message": str(e), "data": {"answer": "**ERROR**: " + str(e)}}, ensure_ascii=False)).encode("utf-8")

    resp = Response(stream_audio(), mimetype="audio/mpeg")
    resp.headers.add_header("Cache-Control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")
    return resp


@manager.route("/chat/audio/transcription", methods=["POST"])  # noqa: F821
@login_required
async def transcription():
    req = await request.form
    stream_mode = req.get("stream", "false").lower() == "true"
    files = await request.files
    if "file" not in files:
        return get_data_error_result(message="Missing 'file' in multipart form-data")

    uploaded = files["file"]

    ALLOWED_EXTS = {
        ".wav", ".mp3", ".m4a", ".aac",
        ".flac", ".ogg", ".webm",
        ".opus", ".wma",
    }

    filename = uploaded.filename or ""
    suffix = os.path.splitext(filename)[-1].lower()
    if suffix not in ALLOWED_EXTS:
        return get_data_error_result(
            message=f"Unsupported audio format: {suffix}. Allowed: {', '.join(sorted(ALLOWED_EXTS))}"
        )

    fd, temp_audio_path = tempfile.mkstemp(suffix=suffix)
    os.close(fd)
    await uploaded.save(temp_audio_path)

    try:
        default_asr_model_config = get_tenant_default_model_by_type(current_user.id, LLMType.SPEECH2TEXT)
    except Exception as e:
        return get_data_error_result(message=str(e))

    asr_mdl = LLMBundle(current_user.id, default_asr_model_config)
    if not stream_mode:
        text = asr_mdl.transcription(temp_audio_path)
        try:
            os.remove(temp_audio_path)
        except Exception as e:
            logging.error(f"Failed to remove temp audio file: {str(e)}")
        return get_json_result(data={"text": text})

    async def event_stream():
        try:
            for evt in asr_mdl.stream_transcription(temp_audio_path):
                yield f"data: {json.dumps(evt, ensure_ascii=False)}\n\n"
        except Exception as e:
            err = {"event": "error", "text": str(e)}
            yield f"data: {json.dumps(err, ensure_ascii=False)}\n\n"
        finally:
            try:
                os.remove(temp_audio_path)
            except Exception as e:
                logging.error(f"Failed to remove temp audio file: {str(e)}")

    return Response(event_stream(), content_type="text/event-stream")


@manager.route("/chat/mindmap", methods=["POST"])  # noqa: F821
@login_required
@validate_request("question", "kb_ids")
async def mindmap():
    req = await get_request_json()
    search_id = req.get("search_id", "")
    search_app = SearchService.get_detail(search_id) if search_id else {}
    search_config = search_app.get("search_config", {}) if search_app else {}
    kb_ids = search_config.get("kb_ids", [])
    kb_ids.extend(req["kb_ids"])
    kb_ids = list(set(kb_ids))

    mind_map = await gen_mindmap(req["question"], kb_ids, search_app.get("tenant_id", current_user.id), search_config)
    if "error" in mind_map:
        return server_error_response(Exception(mind_map["error"]))
    return get_json_result(data=mind_map)


@manager.route("/chat/recommendation", methods=["POST"])  # noqa: F821
@login_required
@validate_request("question")
async def recommendation():
    req = await get_request_json()

    search_id = req.get("search_id", "")
    search_config = {}
    if search_id:
        if search_app := SearchService.get_detail(search_id):
            search_config = search_app.get("search_config", {})

    question = req["question"]

    chat_id = search_config.get("chat_id", "")
    if chat_id:
        chat_model_config = get_model_config_by_type_and_name(current_user.id, LLMType.CHAT, chat_id)
    else:
        chat_model_config = get_tenant_default_model_by_type(current_user.id, LLMType.CHAT)
    chat_mdl = LLMBundle(current_user.id, chat_model_config)

    gen_conf = search_config.get("llm_setting", {"temperature": 0.9})
    if "parameter" in gen_conf:
        del gen_conf["parameter"]
    prompt = load_prompt("related_question")
    ans = await chat_mdl.async_chat(
        prompt,
        [
            {
                "role": "user",
                "content": f"\nKeywords: {question}\nRelated search terms:\n    ",
            }
        ],
        gen_conf,
    )
    return get_json_result(data=[re.sub(r"^[0-9]\. ", "", a) for a in ans.split("\n") if re.match(r"^[0-9]\. ", a)])


@manager.route("/chat/completions", methods=["POST"])  # noqa: F821
@login_required
@validate_request("messages")
async def session_completion():
    req = await get_request_json()
    msg = []
    for m in req["messages"]:
        if m["role"] == "system":
            continue
        if m["role"] == "assistant" and not msg:
            continue
        msg.append(m)
    message_id = msg[-1].get("id") if msg else None
    chat_id = req.pop("chat_id", "") or ""
    session_id = req.pop("session_id", "") or ""
    chat_model_id = req.pop("llm_id", "")

    chat_model_config = {}
    for model_config in ["temperature", "top_p", "frequency_penalty", "presence_penalty", "max_tokens"]:
        config = req.get(model_config)
        if config:
            chat_model_config[model_config] = config

    try:
        conv = None
        if session_id and not chat_id:
            return get_data_error_result(message="`chat_id` is required when `session_id` is provided.")

        if chat_id:
            if not _ensure_owned_chat(chat_id):
                return get_json_result(
                    data=False,
                    message="No authorization.",
                    code=RetCode.AUTHENTICATION_ERROR,
                )
            e, dia = DialogService.get_by_id(chat_id)
            if not e:
                return get_data_error_result(message="Chat not found!")
            if session_id:
                e, conv = ConversationService.get_by_id(session_id)
                if not e:
                    return get_data_error_result(message="Session not found!")
                if conv.dialog_id != chat_id:
                    return get_data_error_result(message="Session does not belong to this chat!")
            else:
                conv = _create_session_for_completion(chat_id, dia, req.get("user_id", current_user.id))
                session_id = conv.id
            conv.message = deepcopy(req["messages"])
        else:
            dia = _build_default_completion_dialog()
            dia.llm_setting = chat_model_config

        del req["messages"]

        if conv is not None:
            if not conv.reference:
                conv.reference = []
            conv.reference = [r for r in conv.reference if r]
            conv.reference.append({"chunks": [], "doc_aggs": []})

        if chat_model_id:
            if not TenantLLMService.get_api_key(tenant_id=dia.tenant_id, model_name=chat_model_id):
                return get_data_error_result(message=f"Cannot use specified model {chat_model_id}.")
            dia.llm_id = chat_model_id
            dia.llm_setting = chat_model_config

        stream_mode = req.pop("stream", True)

        def _format_answer(ans):
            formatted = structure_answer(conv, ans, message_id, session_id)
            if chat_id:
                formatted["chat_id"] = chat_id
            return formatted

        async def stream():
            nonlocal dia, msg, req, conv
            try:
                async for ans in async_chat(dia, msg, True, **req):
                    ans = _format_answer(ans)
                    yield "data:" + json.dumps({"code": 0, "message": "", "data": ans}, ensure_ascii=False) + "\n\n"
                if conv is not None:
                    ConversationService.update_by_id(conv.id, conv.to_dict())
            except Exception as ex:
                logging.exception(ex)
                yield "data:" + json.dumps({"code": 500, "message": str(ex), "data": {"answer": "**ERROR**: " + str(ex), "reference": []}}, ensure_ascii=False) + "\n\n"
            yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

        if stream_mode:
            resp = Response(stream(), mimetype="text/event-stream")
            resp.headers.add_header("Cache-control", "no-cache")
            resp.headers.add_header("Connection", "keep-alive")
            resp.headers.add_header("X-Accel-Buffering", "no")
            resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
            return resp

        answer = None
        async for ans in async_chat(dia, msg, **req):
            answer = _format_answer(ans)
            if conv is not None:
                ConversationService.update_by_id(conv.id, conv.to_dict())
            break
        return get_json_result(data=answer)
    except Exception as ex:
        return server_error_response(ex)
