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
"""Shared request validators for REST API handlers.

All validators follow a consistent contract:
- Return ``None`` on success.
- Return an error message string on failure.

``validate_dataset_ids`` is the exception: it returns the validated list of
IDs on success, or an error message string on failure.
"""

import logging

from api.db.joint_services.tenant_model_service import (
    get_model_config_from_provider_instance,
    split_model_name,
)
from api.db.services.knowledgebase_service import KnowledgebaseService
from common.misc_utils import thread_pool_exec

_DEFAULT_RERANK_MODELS = {"BAAI/bge-reranker-v2-m3", "maidalun1020/bce-reranker-base_v1"}


def validate_name(name, *, required=True):
    """Validate a chat/agent name field.

    Returns ``(cleaned_name, None)`` on success or ``(None, error_message)`` on failure.
    """
    if name is None:
        if required:
            return None, "`name` is required."
        return None, None
    if not isinstance(name, str):
        return None, "Name must be a string."
    name = name.strip()
    if not name:
        return None, "`name` is required." if required else "`name` cannot be empty."
    if len(name.encode("utf-8")) > 255:
        return None, f"Name length is {len(name.encode('utf-8'))} which is larger than 255."
    return name, None


async def validate_llm_id(llm_id, tenant_id, llm_setting=None):
    """Return an error message string if ``llm_id`` is invalid, else ``None``.

    Handles both scalar and list ``model_type`` values inside ``llm_setting``
    (the sync copy in ``openai_api.py`` only handled the scalar case and omitted
    the list branch — this version is the authoritative implementation).
    """
    if not llm_id:
        return None

    conf_model_type = (llm_setting or {}).get("model_type")
    if isinstance(conf_model_type, str):
        model_type = conf_model_type if conf_model_type in {"chat", "image2text"} else "chat"
    elif isinstance(conf_model_type, list):
        model_type = "image2text" if "image2text" in conf_model_type else "chat"
    else:
        model_type = "chat"

    try:
        await thread_pool_exec(
            get_model_config_from_provider_instance,
            tenant_id=tenant_id,
            model_name=llm_id,
            model_type=model_type,
        )
    except Exception as e:
        logging.error(f"Fail to get model config for {llm_id}: {e}")
        return f"`llm_id` {llm_id} doesn't exist"

    return None


async def validate_rerank_id(rerank_id, tenant_id):
    """Return an error message string if ``rerank_id`` is invalid, else ``None``."""
    if not rerank_id:
        return None
    parts = rerank_id.split("@")
    llm_name = parts[0]
    if llm_name in _DEFAULT_RERANK_MODELS:
        return None
    try:
        await thread_pool_exec(
            get_model_config_from_provider_instance,
            tenant_id=tenant_id,
            model_name=rerank_id,
            model_type="rerank",
        )
    except Exception as e:
        logging.error(f"Fail to get model config for {rerank_id}: {e}")
        return f"`rerank_id` {rerank_id} doesn't exist"
    return None


async def validate_dataset_ids(dataset_ids, tenant_id):
    """Validate that the caller owns every dataset in ``dataset_ids``.

    Uses ``KnowledgebaseService.accessible`` which internally checks the
    ``UserTenantService`` multi-tenant membership table, closing the
    cross-tenant reference gap that existed in the ``bot_api`` inline check.

    Returns:
        list[str] – validated, non-empty IDs on success.
        str       – error message on failure.
    """
    if dataset_ids is None:
        return []
    if not isinstance(dataset_ids, list):
        return "`dataset_ids` should be a list."

    normalized_ids = [dataset_id for dataset_id in dataset_ids if dataset_id]
    kbs = []
    for dataset_id in normalized_ids:
        if not await thread_pool_exec(KnowledgebaseService.accessible, kb_id=dataset_id, user_id=tenant_id):
            return f"You don't own the dataset {dataset_id}"
        matches = await thread_pool_exec(KnowledgebaseService.query, id=dataset_id)
        if not matches:
            return f"You don't own the dataset {dataset_id}"
        kb = matches[0]
        if kb.chunk_num == 0:
            return f"The dataset {dataset_id} doesn't own parsed file"
        kbs.append(kb)

    embd_ids = [split_model_name(kb.embd_id)[0] for kb in kbs]
    if len(set(embd_ids)) > 1:
        return f"Datasets use different embedding models: {[kb.embd_id for kb in kbs]}"

    return normalized_ids


async def validate_chat_config(req, tenant_id):
    """Run dataset_ids → llm_id → rerank_id validation in canonical order.

    Mutates ``req`` in place: replaces ``dataset_ids`` with ``kb_ids`` when
    present.

    Returns:
        (req, None)          on success.
        (req, error_message) on first validation failure.
    """
    if "dataset_ids" in req:
        kb_ids = await validate_dataset_ids(req.get("dataset_ids"), tenant_id)
        if isinstance(kb_ids, str):
            return req, kb_ids
        req["kb_ids"] = kb_ids
        req.pop("dataset_ids", None)

    if "llm_id" in req:
        err = await validate_llm_id(req.get("llm_id"), tenant_id, req.get("llm_setting"))
        if err:
            return req, err

    if "rerank_id" in req:
        err = await validate_rerank_id(req.get("rerank_id"), tenant_id)
        if err:
            return req, err

    return req, None
