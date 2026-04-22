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
from api.apps import login_required
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    get_error_data_result,
    get_request_json,
    get_result,
    validate_request,
)
from api.apps.services import model_api_service


@manager.route("/models", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def get_models(tenant_id: str = None):
    """
    List tenant default models for each model type.
    ---
    tags:
      - models
    responses:
      200:
        description: List of default models
    """
    success, result = model_api_service.get_models(tenant_id)
    if success:
        return get_result(data=result)
    return get_error_data_result(message=result)


@manager.route("/models", methods=["PATCH"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
@validate_request("model_type")
async def set_models(tenant_id: str = None):
    """
    Set tenant default model for a given model type.
    ---
    tags:
      - models
    parameters:
      - in: body
        name: body
        required: true
        schema:
          type: object
          required:
            - model_type
          properties:
            model_provider:
              type: string
            model_instance:
              type: string
            model_name:
              type: string
            model_type:
              type: string
              description: "chat, embedding, rerank, asr, vision, tts"
    responses:
      200:
        description: Model set successfully
    """
    req = await get_request_json()
    success, result = model_api_service.set_models(
        tenant_id,
        model_provider=req.get("model_provider", ""),
        model_instance=req.get("model_instance", ""),
        model_name=req.get("model_name", ""),
        model_type=req["model_type"],
    )
    if success:
        return get_result()
    return get_error_data_result(message=result)
