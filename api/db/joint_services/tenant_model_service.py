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
import os
from common import settings
from common.constants import LLMType
from api.db.services.llm_service import LLMService
from api.db.services.tenant_llm_service import TenantLLMService, TenantService


def get_model_config_by_id(tenant_model_id: int) -> dict:
    found, model_config = TenantLLMService.get_by_id(tenant_model_id)
    if not found:
        raise LookupError(f"Tenant Model with id {tenant_model_id} not found")
    config_dict = model_config.to_dict()
    llm = LLMService.query(llm_name=config_dict["llm_name"])
    if llm:
        config_dict["is_tools"] = llm[0].is_tools
    return config_dict


def get_model_config_by_type_and_name(tenant_id: str, model_type: str, model_name: str):
    if not model_name:
        raise Exception("Model Name is required")
    model_config = TenantLLMService.get_api_key(tenant_id, model_name)
    if not model_config:
        # model_name in format 'name@factory', split model_name and try again
        pure_model_name, fid = TenantLLMService.split_model_name_and_factory(model_name)
        if model_type == LLMType.EMBEDDING and fid == "Builtin" and "tei-" in os.getenv("COMPOSE_PROFILES", "") and pure_model_name == os.getenv("TEI_MODEL", ""):
            # configured local embedding model
            embedding_cfg = settings.EMBEDDING_CFG
            config_dict = {
                "llm_factory": "Builtin",
                "api_key": embedding_cfg["api_key"],
                "llm_name": pure_model_name,
                "api_base": embedding_cfg["base_url"],
                "model_type": LLMType.EMBEDDING,
            }
        else:
            model_config = TenantLLMService.get_api_key(tenant_id, pure_model_name)
            if not model_config:
                raise LookupError(f"Tenant Model with name {model_name} not found")
            config_dict = model_config.to_dict()
    else:
        # model_name without @factory
        config_dict = model_config.to_dict()
    llm = LLMService.query(llm_name=config_dict["llm_name"])
    if llm:
        config_dict["is_tools"] = llm[0].is_tools
    return config_dict


def get_tenant_default_model_by_type(tenant_id: str, model_type: str):
    exist, tenant = TenantService.get_by_id(tenant_id)
    if not exist:
        raise LookupError("Tenant not found")
    model_name: str = ""
    match model_type:
        case LLMType.EMBEDDING:
            model_name = tenant.embd_id
        case LLMType.SPEECH2TEXT:
            model_name =  tenant.asr_id
        case LLMType.IMAGE2TEXT:
            model_name = tenant.img2txt_id
        case LLMType.CHAT:
            model_name = tenant.llm_id
        case LLMType.RERANK:
            model_name = tenant.rerank_id
        case LLMType.TTS:
            model_name = tenant.tts_id
        case LLMType.OCR:
            raise Exception("OCR model name is required")
        case _:
            raise Exception(f"Unknown model type {model_type}")
    if not model_name:
        raise Exception(f"No default {model_type} model is set.")
    return get_model_config_by_type_and_name(tenant_id, model_type, model_name)
