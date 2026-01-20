#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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


from typing import Annotated

from pydantic import BaseModel, ConfigDict, Field
from quart import Response
from quart_schema import tag, validate_response

from api.apps import login_required
from api.utils.api_utils import get_json_result
from plugin import GlobalPluginManager


# Pydantic Schemas for OpenAPI Documentation


class BaseSchema(BaseModel):
    """Base schema with common configuration."""
    model_config = ConfigDict(extra="forbid", strict=True)


class LLMToolParameter(BaseModel):
    """Schema for LLM tool parameter definition."""
    type: Annotated[str, Field(..., description="Parameter type (e.g., 'string', 'integer', 'boolean')")]
    description: Annotated[str, Field(..., description="Parameter description for the LLM")]
    displayDescription: Annotated[str, Field(..., description="Human-readable parameter description")]
    required: Annotated[bool, Field(..., description="Whether the parameter is required")]


class LLMToolMetadata(BaseModel):
    """Schema for LLM tool metadata."""
    name: Annotated[str, Field(..., description="Unique tool name identifier")]
    displayName: Annotated[str, Field(..., description="Human-readable tool name")]
    description: Annotated[str, Field(..., description="Tool description for the LLM")]
    displayDescription: Annotated[str, Field(..., description="Human-readable tool description")]
    parameters: Annotated[dict[str, LLMToolParameter], Field(default_factory=dict, description="Tool parameters schema")]


class LLMToolsResponse(BaseModel):
    """Response schema for getting LLM tools."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[LLMToolMetadata], Field(..., description="List of available LLM tools with their metadata")]
    message: Annotated[str, Field("Success", description="Response message")]


# Route Handlers


@manager.route('/llm_tools', methods=['GET'])  # noqa: F821
@login_required
@validate_response(200, LLMToolsResponse)
@tag(["Plugins"], "Operations for managing and querying plugins")
def llm_tools() -> Response:
    """
    Get all available LLM tools.

    Returns a list of all registered LLM tool plugins with their metadata,
    including parameter definitions and descriptions. These tools can be used
    by LLMs to extend their capabilities.
    """
    tools = GlobalPluginManager.get_llm_tools()
    tools_metadata = [t.get_metadata() for t in tools]

    return get_json_result(data=tools_metadata)
