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

from common.settings import FACTORY_LLM_INFOS
from api.db.services.tenant_model_provider_service import TenantModelProviderService
from api.db.services.tenant_model_instance_service import TenantModelInstanceService
from api.db.services.tenant_model_service import TenantModelService
from api.db.services.user_service import TenantService

# Mapping from model_type string to Tenant model field name
MODEL_TYPE_TO_FIELD = {
    "chat": "llm_id",
    "embedding": "embd_id",
    "rerank": "rerank_id",
    "asr": "asr_id",
    "vision": "img2txt_id",
    "tts": "tts_id",
    "ocr": "ocr_id",
}


def _get_model_info(tenant_id: str, default_model: str, model_type: str):
    """
    Parse a composite model string (modelName@instanceName@providerName or modelName@providerName)
    and validate that the provider, instance, and model exist.

    Returns a dict with model info or None on error.
    """
    if not default_model:
        return None

    parts = default_model.split("@")
    if len(parts) == 3:
        model_name, instance_name, provider_name = parts
    elif len(parts) == 2:
        model_name, provider_name = parts
        instance_name = "default"
    else:
        logging.warning(f"Invalid model string: {default_model}")
        return None

    # Special case: OCR with infiniflow@default@deepdoc is always enabled
    if model_type == "ocr" and provider_name == "infiniflow" and instance_name == "default" and model_name == "deepdoc":
        return {
            "model_provider": provider_name,
            "model_instance": instance_name,
            "model_name": model_name,
            "model_type": model_type,
            "enable": True,
        }

    # Check if the provider exists for the tenant
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        logging.warning(f"Provider '{provider_name}' not found for tenant '{tenant_id}'")
        return None

    # Check if the instance exists
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        logging.warning(f"Instance '{instance_name}' not found for provider '{provider_name}'")
        return None

    # Check if model is in the LLM factory info
    factory_info = [f for f in (FACTORY_LLM_INFOS or []) if f["name"] == provider_name]
    if not factory_info:
        logging.warning(f"Provider '{provider_name}' not found in factory info")
        return None

    llms = factory_info[0].get("llm", [])
    target_llm = [llm for llm in llms if llm["llm_name"] == model_name]
    if not target_llm:
        logging.warning(f"Model '{model_name}' not found for provider '{provider_name}'")
        return None

    # Check if the model_type matches
    if target_llm[0].get("model_type") != model_type:
        logging.warning(f"Model '{model_name}' isn't a {model_type} model")
        return None

    # Check if model is enabled (no TenantModel record or status != inactive means enabled)
    model_entity = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(
        provider_obj.id, instance_obj.id, model_name
    )
    enable = model_entity is None or model_entity.status != "inactive"

    return {
        "model_provider": provider_name,
        "model_instance": instance_name,
        "model_name": model_name,
        "model_type": model_type,
        "enable": enable,
    }


def _check_model_available(tenant_id: str, provider_name: str, instance_name: str, model_name: str, model_type: str):
    """
    Validate that a model is available for the tenant:
    - Provider exists for the tenant
    - Instance exists under the provider
    - Model is in the LLM factory info for the provider
    - Model type matches
    - Model is not disabled in TenantModel table

    Returns (success, error_message).
    """
    # Check provider
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"Provider '{provider_name}' not found"

    # Check instance
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        return False, f"Instance '{instance_name}' not found for provider '{provider_name}'"

    # Check model schema
    factory_info = [f for f in (FACTORY_LLM_INFOS or []) if f["name"] == provider_name]
    if not factory_info:
        return False, f"Provider '{provider_name}' not found in factory info"

    llms = factory_info[0].get("llm", [])
    target_llm = [llm for llm in llms if llm["llm_name"] == model_name]
    if not target_llm:
        return False, f"Model '{model_name}' not found for provider '{provider_name}'"

    if target_llm[0].get("model_type") != model_type:
        return False, f"Model '{model_name}' isn't a {model_type} model"

    # Check if model is disabled
    model_entity = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(
        provider_obj.id, instance_obj.id, model_name
    )
    if model_entity and model_entity.status == "inactive":
        return False, f"Model '{model_name}' isn't available"

    return True, None


def list_tenant_default_models(tenant_id: str):
    """
    List all default models for a tenant.

    For each model type (chat, embedding, rerank, asr, vision, tts, ocr),
    reads the composite model ID string from the Tenant record and resolves
    it into provider/instance/name components.

    :param tenant_id: tenant ID
    :return: (success, result_or_error_message)
    """
    e, tenant = TenantService.get_by_id(tenant_id)
    if not e:
        return False, "Tenant not found"

    models = []

    for model_type, field_name in MODEL_TYPE_TO_FIELD.items():
        default_model = getattr(tenant, field_name, None)
        if not default_model:
            continue
        model_info = _get_model_info(tenant_id, default_model, model_type)
        if model_info:
            models.append(model_info)

    return True, {"models": models}


def set_tenant_default_models(tenant_id: str, model_provider: str, model_instance: str, model_name: str, model_type: str):
    """
    Set or clear a tenant default model.

    If model_provider, model_instance, and model_name are all provided,
    validates the model and sets it as the default.
    If all three are empty, clears the default for the given model type.

    :param tenant_id: tenant ID
    :param model_provider: provider name
    :param model_instance: instance name
    :param model_name: model name
    :param model_type: model type (chat, embedding, rerank, asr, vision, tts, ocr)
    :return: (success, result_or_error_message)
    """
    field_name = MODEL_TYPE_TO_FIELD.get(model_type)
    if not field_name:
        return False, f"model type '{model_type}' is invalid"

    e, tenant = TenantService.get_by_id(tenant_id)
    if not e:
        return False, "Tenant not found"

    if not model_provider and not model_instance and not model_name:
        # Clear the default model
        default_model = ""
    elif model_provider and model_instance and model_name:
        # Validate and set the default model
        success, msg = _check_model_available(tenant_id, model_provider, model_instance, model_name, model_type)
        if not success:
            return False, msg
        default_model = f"{model_name}@{model_instance}@{model_provider}"
    else:
        return False, "model_provider, model_instance and model_name must be specified together"

    TenantService.update_by_id(tenant_id, {field_name: default_model})
    return True, "success"
