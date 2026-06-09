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

from quart import request

from api.apps import login_required
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    get_error_argument_result,
    get_error_data_result,
    get_result,
)
from api.apps.services import provider_api_service


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
        success, msg = await provider_api_service.create_provider_instance(tenant_id, provider_name, instance_name, api_key, base_url, region, model_info)
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
