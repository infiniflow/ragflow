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
import logging
import os
import enum
from common import settings
from common.constants import LLMType
from api.db.services.llm_service import LLMService
from api.db.services.tenant_llm_service import TenantLLMService, TenantService

logger = logging.getLogger(__name__)


def get_model_config_by_id(
    tenant_model_id: int,
    allowed_tenant_ids: str | list[str] | set[str] | tuple[str, ...] | None = None,
    requester_tenant_id: str | None = None,
) -> dict:
    found, model_config = TenantLLMService.get_by_id(tenant_model_id)
    if not found:
        raise LookupError(f"Tenant Model with id {tenant_model_id} not found")
    if allowed_tenant_ids is not None:
        if isinstance(allowed_tenant_ids, str):
            allowed_tenant_ids = {allowed_tenant_ids}
        else:
            allowed_tenant_ids = {str(tenant_id) for tenant_id in allowed_tenant_ids if tenant_id}
        if str(model_config.tenant_id) not in allowed_tenant_ids:
            logger.warning(
                "Denied tenant model access: tenant_model_id=%s model_tenant_id=%s "
                "allowed_tenant_ids=%s requester_tenant_id=%s",
                tenant_model_id,
                model_config.tenant_id,
                sorted(allowed_tenant_ids),
                requester_tenant_id,
            )
            raise LookupError(f"Tenant Model with id {tenant_model_id} not authorized")
    config_dict = model_config.to_dict()
    api_key, is_tools, api_key_payload = TenantLLMService._decode_api_key_config(config_dict.get("api_key", ""))
    config_dict["api_key"] = api_key
    if api_key_payload is not None:
        config_dict["api_key_payload"] = api_key_payload
    if is_tools is not None:
        config_dict["is_tools"] = is_tools
    llm = LLMService.query(llm_name=config_dict["llm_name"])
    if "is_tools" not in config_dict and llm:
        config_dict["is_tools"] = llm[0].is_tools
    return config_dict


def get_model_config_by_type_and_name(tenant_id: str, model_type: str, model_name: str):
    if not model_name:
        raise Exception("Model Name is required")
    model_type_val = model_type.value if hasattr(model_type, "value") else model_type
    model_config = TenantLLMService.get_api_key(tenant_id, model_name, model_type_val)
    if not model_config:
        # model_name in format 'name@factory', split model_name and try again
        pure_model_name, fid = TenantLLMService.split_model_name_and_factory(model_name)
        compose_profiles = os.getenv("COMPOSE_PROFILES", "")
        is_tei_builtin_embedding = (
            model_type_val == LLMType.EMBEDDING.value
            and "tei-" in compose_profiles
            and pure_model_name == os.getenv("TEI_MODEL", "")
            and (fid == "Builtin" or fid is None)
        )
        if is_tei_builtin_embedding:
            # configured local embedding model
            embedding_cfg = settings.EMBEDDING_CFG
            config_dict = {
                "llm_factory": "Builtin",
                "api_key": embedding_cfg["api_key"],
                "llm_name": pure_model_name,
                "api_base": embedding_cfg["base_url"],
                "model_type": LLMType.EMBEDDING.value,
            }
        elif model_type_val == LLMType.CHAT.value:
            # Retry as CHAT with pure_model_name first; then fall back to a multimodal model registered under IMAGE2TEXT.
            model_config = TenantLLMService.get_api_key(tenant_id, pure_model_name, LLMType.CHAT.value)
            if not model_config:
                model_config = TenantLLMService.get_api_key(tenant_id, pure_model_name, LLMType.IMAGE2TEXT.value)
            if not model_config:
                raise LookupError(f"Tenant Model with name {model_name} and type {model_type_val} not found")
            config_dict = model_config.to_dict()
        elif model_type_val == LLMType.IMAGE2TEXT.value:
            model_config = TenantLLMService.get_api_key(tenant_id, pure_model_name, LLMType.IMAGE2TEXT.value)
            if not model_config:
                # Fall back to a chat model only if it has declared IMAGE2TEXT capability (tag check via llm table)
                chat_config = TenantLLMService.get_api_key(tenant_id, pure_model_name, LLMType.CHAT.value)
                logger.debug("IMAGE2TEXT config not found for %s; chat_config found: %s", pure_model_name, chat_config is not None)
                if chat_config:
                    llm_entry = LLMService.query(fid=chat_config.llm_factory, llm_name=chat_config.llm_name)
                    tags = [t.strip() for t in (llm_entry[0].tags or "").split(",")] if llm_entry else []
                    logger.debug("LLM tags for %s/%s: %s", chat_config.llm_factory, chat_config.llm_name, tags)
                    if "IMAGE2TEXT" in tags:
                        logger.debug("Promoting chat config to IMAGE2TEXT for %s", pure_model_name)
                        model_config = chat_config
            if not model_config:
                raise LookupError(f"Tenant Model with name {model_name} and type {model_type_val} not found")
            config_dict = model_config.to_dict()
            config_dict["model_type"] = LLMType.IMAGE2TEXT.value
        else:
            model_config = TenantLLMService.get_api_key(tenant_id, pure_model_name, model_type_val)
            if not model_config:
                raise LookupError(f"Tenant Model with name {model_name} and type {model_type_val} not found")
            config_dict = model_config.to_dict()
    else:
        # model_name without @factory
        config_dict = model_config.to_dict()
    api_key, is_tools, api_key_payload = TenantLLMService._decode_api_key_config(config_dict.get("api_key", ""))
    config_dict["api_key"] = api_key
    if api_key_payload is not None:
        config_dict["api_key_payload"] = api_key_payload
    if is_tools is not None:
        config_dict["is_tools"] = is_tools
    config_model_type = config_dict.get("model_type")
    config_model_type = config_model_type.value if hasattr(config_model_type, "value") else config_model_type
    if config_model_type != model_type_val and not (
            model_type_val == LLMType.CHAT.value
            and config_model_type == LLMType.IMAGE2TEXT.value
    ) and not (
            model_type_val == LLMType.IMAGE2TEXT.value
            and config_model_type == LLMType.CHAT.value
    ):
        raise LookupError(
            f"Tenant Model with name {model_name} has type {config_model_type}, expected {model_type_val}"
        )
    llm = LLMService.query(llm_name=config_dict["llm_name"])
    if "is_tools" not in config_dict and llm:
        config_dict["is_tools"] = llm[0].is_tools
    return config_dict


def get_tenant_default_model_by_type(tenant_id: str, model_type: str|enum.Enum):
    exist, tenant = TenantService.get_by_id(tenant_id)
    if not exist:
        raise LookupError("Tenant not found")
    model_type_val = model_type if isinstance(model_type, str) else model_type.value
    model_name: str = ""
    match model_type_val:
        case LLMType.EMBEDDING.value:
            model_name = tenant.embd_id
        case LLMType.SPEECH2TEXT.value:
            model_name =  tenant.asr_id
        case LLMType.IMAGE2TEXT.value:
            model_name = tenant.img2txt_id
        case LLMType.CHAT.value:
            model_name = tenant.llm_id
        case LLMType.RERANK.value:
            model_name = tenant.rerank_id
        case LLMType.TTS.value:
            model_name = tenant.tts_id
        case LLMType.OCR.value:
            raise Exception("OCR model name is required")
        case _:
            raise Exception(f"Unknown model type {model_type}")
    if not model_name:
        raise Exception(f"No default {model_type} model is set.")
    return get_model_config_by_type_and_name(tenant_id, model_type, model_name)
