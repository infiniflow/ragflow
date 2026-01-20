#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import json
import os
from typing import Annotated
from quart import request
from pydantic import BaseModel, ConfigDict, Field
from quart_schema import validate_request as qs_validate_request, validate_response, tag

from api.apps import login_required, current_user
from api.db.services.tenant_llm_service import LLMFactoriesService, TenantLLMService
from api.db.services.llm_service import LLMService
from api.utils.api_utils import get_allowed_llm_factories, get_data_error_result, get_json_result, get_request_json, server_error_response
from common.constants import StatusEnum, LLMType
from api.db.db_models import TenantLLM
from rag.utils.base64_image import test_image
from rag.llm import EmbeddingModel, ChatModel, RerankModel, CvModel, TTSModel, OcrModel, Seq2txtModel


# Pydantic Schemas for OpenAPI Documentation

class BaseSchema(BaseModel):
    """Base schema with common configuration."""
    model_config = ConfigDict(extra="forbid", strict=True)


class StandardResponse(BaseModel):
    """Standard API response schema."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[object, Field(..., description="Response data")]
    message: Annotated[str, Field("success", description="Response message")]


class FactoriesResponse(BaseModel):
    """Response schema for getting LLM factories."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[dict], Field(..., description="List of available LLM factories with their model types")]
    message: Annotated[str, Field("success", description="Response message")]


class SetAPIKeyRequest(BaseSchema):
    """Request schema for setting LLM API key."""
    llm_factory: Annotated[str, Field(..., description="LLM factory name (e.g., 'OpenAI', 'Azure-OpenAI')")]
    api_key: Annotated[str, Field(..., description="API key for the LLM factory")]
    base_url: Annotated[str | None, Field(None, description="Optional custom base URL for API requests")]
    model_type: Annotated[str | None, Field(None, description="Specific model type to configure")]
    llm_name: Annotated[str | None, Field(None, description="Specific model name to configure")]


class SetAPIKeyResponse(BaseModel):
    """Response schema for setting LLM API key."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="True if API key was set successfully")]
    message: Annotated[str, Field("success", description="Response message")]


class AddLLMRequest(BaseSchema):
    """Request schema for adding a new LLM configuration."""
    llm_factory: Annotated[str, Field(..., description="LLM factory name (e.g., 'VolcEngine', 'LocalAI')")]
    api_key: Annotated[str, Field(..., description="API key for the LLM")]
    api_base: Annotated[str | None, Field(None, description="Optional custom API base URL")]
    llm_name: Annotated[str, Field(..., description="Name of the LLM model")]
    model_type: Annotated[str, Field(..., description="Type of model (chat, embedding, rerank, etc.)")]
    max_tokens: Annotated[int | None, Field(None, description="Maximum tokens for the model")]
    # Factory-specific fields
    ark_api_key: Annotated[str | None, Field(None, description="VolcEngine ark API key")]
    endpoint_id: Annotated[str | None, Field(None, description="VolcEngine endpoint ID")]
    hunyuan_sid: Annotated[str | None, Field(None, description="Tencent Hunyuan session ID")]
    hunyuan_sk: Annotated[str | None, Field(None, description="Tencent Hunyuan secret key")]
    tencent_cloud_sid: Annotated[str | None, Field(None, description="Tencent Cloud session ID")]
    tencent_cloud_sk: Annotated[str | None, Field(None, description="Tencent Cloud secret key")]
    auth_mode: Annotated[str | None, Field(None, description="Bedrock authentication mode")]
    bedrock_ak: Annotated[str | None, Field(None, description="Bedrock access key")]
    bedrock_sk: Annotated[str | None, Field(None, description="Bedrock secret key")]
    bedrock_region: Annotated[str | None, Field(None, description="Bedrock region")]
    aws_role_arn: Annotated[str | None, Field(None, description="AWS role ARN for Bedrock")]
    spark_api_password: Annotated[str | None, Field(None, description="XunFei Spark API password")]
    spark_app_id: Annotated[str | None, Field(None, description="XunFei Spark app ID")]
    spark_api_secret: Annotated[str | None, Field(None, description="XunFei Spark API secret")]
    spark_api_key: Annotated[str | None, Field(None, description="XunFei Spark API key")]
    yiyan_ak: Annotated[str | None, Field(None, description="Baidu Yiyan access key")]
    yiyan_sk: Annotated[str | None, Field(None, description="Baidu Yiyan secret key")]
    fish_audio_ak: Annotated[str | None, Field(None, description="Fish Audio access key")]
    fish_audio_refid: Annotated[str | None, Field(None, description="Fish Audio reference ID")]
    google_project_id: Annotated[str | None, Field(None, description="Google Cloud project ID")]
    google_region: Annotated[str | None, Field(None, description="Google Cloud region")]
    google_service_account_key: Annotated[str | None, Field(None, description="Google Cloud service account key")]
    api_version: Annotated[str | None, Field(None, description="Azure OpenAI API version")]
    provider_order: Annotated[str | None, Field(None, description="Provider order for some factories")]


class AddLLMResponse(BaseModel):
    """Response schema for adding a new LLM configuration."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="True if LLM was added successfully")]
    message: Annotated[str, Field("success", description="Response message")]


class DeleteLLMRequest(BaseSchema):
    """Request schema for deleting an LLM configuration."""
    llm_factory: Annotated[str, Field(..., description="LLM factory name")]
    llm_name: Annotated[str, Field(..., description="LLM model name")]


class DeleteLLMResponse(BaseModel):
    """Response schema for deleting an LLM configuration."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="True if LLM was deleted successfully")]
    message: Annotated[str, Field("success", description="Response message")]


class EnableLLMRequest(BaseSchema):
    """Request schema for enabling/disabling an LLM configuration."""
    llm_factory: Annotated[str, Field(..., description="LLM factory name")]
    llm_name: Annotated[str, Field(..., description="LLM model name")]
    status: Annotated[str | None, Field("1", description="Status value ('1' for enabled, '0' for disabled)")]


class EnableLLMResponse(BaseModel):
    """Response schema for enabling/disabling an LLM configuration."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="True if status was updated successfully")]
    message: Annotated[str, Field("success", description="Response message")]


class DeleteFactoryRequest(BaseSchema):
    """Request schema for deleting an LLM factory and all its models."""
    llm_factory: Annotated[str, Field(..., description="LLM factory name to delete")]


class DeleteFactoryResponse(BaseModel):
    """Response schema for deleting an LLM factory."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="True if factory was deleted successfully")]
    message: Annotated[str, Field("success", description="Response message")]


class MyLLMsResponse(BaseModel):
    """Response schema for getting user's LLM configurations."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Dictionary mapping factory names to their LLM configurations")]
    message: Annotated[str, Field("success", description="Response message")]


class ListLLMsResponse(BaseModel):
    """Response schema for listing available LLM models."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Dictionary mapping factory names to their available models")]
    message: Annotated[str, Field("success", description="Response message")]


# Create tag for LLM management endpoints
llm_tag = tag("llm", description="LLM factory and model management endpoints")


@manager.route("/factories", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, FactoriesResponse)
@llm_tag
def factories():
    """
    Get available LLM factories.

    Returns a list of LLM factories (OpenAI, Azure, etc.) available for use,
    along with the model types each factory supports (chat, embedding, rerank, etc.).
    Filters out internal factories like Youdao, FastEmbed, BAAI, and Builtin.
    """
    try:
        fac = get_allowed_llm_factories()
        fac = [f.to_dict() for f in fac if f.name not in ["Youdao", "FastEmbed", "BAAI", "Builtin"]]
        llms = LLMService.get_all()
        mdl_types = {}
        for m in llms:
            if m.status != StatusEnum.VALID.value:
                continue
            if m.fid not in mdl_types:
                mdl_types[m.fid] = set([])
            mdl_types[m.fid].add(m.model_type)
        for f in fac:
            f["model_types"] = list(
                mdl_types.get(
                    f["name"],
                    [LLMType.CHAT, LLMType.EMBEDDING, LLMType.RERANK, LLMType.IMAGE2TEXT, LLMType.SPEECH2TEXT, LLMType.TTS, LLMType.OCR],
                )
            )

        return get_json_result(data=fac)
    except Exception as e:
        return server_error_response(e)


@manager.route("/set_api_key", methods=["POST"])  # noqa: F821
@login_required
@qs_validate_request(SetAPIKeyRequest)
@validate_response(200, SetAPIKeyResponse)
@llm_tag
async def set_api_key():
    """
    Set API key for an LLM factory.

    Validates the provided API key by testing it against available models
    (chat, embedding, rerank) from the specified factory. If validation succeeds,
    the API key is stored for all models from that factory.

    Supports custom base URLs for OpenAI-compatible APIs.
    """
    req = await get_request_json()
    # test if api key works
    chat_passed, embd_passed, rerank_passed = False, False, False
    factory = req["llm_factory"]
    extra = {"provider": factory}
    msg = ""
    for llm in LLMService.query(fid=factory):
        if not embd_passed and llm.model_type == LLMType.EMBEDDING.value:
            assert factory in EmbeddingModel, f"Embedding model from {factory} is not supported yet."
            mdl = EmbeddingModel[factory](req["api_key"], llm.llm_name, base_url=req.get("base_url"))
            try:
                arr, tc = mdl.encode(["Test if the api key is available"])
                if len(arr[0]) == 0:
                    raise Exception("Fail")
                embd_passed = True
            except Exception as e:
                msg += f"\nFail to access embedding model({llm.llm_name}) using this api key." + str(e)
        elif not chat_passed and llm.model_type == LLMType.CHAT.value:
            assert factory in ChatModel, f"Chat model from {factory} is not supported yet."
            mdl = ChatModel[factory](req["api_key"], llm.llm_name, base_url=req.get("base_url"), **extra)
            try:
                m, tc = await mdl.async_chat(None, [{"role": "user", "content": "Hello! How are you doing!"}], {"temperature": 0.9, "max_tokens": 50})
                if m.find("**ERROR**") >= 0:
                    raise Exception(m)
                chat_passed = True
            except Exception as e:
                msg += f"\nFail to access model({llm.fid}/{llm.llm_name}) using this api key." + str(e)
        elif not rerank_passed and llm.model_type == LLMType.RERANK:
            assert factory in RerankModel, f"Re-rank model from {factory} is not supported yet."
            mdl = RerankModel[factory](req["api_key"], llm.llm_name, base_url=req.get("base_url"))
            try:
                arr, tc = mdl.similarity("What's the weather?", ["Is it sunny today?"])
                if len(arr) == 0 or tc == 0:
                    raise Exception("Fail")
                rerank_passed = True
                logging.debug(f"passed model rerank {llm.llm_name}")
            except Exception as e:
                msg += f"\nFail to access model({llm.fid}/{llm.llm_name}) using this api key." + str(e)
        if any([embd_passed, chat_passed, rerank_passed]):
            msg = ""
            break

    if msg:
        return get_data_error_result(message=msg)

    llm_config = {"api_key": req["api_key"], "api_base": req.get("base_url", "")}
    for n in ["model_type", "llm_name"]:
        if n in req:
            llm_config[n] = req[n]

    for llm in LLMService.query(fid=factory):
        llm_config["max_tokens"] = llm.max_tokens
        if not TenantLLMService.filter_update([TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == factory, TenantLLM.llm_name == llm.llm_name], llm_config):
            TenantLLMService.save(
                tenant_id=current_user.id,
                llm_factory=factory,
                llm_name=llm.llm_name,
                model_type=llm.model_type,
                api_key=llm_config["api_key"],
                api_base=llm_config["api_base"],
                max_tokens=llm_config["max_tokens"],
            )

    return get_json_result(data=True)


@manager.route("/add_llm", methods=["POST"])  # noqa: F821
@login_required
@qs_validate_request(AddLLMRequest)
@validate_response(200, AddLLMResponse)
@llm_tag
async def add_llm():
    """
    Add a new LLM configuration.

    Adds and validates a new LLM model configuration for the current tenant.
    Supports various LLM factories with factory-specific authentication methods.

    Factory-specific authentication:
    - VolcEngine: ark_api_key, endpoint_id
    - Tencent Hunyuan: hunyuan_sid, hunyuan_sk
    - Tencent Cloud: tencent_cloud_sid, tencent_cloud_sk
    - Bedrock: bedrock_ak, bedrock_sk, bedrock_region, aws_role_arn
    - XunFei Spark: spark_api_password (for chat) or spark_app_id, spark_api_secret, spark_api_key (for tts)
    - BaiduYiyan: yiyan_ak, yiyan_sk
    - Fish Audio: fish_audio_ak, fish_audio_refid
    - Google Cloud: google_project_id, google_region, google_service_account_key
    - Azure-OpenAI: api_version
    - OpenRouter: provider_order
    """
    req = await get_request_json()
    factory = req["llm_factory"]
    api_key = req.get("api_key", "x")
    llm_name = req.get("llm_name")

    if factory not in [f.name for f in get_allowed_llm_factories()]:
        return get_data_error_result(message=f"LLM factory {factory} is not allowed")

    def apikey_json(keys):
        nonlocal req
        return json.dumps({k: req.get(k, "") for k in keys})

    if factory == "VolcEngine":
        # For VolcEngine, due to its special authentication method
        # Assemble ark_api_key endpoint_id into api_key
        api_key = apikey_json(["ark_api_key", "endpoint_id"])

    elif factory == "Tencent Hunyuan":
        req["api_key"] = apikey_json(["hunyuan_sid", "hunyuan_sk"])
        return await set_api_key()

    elif factory == "Tencent Cloud":
        req["api_key"] = apikey_json(["tencent_cloud_sid", "tencent_cloud_sk"])
        return await set_api_key()

    elif factory == "Bedrock":
        # For Bedrock, due to its special authentication method
        # Assemble bedrock_ak, bedrock_sk, bedrock_region
        api_key = apikey_json(["auth_mode", "bedrock_ak", "bedrock_sk", "bedrock_region", "aws_role_arn"])

    elif factory == "LocalAI":
        llm_name += "___LocalAI"

    elif factory == "HuggingFace":
        llm_name += "___HuggingFace"

    elif factory == "OpenAI-API-Compatible":
        llm_name += "___OpenAI-API"

    elif factory == "VLLM":
        llm_name += "___VLLM"

    elif factory == "XunFei Spark":
        if req["model_type"] == "chat":
            api_key = req.get("spark_api_password", "")
        elif req["model_type"] == "tts":
            api_key = apikey_json(["spark_app_id", "spark_api_secret", "spark_api_key"])

    elif factory == "BaiduYiyan":
        api_key = apikey_json(["yiyan_ak", "yiyan_sk"])

    elif factory == "Fish Audio":
        api_key = apikey_json(["fish_audio_ak", "fish_audio_refid"])

    elif factory == "Google Cloud":
        api_key = apikey_json(["google_project_id", "google_region", "google_service_account_key"])

    elif factory == "Azure-OpenAI":
        api_key = apikey_json(["api_key", "api_version"])

    elif factory == "OpenRouter":
        api_key = apikey_json(["api_key", "provider_order"])

    elif factory == "MinerU":
        api_key = apikey_json(["api_key", "provider_order"])

    elif factory == "PaddleOCR":
        api_key = apikey_json(["api_key", "provider_order"])

    llm = {
        "tenant_id": current_user.id,
        "llm_factory": factory,
        "model_type": req["model_type"],
        "llm_name": llm_name,
        "api_base": req.get("api_base", ""),
        "api_key": api_key,
        "max_tokens": req.get("max_tokens"),
    }

    msg = ""
    mdl_nm = llm["llm_name"].split("___")[0]
    extra = {"provider": factory}
    model_type = llm["model_type"]
    model_api_key = llm["api_key"]
    model_base_url = llm.get("api_base", "")
    match model_type:
        case LLMType.EMBEDDING.value:
            assert factory in EmbeddingModel, f"Embedding model from {factory} is not supported yet."
            mdl = EmbeddingModel[factory](key=model_api_key, model_name=mdl_nm, base_url=model_base_url)
            try:
                arr, tc = mdl.encode(["Test if the api key is available"])
                if len(arr[0]) == 0:
                    raise Exception("Fail")
            except Exception as e:
                msg += f"\nFail to access embedding model({mdl_nm})." + str(e)
        case LLMType.CHAT.value:
            assert factory in ChatModel, f"Chat model from {factory} is not supported yet."
            mdl = ChatModel[factory](
                key=model_api_key,
                model_name=mdl_nm,
                base_url=model_base_url,
                **extra,
            )
            try:
                m, tc = await mdl.async_chat(None, [{"role": "user", "content": "Hello! How are you doing!"}], {"temperature": 0.9})
                if not tc and m.find("**ERROR**:") >= 0:
                    raise Exception(m)
            except Exception as e:
                msg += f"\nFail to access model({factory}/{mdl_nm})." + str(e)

        case LLMType.RERANK.value:
            assert factory in RerankModel, f"RE-rank model from {factory} is not supported yet."
            try:
                mdl = RerankModel[factory](key=model_api_key, model_name=mdl_nm, base_url=model_base_url)
                arr, tc = mdl.similarity("Hello~ RAGFlower!", ["Hi, there!", "Ohh, my friend!"])
                if len(arr) == 0:
                    raise Exception("Not known.")
            except KeyError:
                msg += f"{factory} dose not support this model({factory}/{mdl_nm})"
            except Exception as e:
                msg += f"\nFail to access model({factory}/{mdl_nm})." + str(e)

        case LLMType.IMAGE2TEXT.value:
            assert factory in CvModel, f"Image to text model from {factory} is not supported yet."
            mdl = CvModel[factory](key=model_api_key, model_name=mdl_nm, base_url=model_base_url)
            try:
                image_data = test_image
                m, tc = mdl.describe(image_data)
                if not tc and m.find("**ERROR**:") >= 0:
                    raise Exception(m)
            except Exception as e:
                msg += f"\nFail to access model({factory}/{mdl_nm})." + str(e)
        case LLMType.TTS.value:
            assert factory in TTSModel, f"TTS model from {factory} is not supported yet."
            mdl = TTSModel[factory](key=model_api_key, model_name=mdl_nm, base_url=model_base_url)
            try:
                for resp in mdl.tts("Hello~ RAGFlower!"):
                    pass
            except RuntimeError as e:
                msg += f"\nFail to access model({factory}/{mdl_nm})." + str(e)
        case LLMType.OCR.value:
            assert factory in OcrModel, f"OCR model from {factory} is not supported yet."
            try:
                mdl = OcrModel[factory](key=model_api_key, model_name=mdl_nm, base_url=model_base_url)
                ok, reason = mdl.check_available()
                if not ok:
                    raise RuntimeError(reason or "Model not available")
            except Exception as e:
                msg += f"\nFail to access model({factory}/{mdl_nm})." + str(e)
        case LLMType.SPEECH2TEXT:
            assert factory in Seq2txtModel, f"Speech model from {factory} is not supported yet."
            try:
                mdl = Seq2txtModel[factory](key=model_api_key, model_name=mdl_nm, base_url=model_base_url)
                # TODO: check the availability
            except Exception as e:
                msg += f"\nFail to access model({factory}/{mdl_nm})." + str(e)
        case _:
            raise RuntimeError(f"Unknown model type: {model_type}")

    if msg:
        return get_data_error_result(message=msg)

    if not TenantLLMService.filter_update([TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == factory, TenantLLM.llm_name == llm["llm_name"]], llm):
        TenantLLMService.save(**llm)

    return get_json_result(data=True)


@manager.route("/delete_llm", methods=["POST"])  # noqa: F821
@login_required
@qs_validate_request(DeleteLLMRequest)
@validate_response(200, DeleteLLMResponse)
@llm_tag
async def delete_llm():
    """
    Delete an LLM configuration.

    Removes the specified LLM model configuration from the current tenant's
    available models. This action cannot be undone.
    """
    req = await get_request_json()
    TenantLLMService.filter_delete([TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == req["llm_factory"], TenantLLM.llm_name == req["llm_name"]])
    return get_json_result(data=True)


@manager.route("/enable_llm", methods=["POST"])  # noqa: F821
@login_required
@qs_validate_request(EnableLLMRequest)
@validate_response(200, EnableLLMResponse)
@llm_tag
async def enable_llm():
    """
    Enable or disable an LLM configuration.

    Updates the status of the specified LLM model configuration.
    Use status='1' to enable and status='0' to disable the model.
    """
    req = await get_request_json()
    TenantLLMService.filter_update(
        [TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == req["llm_factory"], TenantLLM.llm_name == req["llm_name"]], {"status": str(req.get("status", "1"))}
    )
    return get_json_result(data=True)


@manager.route("/delete_factory", methods=["POST"])  # noqa: F821
@login_required
@qs_validate_request(DeleteFactoryRequest)
@validate_response(200, DeleteFactoryResponse)
@llm_tag
async def delete_factory():
    """
    Delete an entire LLM factory and all its models.

    Removes all LLM model configurations associated with the specified
    factory from the current tenant's available models. This action cannot be undone.
    """
    req = await get_request_json()
    TenantLLMService.filter_delete([TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == req["llm_factory"]])
    return get_json_result(data=True)


@manager.route("/my_llms", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, MyLLMsResponse)
@llm_tag
def my_llms():
    """
    Get current tenant's LLM configurations.

    Returns all LLM model configurations available to the current tenant,
    grouped by factory. Includes information about model types, usage statistics,
    and configuration details.

    Query parameters:
    - include_details: If 'true', returns detailed information including api_base and max_tokens
    """
    try:
        TenantLLMService.ensure_mineru_from_env(current_user.id)
        include_details = request.args.get("include_details", "false").lower() == "true"

        if include_details:
            res = {}
            objs = TenantLLMService.query(tenant_id=current_user.id)
            factories = LLMFactoriesService.query(status=StatusEnum.VALID.value)

            for o in objs:
                o_dict = o.to_dict()
                factory_tags = None
                for f in factories:
                    if f.name == o_dict["llm_factory"]:
                        factory_tags = f.tags
                        break

                if o_dict["llm_factory"] not in res:
                    res[o_dict["llm_factory"]] = {"tags": factory_tags, "llm": []}

                res[o_dict["llm_factory"]]["llm"].append(
                    {
                        "type": o_dict["model_type"],
                        "name": o_dict["llm_name"],
                        "used_token": o_dict["used_tokens"],
                        "api_base": o_dict["api_base"] or "",
                        "max_tokens": o_dict["max_tokens"] or 8192,
                        "status": o_dict["status"] or "1",
                    }
                )
        else:
            res = {}
            for o in TenantLLMService.get_my_llms(current_user.id):
                if o["llm_factory"] not in res:
                    res[o["llm_factory"]] = {"tags": o["tags"], "llm": []}
                res[o["llm_factory"]]["llm"].append({"type": o["model_type"], "name": o["llm_name"], "used_token": o["used_tokens"], "status": o["status"]})

        return get_json_result(data=res)
    except Exception as e:
        return server_error_response(e)


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, ListLLMsResponse)
@llm_tag
async def list_app():
    """
    List all available LLM models.

    Returns a comprehensive list of all LLM models available to the current tenant,
    including system built-in models and tenant-configured models. Models are
    grouped by factory and include availability status.

    Query parameters:
    - model_type: Optional filter to return only models of a specific type
                  (e.g., 'chat', 'embedding', 'rerank', 'image2text', etc.)
    """
    self_deployed = ["FastEmbed", "Ollama", "Xinference", "LocalAI", "LM-Studio", "GPUStack"]
    weighted = []
    model_type = request.args.get("model_type")
    tenant_id = current_user.id
    try:
        TenantLLMService.ensure_mineru_from_env(tenant_id)
        objs = TenantLLMService.query(tenant_id=tenant_id)
        facts = set([o.to_dict()["llm_factory"] for o in objs if o.api_key and o.status == StatusEnum.VALID.value])
        status = {(o.llm_name + "@" + o.llm_factory) for o in objs if o.status == StatusEnum.VALID.value}
        llms = LLMService.get_all()
        llms = [m.to_dict() for m in llms if m.status == StatusEnum.VALID.value and m.fid not in weighted and (m.fid == "Builtin" or (m.llm_name + "@" + m.fid) in status)]
        for m in llms:
            m["available"] = m["fid"] in facts or m["llm_name"].lower() == "flag-embedding" or m["fid"] in self_deployed
            if "tei-" in os.getenv("COMPOSE_PROFILES", "") and m["model_type"] == LLMType.EMBEDDING and m["fid"] == "Builtin" and m["llm_name"] == os.getenv("TEI_MODEL", ""):
                m["available"] = True

        llm_set = set([m["llm_name"] + "@" + m["fid"] for m in llms])
        for o in objs:
            if o.llm_name + "@" + o.llm_factory in llm_set:
                continue
            llms.append({"llm_name": o.llm_name, "model_type": o.model_type, "fid": o.llm_factory, "available": True, "status": StatusEnum.VALID.value})

        res = {}
        for m in llms:
            if model_type and m["model_type"].find(model_type) < 0:
                continue
            if m["fid"] not in res:
                res[m["fid"]] = []
            res[m["fid"]].append(m)

        return get_json_result(data=res)
    except Exception as e:
        return server_error_response(e)
