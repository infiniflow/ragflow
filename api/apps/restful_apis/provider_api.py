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
from quart import request

from api.apps import login_required
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    get_error_data_result,
    get_request_json,
    get_result,
    validate_request,
)
from api.apps.services import provider_api_service


@manager.route("/providers", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_providers(tenant_id: str = None):
    """
    List providers.
    If query param `available=true`, return all pool providers;
    otherwise return tenant's own providers.
    ---
    tags:
      - providers
    parameters:
      - name: available
        in: query
        type: string
        required: false
        description: "Set to 'true' to list all pool providers"
    responses:
      200:
        description: List of providers
    """
    available = request.args.get("available", "").lower() == "true"
    success, result = provider_api_service.list_providers(tenant_id, available)
    if success:
        return get_result(data=result)
    return get_error_data_result(message=result)


@manager.route("/providers", methods=["PUT"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
@validate_request("provider_name")
async def add_provider(tenant_id: str = None):
    """
    Add a model provider for the tenant.
    ---
    tags:
      - providers
    parameters:
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - provider_name
          properties:
            provider_name:
              type: string
    responses:
      200:
        description: Provider added
    """
    req = await get_request_json()
    success, result = provider_api_service.add_provider(tenant_id, req["provider_name"])
    if success:
        return get_result()
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>", methods=["GET"])  # noqa: F821
@login_required
def show_provider(provider_name):
    """
    Show provider details from the global pool.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
    responses:
      200:
        description: Provider details
    """
    success, result = provider_api_service.show_provider(provider_name)
    if success:
        return get_result(data=result)
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>", methods=["DELETE"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def delete_provider(provider_name, tenant_id: str = None):
    """
    Delete a model provider for the tenant.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
    responses:
      200:
        description: Provider deleted
    """
    success, result = provider_api_service.delete_provider(tenant_id, provider_name)
    if success:
        return get_result()
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>/models", methods=["GET"])  # noqa: F821
@login_required
def list_models(provider_name):
    """
    List all models for a provider.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
    responses:
      200:
        description: List of models
    """
    success, result = provider_api_service.list_models(provider_name)
    if success:
        return get_result(data=result)
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>/models/<model_name>", methods=["GET"])  # noqa: F821
@login_required
def show_model(provider_name, model_name):
    """
    Show a specific model for a provider.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
      - name: model_name
        in: path
        type: string
        required: true
    responses:
      200:
        description: Model details
    """
    success, result = provider_api_service.show_model(provider_name, model_name)
    if success:
        return get_result(data=result)
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>/instances", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
@validate_request("instance_name", "api_key")
async def create_provider_instance(provider_name, tenant_id: str = None):
    """
    Create a provider instance (API key binding) under a provider.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - instance_name
            - api_key
          properties:
            instance_name:
              type: string
            api_key:
              type: string
    responses:
      200:
        description: Instance created
    """
    req = await get_request_json()
    success, result = provider_api_service.create_provider_instance(
        tenant_id, provider_name, req["instance_name"], req["api_key"]
    )
    if success:
        return get_result()
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>/instances", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_provider_instances(provider_name, tenant_id: str = None):
    """
    List all instances under a provider.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
    responses:
      200:
        description: List of instances
    """
    success, result = provider_api_service.list_provider_instances(tenant_id, provider_name)
    if success:
        return get_result(data=result)
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>/instances/<instance_name>", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def show_provider_instance(provider_name, instance_name, tenant_id: str = None):
    """
    Show a specific provider instance.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
      - name: instance_name
        in: path
        type: string
        required: true
    responses:
      200:
        description: Instance details
    """
    success, result = provider_api_service.show_provider_instance(tenant_id, provider_name, instance_name)
    if success:
        return get_result(data=result)
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>/instances/<instance_name>", methods=["PUT"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def alter_provider_instance(provider_name, instance_name, tenant_id: str = None):
    """
    Alter a provider instance (not yet implemented).
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
      - name: instance_name
        in: path
        type: string
        required: true
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - llm_name
          properties:
            llm_name:
              type: string
    responses:
      200:
        description: Instance altered
    """
    return get_error_data_result(message="Not implemented yet")


@manager.route("/providers/<provider_name>/instances/delete", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
@validate_request("instances")
async def drop_provider_instances(provider_name, tenant_id: str = None):
    """
    Delete provider instances by name.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
      - in: body
        name: body
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
    responses:
      200:
        description: Instances deleted
    """
    req = await get_request_json()
    success, result = provider_api_service.drop_provider_instances(tenant_id, provider_name, req["instances"])
    if success:
        return get_result()
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>/instances/<instance_name>/models", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_instance_models(provider_name, instance_name, tenant_id: str = None):
    """
    List models under an instance, marking enabled/disabled status.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
      - name: instance_name
        in: path
        type: string
        required: true
    responses:
      200:
        description: List of models with status
    """
    success, result = provider_api_service.list_instance_models(tenant_id, provider_name, instance_name)
    if success:
        return get_result(data=result)
    return get_error_data_result(message=result)


@manager.route("/providers/<provider_name>/instances/<instance_name>/models/<model_name>", methods=["PATCH"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
@validate_request("status")
async def enable_or_disable_model(provider_name, instance_name, model_name, tenant_id: str = None):
    """
    Toggle model enabled/disabled status.
    ---
    tags:
      - providers
    parameters:
      - name: provider_name
        in: path
        type: string
        required: true
      - name: instance_name
        in: path
        type: string
        required: true
      - name: model_name
        in: path
        type: string
        required: true
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - status
          properties:
            status:
              type: string
    responses:
      200:
        description: Model status updated
    """
    req = await get_request_json()
    success, result = provider_api_service.update_model_status(
        tenant_id, provider_name, instance_name, model_name, req["status"]
    )
    if success:
        return get_result()
    return get_error_data_result(message=result)
