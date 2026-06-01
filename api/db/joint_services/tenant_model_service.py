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
from common.constants import LLMType, ActiveStatusEnum
from api.db.services.tenant_llm_service import TenantLLMService, TenantService
from api.db.services.tenant_model_provider_service import TenantModelProviderService
from api.db.services.tenant_model_instance_service import TenantModelInstanceService
from api.db.services.tenant_model_service import TenantModelService

logger = logging.getLogger(__name__)


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
    return get_model_config_from_provider_instance(tenant_id, model_type, model_name)


def split_model_name(model_name: str):
    # Parse model_name: {model_name} or {model_name}@{factory_name} or {model_name}@{instance_name}@{factory_name}
    parts = model_name.split("@")
    if len(parts) == 1:
        pure_model_name = parts[0]
        provider_name = ""
        instance_name = ""
    elif len(parts) == 2:
        pure_model_name = parts[0]
        provider_name = parts[1]
        instance_name = "default"
    else:
        pure_model_name = parts[0]
        instance_name = parts[1]
        provider_name = parts[2]
    return pure_model_name, instance_name, provider_name


def get_model_config_from_provider_instance(tenant_id, model_type: str|enum.Enum, model_name: str):
    pure_model_name, instance_name, provider_name = split_model_name(model_name)
    model_type_val = model_type if isinstance(model_type, str) else model_type.value
    # Builtin embedding model
    compose_profiles = os.getenv("COMPOSE_PROFILES", "")
    is_tei_builtin_embedding = (
            model_type_val == LLMType.EMBEDDING.value
            and "tei-" in compose_profiles
            and pure_model_name == os.getenv("TEI_MODEL", "")
            and (provider_name == "Builtin" or provider_name is None)
    )
    if is_tei_builtin_embedding:
        # configured local embedding model
        embedding_cfg = settings.EMBEDDING_CFG
        return {
            "llm_factory": "Builtin",
            "api_key": embedding_cfg["api_key"],
            "llm_name": pure_model_name,
            "api_base": embedding_cfg["base_url"],
            "model_type": LLMType.EMBEDDING.value,
        }

    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        raise LookupError(f"Provider {provider_name} not found for model {model_name}.")
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        raise LookupError(f"Instance {instance_name} not found for model {model_name}.")
    model_obj = TenantModelService.get_by_provider_id_and_instance_id_and_model_type_and_model_name(provider_obj.id, instance_obj.id, model_type_val, pure_model_name)

    import json
    api_key, is_tool, api_key_payload = TenantLLMService._decode_api_key_config(instance_obj.api_key)
    extra_fields = json.loads(instance_obj.extra) if instance_obj.extra else {}

    if model_obj:
        if model_obj.status == ActiveStatusEnum.INACTIVE.value:
            raise LookupError(f"Model {model_name} is disabled.")

        model_config = {
            "llm_factory": provider_obj.provider_name,
            "api_key": api_key,
            "llm_name": model_obj.model_name,
            "api_base": extra_fields.get("base_url", ""),
            "model_type": model_obj.model_type,
            "is_tool": extra_fields.get("is_tool", is_tool)
        }
        if api_key_payload is not None:
            model_config["api_key_payload"] = api_key_payload

        return model_config
    else:
        fac_list = [f for f in settings.FACTORY_LLM_INFOS if f["name"] == provider_name]
        if not fac_list:
            raise LookupError(f"Model provider config not found: {provider_name}")
        llm_list = [llm for llm in fac_list[0]["llm"] if llm["llm_name"] == pure_model_name]
        if not llm_list:
            raise LookupError(f"Model config not found: {model_name}")
        llm_info = llm_list[0]
        model_config = {
            "llm_factory": provider_obj.provider_name,
            "api_key": api_key,
            "llm_name": llm_info["llm_name"],
            "api_base": extra_fields.get("base_url", ""),
            "model_type": llm_info["model_type"],
            "is_tool": llm_info.get("is_tool", is_tool)
        }
        if api_key_payload is not None:
            model_config["api_key_payload"] = api_key_payload
        return model_config


def get_api_key(tenant_id: str, model_name: str):
    _, instance_name, provider_name = split_model_name(model_name)

    if not provider_name:
        raise LookupError("Provider name is required.")
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        raise LookupError(f"Provider {provider_name} not found.")
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        raise LookupError(f"Instance {instance_name} not found.")
    return instance_obj.api_key


def get_model_type_by_name(tenant_id: str, model_name: str):
    pure_model_name, instance_name, provider_name = split_model_name(model_name)
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        raise LookupError(f"Provider {provider_name} not found for model {model_name}.")
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        raise LookupError(f"Instance {instance_name} not found for model {model_name}.")
    model_objs = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(provider_obj.id, instance_obj.id, pure_model_name)
    if not model_objs:
        fac_list = [f for f in settings.FACTORY_LLM_INFOS if f["name"] == provider_name]
        if not fac_list:
            raise LookupError(f"Model provider config not found: {provider_name}")
        llm_list = [llm for llm in fac_list[0]["llm"] if llm["llm_name"] == pure_model_name]
        if not llm_list:
            raise LookupError(f"Model {pure_model_name} not found for model {model_name}.")
        return [llm_list[0]["model_type"]]
    return [model_obj.model_type for model_obj in model_objs]


def delete_models_by_instance_ids(instance_ids: list[str]):
    return TenantModelService.delete_by_instance_ids(instance_ids)


def delete_instances_by_provider_ids(provider_ids: list[str]):
    return TenantModelInstanceService.delete_by_provider_ids(provider_ids)
