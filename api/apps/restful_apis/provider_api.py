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
import asyncio
import logging

from quart import request

from api.apps import login_required
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    get_error_argument_result,
    get_error_data_result,
    get_json_result,
    get_result,
)
from api.apps.services import provider_api_service
from common.constants import LLMType


async def _verify_provider_instance_model(provider_name: str, data: dict):
    model_info = data.get("model_info") or {}
    model_type = data.get("model_type") or model_info.get("model_type")
    if isinstance(model_type, list):
        model_type = model_type[0] if model_type else None
    model_name = (
        data.get("llm_name")
        or data.get("model_name")
        or model_info.get("model_name")
        or ""
    )
    api_key = data.get("api_key", "")
    base_url = data.get("base_url") or data.get("api_base", "")
    timeout_seconds = int(data.get("verify_timeout", 60))

    if model_type == LLMType.OCR.value:
        from rag.llm import OcrModel

        logging.info(
            "OCR verify start: provider=%s model=%s timeout=%ss",
            provider_name, model_name, timeout_seconds,
        )
        if provider_name not in OcrModel:
            logging.warning("OCR verify rejected: unsupported provider %s", provider_name)
            raise RuntimeError(f"OCR model from {provider_name} is not supported yet.")

        # Accept somark_* extras from either top-level (legacy frontend payload)
        # or model_info.extra (new upstream provider-instance shape).
        model_info_extra = model_info.get("extra") or {}
        extra = {
            key: value
            for key, value in model_info_extra.items()
            if key.startswith("somark_") and key not in {"somark_api_key", "somark_base_url"}
        }
        extra.update(
            {
                key: value
                for key, value in data.items()
                if key.startswith("somark_") and key not in {"somark_api_key", "somark_base_url"}
            }
        )
        mdl = OcrModel[provider_name](
            key=api_key,
            model_name=model_name,
            base_url=base_url,
            **extra,
        )
        try:
            ok, reason = await asyncio.wait_for(
                asyncio.to_thread(mdl.check_available),
                timeout=timeout_seconds,
            )
        except asyncio.TimeoutError:
            logging.error(
                "OCR verify timed out after %ss: provider=%s model=%s",
                timeout_seconds, provider_name, model_name,
            )
            raise RuntimeError(f"Verify timed out after {timeout_seconds}s")
        if not ok:
            logging.warning(
                "OCR verify failed: provider=%s model=%s reason=%s",
                provider_name, model_name, reason,
            )
            raise RuntimeError(reason or "Model not available")
        logging.info("OCR verify ok: provider=%s model=%s", provider_name, model_name)
        return

    raise RuntimeError(f"Verify is not supported for model type: {model_type}")


async def _create_ocr_provider_instance(
    tenant_id: str,
    provider_name: str,
    instance_name: str,
    api_key,
    base_url: str,
    region: str,
    model_info: dict,
):
    """
    Controller-side fallback for OCR provider instance creation.

    Mirrors ``provider_api_service.create_provider_instance`` but skips the
    service-level ``verify_api_key`` call, which only supports
    chat/embedding/rerank. OCR verification is performed separately by
    ``_verify_provider_instance_model`` before this function is invoked.
    """
    import json

    from api.apps.services.provider_api_service import add_model_to_instance
    from api.db.services.tenant_model_instance_service import TenantModelInstanceService
    from api.db.services.tenant_model_provider_service import TenantModelProviderService
    from common.settings import FACTORY_LLM_INFOS

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

    api_key_str = ""
    if api_key:
        api_key_str = api_key if isinstance(api_key, str) else json.dumps(api_key)
        same_key_instance = TenantModelInstanceService.get_by_provider_id_and_api_key(provider_obj.id, api_key_str)
        if same_key_instance:
            return False, f"Already exist instance: {same_key_instance.instance_name}"

    extra_fields = {}
    if base_url:
        extra_fields["base_url"] = base_url
    if region:
        extra_fields["region"] = region
    created = TenantModelInstanceService.create_instance(
        provider_id=provider_obj.id,
        instance_name=instance_name,
        api_key=api_key_str,
        extra=json.dumps(extra_fields),
    )
    # ``create_instance`` may dedup the name, so use the persisted value for both
    # the model attach and any rollback below.
    actual_instance_name = getattr(created, "instance_name", None) or instance_name
    logging.info(
        "OCR instance created: tenant=%s provider=%s instance=%s",
        tenant_id, provider_name, actual_instance_name,
    )
    if model_info:
        # Roll back the instance row if the model attach fails, so the tenant is
        # never left in a partially-created state where a retry would trip the
        # duplicate check against an orphaned instance.
        try:
            success, msg = add_model_to_instance(tenant_id, provider_name, actual_instance_name, **model_info)
        except Exception:
            TenantModelInstanceService.delete_by_provider_id_and_instance_name(provider_obj.id, actual_instance_name)
            logging.exception(
                "OCR add_model_to_instance raised, rolled back instance: tenant=%s provider=%s instance=%s",
                tenant_id, provider_name, actual_instance_name,
            )
            raise
        if not success:
            TenantModelInstanceService.delete_by_provider_id_and_instance_name(provider_obj.id, actual_instance_name)
            logging.error(
                "OCR add_model_to_instance failed, rolled back instance: tenant=%s provider=%s instance=%s msg=%s",
                tenant_id, provider_name, actual_instance_name, msg,
            )
            return False, msg

    return True, "success"


@manager.route("/providers", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_providers(tenant_id: str = None):
    """
    List providers.
    ---
    parameters:
      - in: query
        name: available
        type: string
        required: false
        description: "If 'true', list all available system providers; otherwise list tenant-configured providers."
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: List of providers.
        schema:
          type: object
          properties:
            data:
              type: array
              items:
                type: object
    """
    available_only = request.args.get("available", "").lower() == "true"
    try:
        success, result = provider_api_service.list_providers(tenant_id, available_only)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers", methods=["PUT"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def add_provider(tenant_id: str = None):
    """
    Add a provider for the tenant.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Provider creation parameters.
        required: true
        schema:
          type: object
          required:
            - provider_name
          properties:
            provider_name:
              type: string
              description: Provider/factory name.
    responses:
      200:
        description: Provider added successfully.
        schema:
          type: object
    """
    data = await request.get_json()
    if not data or "provider_name" not in data:
        return get_error_argument_result(message="provider_name is required")

    provider_name = data["provider_name"]

    try:
        success, msg = provider_api_service.add_provider(tenant_id, provider_name)
        if success:
            return get_result(message=msg)
        else:
            return get_error_data_result(message=msg)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>", methods=["GET"])  # noqa: F821
@login_required
def show_provider(provider_name: str):
    """
    Show provider details.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Provider details.
        schema:
          type: object
    """
    try:
        success, result = provider_api_service.show_provider(provider_name)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>", methods=["DELETE"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def delete_provider(tenant_id: str = None, provider_name: str = None):
    """
    Delete a provider and all its models for the tenant.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Provider deleted successfully.
        schema:
          type: object
    """
    try:
        success, msg = provider_api_service.delete_provider(tenant_id, provider_name)
        if success:
            return get_result(message=msg)
        else:
            return get_error_data_result(message=msg)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/models", methods=["GET"])  # noqa: F821
@login_required
async def list_provider_models(provider_name: str):
    """
    List models for a provider.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: List of models for the provider.
        schema:
          type: object
          properties:
            data:
              type: array
              items:
                type: object
    """
    try:
        api_key = request.args.get("api_key")
        base_url = request.args.get("base_url")
        success, result = await provider_api_service.list_provider_models(provider_name, api_key, base_url)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/models/<path:model_name>", methods=["GET"])  # noqa: F821
@login_required
def show_provider_model(provider_name: str, model_name: str):
    """
    Show a specific model for a provider.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: path
        name: model_name
        type: string
        required: true
        description: Model name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Model details.
        schema:
          type: object
    """
    try:
        success, result = provider_api_service.show_provider_model(provider_name, model_name)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/instances", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def create_provider_instance(tenant_id: str = None, provider_name: str = None):
    """
    Create a provider instance.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Instance creation parameters.
        required: true
        schema:
          type: object
          required:
            - instance_name
            - api_key
          properties:
            instance_name:
              type: string
              description: Instance name.
            api_key:
              type: string
              description: API key.
            region:
              type: string
              description: Region.
            model_info:
              type: object
              description: Model info.
    responses:
      200:
        description: Instance created successfully.
        schema:
          type: object
    """
    data = await request.get_json()
    if not data or "instance_name" not in data or "api_key" not in data:
        return get_error_argument_result(message="instance_name and api_key are required")

    instance_name = data["instance_name"]
    api_key = data["api_key"]
    base_url = data.get("base_url", "")
    region = data.get("region", "")
    model_info = data.get("model_info", [])

    try:
        # OCR providers (e.g. SoMark) need a dedicated verifier because
        # provider_api_service.verify_api_key (now always invoked by the
        # service-level create_provider_instance) only covers
        # chat/embedding/rerank. We verify here and create the row directly.
        if data.get("model_type") == LLMType.OCR.value:
            try:
                await _verify_provider_instance_model(provider_name, data)
            except Exception as e:
                logging.warning(
                    "OCR create rejected at verify step: tenant=%s provider=%s instance=%s err=%s",
                    tenant_id, provider_name, instance_name, e,
                )
                return get_error_data_result(message=str(e))
            success, msg = await _create_ocr_provider_instance(
                tenant_id, provider_name, instance_name, api_key, base_url, region, model_info
            )
        else:
            success, msg = await provider_api_service.create_provider_instance(
                tenant_id, provider_name, instance_name, api_key, base_url, region, model_info
            )
        if success:
            return get_result(message=msg)
        else:
            return get_error_data_result(message=msg)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/connection", methods=["POST"])  # noqa: F821
@login_required
async def verify_provider_api_key(provider_name: str = None):
    """
    Verify api key.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Instance creation parameters.
        required: true
        schema:
          type: object
          required:
            - api_key
          properties:
            api_key:
              type: string
              description: API key.
            base_url:
              type: string
              description: Base URL.
            region:
              type: string
              description: Region.
            model_info:
              type: object
              description: Model info.
    responses:
      200:
        description: Instance created successfully.
        schema:
          type: object
    """
    data = await request.get_json()
    if not data or "api_key" not in data:
        return get_error_argument_result(message="api_key is required")

    base_url = data.get("base_url", "")
    api_key = data["api_key"]
    region = data.get("region", "default")
    model_info = data.get("model_info", [])

    try:
        # Route OCR verify through our SoMark-aware verifier.
        if data.get("model_type") == LLMType.OCR.value:
            msg = ""
            try:
                await _verify_provider_instance_model(provider_name, data)
            except Exception as e:
                msg = str(e)
                logging.warning(
                    "OCR connection check failed: provider=%s err=%s", provider_name, e,
                )
            return get_json_result(data={"message": msg, "success": len(msg.strip()) == 0})

        success, msg = await provider_api_service.verify_api_key(provider_name, api_key, base_url, region, model_info)
        if success:
            return get_result(message=msg)
        else:
            return get_error_data_result(message=msg)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/instances", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_provider_instances(tenant_id: str = None, provider_name: str = None):
    """
    List provider instances.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: List of provider instances.
        schema:
          type: object
          properties:
            data:
              type: array
              items:
                type: object
    """
    try:
        success, result = provider_api_service.list_provider_instances(tenant_id, provider_name)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/instances/<instance_name>", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def show_provider_instance(tenant_id: str = None, provider_name: str = None, instance_name: str = None):
    """
    Show a provider instance.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: path
        name: instance_name
        type: string
        required: true
        description: Instance name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Instance details.
        schema:
          type: object
    """
    try:
        success, result = provider_api_service.show_provider_instance(tenant_id, provider_name, instance_name)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/instances", methods=["DELETE"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def drop_provider_instances(tenant_id: str = None, provider_name: str = None):
    """
    Drop provider instances.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Instance deletion parameters.
        required: true
        schema:
          type: object
          required:
            - instances
          properties:
            instances:
              type: array
              items:
                type: string
              description: List of instance names to drop.
    responses:
      200:
        description: Instances dropped successfully.
        schema:
          type: object
    """
    data = await request.get_json()
    if not data or "instances" not in data:
        return get_error_argument_result(message="instances is required")

    instances = data["instances"]
    if not instances:
        return get_error_argument_result(message="instances is required")

    try:
        success, msg = provider_api_service.drop_provider_instances(tenant_id, provider_name, instances)
        if success:
            return get_result(message=msg)
        else:
            return get_error_data_result(message=msg)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/instances/<instance_name>/models", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_instance_models(tenant_id: str = None, provider_name: str = None, instance_name: str = None):
    """
    List models for a provider instance.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: path
        name: instance_name
        type: string
        required: true
        description: Instance name.
      - in: query
        name: supported
        type: string
        required: false
        description: "If 'true', list only supported models."
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: List of models.
        schema:
          type: object
          properties:
            data:
              type: array
              items:
                type: object
    """
    supported_only = request.args.get("supported", "").lower() == "true"
    try:
        success, result = provider_api_service.list_instance_models(
            tenant_id, provider_name, instance_name, supported_only
        )
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/instances/<instance_name>/models", methods=["PUT"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def update_instance_models(tenant_id: str, provider_name: str, instance_name: str):
    """
    Batch update model_type for models in instance.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: path
        name: instance_name
        type: string
        required: true
        description: Instance name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Model details.
        required: true
        schema:
          type: object
          required:
            - model_name
            - model_type
          properties:
            model_name:
              type: list of string
              description: Model name.
            model_type:
              type: list of string
              description: Model type.
    """
    data = await request.get_json()
    if not data or "model_name" not in data or "model_type" not in data:
        return get_error_argument_result(message="model_name and model_type are required")
    model_name = data["model_name"]
    model_type = data["model_type"]
    try:
        success, msg = provider_api_service.update_instance_models(tenant_id, provider_name, instance_name, model_name, model_type)
        if success:
            return get_result(message=msg)
        else:
            return get_error_data_result(message=msg)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/instances/<instance_name>/models", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def add_model_to_instance(tenant_id: str, provider_name: str, instance_name: str):
    """
    Add a model to an instance.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: path
        name: instance_name
        type: string
        required: true
        description: Instance name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Model details.
        required: true
        schema:
          type: object
          required:
            - model_name
            - model_type
          properties:
            model_name:
              type: string
              description: Model name.
            model_type:
              type: string
              description: Model type.
            max_tokens:
              type: integer
              description: Maximum number of tokens.
            extra:
              type: object
              description: Extra model details.
    responses:
      200:
        description: Model added successfully.
    """
    data = await request.get_json()
    if not data or "model_name" not in data or "model_type" not in data:
        return get_error_argument_result(message="model_name and model_type are required")

    model_name = data["model_name"]
    model_type = data["model_type"]
    max_tokens = data.get("max_tokens", 8192)
    extra = data.get("extra", {})

    try:
        success, result = provider_api_service.add_model_to_instance(
            tenant_id, provider_name, instance_name, model_name, model_type, max_tokens, extra
        )
        if success:
            return get_result(message=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/instances/<instance_name>/models/<path:model_name>", methods=["PATCH"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def enable_or_disable_model(tenant_id: str = None, provider_name: str = None, instance_name: str = None, model_name: str = None):
    """
    Enable or disable a model.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: path
        name: instance_name
        type: string
        required: true
        description: Instance name.
      - in: path
        name: model_name
        type: string
        required: true
        description: Model name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Model status update.
        required: true
        schema:
          type: object
          required:
            - status
          properties:
            status:
              type: string
              enum: ["active", "inactive"]
              description: Model status.
    responses:
      200:
        description: Model status updated.
        schema:
          type: object
    """
    data = await request.get_json()
    if not data or "status" not in data:
        return get_error_argument_result(message="status is required")

    status = data["status"]
    if status not in ("active", "inactive"):
        return get_error_argument_result(message="status must be 'active' or 'inactive'")

    try:
        success, msg = provider_api_service.update_model_status(tenant_id, provider_name, instance_name, model_name, status)
        if success:
            return get_result(message=msg)
        else:
            return get_error_data_result(message=msg)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/providers/<provider_name>/instances/<instance_name>/models/<path:model_name>", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def chat_to_model(tenant_id: str = None, provider_name: str = None, instance_name: str = None, model_name: str = None):
    """
    Chat to a model.
    ---
    tags:
      - Providers
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: provider_name
        type: string
        required: true
        description: Provider name.
      - in: path
        name: instance_name
        type: string
        required: true
        description: Instance name.
      - in: path
        name: model_name
        type: string
        required: true
        description: Model name.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Chat request.
        required: true
        schema:
          type: object
          required:
            - message
          properties:
            message:
              type: string
              description: Chat message.
            stream:
              type: boolean
              description: Whether to stream the response.
            thinking:
              type: boolean
              description: Whether to enable thinking/reasoning.
    responses:
      200:
        description: Chat response.
        schema:
          type: object
    """
    data = await request.get_json()
    if not data or "message" not in data:
        return get_error_argument_result(message="message is required")

    message = data["message"]
    stream = data.get("stream", False)
    thinking = data.get("thinking", False)

    try:
        success, result = await provider_api_service.chat_to_model(
            tenant_id, provider_name, instance_name, model_name, message, stream, thinking
        )
        if not success:
            return get_error_data_result(message=result)

        if stream and isinstance(result, dict) and result.get("type") == "stream":
            # Streaming response using SSE
            from quart import Response
            llm = result["llm"]

            async def generate():
                async for chunk in llm.async_chat_streamly(
                    None,
                    [{"role": "user", "content": message}],
                    {"temperature": 0.9},
                ):
                    if chunk and isinstance(chunk, str) and chunk.find("**ERROR**") < 0:
                        yield f"data: [MESSAGE]{chunk}\n\n"
                yield "data: [DONE]\n\n"

            return Response(generate(), mimetype="text/event-stream", headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
            })

        # Non-streaming response
        return get_result(data=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")
