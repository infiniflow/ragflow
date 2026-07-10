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

from api.db.joint_services.tenant_model_service import ensure_mineru_from_env, ensure_opendataloader_from_env, ensure_paddleocr_from_env, resolve_model_id
from api.db.services.tenant_model_instance_service import TenantModelInstanceService
from api.db.services.tenant_model_provider_service import TenantModelProviderService
from api.db.services.tenant_model_service import TenantModelService
from api.db.services.user_service import TenantService
from api.utils.model_utils import get_model_type_human, calculate_model_type
from common.constants import ActiveStatusEnum, LLMType
from common.settings import FACTORY_LLM_INFOS

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

MODEL_TYPE_TO_TENANT_ID_FIELD = {
    "chat": "tenant_llm_id",
    "embedding": "tenant_embd_id",
    "rerank": "tenant_rerank_id",
    "asr": "tenant_asr_id",
    "vision": "tenant_img2txt_id",
    "tts": "tenant_tts_id",
    "ocr": "tenant_ocr_id",
}

MODEL_TAG_TO_TYPE = {
    "chat": "chat",
    "embedding": "embedding",
    "rerank": "rerank",
    "asr": "speech2text",
    "vision": "image2text",
    "tts": "tts",
    "ocr": "ocr",
}


def _to_int(v, default=500):
    try:
        return int(v)
    except (TypeError, ValueError):
        return default


def _factory_model_types(llm: dict) -> list[str]:
    model_type = llm.get("model_type")
    if isinstance(model_type, list):
        return model_type
    return [model_type] if model_type else []


def _get_model_info(tenant_id: str, default_model: str, model_type: str, model_id: str = ""):
    """
    Parse a composite model string (modelName@instanceName@providerName or modelName@providerName)
    and validate that the provider, instance, and model exist.

    Returns a dict with model info or None on error.
    """
    if not default_model:
        return None

    # The composite key is right-anchored: provider_name is always the *last*
    # '@'-separated field. Use rsplit so a model_name that itself contains '@'
    # (e.g. LM Studio IDs like `text-embedding-nomic-embed-text-v1.5@q8_0`)
    # remains intact in the leftmost field instead of being truncated.
    parts = default_model.rsplit("@", 2)
    if len(parts) == 3:
        model_name, instance_name, provider_name = parts
    elif len(parts) == 2:
        model_name, provider_name = parts
        instance_name = "default"
    else:
        model_name = parts[0]
        provider_name = ""
        instance_name = "default"

    model_type = MODEL_TAG_TO_TYPE.get(model_type, model_type)
    # Special case: OCR with infiniflow@default@deepdoc is always enabled
    if model_type == "ocr" and provider_name == "infiniflow" and instance_name == "default" and model_name == "deepdoc":
        return {
            "model_id": model_id,
            "model_provider": provider_name,
            "model_instance": instance_name,
            "model_name": model_name,
            "model_type": model_type,
            "enable": True,
        }

    # Special case: TEI Builtin embedding model
    compose_profiles = os.getenv("COMPOSE_PROFILES", "")
    tei_model = os.getenv("TEI_MODEL", "")
    if model_type == "embedding" and "tei-" in compose_profiles and tei_model and model_name == tei_model and (not provider_name or provider_name == "Builtin"):
        return {
            "model_id": model_id,
            "model_provider": "Builtin",
            "model_instance": "default",
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

    model_record = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(provider_obj.id, instance_obj.id, model_name)
    if not model_record:
        logging.warning(f"Model '{model_name}' not found for provider '{provider_name}' and instance '{instance_name}'")
        return None

    if not model_record.status == ActiveStatusEnum.ACTIVE.value:
        logging.warning(f"Model '{model_name}' is disabled")
        return None

    return {"model_id": model_id, "model_provider": provider_name, "model_instance": instance_name, "model_name": model_name, "model_type": model_type, "enable": True}


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
    if provider_name == "infiniflow" and instance_name == "default" and model_name == "deepdoc":
        return True, None

    if model_type == "ocr" and provider_name == "infiniflow" and instance_name == "default" and model_name == "deepdoc":
        return True, None

    compose_profiles = os.getenv("COMPOSE_PROFILES", "")
    is_tei_builtin_embedding = model_type == LLMType.EMBEDDING.value and "tei-" in compose_profiles and model_name == os.getenv("TEI_MODEL", "") and (provider_name == "Builtin" or not provider_name)
    if is_tei_builtin_embedding:
        return True, None

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
    model_type = MODEL_TAG_TO_TYPE.get(model_type, model_type)
    # Check if model is disabled
    model_record = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(provider_obj.id, instance_obj.id, model_name)
    if model_record.status != ActiveStatusEnum.ACTIVE.value:
        return False, f"Model '{model_name}' isn't available"
    if model_type not in get_model_type_human(model_record.model_type):
        return False, f"Model '{model_name}' isn't a {model_type} model"
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
        tenant_model_id = getattr(tenant, MODEL_TYPE_TO_TENANT_ID_FIELD[model_type], None)
        model_info = _get_model_info(tenant_id, default_model, model_type, tenant_model_id)
        if model_info:
            models.append(model_info)

    return True, {"models": models}


def set_tenant_default_models(tenant_id: str, model_provider: str, model_instance: str, model_name: str, model_type: str, model_id: str = ""):
    """
    Set or clear a tenant default model.

    If model_id is provided, resolves it to provider/instance/name and uses it.
    Otherwise falls back to the legacy model_provider/model_instance/model_name triplet.
    If all model selectors are empty, clears the default for the given model type.

    :param tenant_id: tenant ID
    :param model_provider: provider name
    :param model_instance: instance name
    :param model_name: model name
    :param model_type: model type (chat, embedding, rerank, asr, vision, tts, ocr)
    :param model_id: tenant_model id
    :return: (success, result_or_error_message)
    """
    field_name = MODEL_TYPE_TO_FIELD.get(model_type)
    if not field_name:
        return False, f"model type '{model_type}' is invalid"
    tenant_field_name = MODEL_TYPE_TO_TENANT_ID_FIELD.get(model_type)

    e, tenant = TenantService.get_by_id(tenant_id)
    if not e:
        return False, "Tenant not found"

    tenant_model_id = None
    if model_id:
        model_ok, model_obj = TenantModelService.get_by_id(model_id)
        if not model_ok:
            return False, f"model_id '{model_id}' not found"

        provider_ok, provider_obj = TenantModelProviderService.get_by_id(model_obj.provider_id)
        if not provider_ok:
            return False, f"Provider id '{model_obj.provider_id}' not found for model_id '{model_id}'"
        if provider_obj.tenant_id != tenant_id:
            return False, "Permission denied"

        instance_ok, instance_obj = TenantModelInstanceService.get_by_id(model_obj.instance_id)
        if not instance_ok:
            return False, f"Instance id '{model_obj.instance_id}' not found for model_id '{model_id}'"

        normalized_model_type = MODEL_TAG_TO_TYPE.get(model_type, model_type)
        if normalized_model_type not in get_model_type_human(model_obj.model_type):
            return False, f"Model '{model_obj.model_name}' isn't a {model_type} model"
        if model_obj.status != ActiveStatusEnum.ACTIVE.value:
            return False, f"Model '{model_obj.model_name}' isn't available"

        default_model = f"{model_obj.model_name}@{instance_obj.instance_name}@{provider_obj.provider_name}"
        tenant_model_id = model_id
    elif model_provider and model_instance and model_name:
        # Validate and set the default model
        success, msg = _check_model_available(tenant_id, model_provider, model_instance, model_name, model_type)
        if not success:
            return False, msg
        default_model = f"{model_name}@{model_instance}@{model_provider}"
        normalized_model_type = MODEL_TAG_TO_TYPE.get(model_type, model_type)
        try:
            tenant_model_id = resolve_model_id(tenant_id, normalized_model_type, default_model)
        except LookupError as exc:
            return False, str(exc)
    else:
        # Clear the default model
        default_model = ""
        tenant_model_id = None

    update_fields = {field_name: default_model, tenant_field_name: tenant_model_id}
    TenantService.update_by_id(tenant_id, update_fields)
    return True, "success"


def list_tenant_added_models(tenant_id: str, model_type_filter: str = None):
    """
    List all added models for a tenant.

    :param tenant_id: tenant ID
    :param model_type_filter: model type filter (chat, embedding, rerank, asr, vision, tts, ocr)
    :return: (success, result_or_error_message)
    """
    e, tenant = TenantService.get_by_id(tenant_id)
    if not e:
        return False, "Tenant not found"

    ensure_mineru_from_env(tenant_id)
    ensure_paddleocr_from_env(tenant_id)
    ensure_opendataloader_from_env(tenant_id)

    model_type_filter_bin = calculate_model_type(model_type_filter.lower()) if model_type_filter else None

    providers = TenantModelProviderService.get_by_tenant_id(tenant_id)
    if not providers:
        return True, []

    provider_ids = [provider.id for provider in providers]
    instances = TenantModelInstanceService.get_by_provider_ids(provider_ids)
    if not instances:
        return True, []
    provider_instance_map: dict = {}
    provider_info_map = {provider.id: provider for provider in providers}
    instance_info_map = {instance.id: instance for instance in instances}
    for provider_instance_record in instances:
        provider_name = provider_info_map[provider_instance_record.provider_id].provider_name if provider_info_map.get(provider_instance_record.provider_id) else ""
        if provider_instance_map.get(provider_name):
            provider_instance_map[provider_name].append(provider_instance_record)
        else:
            provider_instance_map[provider_name] = [provider_instance_record]

    model_records = TenantModelService.get_models_by_provider_ids_and_instance_ids(provider_ids, list({instance.id for instance in instances}))
    target_type_records = [record for record in model_records if record.model_type & model_type_filter_bin] if model_type_filter_bin else model_records

    model_rank_map: dict = {}
    for factory in FACTORY_LLM_INFOS:
        for llm in factory.get("llm", []):
            model_rank_map[(factory["name"], llm["llm_name"])] = _to_int(llm.get("rank", 500))

    factory_rank_mapping = {factory["name"]: -_to_int(factory.get("rank", "500")) for factory in FACTORY_LLM_INFOS}
    added_models = [
        {
            "model_id": model_record.id,
            "tenant_id": provider_info_map[model_record.provider_id].tenant_id,
            "tenant_name": tenant.name,
            "model_type": get_model_type_human(model_record.model_type),
            "name": model_record.model_name,
            "provider_id": model_record.provider_id,
            "provider_name": provider_info_map[model_record.provider_id].provider_name,
            "instance_id": model_record.instance_id,
            "instance_name": instance_info_map[model_record.instance_id].instance_name,
            "rank": model_rank_map.get((provider_info_map[model_record.provider_id].provider_name, model_record.model_name), 500),
        }
        for model_record in target_type_records
    ]

    # Add TEI Builtin embedding model if configured
    compose_profiles = os.getenv("COMPOSE_PROFILES", "")
    tei_model = os.getenv("TEI_MODEL", "")
    if "tei-" in compose_profiles and tei_model:
        if not model_type_filter or model_type_filter == "embedding":
            tei_already_added = any(m["provider_name"] == "Builtin" and m["name"] == tei_model for m in added_models)
            if not tei_already_added:
                added_models.append(
                    {
                        "model_id": "",
                        "tenant_id": tenant.id,
                        "tenant_name": tenant.name,
                        "model_type": ["embedding"],
                        "name": tei_model,
                        "provider_id": "",
                        "provider_name": "Builtin",
                        "instance_id": "",
                        "instance_name": "default",
                        "rank": model_rank_map.get(("Builtin", tei_model), 500),
                    }
                )

    added_models.sort(key=lambda x: (factory_rank_mapping.get(x["provider_name"]), x["provider_name"], x["instance_name"], -x["rank"], x["name"]))

    return True, added_models
