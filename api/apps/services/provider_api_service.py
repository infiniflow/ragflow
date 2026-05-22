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

from common.constants import LLMType, ActiveStatusEnum
from common.misc_utils import get_uuid
from common.settings import FACTORY_LLM_INFOS
from api.db.joint_services.tenant_model_service import get_model_config_from_provider_instance, delete_models_by_instance_ids, delete_instances_by_provider_ids
from api.db.services.tenant_model_provider_service import TenantModelProviderService
from api.db.services.tenant_model_instance_service import TenantModelInstanceService
from api.db.services.tenant_model_service import TenantModelService


def list_providers(tenant_id: str, all_available: bool = False):
    """
    List providers for a tenant.

    If available_only is True, list all system-wide providers (pool providers).
    Otherwise, list providers that the tenant has configured.

    :param tenant_id: tenant ID
    :param all_available: whether to list all available providers
    :return: (success, result)
    """
    if not FACTORY_LLM_INFOS:
        return False, []

    if all_available:
        providers = []
        for factory_info in FACTORY_LLM_INFOS:
            model_types = sorted(set(
                llm["model_type"]
                for llm in factory_info.get("llm", [])
                if llm.get("model_type")
            ))
            providers.append({
                "model_types": model_types,
                "name": factory_info["name"],
                "url": {
                    "default": factory_info.get("url", "")
                }
            })
        return True, providers

    # List tenant-configured providers
    factory_names = TenantModelProviderService.list_provider_names_by_tenant_id(tenant_id)

    providers = []
    factory_info_mapping = {f["name"]: f for f in FACTORY_LLM_INFOS}
    for name in factory_names:
        if factory_info_mapping.get(name):
            factory_info = factory_info_mapping[name]
            model_types = sorted(set(
                llm["model_type"]
                for llm in factory_info.get("llm", [])
                if llm.get("model_type")
            ))
            providers.append({
                "model_types": model_types,
                "name": factory_info["name"],
                "url": {
                    "default": factory_info.get("url", "")
                }
            })

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

    TenantModelProviderService.insert(
        tenant_id=tenant_id,
        provider_name=provider_name
    )
    return True, "success"


def delete_provider(tenant_id: str, provider_name: str):
    """
    Delete all instances and models for a provider.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :return: (success, result_or_error_message)
    """
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"Provider {provider_name} not found"
    instance_objs = TenantModelInstanceService.get_by_provider_id(provider_obj.id)
    if not instance_objs:
        return False, f"No instances found for provider {provider_name}"
    instance_ids = [instance_obj.id for instance_obj in instance_objs]
    delete_models_by_instance_ids(instance_ids)
    delete_instances_by_provider_ids([provider_obj.id])
    TenantModelProviderService.delete_by_tenant_id_and_provider_name(tenant_id, provider_name)
    return True, "success"


def show_provider(provider_name: str):
    """
    Show provider details from LLMFactories.

    :param provider_name: provider/factory name
    :return: (success, result_or_error_message)
    """
    fac_list = [f for f in FACTORY_LLM_INFOS if f["name"]==provider_name]
    if not fac_list:
        return False, f"Provider '{provider_name}' not found"
    factory_info = fac_list[0]
    return True, {
        "base_url": {
            "default": factory_info.get("url", "")
        },
        "name": factory_info["name"],
        "total_models": len(factory_info.get("llm", []))
    }


def list_provider_models(provider_name: str):
    """
    List all models for a provider from the LLM dictionary.

    :param provider_name: provider/factory name
    :return: (success, result_or_error_message)
    """
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"]==provider_name]
    if not factory_info:
        return False, f"Provider '{provider_name}' not found"
    llms = factory_info[0]["llm"]
    if not llms:
        return False, f"No models found for provider '{provider_name}'"

    models = []
    for llm in llms:
        models.append({
            "name": llm["name"],
            "max_tokens": llm["max_tokens"],
            "model_types": [llm["model_type"]],
            "features": None
        })
    return True, models


def show_provider_model(provider_name: str, model_name: str):
    """
    Show a specific model for a provider.

    :param provider_name: provider/factory name
    :param model_name: model name
    :return: (success, result_or_error_message)
    """
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
    if not factory_info:
        return False, f"Provider '{provider_name}' not found"
    llms = factory_info[0]["llm"]
    if not llms:
        return False, f"No models found for provider '{provider_name}'"
    target_llm = [llm for llm in llms if llm["name"] == model_name]
    if not target_llm:
        return False, f"Model '{model_name}' not found"
    llm_info = target_llm[0]

    return True, {
        "name": llm_info["name"],
        "max_tokens": llm_info["max_tokens"],
        "model_types": [llm_info["model_type"]],
        "thinking": None,
        "model_type_map": {
            llm_info["model_type"]: True
        }
    }


def create_provider_instance(tenant_id: str, provider_name: str, instance_name: str, api_key: str, base_url: str, region: str):
    """
    Create a provider instance.

    The instance_name parameter is accepted for API compatibility but in the old
    model all records under a factory share the same API key configuration.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name (used as a logical identifier)
    :param api_key: API key
    :param base_url: base url
    :param region: region
    :return: (success, result_or_error_message)
    """
    if not provider_name:
        return False, "Provider name is required"

    if instance_name == "default":
        return False, "Instance name cannot be 'default'"

    # Check if provider exists in the system
    allowed_factories = [f["name"] for f in FACTORY_LLM_INFOS]
    if provider_name not in allowed_factories:
        return False, f"Provider '{provider_name}' is not allowed"

    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"Provider '{provider_name}' does not exist"

    if api_key:
        same_key_instance = TenantModelInstanceService.get_by_provider_id_and_api_key(provider_obj.id, api_key)
        if same_key_instance:
            return False, f"Already exist instance: {same_key_instance.instance_name} with api_key {api_key}"

    import json
    extra_fields = {}
    if base_url:
        extra_fields["base_url"] = base_url
    if region:
        extra_fields["region"] = region
    TenantModelInstanceService.create_instance(provider_id=provider_obj.id,instance_name=instance_name,api_key=api_key, extra=json.dumps(extra_fields))

    return True, "success"


def list_provider_instances(tenant_id: str, provider_name: str):
    """
    List provider instances for a tenant.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :return: (success, result_or_error_message)
    """
    import json
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_name}'"
    provider_id = provider_obj.id
    instance_objs = TenantModelInstanceService.get_all_by_provider_id(provider_id)
    if not instance_objs:
        return True, []
    instances = []
    for instance_obj in instance_objs:
        extra_fields = json.loads(instance_obj.extra) if instance_obj.extra else {}
        instances.append({
            "api_key": instance_obj.api_key,
            "id": instance_obj.id,
            "instance_name": instance_obj.instance_name,
            "provider_id": provider_id,
            "region": extra_fields.get("region", ""),
            "status": instance_obj.status,
        })

    return True, instances


def show_provider_instance(tenant_id: str, provider_name: str, instance_name: str):
    """
    Show a specific provider instance.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name
    :return: (success, result_or_error_message)
    """
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_name}'"
    provider_id = provider_obj.id
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_id, instance_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_name}' and instance '{instance_name}'"

    import json
    extra_fields = json.loads(instance_obj.extra) if instance_obj.extra else {}
    return True, {
        "id": instance_obj.id,
        "instance_name": instance_obj.instance_name,
        "provider_id": provider_id,
        "region": extra_fields.get("region", ""),
        "status": instance_obj.status
    }


def drop_provider_instances(tenant_id: str, provider_name: str, instance_names: list):
    """
    Drop provider instances.
    for the specified models/instances.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_names: list of instance names to drop
    :return: (success, result_or_error_message)
    """
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_name}'"
    provider_id = provider_obj.id
    not_exist_instances = []
    instance_ids = []
    for instance_name in instance_names:
        instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_id, instance_name)
        if not instance_obj:
            not_exist_instances.append(instance_name)
            continue
        instance_ids.append(instance_obj.id)
    if not_exist_instances:
        return False, f"No instance found for provider '{provider_name}' and instance '{not_exist_instances}'"
    delete_models_by_instance_ids(instance_ids)
    TenantModelInstanceService.delete_by_ids(instance_ids)
    return True, None


def list_instance_models(tenant_id: str, provider_name: str, instance_name: str, supported_only: bool = False):
    """
    List models for a provider instance.

    Follows the Go version's logic:
    - Reads tenant_model table to determine disabled models (records exist = disabled).
    - Lists all models from the LLM dictionary for the provider.
    - Models present in tenant_model table are marked "inactive", others "active".

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name
    :param supported_only: if True, only list supported models (from LLM dictionary)
    :return: (success, result_or_error_message)
    """
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_name}'"

    if supported_only:
        # List all models supported by this provider from the LLM dictionary
        factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
        if not factory_info:
            return False, f"Provider '{provider_name}' not found"
        llms = factory_info[0].get("llm", [])
        models = [{"name": llm["llm_name"]} for llm in llms]
        return True, models

    # Get instance
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_name}' and instance '{instance_name}'"

    # Get model records for this instance from tenant_model table
    model_records = TenantModelService.get_models_by_instance_id(instance_obj.id)
    # Build a map of model_name -> status, type
    model_info_map: dict = {}
    for model_record in model_records:
        if model_info_map.get(model_record.model_name):
            model_info_map[model_record.model_name]["model_type"].append(model_record.model_type)
        else:
            model_info_map[model_record.model_name] = {
                "status": model_record.status,
                "model_type": [model_record.model_type],
                "extra": model_record.extra
            }

    # List all models from the LLM dictionary for this provider
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
    if not factory_info:
        return False, f"Provider '{provider_name}' not found"

    llms = factory_info[0].get("llm", [])
    models = []
    for llm in llms:
        models.append({
            "name": llm["llm_name"],
            "model_type": [llm["model_type"]] + model_info_map.get(llm["llm_name"], {}).get("model_type", []),
            "max_tokens": llm.get("max_tokens"),
            "status": model_info_map.get(llm["llm_name"], {}).get("status", "active"),
        })
    factory_models = [m["name"] for m in models]
    for model_name, model_info_dict in model_info_map.items():
        if model_name not in factory_models:
            extra_fields = json.loads(model_info_dict["extra"]) if model_info_dict["extra"] else {}
            models.append({
                "name": model_name,
                "model_type": model_info_dict["model_type"],
                "max_tokens": extra_fields.get("max_tokens", 8192),
                "status": model_info_dict["status"],
            })

    return True, models


def add_model_to_instance(tenant_id: str, provider_name: str, instance_name: str, model_name: str, model_type: str|list[str], max_tokens: int, extra: dict):
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_name}'"
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_name}' and instance '{instance_name}'"
    model_obj = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(provider_obj.id, instance_obj.id, model_name)
    if model_obj:
        return False, f"Model '{model_name}' already exists for provider '{provider_name}' and instance '{instance_name}'"
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
    if not factory_info:
        return False, f"Provider '{provider_name}' not found"
    llms = factory_info[0].get("llm", [])
    if isinstance(model_type, str):
        model_type = [model_type]

    import json

    for _type in model_type:
        extra_fields = {"max_tokens": max_tokens}
        target_model = [llm for llm in llms if llm["model_type"] == _type and llm["llm_name"] == model_name]
        if target_model:
            extra_fields.update({"is_tool": target_model[0].get("is_tool", False)})
        extra_fields.update(extra)
        TenantModelService.insert(
            model_name=model_name,
            provider_id=provider_obj.id,
            instance_id=instance_obj.id,
            model_type=_type,
            extra=json.dumps(extra_fields)
        )

    return True, "success"


def update_model_status(tenant_id: str, provider_name: str, instance_name: str, model_name: str, status: str):
    """
    Enable or disable a model for a provider instance.

    - If the model record exists in tenant_model, update its status.
    - If the model record does not exist:
      - status="active": no need to add a record (default is active/enabled).
      - status="inactive": create a record with status="inactive".

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name
    :param model_name: model name
    :param status: "active" or "inactive" (ActiveStatusEnum values)
    :return: (success, result_or_error_message)
    """
    if status not in (ActiveStatusEnum.ACTIVE.value, ActiveStatusEnum.INACTIVE.value):
        return False, f"status must be '{ActiveStatusEnum.ACTIVE.value}' or '{ActiveStatusEnum.INACTIVE.value}'"

    # Check if provider exists for this tenant
    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_name}'"

    # Check if instance exists
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_name}' and instance '{instance_name}'"

    # Check if model record already exists in tenant_model table
    model_obj_list = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(
        provider_obj.id, instance_obj.id, model_name
    )

    if model_obj_list:
        # Model record exists — update its status
        TenantModelService.batch_update_model_status([m.id for m in model_obj_list], status)
    else:
        # Model record does not exist
        if status == ActiveStatusEnum.ACTIVE.value:
            # Default is active, no need to add a record
            return True, None
        # status is "inactive" — create a record with inactive status
        # Look up model schema from FACTORY_LLM_INFOS
        factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
        if not factory_info:
            return False, f"Provider '{provider_name}' not found"
        llms = factory_info[0].get("llm", [])
        target_llm = [llm for llm in llms if llm["llm_name"] == model_name]
        if not target_llm:
            return False, f"provider {provider_name} model {model_name} not found"

        TenantModelService.insert(
            id=get_uuid(),
            model_name=model_name,
            model_type=target_llm[0]["model_type"],
            provider_id=provider_obj.id,
            instance_id=instance_obj.id,
            status=status,
        )

    return True, None


async def chat_to_model(tenant_id: str, provider_name: str, instance_name: str, model_name: str, message: str, stream: bool = False, thinking: bool = False):
    """
    Chat to a model.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name
    :param model_name: model name
    :param message: chat message
    :param stream: whether to stream the response
    :param thinking: whether to enable thinking/reasoning
    :return: (success, result_or_error_message)
    """
    from api.db.services.llm_service import LLMBundle

    # Get model config
    composite_name = f"{model_name}@{instance_name}@{provider_name}"
    try:
        model_config = get_model_config_from_provider_instance(tenant_id, LLMType.CHAT.value, composite_name)
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
