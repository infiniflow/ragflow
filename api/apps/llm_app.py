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
import time
from quart import Blueprint, request

from api.apps import login_required, current_user
from api.db.services.tenant_llm_service import LLMFactoriesService, TenantLLMService
from api.db.services.llm_service import LLMService
from api.utils.api_utils import get_allowed_llm_factories, get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from common.constants import StatusEnum, LLMType, RetCode
from api.db.db_models import TenantLLM
from rag.utils.base64_image import test_image
from rag.llm import EmbeddingModel, ChatModel, RerankModel, CvModel, TTSModel, OcrModel, Seq2txtModel

# Cache refresh rate limiting (in seconds)
REFRESH_COOLDOWN = 300  # 5 minutes

manager = Blueprint("llm_app", __name__, url_prefix="/llm")

@manager.route("/factories", methods=["GET"])
@login_required
async def factories():
    from api.db.services.dynamic_model_provider import is_dynamic_provider

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
            f["is_dynamic"] = is_dynamic_provider(f["name"])

        return get_json_result(data=fac)
    except Exception as e:
        return server_error_response(e)


@manager.route("/factories/<factory>/models", methods=["GET"])  # noqa: F821
@login_required
async def get_factory_models(factory: str):
    """
    Get available models for a factory.

    For dynamic providers (OpenRouter), fetches from their API with caching.
    For static providers, returns models from database.

    Query params:
        - refresh: bool (optional) - Force cache refresh
    """
    from api.db.services.dynamic_model_provider import get_provider, is_dynamic_provider

    try:
        # Check if factory supports dynamic discovery
        if is_dynamic_provider(factory):
            provider = get_provider(factory)
            if not provider:
                return get_json_result(
                    data=False,
                    message=f"Provider {factory} not found",
                    code=RetCode.ARGUMENT_ERROR
                )

            # Check if user wants to force refresh
            refresh = request.args.get("refresh", "false").lower() == "true"
            if refresh:
                # Clear cache to force refresh with rate limiting
                from rag.utils.redis_conn import REDIS_CONN
                
                rate_limit_key = f"refresh_limit:{current_user.id}:{factory}"
                
                try:
                    # Check if user is within cooldown period
                    last_refresh = REDIS_CONN.get(rate_limit_key)
                    if last_refresh:
                        # Decode bytes if necessary
                        if isinstance(last_refresh, bytes):
                            last_refresh_str = last_refresh.decode('utf-8')
                        else:
                            last_refresh_str = str(last_refresh)
                        
                        last_refresh_time = float(last_refresh_str)
                        time_since_refresh = time.time() - last_refresh_time
                        
                        if time_since_refresh < REFRESH_COOLDOWN:
                            remaining = int(REFRESH_COOLDOWN - time_since_refresh)
                            return get_json_result(
                                data=False,
                                message=f"Cache refresh rate limit exceeded. Please wait {remaining} seconds.",
                                code=RetCode.OPERATING_ERROR
                            )
                    
                    # Delete cache and set rate limit only if deletion succeeds
                    # Note: api_key may not be available yet, so we clear the base key
                    # In practice, most users will have the same cache (public endpoint)
                    cache_key_to_delete = provider.get_cache_key()
                    deleted_count = REDIS_CONN.delete(cache_key_to_delete)
                    
                    if deleted_count > 0:
                        # Cache successfully deleted, set rate limit
                        REDIS_CONN.set(rate_limit_key, str(time.time()), REFRESH_COOLDOWN)
                        logging.info(f"Cache cleared for {factory} by user {current_user.id}")
                    else:
                        # Cache key didn't exist or delete failed
                        logging.warning(f"Failed to delete cache for {factory}: key '{cache_key_to_delete}' not found or already deleted")
                    
                except Exception as e:
                    logging.warning(f"Failed to clear cache for {factory}: {e}")

            # Get user's API key if they have one configured
            # Note: For factory-level model discovery, we only need one valid API key
            # for the factory (not model-specific). We use the first available active
            # configuration since API keys are typically factory-wide credentials.
            api_key = None
            try:
                tenant_llms = TenantLLMService.query(
                    tenant_id=current_user.id,
                    llm_factory=factory
                )
                if tenant_llms:
                    # Find first valid configuration with an API key
                    valid_llm = next(
                        (llm for llm in tenant_llms if llm.status == StatusEnum.VALID.value and llm.api_key),
                        None
                    )
                    if valid_llm:
                        api_key = valid_llm.api_key
            except Exception as e:
                logging.warning(f"Failed to retrieve user API key for {factory}: {e}", exc_info=True)

            # Fetch models (uses cache if available)
            all_models, cache_hit = await provider.fetch_available_models(api_key=api_key)

            # Check if category filter is requested
            category_filter = request.args.get("category", None)

            # Organize models by category
            models_by_category = {}
            for model in all_models:
                cat = model["model_type"]
                if cat not in models_by_category:
                    models_by_category[cat] = []
                models_by_category[cat].append(model)

            # If category filter is provided, return only models from that category
            if category_filter and category_filter in models_by_category:
                filtered_models = models_by_category[category_filter]
            else:
                # No filter or invalid category - return all models
                filtered_models = all_models

            return get_json_result(data={
                "factory": factory,
                "models": filtered_models,  # Return filtered models
                "models_by_category": models_by_category,  # Full categorization
                "supported_categories": list(provider.get_supported_categories()),
                "default_base_url": provider.get_default_base_url(),
                "cached": cache_hit,
                "is_dynamic": True
            })
        else:
            # Static provider - return from database
            llms = LLMService.query(fid=factory, status=StatusEnum.VALID.value)
            
            # Validate factory exists if no models found
            if not llms:
                # Check if factory is known at all
                known_factories = [f.name for f in get_allowed_llm_factories()]
                if factory not in known_factories:
                    return get_json_result(
                        data=False,
                        message=f"Provider {factory} not found",
                        code=RetCode.ARGUMENT_ERROR
                    )
            
            models = [llm.to_dict() for llm in llms]

            # Organize models by category for consistent response shape
            models_by_category = {}
            for model in models:
                cat = model.get("model_type", "chat")
                if cat not in models_by_category:
                    models_by_category[cat] = []
                models_by_category[cat].append(model)

            return get_json_result(data={
                "factory": factory,
                "models": models,
                "models_by_category": models_by_category,
                "supported_categories": [],
                "default_base_url": None,
                "cached": False,
                "is_dynamic": False
            })

    except Exception as e:
        return server_error_response(e)


@manager.route("/set_api_key", methods=["POST"])  # noqa: F821
@login_required
@validate_request("llm_factory", "api_key")
async def set_api_key():
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
@validate_request("llm_factory")
async def add_llm():
    req = await get_request_json()
    factory = req["llm_factory"]
    api_key = req.get("api_key", "x")
    llm_name = req.get("llm_name", "")

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
        # For dynamic providers, allow API key reuse from existing models
        provided_api_key = req.get("api_key", "")
        if not provided_api_key or provided_api_key.strip() == "":
            # Try to retrieve existing API key for this factory
            existing_llms = TenantLLMService.query(
                tenant_id=current_user.id,
                llm_factory=factory
            )
            if existing_llms:
                # Reuse the API key from the first existing model
                api_key = existing_llms[0].api_key
            else:
                # No existing key and no provided key - assemble with empty values
                api_key = apikey_json(["api_key", "provider_order"])
        else:
            # User provided a new API key - assemble it
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
@validate_request("llm_factory", "llm_name")
async def delete_llm():
    req = await get_request_json()
    TenantLLMService.filter_delete([TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == req["llm_factory"], TenantLLM.llm_name == req["llm_name"]])
    return get_json_result(data=True)


@manager.route("/enable_llm", methods=["POST"])  # noqa: F821
@login_required
@validate_request("llm_factory", "llm_name")
async def enable_llm():
    req = await get_request_json()
    TenantLLMService.filter_update(
        [TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == req["llm_factory"], TenantLLM.llm_name == req["llm_name"]], {"status": str(req.get("status", "1"))}
    )
    return get_json_result(data=True)


@manager.route("/delete_factory", methods=["POST"])  # noqa: F821
@login_required
@validate_request("llm_factory")
async def delete_factory():
    req = await get_request_json()
    TenantLLMService.filter_delete([TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == req["llm_factory"]])
    return get_json_result(data=True)


@manager.route("/my_llms", methods=["GET"])  # noqa: F821
@login_required
async def my_llms():
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
async def list_app():
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
