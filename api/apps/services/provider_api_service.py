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

from common.constants import LLMType, StatusEnum
from common.settings import FACTORY_LLM_INFOS
from api.db.db_models import TenantLLM
from api.db.services.llm_service import LLMService
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.tenant_model_provider_service import TenantModelProviderService
from api.utils.api_utils import get_allowed_llm_factories


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
            factory_info = factory_info_mapping["name"]
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
    Add a provider (factory) for a tenant by creating TenantLLM entries

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

    try:
        TenantModelProviderService.insert(
            tenant_id=tenant_id,
            provider_name=provider_name
        )
        return True, "success"
    except Exception as e:
        logging.exception(str(e))
        return False, "Failed to add provider"


def delete_provider(tenant_id: str, provider_name: str):
    """
    Delete all TenantLLM entries for a provider.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :return: (success, result_or_error_message)
    """
    TenantLLMService.filter_delete(
        [TenantLLM.tenant_id == tenant_id, TenantLLM.llm_factory == provider_name]
    )
    return True, None


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
    llms = LLMService.query(fid=provider_name)
    if not llms:
        return False, f"No models found for provider '{provider_name}'"

    models = []
    for llm in llms:
        if llm.status != StatusEnum.VALID.value:
            continue
        models.append(llm.to_dict())
    return True, models


def show_provider_model(provider_name: str, model_name: str):
    """
    Show a specific model for a provider.

    :param provider_name: provider/factory name
    :param model_name: model name
    :return: (success, result_or_error_message)
    """
    llms = LLMService.query(fid=provider_name, llm_name=model_name)
    if not llms:
        return False, f"Model '{model_name}' not found for provider '{provider_name}'"
    return True, llms[0].to_dict()


def create_provider_instance(tenant_id: str, provider_name: str, instance_name: str, api_key: str, api_base: str = ""):
    """
    Create a provider instance. In the old model, this maps to creating/updating
    TenantLLM entries for the provider with the given API key.

    The instance_name parameter is accepted for API compatibility but in the old
    model all records under a factory share the same API key configuration.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name (used as a logical identifier)
    :param api_key: API key
    :param api_base: API base URL
    :return: (success, result_or_error_message)
    """
    if instance_name == "default":
        return False, "Instance name cannot be 'default'"

    # Check if provider exists in the system
    allowed_factories = [f.name for f in get_allowed_llm_factories()]
    if provider_name not in allowed_factories:
        return False, f"Provider '{provider_name}' is not allowed"

    # Get all models for this factory
    llms = LLMService.query(fid=provider_name)
    if not llms:
        return False, f"No models found for provider '{provider_name}'"

    # Create/update TenantLLM entries with the api_key
    for llm in llms:
        llm_config = {
            "api_key": api_key,
            "api_base": api_base,
            "max_tokens": llm.max_tokens,
        }
        if not TenantLLMService.filter_update(
            [TenantLLM.tenant_id == tenant_id, TenantLLM.llm_factory == provider_name, TenantLLM.llm_name == llm.llm_name],
            llm_config,
        ):
            TenantLLMService.save(
                tenant_id=tenant_id,
                llm_factory=provider_name,
                llm_name=llm.llm_name,
                model_type=llm.model_type,
                api_key=api_key,
                api_base=api_base,
                max_tokens=llm.max_tokens,
            )

    return True, None


def list_provider_instances(tenant_id: str, provider_name: str):
    """
    List provider instances for a tenant. In the old model, instances map to
    unique API key configurations under a provider.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :return: (success, result_or_error_message)
    """
    objs = TenantLLMService.query(tenant_id=tenant_id, llm_factory=provider_name)
    if not objs:
        return True, []

    # Group by api_key to represent instances
    instances_by_key = {}
    for o in objs:
        key = o.api_key or ""
        if key not in instances_by_key:
            instances_by_key[key] = {
                "instance_name": "default" if not key else f"instance_{len(instances_by_key)}",
                "provider_name": provider_name,
                "api_key": key,
                "status": "enable",
            }
        # Update status if any model is valid
        if o.status == StatusEnum.VALID.value:
            instances_by_key[key]["status"] = "enable"

    return True, list(instances_by_key.values())


def show_provider_instance(tenant_id: str, provider_name: str, instance_name: str):
    """
    Show a specific provider instance.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name
    :return: (success, result_or_error_message)
    """
    objs = TenantLLMService.query(tenant_id=tenant_id, llm_factory=provider_name)
    if not objs:
        return False, f"No instances found for provider '{provider_name}'"

    # In the old model, return the default instance
    first_obj = objs[0]
    result = {
        "instance_name": instance_name,
        "provider_name": provider_name,
        "api_key": first_obj.api_key,
        "api_base": first_obj.api_base or "",
        "status": "enable" if first_obj.status == StatusEnum.VALID.value else "disable",
    }
    return True, result


def show_instance_balance(tenant_id: str, provider_name: str, instance_name: str):
    """
    Show instance balance. This is not directly supported in the old model.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name
    :return: (success, result_or_error_message)
    """
    objs = TenantLLMService.query(tenant_id=tenant_id, llm_factory=provider_name)
    if not objs:
        return False, f"No instances found for provider '{provider_name}'"

    # Return total used tokens as a balance proxy
    total_used = sum(o.used_tokens or 0 for o in objs)
    return True, {"used_tokens": total_used, "message": "Balance check not supported for this provider"}


def check_provider_connection(tenant_id: str, provider_name: str, instance_name: str):
    """
    Check provider connection by verifying the API key works.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name
    :return: (success, result_or_error_message)
    """
    objs = TenantLLMService.query(tenant_id=tenant_id, llm_factory=provider_name)
    if not objs:
        return False, f"No instances found for provider '{provider_name}'"

    # Find a valid model to test with
    from rag.llm import ChatModel, EmbeddingModel

    api_key = None
    base_url = ""
    test_model_name = None
    test_model_type = None

    for o in objs:
        if o.api_key and o.status == StatusEnum.VALID.value:
            api_key = o.api_key
            base_url = o.api_base or ""
            test_model_name = o.llm_name
            test_model_type = o.model_type
            break

    if not api_key:
        return False, "No valid API key found for this provider"

    # Try to instantiate and test the model
    try:
        if test_model_type == LLMType.EMBEDDING.value:
            if provider_name in EmbeddingModel:
                mdl = EmbeddingModel[provider_name](api_key, test_model_name, base_url=base_url)
                arr, _ = mdl.encode(["test"])
                if len(arr[0]) == 0:
                    return False, "Connection test failed: empty embedding result"
        elif test_model_type == LLMType.CHAT.value:
            if provider_name in ChatModel:
                mdl = ChatModel[provider_name](api_key, test_model_name, base_url=base_url, provider=provider_name)
                # Basic instantiation check
        else:
            # For other types, just check the API key exists
            pass
    except Exception as e:
        return False, f"Connection test failed: {str(e)}"

    return True, None


def alter_provider_instance(tenant_id: str, provider_name: str, instance_name: str, llm_name: str = None, api_key: str = None, api_base: str = None):
    """
    Alter a provider instance by updating TenantLLM entries.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name
    :param llm_name: new model name (optional)
    :param api_key: new API key (optional)
    :param api_base: new API base URL (optional)
    :return: (success, result_or_error_message)
    """
    update_fields = {}
    if api_key is not None:
        update_fields["api_key"] = api_key
    if api_base is not None:
        update_fields["api_base"] = api_base

    if not update_fields:
        return True, None

    conditions = [TenantLLM.tenant_id == tenant_id, TenantLLM.llm_factory == provider_name]
    if llm_name:
        conditions.append(TenantLLM.llm_name == llm_name)

    TenantLLMService.filter_update(conditions, update_fields)
    return True, None


def drop_provider_instances(tenant_id: str, provider_name: str, instance_names: list):
    """
    Drop provider instances. In the old model, this deletes TenantLLM entries
    for the specified models/instances.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_names: list of instance names to drop
    :return: (success, result_or_error_message)
    """
    for instance_name in instance_names:
        TenantLLMService.filter_delete(
            [TenantLLM.tenant_id == tenant_id, TenantLLM.llm_factory == provider_name, TenantLLM.llm_name == instance_name]
        )
    return True, None


def list_instance_models(tenant_id: str, provider_name: str, instance_name: str, supported_only: bool = False):
    """
    List models for a provider instance.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param instance_name: instance name
    :param supported_only: if True, only list supported models (from LLM dictionary)
    :return: (success, result_or_error_message)
    """
    if supported_only:
        # List all models supported by this provider from the LLM dictionary
        llms = LLMService.query(fid=provider_name)
        models = []
        for llm in llms:
            if llm.status == StatusEnum.VALID.value:
                models.append({"model_name": llm.llm_name})
        return True, models

    # List instance models with their enabled/disabled status
    llms = LLMService.query(fid=provider_name)
    tenant_objs = TenantLLMService.query(tenant_id=tenant_id, llm_factory=provider_name)
    # Build a set of model names that the tenant has configured
    tenant_model_names = {o.llm_name for o in tenant_objs if o.status == StatusEnum.VALID.value}

    models = []
    for llm in llms:
        if llm.status != StatusEnum.VALID.value:
            continue
        m_dict = llm.to_dict()
        m_dict["status"] = "enabled" if llm.llm_name in tenant_model_names else "disabled"
        models.append(m_dict)

    # Also include tenant models not in the LLM dictionary
    llm_model_names = {llm.llm_name for llm in llms}
    for o in tenant_objs:
        if o.llm_name not in llm_model_names:
            models.append({
                "llm_name": o.llm_name,
                "model_type": o.model_type,
                "fid": provider_name,
                "status": "enabled" if o.status == StatusEnum.VALID.value else "disabled",
            })

    return True, models


def update_model_status(tenant_id: str, provider_name: str, model_name: str, status: str):
    """
    Enable or disable a model for a provider instance.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param model_name: model name
    :param status: "enabled" or "disabled"
    :return: (success, result_or_error_message)
    """
    # Map status string to TenantLLM status value
    status_value = StatusEnum.VALID.value if status == "enabled" else StatusEnum.INVALID.value

    obj = TenantLLMService.get_or_none(
        tenant_id=tenant_id, llm_factory=provider_name, llm_name=model_name
    )
    if obj:
        TenantLLMService.filter_update(
            [TenantLLM.tenant_id == tenant_id, TenantLLM.llm_factory == provider_name, TenantLLM.llm_name == model_name],
            {"status": status_value},
        )
    else:
        # Model doesn't exist for this tenant yet; create it if enabling
        if status == "enabled":
            llm = LLMService.query(fid=provider_name, llm_name=model_name)
            if not llm:
                return False, f"Model '{model_name}' not found for provider '{provider_name}'"
            TenantLLMService.save(
                tenant_id=tenant_id,
                llm_factory=provider_name,
                llm_name=model_name,
                model_type=llm[0].model_type,
                api_key="",
                api_base="",
                max_tokens=llm[0].max_tokens,
                status=status_value,
            )
    return True, None


async def chat_to_model(tenant_id: str, provider_name: str, model_name: str, message: str, stream: bool = False, thinking: bool = False):
    """
    Chat to a model.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :param model_name: model name
    :param message: chat message
    :param stream: whether to stream the response
    :param thinking: whether to enable thinking/reasoning
    :return: (success, result_or_error_message)
    """
    from api.db.services.llm_service import LLMBundle

    # Get model config
    composite_name = f"{model_name}@{provider_name}"
    try:
        model_config = TenantLLMService.get_model_config(tenant_id, LLMType.CHAT.value, composite_name)
    except LookupError:
        return False, f"Model '{composite_name}' not authorized"

    if not model_config:
        return False, f"Model '{composite_name}' not found"

    # Check if model is enabled
    obj = TenantLLMService.get_or_none(
        tenant_id=tenant_id, llm_factory=provider_name, llm_name=model_name
    )
    if obj and obj.status != StatusEnum.VALID.value:
        return False, f"Model '{model_name}' is disabled"

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
