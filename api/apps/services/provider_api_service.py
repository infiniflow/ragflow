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

from common.model_provider_manager import get_model_provider_manager
from api.db.services.tenant_model_provider_service import TenantModelProviderService
from api.db.services.tenant_model_instance_service import TenantModelInstanceService
from api.db.services.tenant_model_service import TenantModelService


def list_providers(tenant_id: str, available: bool = False):
    """
    List providers. If available=True, list all pool providers;
    otherwise list tenant's own providers.

    :param tenant_id: tenant ID
    :param available: whether to list pool providers
    :return: (success, result)
    """
    try:
        pm = get_model_provider_manager()
        if available:
            providers = pm.list_providers()
            # Remove url_suffix and tags as Go handler does
            for p in providers:
                p.pop("url_suffix", None)
                p.pop("tags", None)
            return True, providers

        # List tenant's own providers
        provider_names = TenantModelProviderService.list_provider_names_by_tenant_id(tenant_id)
        result = []
        for name in provider_names:
            provider_info = pm.get_provider_by_name(name)
            if provider_info is not None:
                result.append(provider_info)
        return True, result
    except Exception as e:
        logging.exception("Failed to list providers")
        return False, str(e)


def add_provider(tenant_id: str, provider_name: str):
    """
    Add a model provider for the tenant.

    :param tenant_id: tenant ID
    :param provider_name: provider name
    :return: (success, message)
    """
    try:
        pm = get_model_provider_manager()
        provider = pm.find_provider(provider_name)
        if provider is None:
            return False, f"Provider {provider_name} not found"

        existing = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
        if existing:
            return False, f"Provider {provider_name} already exists"

        TenantModelProviderService.insert(
            tenant_id=tenant_id,
            provider_name=provider_name,
        )
        return True, "success"
    except Exception as e:
        logging.exception("Failed to add provider")
        return False, str(e)


def delete_provider(tenant_id: str, provider_name: str):
    """
    Delete a model provider for the tenant.

    :param tenant_id: tenant ID
    :param provider_name: provider name
    :return: (success, message)
    """
    try:
        count = TenantModelProviderService.delete_by_tenant_id_and_provider_name(tenant_id, provider_name)
        if count == 0:
            return False, f"Provider {provider_name} not found"
        return True, "success"
    except Exception as e:
        logging.exception("Failed to delete provider")
        return False, str(e)


def show_provider(provider_name: str):
    """
    Show provider details from the global pool.

    :param provider_name: provider name
    :return: (success, result)
    """
    pm = get_model_provider_manager()
    provider_info = pm.get_provider_by_name(provider_name)
    if provider_info is None:
        return False, f"Provider {provider_name} not found"
    return True, provider_info


def list_models(provider_name: str):
    """
    List all models for a provider from the global pool.

    :param provider_name: provider name
    :return: (success, result)
    """
    try:
        pm = get_model_provider_manager()
        models = pm.list_models(provider_name)
        if models is None:
            return False, f"Provider {provider_name} not found"
        return True, models
    except Exception as e:
        logging.exception("Failed to list models")
        return False, str(e)


def show_model(provider_name: str, model_name: str):
    """
    Show a specific model for a provider.

    :param provider_name: provider name
    :param model_name: model name
    :return: (success, result)
    """
    try:
        pm = get_model_provider_manager()
        model = pm.get_model_by_name(provider_name, model_name)
        if model is None:
            return False, f"Model {model_name} not found for provider {provider_name}"
        return True, model
    except Exception as e:
        logging.exception("Failed to show model")
        return False, str(e)


def create_provider_instance(tenant_id: str, provider_name: str, instance_name: str, api_key: str):
    """
    Create a provider instance (API key binding) under a provider.

    :param tenant_id: tenant ID
    :param provider_name: provider name
    :param instance_name: instance name
    :param api_key: API key
    :return: (success, message)
    """
    try:
        if instance_name == "default":
            return False, "Instance name cannot be 'default'"

        provider = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
        if not provider:
            return False, f"Provider {provider_name} not found for this tenant"

        existing = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider.id, instance_name)
        if existing:
            return False, f"Instance {instance_name} already exists"

        extra = json.dumps({"region": "default"})
        TenantModelInstanceService.insert(
            instance_name=instance_name,
            provider_id=provider.id,
            api_key=api_key,
            status="enable",
            extra=extra,
        )
        return True, "success"
    except Exception as e:
        logging.exception("Failed to create provider instance")
        return False, str(e)


def list_provider_instances(tenant_id: str, provider_name: str):
    """
    List all instances under a provider.

    :param tenant_id: tenant ID
    :param provider_name: provider name
    :return: (success, result)
    """
    try:
        provider = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
        if not provider:
            return False, f"Provider {provider_name} not found for this tenant"

        instances = TenantModelInstanceService.get_all_by_provider_id(provider.id)
        result = []
        for inst in instances:
            extra = {}
            try:
                extra = json.loads(inst.extra) if inst.extra else {}
            except (json.JSONDecodeError, TypeError):
                pass
            result.append({
                "id": inst.id,
                "instance_name": inst.instance_name,
                "provider_id": inst.provider_id,
                "api_key": inst.api_key,
                "status": inst.status,
                "region": extra.get("region", ""),
            })
        return True, result
    except Exception as e:
        logging.exception("Failed to list provider instances")
        return False, str(e)


def show_provider_instance(tenant_id: str, provider_name: str, instance_name: str):
    """
    Show a specific provider instance.

    :param tenant_id: tenant ID
    :param provider_name: provider name
    :param instance_name: instance name
    :return: (success, result)
    """
    try:
        provider = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
        if not provider:
            return False, f"Provider {provider_name} not found for this tenant"

        instance = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider.id, instance_name)
        if not instance:
            return False, f"Instance {instance_name} not found"

        extra = {}
        try:
            extra = json.loads(instance.extra) if instance.extra else {}
        except (json.JSONDecodeError, TypeError):
            pass

        result = {
            "id": instance.id,
            "instance_name": instance.instance_name,
            "provider_id": instance.provider_id,
            "status": instance.status,
            "region": extra.get("region", ""),
        }
        return True, result
    except Exception as e:
        logging.exception("Failed to show provider instance")
        return False, str(e)


def drop_provider_instances(tenant_id: str, provider_name: str, instance_names: list):
    """
    Delete provider instances by name.

    :param tenant_id: tenant ID
    :param provider_name: provider name
    :param instance_names: list of instance names to delete
    :return: (success, message)
    """
    try:
        provider = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
        if not provider:
            return False, f"Provider {provider_name} not found for this tenant"

        for name in instance_names:
            count = TenantModelInstanceService.delete_by_provider_id_and_instance_name(provider.id, name)
            if count == 0:
                return False, f"Instance {name} not found"
        return True, "success"
    except Exception as e:
        logging.exception("Failed to drop provider instances")
        return False, str(e)


def list_instance_models(tenant_id: str, provider_name: str, instance_name: str):
    """
    List models under an instance, marking enabled/disabled status.

    :param tenant_id: tenant ID
    :param provider_name: provider name
    :param instance_name: instance name
    :return: (success, result)
    """
    try:
        provider = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
        if not provider:
            return False, f"Provider {provider_name} not found for this tenant"

        instance = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider.id, instance_name)
        if not instance:
            return False, f"Instance {instance_name} not found"

        # Get disabled models (records in tenant_model table)
        disabled_models = TenantModelService.get_models_by_instance_id(instance.id)
        disabled_names = {m.model_name for m in disabled_models}

        # Get all available models for this provider from ProviderManager
        pm = get_model_provider_manager()
        all_models = pm.list_models(provider_name)
        if all_models is None:
            return False, f"Provider {provider_name} not found"

        result = []
        for model in all_models:
            model_data = dict(model)
            model_data["status"] = "disabled" if model.get("name") in disabled_names else "enabled"
            result.append(model_data)
        return True, result
    except Exception as e:
        logging.exception("Failed to list instance models")
        return False, str(e)


def update_model_status(tenant_id: str, provider_name: str, instance_name: str, model_name: str, status: str):
    """
    Toggle model enabled/disabled status.
    If a tenant_model record exists for this model -> delete it (enable).
    If not -> create it (disable).

    :param tenant_id: tenant ID
    :param provider_name: provider name
    :param instance_name: instance name
    :param model_name: model name
    :param status: status string
    :return: (success, message)
    """
    try:
        provider = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
        if not provider:
            return False, f"Provider {provider_name} not found for this tenant"

        instance = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider.id, instance_name)
        if not instance:
            return False, f"Instance {instance_name} not found"

        existing = TenantModelService.get_by_provider_id_and_instance_id_and_model_name(
            provider.id, instance.id, model_name
        )

        if existing:
            # Record exists -> delete to enable
            TenantModelService.delete_by_id(existing.id)
        else:
            # No record -> create to disable
            # Get model type from ProviderManager
            pm = get_model_provider_manager()
            model_schema = pm.get_model_by_name(provider_name, model_name)
            model_types = model_schema.get("model_types", []) if model_schema else []
            model_type = model_types[0] if model_types else ""

            TenantModelService.insert(
                model_name=model_name,
                provider_id=provider.id,
                instance_id=instance.id,
                model_type=model_type,
                status=status,
            )
        return True, "success"
    except Exception as e:
        logging.exception("Failed to update model status")
        return False, str(e)
