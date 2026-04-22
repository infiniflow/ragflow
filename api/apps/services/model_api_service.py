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

from common.model_provider_manager import get_model_provider_manager
from api.db.services.tenant_model_provider_service import TenantModelProviderService
from api.db.services.tenant_model_instance_service import TenantModelInstanceService
from api.db.services.tenant_model_service import TenantModelService
from api.db.services.user_service import TenantService


# Mapping from model_type to the Tenant field that stores the default model ID
_MODEL_TYPE_FIELD_MAP = {
    "chat": "llm_id",
    "embedding": "embd_id",
    "rerank": "rerank_id",
    "asr": "asr_id",
    "vision": "img2txt_id",
    "tts": "tts_id",
    "ocr": "ocr_id",
}

# Model types to list in GetModels (same order as Go)
_DEFAULT_MODEL_TYPES = ["chat", "embedding", "rerank", "asr", "vision", "ocr", "tts"]


def get_models(tenant_id: str):
    """
    List tenant default models for each model type.

    Equivalent to Go's TenantService.ListTenantDefaultModels().

    :param tenant_id: tenant ID
    :return: (success, result)
    """
    try:
        tenant_infos = TenantService.get_info_by(tenant_id)
        if not tenant_infos:
            return False, "No tenant found"

        tenant = tenant_infos[0]

        result = []
        for model_type in _DEFAULT_MODEL_TYPES:
            field = _MODEL_TYPE_FIELD_MAP.get(model_type)
            default_model = tenant.get(field, "")
            if not default_model:
                continue

            item = _resolve_model_info(tenant.get("tenant_id", tenant_id), default_model, model_type)
            if item is not None:
                result.append(item)

        return True, result
    except Exception as e:
        logging.exception("Failed to get models")
        return False, str(e)


def set_models(tenant_id: str, model_provider: str, model_instance: str, model_name: str, model_type: str):
    """
    Set tenant default model for a given model type.

    Equivalent to Go's TenantService.SetTenantDefaultModels().

    :param tenant_id: tenant ID
    :param model_provider: provider name
    :param model_instance: instance name
    :param model_name: model name
    :param model_type: model type (chat, embedding, rerank, asr, vision, tts)
    :return: (success, message)
    """
    try:
        field = _MODEL_TYPE_FIELD_MAP.get(model_type)
        if not field:
            return False, f"Model type {model_type} is invalid"

        tenant_infos = TenantService.get_info_by(tenant_id)
        if not tenant_infos:
            return False, "No tenant found"

        tid = tenant_infos[0].get("tenant_id", tenant_id)

        # Determine the default model string
        if not model_provider and not model_instance and not model_name:
            default_model = ""
        elif model_provider and model_instance and model_name:
            # Check model availability
            err = _check_model_available(tid, model_provider, model_instance, model_name, model_type)
            if err is not None:
                return False, err
            default_model = f"{model_name}@{model_instance}@{model_provider}"
        else:
            return False, "model provider, instance and name must be specified together"

        TenantService.update_by_id(tid, {field: default_model})
        return True, "success"
    except Exception as e:
        logging.exception("Failed to set models")
        return False, str(e)


def _resolve_model_info(tenant_id: str, default_model: str, model_type: str):
    """
    Parse default model string and resolve provider/instance/name and enable status.

    The default model string format is: modelName@instanceName@providerName
    or legacy format: modelName@providerName

    Equivalent to Go's TenantService.GetModelInfo().
    """
    parts = default_model.split("@")
    if len(parts) == 3:
        model_name = parts[0]
        instance_name = parts[1]
        provider_name = parts[2]
    elif len(parts) == 2:
        model_name = parts[0]
        instance_name = "default"
        provider_name = parts[1]
    else:
        return None

    # Special case for OCR
    if model_type == "ocr":
        if provider_name == "infiniflow" and instance_name == "default" and model_name == "deepdoc":
            return {
                "model_provider": provider_name,
                "model_instance": instance_name,
                "model_name": model_name,
                "model_type": model_type,
                "enable": True,
            }

    # Check provider exists
    try:
        provider = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    except Exception:
        return None
    if not provider:
        return None

    # Check instance exists
    try:
        instance = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider.id, instance_name)
    except Exception:
        return None
    if not instance:
        return None

    # Check model exists in global provider config
    pm = get_model_provider_manager()
    model_schema = pm.get_model_by_name(provider_name, model_name)
    if model_schema is None:
        return None

    # Check model supports the requested type
    model_types = model_schema.get("model_types", [])
    if model_type not in model_types:
        return None

    # Check if model is enabled (no record in tenant_model table means enabled)
    try:
        existing = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(
            provider.id, instance.id, model_name
        )
        enable = existing is None
    except Exception:
        enable = True

    return {
        "model_provider": provider_name,
        "model_instance": instance_name,
        "model_name": model_name,
        "model_type": model_type,
        "enable": enable,
    }


def _check_model_available(tenant_id: str, provider_name: str, instance_name: str, model_name: str, model_type: str):
    """
    Check if a model is available for a tenant.

    Equivalent to Go's TenantService.checkModelAvailable().
    Returns None if available, or an error message string.
    """
    # Check provider
    provider = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider:
        return f"Provider {provider_name} not found"

    # Check instance
    instance = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider.id, instance_name)
    if not instance:
        return f"Instance {instance_name} not found"

    # Check model in global config
    pm = get_model_provider_manager()
    model_schema = pm.get_model_by_name(provider_name, model_name)
    if model_schema is None:
        return f"Model {model_name} not found for provider {provider_name}"

    # Check model type
    model_types = model_schema.get("model_types", [])
    if model_type not in model_types:
        return f"Model {model_name} isn't a {model_type} model"

    # Check if model is disabled (record exists in tenant_model table)
    try:
        existing = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(
            provider.id, instance.id, model_name
        )
        if existing is not None:
            return f"Model {model_name} isn't available"
    except Exception:
        pass

    return None
