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

from copy import deepcopy

from quart import request

from api.apps import current_user, login_required
from api.db.services.dialog_service import DialogService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import (
    check_duplicate_ids,
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
)
from api.utils.tenant_utils import ensure_tenant_model_id_for_params
from common.constants import RetCode, StatusEnum
from common.misc_utils import get_uuid

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


def _ensure_owned_chat(chat_id):
    return DialogService.query(
        tenant_id=current_user.id, id=chat_id, status=StatusEnum.VALID.value
    )


def _validate_llm_id(llm_id, tenant_id, llm_setting=None):
    if not llm_id:
        return None

    llm_name, llm_factory = TenantLLMService.split_model_name_and_factory(llm_id)
    model_type = (llm_setting or {}).get("model_type")
    if model_type not in {"chat", "image2text"}:
        model_type = "chat"

    if not TenantLLMService.query(
        tenant_id=tenant_id,
        llm_name=llm_name,
        llm_factory=llm_factory,
        model_type=model_type,
    ):
        return f"`llm_id` {llm_id} doesn't exist"
    return None


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

        if req.get("tenant_id"):
            return get_data_error_result(message="`tenant_id` must not be provided.")

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
