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
import json
import logging
import asyncio

from common.constants import LLMType, ActiveStatusEnum, ModelVerifyStatusEnum
from common.settings import FACTORY_LLM_INFOS
from api.db.joint_services.tenant_model_service import resolve_model_config, delete_models_by_instance_ids, delete_instances_by_provider_ids
from api.db.services.tenant_model_provider_service import TenantModelProviderService
from api.db.services.tenant_model_instance_service import TenantModelInstanceService
from api.db.services.tenant_model_service import TenantModelService
from api.utils.model_utils import get_model_type_human, calculate_model_type
from rag.llm import ChatModel, CvModel, EmbeddingModel, ModelMeta, OcrModel, RerankModel, Seq2txtModel, TTSModel


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


def _normalize_provider_base_url(provider_name: str, base_url: str | None):
    if provider_name != "VLLM" or not base_url:
        return base_url
    base_url = base_url.strip().rstrip("/")
    if not base_url.endswith("/v1"):
        base_url += "/v1"
    return base_url


def _normalize_provider_api_key(provider_name: str, api_key: str | dict | None):
    if provider_name == "VLLM" and not api_key:
        return "x"
    return api_key


def _factory_llm_name(llm: dict) -> str:
    return llm.get("name") or llm.get("llm_name", "")


def list_providers(tenant_id: str, all_available: bool = False):
    """
    List providers for a tenant.

    If available_only is True, list all system-wide providers (pool providers).
    Otherwise, list providers that the tenant has configured, with a has_instance flag.

    :param tenant_id: tenant ID
    :param all_available: whether to list all available providers
    :return: (success, result)
    """
    if not FACTORY_LLM_INFOS:
        return False, []

    factory_rank_mapping = {factory["name"]: -_to_int(factory.get("rank", "500")) for factory in FACTORY_LLM_INFOS}
    factory_info_map = {f["name"]: f for f in FACTORY_LLM_INFOS}
    if all_available:
        providers = []
        for factory_info in FACTORY_LLM_INFOS:
            if factory_info["name"] in ["Youdao", "FastEmbed", "BAAI", "Builtin", "siliconflow_intl"]:
                continue
            model_types = sorted(set(model_type for llm in factory_info.get("llm", []) for model_type in _factory_model_types(llm))) if factory_info.get("llm", []) else []
            if factory_info["name"] in ["MinerU", "PaddleOCR", "OpenDataLoader"]:
                model_types.append("ocr")
            provider = {"model_types": model_types, "name": factory_info["name"], "url": {"default": factory_info.get("url", "")}}
            if factory_info["name"].lower() == "siliconflow":
                provider["url"]["intl"] = factory_info_map.get("siliconflow_intl", {}).get("url", "https://api.siliconflow.com/v1")
            elif factory_info["name"] == "Tongyi-Qianwen":
                provider["url"]["intl"] = "https://dashscope-intl.aliyuncs.com/compatible-model/v1"
            providers.append(provider)
        providers.sort(key=lambda x: (factory_rank_mapping.get(x["name"]), x["name"]))
        return True, providers

    # List tenant-configured providers
    factory_names = TenantModelProviderService.list_provider_names_by_tenant_id(tenant_id)

    providers = []
    factory_info_mapping = {f["name"]: f for f in FACTORY_LLM_INFOS}
    for name in factory_names:
        if name not in ["Youdao", "FastEmbed", "BAAI", "Builtin", "siliconflow_intl"] and factory_info_mapping.get(name):
            factory_info = factory_info_mapping[name]
            provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, name)
            has_instance = bool(provider_obj and TenantModelInstanceService.get_all_by_provider_id(provider_obj.id))
            model_types = sorted(set(model_type for llm in factory_info.get("llm", []) for model_type in _factory_model_types(llm))) if factory_info.get("llm", []) else []
            if name in ["MinerU", "PaddleOCR", "OpenDataLoader"]:
                model_types.append("ocr")

            provider = {"has_instance": has_instance, "model_types": model_types, "name": factory_info["name"], "url": {"default": factory_info.get("url", "")}}
            if factory_info["name"].lower() == "siliconflow":
                provider["url"]["intl"] = factory_info_map.get("siliconflow_intl", {}).get("url", "https://api.siliconflow.com/v1")
            elif factory_info["name"] == "Tongyi-Qianwen":
                provider["url"]["intl"] = "https://dashscope-intl.aliyuncs.com/compatible-model/v1"
            providers.append(provider)
    providers.sort(key=lambda x: (factory_rank_mapping.get(x["name"]), x["name"]))
    return True, providers


def add_provider(tenant_id: str, provider_name: str):
    """
    Add a provider (factory) for a tenant.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :return: (success, result_or_error_message)
    """
    if not FACTORY_LLM_INFOS:
        return False, "No providers found"
    # Check if factory is allowed
    allowed_factories = [f["name"] for f in FACTORY_LLM_INFOS]
    if provider_name not in allowed_factories:
        return False, f"Provider '{provider_name}' is not allowed"

    existing = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if existing:
        return False, f"Provider {provider_name} already exists"

    TenantModelProviderService.insert(tenant_id=tenant_id, provider_name=provider_name)
    return True, "success"


def delete_provider(tenant_id: str, provider_id_or_name: str):
    """
    Delete all instances and models for a provider.

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider ID or provider/factory name
    :return: (success, result_or_error_message)
    """
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"Provider {provider_id_or_name} not found"
    instance_objs = TenantModelInstanceService.get_all_by_provider_id(provider_obj.id)
    if instance_objs:
        instance_ids = [instance_obj.id for instance_obj in instance_objs]
        delete_models_by_instance_ids(instance_ids)
        delete_instances_by_provider_ids([provider_obj.id])
    TenantModelProviderService.delete_by_tenant_id_and_provider_name(tenant_id, provider_obj.provider_name)
    return True, "success"


def show_provider(provider_id_or_name: str):
    """
    Show provider details from LLMFactories.

    :param provider_id_or_name: provider/factory ID or name
    :return: (success, result_or_error_message)
    """
    provider_obj = None
    if provider_id_or_name:
        _, provider_obj = TenantModelProviderService.get_by_id(provider_id_or_name)
    provider_name = provider_obj.provider_name if provider_obj else provider_id_or_name
    fac_list = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
    if not fac_list:
        return False, f"Provider '{provider_id_or_name}' not found"
    factory_info = fac_list[0]
    return True, {"base_url": {"default": factory_info.get("url", "")}, "name": factory_info["name"], "total_models": len(factory_info.get("llm", []))}


async def list_provider_models(provider_id_or_name: str, api_key: str = None, base_url: str = None):
    """
    List all models for a provider from the LLM dictionary.

    :param provider_id_or_name: provider ID or provider/factory name
    :param api_key: api key
    :param base_url: base url
    :return: (success, result_or_error_message)
    """
    provider_obj = None
    if provider_id_or_name:
        _, provider_obj = TenantModelProviderService.get_by_id(provider_id_or_name)
    provider_name = provider_obj.provider_name if provider_obj else provider_id_or_name
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
    if not factory_info:
        return False, f"Provider '{provider_id_or_name}' not found"
    api_key = _normalize_provider_api_key(provider_name, api_key)
    static_llms = [
        {
            "name": _factory_llm_name(llm),
            "max_tokens": llm.get("max_tokens", 8192),
            "model_types": _factory_model_types(llm),
            "features": (llm.get("features") if llm.get("features") is not None else ((["is_tools"] if llm.get("is_tools") else []) + (["thinking"] if llm.get("thinking") else []))),
        }
        for llm in factory_info[0]["llm"]
    ]

    model_base_url = _normalize_provider_base_url(provider_name, base_url) or factory_info[0].get("url", "")
    remote_models = []
    if provider_name in ModelMeta:
        remote_models = await ModelMeta[provider_name](api_key, model_base_url).get_model_list()

    if not static_llms and not remote_models:
        return True, []

    # Merge static and remote models, preferring remote_models on name conflicts
    merged = {m["name"]: m for m in static_llms}
    merged.update({m["name"]: m for m in remote_models})
    models = list(merged.values())

    models.sort(key=lambda x: x["name"])
    return True, models


def show_provider_model(provider_id_or_name: str, model_name: str):
    """
    Show a specific model for a provider.

    :param provider_id_or_name: provider/factory ID or name
    :param model_name: model name
    :return: (success, result_or_error_message)
    """
    provider_obj = None
    if provider_id_or_name:
        _, provider_obj = TenantModelProviderService.get_by_id(provider_id_or_name)
    provider_name = provider_obj.provider_name if provider_obj else provider_id_or_name
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
    if not factory_info:
        return False, f"Provider '{provider_id_or_name}' not found"
    llms = factory_info[0]["llm"]
    if not llms:
        return False, f"No models found for provider '{provider_id_or_name}'"
    target_llm = [llm for llm in llms if _factory_llm_name(llm) == model_name]
    if not target_llm:
        return False, f"Model '{model_name}' not found"
    llm_info = target_llm[0]

    return True, {
        "name": _factory_llm_name(llm_info),
        "max_tokens": llm_info["max_tokens"],
        "model_types": _factory_model_types(llm_info),
        "thinking": None,
        "model_type_map": {model_type: True for model_type in _factory_model_types(llm_info)},
    }


async def update_provider_instance(
    tenant_id: str, provider_id_or_name: str, instance_id_or_name: str, instance_name: str, api_key: str | dict, base_url: str, region: str, model_info: list[dict] = None, verify: bool = True
):
    """
    Update a provider instance.

    Updates the instance's api_key, base_url, region, and re-creates all models
    based on the provided model_info list.

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider/factory ID or name
    :param instance_id_or_name: instance ID or name
    :param instance_name: instance name (used as a logical identifier)
    :param api_key: API key
    :param base_url: base url
    :param region: region
    :param model_info: model info, [{
        "model_type": ["chat"],  # support multiple
        "model_name": "name",
        "max_tokens": 4096,
        "extra": {
            "is_tools": True
        }
    }]
    :param verify: verify api_key
    :return: (success, result_or_error_message)
    """
    if not provider_id_or_name:
        return False, "Provider ID or name is required"

    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"Provider '{provider_id_or_name}' does not exist"

    provider_name = provider_obj.provider_name

    # Find the instance
    instance_obj = None
    if instance_id_or_name:
        _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
    if instance_obj and instance_obj.provider_id != provider_obj.id:
        instance_obj = None
    if not instance_obj:
        instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_id_or_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"

    base_url = _normalize_provider_base_url(provider_name, base_url)
    api_key = _normalize_provider_api_key(provider_name, api_key)

    api_key_str = ""
    if api_key:
        api_key_str = api_key if isinstance(api_key, str) else json.dumps(api_key)

    # Verify api_key
    model_verify_result = {}
    if verify:
        success, msg, model_verify_result = await verify_api_key(provider_name, api_key, base_url, region, model_info)
        if not success:
            return False, msg

    # Update instance record
    update_dict = {
        "api_key": api_key_str,
    }
    if instance_name != instance_obj.instance_name:
        update_dict["instance_name"] = instance_name

    extra_fields = {}
    if base_url:
        extra_fields["base_url"] = base_url
    if region:
        extra_fields["region"] = region
    # Preserve existing extra fields not overwritten
    existing_extra = json.loads(instance_obj.extra) if instance_obj.extra else {}
    existing_extra.update(extra_fields)
    update_dict["extra"] = json.dumps(existing_extra)
    TenantModelInstanceService.update_by_id(instance_obj.id, update_dict)

    # Use the (possibly updated) instance_name for model recreation
    effective_instance_name = instance_name

    # Upsert models: add new ones, update existing ones, remove ones no longer selected
    existing_model_objs = TenantModelService.get_models_by_instance_id(instance_obj.id)
    existing_model_names = {model_obj.model_name: model_obj for model_obj in existing_model_objs}

    # Delete models that are no longer in the submitted model_info
    submitted_model_names = set()
    if model_info:
        submitted_model_names = {m.get("model_name") for m in model_info if m.get("model_name")}
    elif model_info is not None:
        # model_info is explicitly an empty list — remove all models
        submitted_model_names = set()
    models_to_remove = set(existing_model_names.keys()) - submitted_model_names
    if models_to_remove:
        TenantModelService.delete_by_ids([existing_model_names[n].id for n in models_to_remove])

    msg = ""
    if model_info:
        for model in model_info:
            model_name = model.get("model_name")
            if not model_name:
                continue
            if verify:
                verify_status = model_verify_result.get(model_name, ModelVerifyStatusEnum.UNKNOWN.value)
                if model.get("extra"):
                    model["extra"].update({"verify": verify_status})
                else:
                    model["extra"] = {"verify": verify_status}

            if model_name in existing_model_names:
                # Update existing model
                update_dict = {}
                if isinstance(model.get("model_type"), (str, list)):
                    target_model_type = calculate_model_type(model["model_type"])
                    if target_model_type != existing_model_names[model_name].model_type:
                        update_dict["model_type"] = target_model_type
                merged_extra = json.loads(existing_model_names[model_name].extra) if existing_model_names[model_name].extra else {}
                merged_extra.update(model["extra"])
                if "max_tokens" in model:
                    merged_extra.update({"max_tokens": model["max_tokens"]})
                update_dict["extra"] = json.dumps(merged_extra)
                if update_dict:
                    TenantModelService.update_model(existing_model_names[model_name].id, update_dict)
            else:
                # Add new model
                success, _msg = add_model_to_instance(tenant_id, provider_name, effective_instance_name, **model)
                if not success:
                    msg += _msg
    else:
        if model_info is None:
            # model_info not provided — add all factory default models (same as create)
            factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
            factory_llms = factory_info[0]["llm"]
            for llm in factory_llms:
                llm_name = _factory_llm_name(llm)
                if llm_name in existing_model_names:
                    # Update existing
                    update_dict = {}
                    target_model_type = calculate_model_type(_factory_model_types(llm))
                    if target_model_type != existing_model_names[llm_name].model_type:
                        update_dict["model_type"] = target_model_type
                    db_extra = json.loads(existing_model_names[llm_name].extra) if existing_model_names[llm_name].extra else {}
                    db_extra_fields = {
                        "max_tokens": llm["max_tokens"],
                        "is_tools": llm.get("is_tools", False),
                        "thinking": "thinking" in llm.get("features", []),
                    }
                    if verify:
                        verify_status = model_verify_result.get(llm_name, ModelVerifyStatusEnum.UNKNOWN.value)
                        db_extra_fields["verify"] = verify_status
                    db_extra.update(db_extra_fields)
                    update_dict["extra"] = json.dumps(db_extra)
                    if update_dict:
                        TenantModelService.update_model(existing_model_names[llm_name].id, update_dict)
                else:
                    extra_fields = {
                        "is_tools": llm.get("is_tools", False),
                        "thinking": "thinking" in llm.get("features", []),
                    }
                    if verify:
                        verify_status = model_verify_result.get(llm_name, ModelVerifyStatusEnum.UNKNOWN.value)
                        extra_fields["verify"] = verify_status
                    success, _msg = add_model_to_instance(
                        tenant_id, provider_name, effective_instance_name, **{"model_type": _factory_model_types(llm), "model_name": llm_name, "max_tokens": llm["max_tokens"], "extra": extra_fields}
                    )
                    if not success:
                        msg += _msg

    return True, "success"


async def create_provider_instance(tenant_id: str, provider_id_or_name: str, instance_name: str, api_key: str | dict, base_url: str, region: str, model_info: list[dict] = None):
    """
    Create a provider instance.

    The instance_name parameter is accepted for API compatibility but in the old
    model all records under a factory share the same API key configuration.

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider/factory ID or name
    :param instance_name: instance name (used as a logical identifier)
    :param api_key: API key
    :param base_url: base url
    :param region: region
    :param model_info: model info, [{
        "model_type": ["chat"],  # support multiple
        "model_name": "name",
        "max_tokens": 4096,
        "extra": {
            "field1": "value1",
            "field2": "'value2"
        }
    }]
    :return: (success, result_or_error_message)
    """
    if not provider_id_or_name:
        return False, "Provider ID or name is required"

    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"Provider '{provider_id_or_name}' does not exist"

    provider_name = provider_obj.provider_name

    base_url = _normalize_provider_base_url(provider_name, base_url)
    api_key = _normalize_provider_api_key(provider_name, api_key)

    if instance_name == "default":
        return False, "Instance name cannot be 'default'"

    # Check if provider exists in the system
    allowed_factories = [f["name"] for f in FACTORY_LLM_INFOS]
    if provider_name not in allowed_factories:
        return False, f"Provider '{provider_name}' is not allowed"

    api_key_str = ""
    if api_key:
        api_key_str = api_key if isinstance(api_key, str) else json.dumps(api_key)

    success, verify_msg, model_verify_result = await verify_api_key(provider_name, api_key, base_url, region, model_info)
    if not success:
        return False, verify_msg

    extra_fields = {}
    if base_url:
        extra_fields["base_url"] = base_url
    if region:
        extra_fields["region"] = region
    TenantModelInstanceService.create_instance(provider_id=provider_obj.id, instance_name=instance_name, api_key=api_key_str, extra=json.dumps(extra_fields))
    if model_info:
        msg = ""
        for model in model_info:
            if model.get("extra"):
                model["extra"].update({"verify": model_verify_result.get(model["model_name"], ModelVerifyStatusEnum.UNKNOWN.value)})
            else:
                model["extra"] = {"verify": model_verify_result.get(model["model_name"], ModelVerifyStatusEnum.UNKNOWN.value)}
            success, _msg = add_model_to_instance(tenant_id, provider_name, instance_name, **model)
            if not success:
                msg += _msg
        if msg:
            return False, msg
    else:
        msg = ""
        target_factory_name = "siliconflow_intl" if provider_name.lower() == "siliconflow" and region == "intl" else provider_name
        factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == target_factory_name]
        factory_llms = factory_info[0]["llm"]
        for llm in factory_llms:
            llm_name = _factory_llm_name(llm)
            success, _msg = add_model_to_instance(
                tenant_id,
                provider_name,
                instance_name,
                **{
                    "model_type": _factory_model_types(llm),
                    "model_name": llm_name,
                    "max_tokens": llm["max_tokens"],
                    "extra": {
                        "is_tools": llm.get("is_tools", False),
                        "thinking": "thinking" in llm.get("features", []),
                        "verify": model_verify_result.get(llm_name, ModelVerifyStatusEnum.UNKNOWN.value),
                    },
                },
            )
            if not success:
                msg += _msg
        if msg:
            return False, msg

    return True, "success"


async def create_name_only_provider_instance(tenant_id: str, provider_name: str, instance_name: str):
    """
    Create a provider instance with only a name (no api_key/base_url validation).

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name (used as a logical identifier)
    :return: (success, result_or_error_message)
    """
    if not provider_name:
        return False, "Provider name is required"

    if instance_name == "default":
        return False, "Instance name cannot be 'default'"

    allowed_factories = [f["name"] for f in FACTORY_LLM_INFOS]
    if provider_name not in allowed_factories:
        return False, f"Provider '{provider_name}' is not allowed"

    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"Provider '{provider_name}' does not exist"

    TenantModelInstanceService.create_instance(provider_id=provider_obj.id, instance_name=instance_name, api_key="", extra=json.dumps({}))
    return True, "success"


def list_provider_instances(tenant_id: str, provider_id_or_name: str):
    """
    List provider instances for a tenant.

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider/factory ID or name
    :return: (success, result_or_error_message)
    """
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_id_or_name}'"
    provider_id = provider_obj.id
    instance_objs = TenantModelInstanceService.get_all_by_provider_id(provider_id)
    if not instance_objs:
        return True, []
    instances = []
    instance_objs.sort(key=lambda x: x.create_time, reverse=True)
    for instance_obj in instance_objs:
        extra_fields = json.loads(instance_obj.extra) if instance_obj.extra else {}
        instances.append(
            {
                "id": instance_obj.id,
                "instance_name": instance_obj.instance_name,
                "provider_id": provider_id,
                "region": extra_fields.get("region", ""),
                "status": instance_obj.status,
            }
        )

    return True, instances


async def _run_verification(label: str, coro, timeout_seconds: int):
    """
    Run a verification coroutine with timeout and uniform error handling.

    Returns (True, result) on success, or (False, error_message) on failure.
    """
    try:
        result = await asyncio.wait_for(coro, timeout=timeout_seconds)
        return True, result
    except asyncio.TimeoutError:
        logging.exception("Timeout accessing %s", label)
        return False, f"\nTimeout accessing {label}."
    except asyncio.CancelledError:
        logging.exception("Verification cancelled for %s", label)
        return False, f"\n{label} verification aborted."
    except Exception as e:
        logging.exception("Fail to access %s", label)
        return False, f"\nFail to access {label}.{str(e)}"


async def verify_api_key(provider_id_or_name: str, api_key: str | dict, base_url: str = None, region: str = None, model_info: list[dict] = None):
    """
    Verify API key for a provider.

    :param provider_id_or_name: provider/factory ID or name
    :param api_key: API key
    :param base_url: base url
    :param region: region
    :param model_info: model info, [{
        "model_type": ["chat"],  # support multiple
        "model_name": "name",
        "max_tokens": 4096,
        "extra": {
            "field1": "value1",
            "field2": "'value2"
        }
    }]
    :return: (success, result_or_error_message)
    """
    if not provider_id_or_name:
        return False, "Provider ID or name is required", {}

    provider_obj = None
    if provider_id_or_name:
        _, provider_obj = TenantModelProviderService.get_by_id(provider_id_or_name)
    provider_name = provider_obj.provider_name if provider_obj else provider_id_or_name

    base_url = _normalize_provider_base_url(provider_name, base_url)
    api_key = _normalize_provider_api_key(provider_name, api_key)

    if region and region == "intl" and provider_name.lower() == "siliconflow":
        target_factory_name = "siliconflow_intl"
    else:
        target_factory_name = provider_name

    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == target_factory_name]
    if not factory_info:
        return False, f"Provider '{provider_id_or_name}' not found", {}

    if model_info:
        factory_llms = [
            {
                "model_type": _type,
                "llm_name": model.get("model_name", ""),
            }
            for model in model_info
            if model
            for _type in model.get("model_type", [])
        ]
        if not factory_llms:
            return False, f"No valid models found for provider '{provider_id_or_name}'", {}
    else:
        factory_llms = factory_info[0]["llm"]
        if not factory_llms:
            return False, f"No models found for provider '{provider_id_or_name}'", {}

    model_verify_result = {}
    # test if api key works
    timeout_seconds = int(os.environ.get("LLM_TIMEOUT_SECONDS", 10))
    extra = {"provider": provider_name}
    msg = ""
    if provider_name == "BaiduYiyan":
        if isinstance(api_key, str):
            try:
                json.loads(api_key)
            except (json.JSONDecodeError, TypeError):
                api_key = {"yiyan_ak": api_key, "yiyan_sk": ""}
    api_key_str = api_key if isinstance(api_key, str) else json.dumps(api_key)
    # check passed types
    passed_types = set()
    for llm in factory_llms:
        model_types = _factory_model_types(llm)
        any_passed = False
        for mt_value in model_types:
            if mt_value in passed_types:
                continue
            passed = False

            if mt_value == LLMType.EMBEDDING.value:
                if provider_name not in EmbeddingModel:
                    msg += f"\nEmbedding model from {provider_name} is not supported yet."
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                mdl = EmbeddingModel[provider_name](api_key_str, llm["llm_name"], base_url=base_url)
                label = f"embedding model({llm['llm_name']})"
                ok, result = await _run_verification(label, asyncio.to_thread(mdl.encode, ["Test if the api key is available"]), timeout_seconds)
                if not ok:
                    msg += result
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                if len(result[0]) == 0:
                    msg += f"\nFail to access {label}."
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                passed = True

            elif mt_value == LLMType.CHAT.value:
                if provider_name not in ChatModel:
                    msg += f"\nChat model from {provider_name} is not supported yet."
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                mdl = ChatModel[provider_name](api_key_str, llm["llm_name"], base_url=base_url, **extra)

                temperature = 1 if llm["llm_name"] in ("kimi-k3", "kimi-k2.7-code") else 0.9

                async def check_streamly():
                    async for chunk in mdl.async_chat_streamly(
                        None,
                        [{"role": "user", "content": "Hi"}],
                        {"temperature": temperature},
                    ):
                        if chunk and isinstance(chunk, str) and chunk.find("**ERROR**") < 0:
                            return True
                    return False

                label = f"model({provider_name}/{llm['llm_name']})"
                ok, result = await _run_verification(label, check_streamly(), timeout_seconds)
                if not ok:
                    msg += result
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                if not result:
                    msg += f"\nFail to access {label}.No valid response received"
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                passed = True

            elif mt_value == LLMType.RERANK.value:
                if provider_name not in RerankModel:
                    msg += f"\nRerank model from {provider_name} is not supported yet."
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                mdl = RerankModel[provider_name](api_key_str, llm["llm_name"], base_url=base_url)
                label = f"model({provider_name}/{llm['llm_name']})"
                ok, result = await _run_verification(label, asyncio.to_thread(mdl.similarity, "What's the weather?", ["Is it sunny today?"]), timeout_seconds)
                if not ok:
                    msg += result
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                arr, tc = result
                if len(arr) == 0 or tc == 0:
                    msg += f"\nFail to access {label}."
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                passed = True

            elif mt_value == LLMType.OCR.value:
                if provider_name not in OcrModel:
                    msg += f"\nOCR model from {provider_name} is not supported yet."
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                mdl = OcrModel[provider_name](key=api_key_str, model_name=llm["llm_name"], base_url=base_url)
                label = f"model({provider_name}/{llm['llm_name']})"
                ok, result = await _run_verification(label, asyncio.to_thread(mdl.check_available), timeout_seconds)
                if not ok:
                    msg += result
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                ok2, reason = result
                if not ok2:
                    msg += f"\nFail to access {label}.{reason or 'Model not available'}"
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                passed = True

            elif mt_value == LLMType.TTS.value:
                if provider_name not in TTSModel:
                    msg += f"\nTTS model from {provider_name} is not supported yet."
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                mdl = TTSModel[provider_name](key=api_key_str, model_name=llm["llm_name"], base_url=base_url)

                def drain_tts():
                    for _ in mdl.tts("Hello~ RAGFlower!"):
                        pass

                label = f"model({provider_name}/{llm['llm_name']})"
                ok, result = await _run_verification(label, asyncio.to_thread(drain_tts), timeout_seconds)
                if not ok:
                    msg += result
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                passed = True

            elif mt_value == LLMType.VISION.value:
                if provider_name not in CvModel:
                    msg += f"\nImage to text model from {provider_name} is not supported yet."
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                from rag.utils.base64_image import test_image

                mdl = CvModel[provider_name](key=api_key_str, model_name=llm["llm_name"], base_url=base_url)
                label = f"model({provider_name}/{llm['llm_name']})"
                ok, result = await _run_verification(label, asyncio.to_thread(mdl.describe, test_image), timeout_seconds)
                if not ok:
                    msg += result
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                m, tc = result
                if not tc and m.find("**ERROR**:") >= 0:
                    msg += f"\nFail to access {label}.{m}"
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                passed = True

            elif mt_value == LLMType.ASR.value:
                if provider_name not in Seq2txtModel:
                    msg += f"\nSpeech model from {provider_name} is not supported yet."
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                mdl = Seq2txtModel[provider_name](key=api_key_str, model_name=llm["llm_name"], base_url=base_url)
                label = f"model({provider_name}/{llm['llm_name']})"
                ok, result = await _run_verification(label, asyncio.to_thread(mdl.check_available), timeout_seconds)
                if not ok:
                    msg += result
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                ok2, reason = result
                if not ok2:
                    msg += f"\nFail to access {label}.{reason or 'Model not available'}"
                    model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
                    continue
                passed = True

            if passed:
                logging.debug("passed model %s type=%s", llm["llm_name"], mt_value)
                passed_types.add(mt_value)
                model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.SUCCESS.value
                any_passed = True
                break
            else:
                model_verify_result[llm["llm_name"]] = ModelVerifyStatusEnum.FAIL.value
        if any_passed:
            msg = ""

    success = bool(passed_types)
    return success, "success" if success else msg, model_verify_result


def show_provider_instance(tenant_id: str, provider_id_or_name: str, instance_id_or_name: str):
    """
    Show a specific provider instance.

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider/factory ID or name
    :param instance_id_or_name: instance ID or name
    :return: (success, result_or_error_message)
    """
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_id_or_name}'"
    provider_id = provider_obj.id
    instance_obj = None
    if instance_id_or_name:
        _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
    if instance_obj and instance_obj.provider_id != provider_id:
        instance_obj = None
    if not instance_obj:
        instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_id, instance_id_or_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"

    extra_fields = json.loads(instance_obj.extra) if instance_obj.extra else {}

    return True, {
        "id": instance_obj.id,
        "instance_name": instance_obj.instance_name,
        "provider_id": provider_id,
        "region": extra_fields.get("region", ""),
        "base_url": extra_fields.get("base_url", ""),
        "api_key": instance_obj.api_key,
        "status": instance_obj.status,
    }


def drop_provider_instances(tenant_id: str, provider_id_or_name: str, instance_id_or_names: list):
    """
    Drop provider instances.
    for the specified models/instances.

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider/factory ID or name
    :param instance_id_or_names: list of instance IDs or names to drop
    :return: (success, result_or_error_message)
    """
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_id_or_name}'"
    provider_id = provider_obj.id
    not_exist_instances = []
    instance_ids = []
    for instance_id_or_name in instance_id_or_names:
        instance_obj = None
        if instance_id_or_name:
            _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
        if instance_obj and instance_obj.provider_id != provider_id:
            instance_obj = None
        if not instance_obj:
            instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_id, instance_id_or_name)
        if not instance_obj:
            not_exist_instances.append(instance_id_or_name)
            continue
        instance_ids.append(instance_obj.id)
    if not_exist_instances:
        return False, f"No instance found for provider '{provider_id_or_name}' and instance '{not_exist_instances}'"
    delete_models_by_instance_ids(instance_ids)
    TenantModelInstanceService.delete_by_ids(instance_ids)
    return True, None


def list_instance_models(tenant_id: str, provider_id_or_name: str, instance_id_or_name: str, supported_only: bool = False):
    """
    List models for a provider instance.

    Follows the Go version's logic:
    - Reads tenant_model table to determine disabled models (records exist = disabled).
    - Lists all models from the LLM dictionary for the provider.
    - Models present in tenant_model table are marked "inactive", others "active".

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider/factory ID or name
    :param instance_id_or_name: instance ID or name
    :param supported_only: if True, only list supported models (from LLM dictionary)
    :return: (success, result_or_error_message)
    """
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_id_or_name}'"

    if supported_only:
        # List all models supported by this provider from the LLM dictionary
        factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_obj.provider_name]
        if not factory_info:
            return False, f"Provider '{provider_id_or_name}' not found"
        llms = factory_info[0].get("llm", [])
        models = [{"name": llm["llm_name"], "rank": _to_int(llm.get("rank", 500))} for llm in llms]
        models.sort(key=lambda x: (-x["rank"], x["name"]))
        return True, models

    # Build rank mapping from LLM dictionary for the provider
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_obj.provider_name]
    model_rank_map = {}
    if factory_info:
        for llm in factory_info[0].get("llm", []):
            model_rank_map[llm["llm_name"]] = _to_int(llm.get("rank", 500))

    # Get instance
    instance_obj = None
    if instance_id_or_name:
        _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
    if instance_obj and instance_obj.provider_id != provider_obj.id:
        instance_obj = None
    if not instance_obj:
        instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_id_or_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"
    # Get models
    model_objs = TenantModelService.get_models_by_instance_id(instance_obj.id)
    model_list = []
    for model in model_objs:
        model_extra = json.loads(model.extra)
        model_list.append(
            {
                "name": model.model_name,
                "model_type": get_model_type_human(model.model_type),
                "max_tokens": model_extra.get("max_tokens", 8192) if model.extra else 8192,
                "status": model.status,
                "verify": model_extra.get("verify", ModelVerifyStatusEnum.UNKNOWN.value),
                "features": (["is_tools"] if model_extra.get("is_tools") else []) + (["thinking"] if model_extra.get("thinking") else []),
                "rank": model_rank_map.get(model.model_name, 500),
            }
        )
    model_list.sort(key=lambda x: (-x["rank"], x["name"]))

    return True, model_list


def update_instance_models(tenant_id: str, provider_id_or_name: str, instance_id_or_name: str, model_names: list, model_types: list):
    if not model_names or not model_types:
        return False, "model_name and model_type are required"

    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_id_or_name}'"
    instance_obj = None
    if instance_id_or_name:
        _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
    if instance_obj and instance_obj.provider_id != provider_obj.id:
        instance_obj = None
    if not instance_obj:
        instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_id_or_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"

    model_objs = TenantModelService.get_models_by_instance_id(instance_obj.id)
    not_exist_models = set(model_names) - {model_obj.model_name for model_obj in model_objs}
    if not_exist_models:
        return False, f"Models {not_exist_models} not found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"

    target_model_type_bin = calculate_model_type(model_types)
    to_update = [model_obj.id for model_obj in model_objs if model_obj.model_type != target_model_type_bin and model_obj.model_name in model_names]
    if to_update:
        TenantModelService.batch_update_model_type(to_update, target_model_type_bin)

    return True, "success"


def add_model_to_instance(tenant_id: str, provider_id_or_name: str, instance_id_or_name: str, model_name: str, model_type: str | list[str], max_tokens: int = 8192, extra: dict = None):
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_id_or_name}'"
    instance_obj = None
    if instance_id_or_name:
        _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
    if instance_obj and instance_obj.provider_id != provider_obj.id:
        instance_obj = None
    if not instance_obj:
        instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_id_or_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"
    model_obj = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(provider_obj.id, instance_obj.id, model_name)
    if model_obj:
        return False, f"Model '{model_name}' already exists for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_obj.provider_name]
    if not factory_info:
        return False, f"Provider '{provider_id_or_name}' not found"
    llms = factory_info[0].get("llm", [])
    if isinstance(model_type, str):
        model_type = [model_type]

    model_type_bin = calculate_model_type(model_type)
    extra_fields = {"max_tokens": max_tokens}
    target_model = [llm for llm in llms if llm["llm_name"] == model_name]
    if target_model:
        extra_fields.update({"is_tools": target_model[0].get("is_tools", False)})
        extra_fields.update({"thinking": "thinking" in target_model[0].get("features", [])})
    if extra:
        extra_fields.update(extra)
    TenantModelService.insert(model_name=model_name, provider_id=provider_obj.id, instance_id=instance_obj.id, model_type=model_type_bin, extra=json.dumps(extra_fields))

    return True, "success"


def update_model(tenant_id: str, provider_id_or_name: str, instance_id_or_name: str, model_name: str, update_dict: dict):
    """
    Enable or disable a model for a provider instance.

    - If the model record exists in tenant_model, update its status.
    - If the model record does not exist:
      - status="active": no need to add a record (default is active/enabled).
      - status="inactive": create a record with status="inactive".

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider/factory ID or name
    :param instance_id_or_name: instance ID or name
    :param model_name: model name
    :param update_dict:
        status: "active" or "inactive" (ActiveStatusEnum values)
        max_tokens: > 0
    :return: (success, result_or_error_message)
    """
    if update_dict.get("status") and update_dict["status"] not in (ActiveStatusEnum.ACTIVE.value, ActiveStatusEnum.INACTIVE.value):
        return False, f"status must be '{ActiveStatusEnum.ACTIVE.value}' or '{ActiveStatusEnum.INACTIVE.value}'"

    # Check if provider exists for this tenant
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_id_or_name}'"

    # Check if instance exists
    instance_obj = None
    if instance_id_or_name:
        _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
    if instance_obj and instance_obj.provider_id != provider_obj.id:
        instance_obj = None
    if not instance_obj:
        instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_id_or_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"

    model_obj = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(provider_obj.id, instance_obj.id, model_name)
    to_update = {}
    if "status" in update_dict and update_dict.get("status") != model_obj.status:
        to_update.update({"status": update_dict["status"]})
    new_extra = update_dict.get("extra", {})
    if "max_tokens" in update_dict:
        new_extra.update({"max_tokens": update_dict["max_tokens"]})
    if "verify" in update_dict:
        new_extra.update({"verify": update_dict["verify"]})
    if new_extra:
        db_extra = json.loads(model_obj.extra)
        db_extra.update(**new_extra)
        to_update.update({"extra": json.dumps(db_extra)})
    if "model_type" in update_dict:
        target_model_type = calculate_model_type(update_dict["model_type"])
        if target_model_type != model_obj.model_type:
            to_update.update({"model_type": target_model_type})

    if to_update:
        TenantModelService.update_model(model_obj.id, to_update)

    return True, "success"


async def delete_models_from_instance(tenant_id: str, provider_id_or_name: str, instance_id_or_name: str, model_name: list[str]):
    """
    Delete models from instance.

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider/factory ID or name
    :param instance_id_or_name: instance ID or name
    :param model_name: list of model name
    """
    # Check if provider exists for this tenant (by ID first, then by name)
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_id_or_name}'"

    # Check if instance exists (by ID first, then by name)
    instance_obj = None
    if instance_id_or_name:
        _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
    if instance_obj and instance_obj.provider_id != provider_obj.id:
        instance_obj = None
    if not instance_obj:
        instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_id_or_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"

    model_objs = TenantModelService.get_models_by_instance_id(instance_obj.id)
    not_exist_models = set(model_name) - {model_obj.model_name for model_obj in model_objs}
    if not_exist_models:
        return False, f"Models {not_exist_models} not found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"

    TenantModelService.delete_by_ids([model_obj.id for model_obj in model_objs if model_obj.model_name in model_name])

    return True, "success"


async def chat_to_model(tenant_id: str, provider_id_or_name: str, instance_id_or_name: str, model_name: str, message: str, stream: bool = False, thinking: bool = False):
    """
    Chat to a model.

    :param tenant_id: tenant ID
    :param provider_id_or_name: provider/factory ID or name
    :param instance_id_or_name: instance ID or name
    :param model_name: model name
    :param message: chat message
    :param stream: whether to stream the response
    :param thinking: whether to enable thinking/reasoning
    :return: (success, result_or_error_message)
    """
    from api.db.services.llm_service import LLMBundle

    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_id(tenant_id, provider_id_or_name)
    if not provider_obj:
        provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_id_or_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_id_or_name}'"

    instance_obj = None
    if instance_id_or_name:
        _, instance_obj = TenantModelInstanceService.get_by_id(instance_id_or_name)
    if instance_obj and instance_obj.provider_id != provider_obj.id:
        instance_obj = None
    if not instance_obj:
        instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_id_or_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_id_or_name}' and instance '{instance_id_or_name}'"

    provider_name = provider_obj.provider_name
    instance_name = instance_obj.instance_name

    # Get model config
    composite_name = f"{model_name}@{instance_name}@{provider_name}"
    try:
        model_config = resolve_model_config(tenant_id, LLMType.CHAT, composite_name)
    except LookupError:
        return False, f"Model '{composite_name}' not authorized"

    if not model_config:
        return False, f"Model '{composite_name}' not found"

    llm = LLMBundle(tenant_id, model_config)

    if stream:
        return True, {"type": "stream", "llm": llm, "model_config": model_config}

    # Non-streaming chat
    try:
        response = await llm.async_chat(
            None,
            [{"role": "user", "content": message}],
            {"temperature": 0.9},
        )
        result = {
            "answer": response,
            "reasoning_content": "",
        }
        return True, result
    except Exception as e:
        logging.exception(f"Chat to model failed: {e}")
        return False, str(e)
