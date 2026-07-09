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
from api.apps.services import models_api_service
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    get_error_argument_result,
    get_error_data_result,
    get_result,
)


@manager.route("/models", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def get_added_models(tenant_id: str):
    """
    List tenant all added models.
    ---
    tags:
      - Models
    security:
      - ApiKeyAuth: []
    parameters:
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: List of added models.
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                models:
                  type: array
                  items:
                    type: object
                    properties:
                      model_provider:
                        type: string
                      model_instance:
                        type: string
                      model_name:
                        type: string
                      model_type:
                        type: string
                      enable:
                        type: boolean
    """
    model_type_filter = request.args.get("type")
    try:
        success, result = models_api_service.list_tenant_added_models(tenant_id, model_type_filter)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/models/default", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def get_default_models(tenant_id: str):
    """
    List tenant default models.
    ---
    tags:
      - Models
    security:
      - ApiKeyAuth: []
    parameters:
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: List of default models.
        schema:
          type: object
          properties:
            data:
              type: object
              properties:
                models:
                  type: array
                  items:
                    type: object
                    properties:
                      model_provider:
                        type: string
                      model_instance:
                        type: string
                      model_name:
                        type: string
                      model_type:
                        type: string
                      enable:
                        type: boolean
    """
    try:
        success, result = models_api_service.list_tenant_default_models(tenant_id)
        if success:
            return get_result(data=result)
        else:
            return get_error_data_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")


@manager.route("/models/default", methods=["PATCH"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def set_default_models(tenant_id: str):
    """
    Set or clear a tenant default model.
    ---
    tags:
      - Models
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
        description: Model configuration.
        required: true
        schema:
          type: object
          required:
            - model_type
          properties:
            model_provider:
              type: string
              description: Provider name. Required when setting a model; omit to clear.
            model_instance:
              type: string
              description: Instance name. Required when setting a model; omit to clear.
            model_name:
              type: string
              description: Model name. Required when setting a model; omit to clear.
            model_type:
              type: string
              description: "Model type: chat, embedding, rerank, asr, vision, tts, ocr"
    responses:
      200:
        description: Default model updated.
        schema:
          type: object
    """
    data = await request.get_json()
    if not data or "model_type" not in data:
        return get_error_argument_result(message="model_type is required")

    model_provider = data.get("model_provider", "")
    model_instance = data.get("model_instance", "")
    model_name = data.get("model_name", "")
    model_type = data["model_type"]

    try:
        success, msg = models_api_service.set_tenant_default_models(tenant_id, model_provider, model_instance, model_name, model_type)
        if success:
            logging.info(f"success: {success}, msg: {msg}")
            return get_result(message=msg)
        else:
            return get_error_data_result(message=msg)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Internal server error")
