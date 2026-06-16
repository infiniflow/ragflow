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

from common.constants import LLMType, ActiveStatusEnum
from common.misc_utils import get_uuid
from common.settings import FACTORY_LLM_INFOS
from api.db.joint_services.tenant_model_service import get_model_config_from_provider_instance, delete_models_by_instance_ids, delete_instances_by_provider_ids
from api.db.services.tenant_model_provider_service import TenantModelProviderService
from api.db.services.tenant_model_instance_service import TenantModelInstanceService
from api.db.services.tenant_model_service import TenantModelService
from rag.llm import ChatModel, EmbeddingModel, ModelMeta, OcrModel, RerankModel, TTSModel


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



def _factory_llm_name(llm: dict) -> str:
    return llm.get("name") or llm.get("llm_name", "")


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

    factory_rank_mapping = {factory["name"]: -_to_int(factory.get("rank", "500")) for factory in FACTORY_LLM_INFOS}
    factory_info_map = {f["name"]: f for f in FACTORY_LLM_INFOS}
    if all_available:
        providers = []
        for factory_info in FACTORY_LLM_INFOS:
            if factory_info["name"] in ["Youdao", "FastEmbed", "BAAI", "Builtin", "siliconflow_intl"]:
                continue
            model_types = sorted(set(
                model_type
                for llm in factory_info.get("llm", [])
                for model_type in _factory_model_types(llm)
            )) if factory_info.get("llm", []) else []
            if factory_info["name"] in ["MinerU", "PaddleOCR", "OpenDataLoader"]:
                model_types.append("ocr")
            provider = {
                "model_types": model_types,
                "name": factory_info["name"],
                "url": {
                    "default": factory_info.get("url", "")
                }
            }
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
            model_types = sorted(set(
                model_type
                for llm in factory_info.get("llm", [])
                for model_type in _factory_model_types(llm)
            )) if factory_info.get("llm", []) else []
            if name in ["MinerU", "PaddleOCR", "OpenDataLoader"]:
                model_types.append("ocr")

            provider = {
                "model_types": model_types,
                "name": factory_info["name"],
                "url": {
                    "default": factory_info.get("url", "")
                }
            }
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
    instance_objs = TenantModelInstanceService.get_all_by_provider_id(provider_obj.id)
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


async def list_provider_models(provider_name: str, api_key: str = None, base_url: str = None):
    """
    List all models for a provider from the LLM dictionary.

    :param provider_name: provider/factory name
    :param api_key: api key
    :param base_url: base url
    :return: (success, result_or_error_message)
    """
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"]==provider_name]
    if not factory_info:
        return False, f"Provider '{provider_name}' not found"
    static_llms = [{
            "name": _factory_llm_name(llm),
            "max_tokens": llm["max_tokens"],
            "model_types": _factory_model_types(llm),
            "features": (
                llm.get("features")
                if llm.get("features") is not None
                else (
                    (["is_tools"] if llm.get("is_tools") else [])
                    + (["thinking"] if llm.get("thinking") else [])
                )
            )
        } for llm in factory_info[0]["llm"]]

    model_base_url = _normalize_provider_base_url(provider_name, base_url) or factory_info[0].get("url", "")
    remote_models = []
    if provider_name in ModelMeta:
        remote_models = await ModelMeta[provider_name](api_key, model_base_url).get_model_list()

    if not static_llms and not remote_models:
        return False, f"No models found for provider '{provider_name}'"

    # Merge static and remote models, preferring remote_models on name conflicts
    merged = {m["name"]: m for m in static_llms}
    merged.update({m["name"]: m for m in remote_models})
    models = list(merged.values())

    models.sort(key=lambda x: x["name"])
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
    target_llm = [llm for llm in llms if _factory_llm_name(llm) == model_name]
    if not target_llm:
        return False, f"Model '{model_name}' not found"
    llm_info = target_llm[0]

    return True, {
        "name": _factory_llm_name(llm_info),
        "max_tokens": llm_info["max_tokens"],
        "model_types": _factory_model_types(llm_info),
        "thinking": None,
        "model_type_map": {model_type: True for model_type in _factory_model_types(llm_info)}
    }


async def create_provider_instance(tenant_id: str, provider_name: str, instance_name: str, api_key: str|dict, base_url: str, region: str, model_info: list[dict]=None):
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
    if not provider_name:
        return False, "Provider name is required"

    base_url = _normalize_provider_base_url(provider_name, base_url)

    if instance_name == "default":
        return False, "Instance name cannot be 'default'"

    # Check if provider exists in the system
    allowed_factories = [f["name"] for f in FACTORY_LLM_INFOS]
    if provider_name not in allowed_factories:
        return False, f"Provider '{provider_name}' is not allowed"

    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"Provider '{provider_name}' does not exist"

    api_key_str = ""
    if api_key:
        api_key_str = api_key if isinstance(api_key, str) else json.dumps(api_key)
    success, msg = await verify_api_key(provider_name, api_key, base_url, region, model_info)
    if not success:
        return False, msg

    extra_fields = {}
    if base_url:
        extra_fields["base_url"] = base_url
    if region:
        extra_fields["region"] = region
    TenantModelInstanceService.create_instance(provider_id=provider_obj.id,instance_name=instance_name,api_key=api_key_str, extra=json.dumps(extra_fields))
    if model_info:
        msg = ""
        for model in model_info:
            success, _msg = add_model_to_instance(tenant_id, provider_name, instance_name, **model)
            if not success:
                msg += _msg
        if msg:
            return False, msg

    return True, "success"


def list_provider_instances(tenant_id: str, provider_name: str):
    """
    List provider instances for a tenant.

    :param tenant_id: tenant ID
    :param provider_name: provider/factory name
    :return: (success, result_or_error_message)
    """
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
            "id": instance_obj.id,
            "instance_name": instance_obj.instance_name,
            "provider_id": provider_id,
            "region": extra_fields.get("region", ""),
            "status": instance_obj.status,
        })

    return True, instances


async def verify_api_key(provider_name: str, api_key: str|dict, base_url: str=None, region: str=None, model_info: list[dict]=None):
    """
    Verify API key for a provider.

    :param provider_name: provider/factory name
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
    if not provider_name:
        return False, "Provider name is required"

    base_url = _normalize_provider_base_url(provider_name, base_url)

    if region and region == "intl" and provider_name.lower() == "siliconflow":
        target_factory_name = "siliconflow_intl"
    else:
        target_factory_name = provider_name

    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == target_factory_name]
    if not factory_info:
        return False, f"Provider '{provider_name}' not found"

    factory_llms = factory_info[0]["llm"]
    if not factory_llms:
        if not model_info:
            return False, f"No models found for provider '{provider_name}'"
        factory_llms = [{
            "model_type": _type,
            "llm_name": model.get("model_name", ""),
        } for model in model_info if model for _type in model.get("model_type", []) ]
        if not factory_llms:
            return False, f"No valid models found for provider '{provider_name}'"

    # test if api key works
    chat_passed, embd_passed, rerank_passed, ocr_passed, tts_passed = False, False, False, False, False
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
    for llm in factory_llms:
        model_types = _factory_model_types(llm)
        if not embd_passed and LLMType.EMBEDDING.value in model_types:
            assert provider_name in EmbeddingModel, f"Embedding model from {provider_name} is not supported yet."
            mdl = EmbeddingModel[provider_name](api_key_str, llm["llm_name"], base_url=base_url)
            try:
                arr, tc = await asyncio.wait_for(
                    asyncio.to_thread(mdl.encode, ["Test if the api key is available"]),
                    timeout=timeout_seconds,
                )
                if len(arr[0]) == 0:
                    raise Exception("Fail")
                embd_passed = True
            except Exception as e:
                logging.exception(
                    "Fail to access embedding model for provider=%s model=%s",
                    provider_name,
                    llm["llm_name"],
                )
                msg += f"\nFail to access embedding model({llm['llm_name']}) using this api key." + str(e)
        elif not chat_passed and LLMType.CHAT.value in model_types:
            assert provider_name in ChatModel, f"Chat model from {provider_name} is not supported yet."
            mdl = ChatModel[provider_name](api_key_str, llm["llm_name"], base_url=base_url, **extra)
            try:
                async def check_streamly():
                    async for chunk in mdl.async_chat_streamly(
                            None,
                            [{"role": "user", "content": "Hi"}],
                            {"temperature": 0.9},
                    ):
                        if chunk and isinstance(chunk, str) and chunk.find("**ERROR**") < 0:
                            return True
                    return False

                result = await asyncio.wait_for(check_streamly(), timeout=timeout_seconds)
                if result:
                    chat_passed = True
                else:
                    raise Exception("No valid response received")
            except Exception as e:
                logging.exception(
                    "Fail to access chat model for provider=%s model=%s",
                    provider_name,
                    llm["llm_name"],
                )
                msg += f"\nFail to access model({provider_name}/{llm['llm_name']}) using this api key." + str(e)
        elif not rerank_passed and LLMType.RERANK.value in model_types:
            if provider_name not in RerankModel:
                unsupported_msg = f"Rerank model from {provider_name} is not supported yet."
                logging.warning(unsupported_msg)
                msg += f"\n{unsupported_msg}"
                continue
            mdl = RerankModel[provider_name](api_key_str, llm["llm_name"], base_url=base_url)
            try:
                arr, tc = await asyncio.wait_for(
                    asyncio.to_thread(mdl.similarity, "What's the weather?", ["Is it sunny today?"]),
                    timeout=timeout_seconds,
                )
                if len(arr) == 0 or tc == 0:
                    raise Exception("Fail")
                rerank_passed = True
                logging.debug(f"passed model rerank {llm['llm_name']}")
            except Exception as e:
                logging.exception(
                    "Fail to access rerank model for provider=%s model=%s",
                    provider_name,
                    llm["llm_name"],
                )
                msg += f"\nFail to access model({provider_name}/{llm['llm_name']}) using this api key." + str(e)
        elif not ocr_passed and LLMType.OCR.value in model_types:
            assert provider_name in OcrModel, f"OCR model from {provider_name} is not supported yet."
            mdl = OcrModel[provider_name](key=api_key_str, model_name=llm["llm_name"], base_url=base_url)
            try:
                ok, reason = await asyncio.wait_for(
                    asyncio.to_thread(mdl.check_available),
                    timeout=timeout_seconds,
                )
                if not ok:
                    raise RuntimeError(reason or "Model not available")
                ocr_passed = True
            except Exception as e:
                logging.exception(
                    "Fail to access OCR model for provider=%s model=%s",
                    provider_name,
                    llm["llm_name"],
                )
                msg += f"\nFail to access model({provider_name}/{llm['llm_name']})." + str(e)
        elif not tts_passed and LLMType.TTS.value in model_types:
            assert provider_name in TTSModel, f"TTS model from {provider_name} is not supported yet."
            mdl = TTSModel[provider_name](key=api_key_str, model_name=llm["llm_name"], base_url=base_url)
            try:
                def drain_tts():
                    for _ in mdl.tts("Hello~ RAGFlower!"):
                        pass

                await asyncio.wait_for(
                    asyncio.to_thread(drain_tts),
                    timeout=timeout_seconds,
                )
                tts_passed = True
            except Exception as e:
                logging.exception(
                    "Fail to access TTS model for provider=%s model=%s",
                    provider_name,
                    llm["llm_name"],
                )
                msg += f"\nFail to access model({provider_name}/{llm['llm_name']})." + str(e)
        if any([embd_passed, chat_passed, rerank_passed, ocr_passed, tts_passed]):
            msg = ""
            break

    success = any([embd_passed, chat_passed, rerank_passed, ocr_passed, tts_passed])
    return success, "success" if success else msg


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


def _hybrid_get_instance_models(provider_name: str, instance_id: str):
    # List all models from the LLM dictionary for this provider
    factory_info = [f for f in FACTORY_LLM_INFOS if f["name"] == provider_name]
    if not factory_info:
        return False, f"Provider '{provider_name}' not found"

    # Get model records for this instance from tenant_model table
    model_records = TenantModelService.get_models_by_instance_id(instance_id)
    # Build a map of model_name -> status, type
    model_info_map: dict = {}
    model_unsupported_type_map = {}
    for model_record in model_records:
        if model_record.status == ActiveStatusEnum.UNSUPPORTED.value:
            if model_unsupported_type_map.get(model_record.model_name):
                model_unsupported_type_map[model_record.model_name].append(model_record.model_type)
            else:
                model_unsupported_type_map[model_record.model_name] = [model_record.model_type]
            continue
        if model_info_map.get(model_record.model_name):
            model_info_map[model_record.model_name]["model_type"].append(model_record.model_type)
        else:
            model_info_map[model_record.model_name] = {
                "status": model_record.status,
                "model_type": [model_record.model_type],
                "extra": model_record.extra
            }

    llms = factory_info[0].get("llm", [])
    models = []
    for llm in llms:
        models.append({
            "name": llm["llm_name"],
            "model_type": list(
                set(_factory_model_types(llm) + model_info_map.get(llm["llm_name"], {}).get("model_type", [])) - set(model_unsupported_type_map.get(llm["llm_name"], []))
            ),
            "max_tokens": llm.get("max_tokens"),
            "status": model_info_map.get(llm["llm_name"], {}).get("status", "active"),
        })
    factory_models = [m["name"] for m in models]
    for model_name, model_info_dict in model_info_map.items():
        if model_name not in factory_models:
            extra_fields = json.loads(model_info_dict["extra"]) if model_info_dict["extra"] else {}
            models.append({
                "name": model_name,
                "model_type": set(model_info_dict["model_type"]) - set(model_unsupported_type_map.get(model_name, [])),
                "max_tokens": extra_fields.get("max_tokens", 8192),
                "status": model_info_dict["status"],
            })
    return True, models


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
        models.sort(key=lambda x: x["name"])
        return True, models

    # Get instance
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_name}' and instance '{instance_name}'"

    return _hybrid_get_instance_models(provider_name, instance_obj.id)


def update_instance_models(tenant_id: str, provider_name: str, instance_name: str, model_names: list, model_types: list):
    if not model_names or not model_types:
        return False, "model_name and model_type are required"

    provider_obj = TenantModelProviderService.get_by_tenant_id_and_provider_name(tenant_id, provider_name)
    if not provider_obj:
        return False, f"No provider found for provider '{provider_name}'"
    instance_obj = TenantModelInstanceService.get_by_provider_id_and_instance_name(provider_obj.id, instance_name)
    if not instance_obj:
        return False, f"No instance found for provider '{provider_name}' and instance '{instance_name}'"

    found, models = _hybrid_get_instance_models(provider_name, instance_obj.id)
    if not found:
        return False, models

    model_info_map = {model["name"]: model for model in models}
    not_exist_models = set(model_names) - set(model_info_map.keys())
    if not_exist_models:
        return False, f"Models {not_exist_models} not found for provider '{provider_name}' and instance '{instance_name}'"
    for model_name in model_names:
        model_info = model_info_map.get(model_name, {})
        TenantModelService.upsert_model_type(
            provider_obj.id,
            instance_obj.id,
            model_name,
            {
                "add": list(set(model_types) - set(model_info["model_type"])),
                "delete": list(set(model_info["model_type"]) - set(model_types))
            }
        )
    return True, "success"


def add_model_to_instance(tenant_id: str, provider_name: str, instance_name: str, model_name: str, model_type: str|list[str], max_tokens: int=8192, extra: dict=None):
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

    for _type in model_type:
        extra_fields = {"max_tokens": max_tokens}
        target_model = [llm for llm in llms if _type in _factory_model_types(llm) and llm["llm_name"] == model_name]
        if target_model:
            extra_fields.update({"is_tools": target_model[0].get("is_tools", False)})
        if extra:
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
        TenantModelService.batch_update_model_status([m.id for m in model_obj_list if m.status != ActiveStatusEnum.UNSUPPORTED.value], status)
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

        for model_type in _factory_model_types(target_llm[0]):
            TenantModelService.insert(
                id=get_uuid(),
                model_name=model_name,
                model_type=model_type,
                provider_id=provider_obj.id,
                instance_id=instance_obj.id,
                status=status,
                extra=json.dumps({"max_tokens": target_llm[0].get("max_tokens", 8192), "is_tools": target_llm[0].get("is_tools", False)})
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
