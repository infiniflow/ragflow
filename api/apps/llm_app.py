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
from flask import request
from flask_login import login_required, current_user
from api.db.services.llm_service import LLMFactoriesService, TenantLLMService, LLMService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.db import StatusEnum, LLMType
from api.db.db_models import TenantLLM
from api.utils.api_utils import get_json_result
from rag.llm import EmbeddingModel, ChatModel, RerankModel, CvModel, TTSModel
import requests


@manager.route('/factories', methods=['GET'])
@login_required
def factories():
    try:
        fac = LLMFactoriesService.get_all()
        fac = [f.to_dict() for f in fac if f.name not in ["Youdao", "FastEmbed", "BAAI"]]
        llms = LLMService.get_all()
        mdl_types = {}
        for m in llms:
            if m.status != StatusEnum.VALID.value:
                continue
            if m.fid not in mdl_types:
                mdl_types[m.fid] = set([])
            mdl_types[m.fid].add(m.model_type)
        for f in fac:
            f["model_types"] = list(mdl_types.get(f["name"], [LLMType.CHAT, LLMType.EMBEDDING, LLMType.RERANK,
                                                              LLMType.IMAGE2TEXT, LLMType.SPEECH2TEXT, LLMType.TTS]))
        return get_json_result(data=fac)
    except Exception as e:
        return server_error_response(e)


@manager.route('/set_api_key', methods=['POST'])
@login_required
@validate_request("llm_factory", "api_key")
def set_api_key():
    req = request.json
    # test if api key works
    chat_passed, embd_passed, rerank_passed = False, False, False
    factory = req["llm_factory"]
    msg = ""
    for llm in LLMService.query(fid=factory)[:3]:
        if not embd_passed and llm.model_type == LLMType.EMBEDDING.value:
            mdl = EmbeddingModel[factory](
                req["api_key"], llm.llm_name, base_url=req.get("base_url"))
            try:
                arr, tc = mdl.encode(["Test if the api key is available"])
                if len(arr[0]) == 0:
                    raise Exception("Fail")
                embd_passed = True
            except Exception as e:
                msg += f"\nFail to access embedding model({llm.llm_name}) using this api key." + str(e)
        elif not chat_passed and llm.model_type == LLMType.CHAT.value:
            mdl = ChatModel[factory](
                req["api_key"], llm.llm_name, base_url=req.get("base_url"))
            try:
                m, tc = mdl.chat(None, [{"role": "user", "content": "Hello! How are you doing!"}], 
                                 {"temperature": 0.9,'max_tokens':50})
                if m.find("**ERROR**") >=0:
                    raise Exception(m)
            except Exception as e:
                msg += f"\nFail to access model({llm.llm_name}) using this api key." + str(
                    e)
            chat_passed = True
        elif not rerank_passed and llm.model_type == LLMType.RERANK:
            mdl = RerankModel[factory](
                req["api_key"], llm.llm_name, base_url=req.get("base_url"))
            try:
                arr, tc = mdl.similarity("What's the weather?", ["Is it sunny today?"])
                if len(arr) == 0 or tc == 0:
                    raise Exception("Fail")
            except Exception as e:
                msg += f"\nFail to access model({llm.llm_name}) using this api key." + str(
                    e)
            rerank_passed = True

    if msg:
        return get_data_error_result(retmsg=msg)

    llm = {
        "api_key": req["api_key"],
        "api_base": req.get("base_url", "")
    }
    for n in ["model_type", "llm_name"]:
        if n in req:
            llm[n] = req[n]

    if not TenantLLMService.filter_update(
            [TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == factory], llm):
        for llm in LLMService.query(fid=factory):
            TenantLLMService.save(
                tenant_id=current_user.id,
                llm_factory=factory,
                llm_name=llm.llm_name,
                model_type=llm.model_type,
                api_key=req["api_key"],
                api_base=req.get("base_url", "")
            )

    return get_json_result(data=True)


@manager.route('/add_llm', methods=['POST'])
@login_required
@validate_request("llm_factory")
def add_llm():
    req = request.json
    factory = req["llm_factory"]

    if factory == "VolcEngine":
        # For VolcEngine, due to its special authentication method
        # Assemble ark_api_key endpoint_id into api_key
        llm_name = req["llm_name"]
        api_key = '{' + f'"ark_api_key": "{req.get("ark_api_key", "")}", ' \
                        f'"ep_id": "{req.get("endpoint_id", "")}", ' + '}'
    elif factory == "Tencent Hunyuan":
        api_key = '{' + f'"hunyuan_sid": "{req.get("hunyuan_sid", "")}", ' \
                        f'"hunyuan_sk": "{req.get("hunyuan_sk", "")}"' + '}'
        req["api_key"] = api_key
        return set_api_key()
    elif factory == "Tencent Cloud":
        api_key = '{' + f'"tencent_cloud_sid": "{req.get("tencent_cloud_sid", "")}", ' \
                f'"tencent_cloud_sk": "{req.get("tencent_cloud_sk", "")}"' + '}'
        req["api_key"] = api_key
    elif factory == "Bedrock":
        # For Bedrock, due to its special authentication method
        # Assemble bedrock_ak, bedrock_sk, bedrock_region
        llm_name = req["llm_name"]
        api_key = '{' + f'"bedrock_ak": "{req.get("bedrock_ak", "")}", ' \
                        f'"bedrock_sk": "{req.get("bedrock_sk", "")}", ' \
                        f'"bedrock_region": "{req.get("bedrock_region", "")}", ' + '}'
    elif factory == "LocalAI":
        llm_name = req["llm_name"]+"___LocalAI"
        api_key = "xxxxxxxxxxxxxxx"
    elif factory == "OpenAI-API-Compatible":
        llm_name = req["llm_name"]+"___OpenAI-API"
        api_key = req.get("api_key","xxxxxxxxxxxxxxx")
    elif factory =="XunFei Spark":
        llm_name = req["llm_name"]
        api_key = req.get("spark_api_password","xxxxxxxxxxxxxxx") 
    elif factory == "BaiduYiyan":
        llm_name = req["llm_name"]
        api_key = '{' + f'"yiyan_ak": "{req.get("yiyan_ak", "")}", ' \
                f'"yiyan_sk": "{req.get("yiyan_sk", "")}"' + '}'
    elif factory == "Fish Audio":
        llm_name = req["llm_name"]
        api_key = '{' + f'"fish_audio_ak": "{req.get("fish_audio_ak", "")}", ' \
                f'"fish_audio_refid": "{req.get("fish_audio_refid", "59cb5986671546eaa6ca8ae6f29f6d22")}"' + '}'
    elif factory == "Google Cloud":
        llm_name = req["llm_name"]
        api_key = (
            "{" + f'"google_project_id": "{req.get("google_project_id", "")}", '
            f'"google_region": "{req.get("google_region", "")}", '
            f'"google_service_account_key": "{req.get("google_service_account_key", "")}"'
            + "}"
        )
    else:
        llm_name = req["llm_name"]
        api_key = req.get("api_key","xxxxxxxxxxxxxxx") 

    llm = {
        "tenant_id": current_user.id,
        "llm_factory": factory,
        "model_type": req["model_type"],
        "llm_name": llm_name,
        "api_base": req.get("api_base", ""),
        "api_key": api_key
    }

    msg = ""
    if llm["model_type"] == LLMType.EMBEDDING.value:
        mdl = EmbeddingModel[factory](
            key=llm['api_key'],
            model_name=llm["llm_name"], 
            base_url=llm["api_base"])
        try:
            arr, tc = mdl.encode(["Test if the api key is available"])
            if len(arr[0]) == 0 or tc == 0:
                raise Exception("Fail")
        except Exception as e:
            msg += f"\nFail to access embedding model({llm['llm_name']})." + str(e)
    elif llm["model_type"] == LLMType.CHAT.value:
        mdl = ChatModel[factory](
            key=llm['api_key'],
            model_name=llm["llm_name"],
            base_url=llm["api_base"]
        )
        try:
            m, tc = mdl.chat(None, [{"role": "user", "content": "Hello! How are you doing!"}], {
                             "temperature": 0.9})
            if not tc:
                raise Exception(m)
        except Exception as e:
            msg += f"\nFail to access model({llm['llm_name']})." + str(
                e)
    elif llm["model_type"] == LLMType.RERANK:
        mdl = RerankModel[factory](
            key=llm["api_key"], 
            model_name=llm["llm_name"], 
            base_url=llm["api_base"]
        )
        try:
            arr, tc = mdl.similarity("Hello~ Ragflower!", ["Hi, there!"])
            if len(arr) == 0 or tc == 0:
                raise Exception("Not known.")
        except Exception as e:
            msg += f"\nFail to access model({llm['llm_name']})." + str(
                e)
    elif llm["model_type"] == LLMType.IMAGE2TEXT.value:
        mdl = CvModel[factory](
            key=llm["api_key"], 
            model_name=llm["llm_name"], 
            base_url=llm["api_base"]
        )
        try:
            img_url = (
                "https://upload.wikimedia.org/wikipedia/comm"
                "ons/thumb/d/dd/Gfp-wisconsin-madison-the-nature-boardwalk.jpg/256"
                "0px-Gfp-wisconsin-madison-the-nature-boardwalk.jpg"
            )
            res = requests.get(img_url)
            if res.status_code == 200:
                m, tc = mdl.describe(res.content)
                if not tc:
                    raise Exception(m)
            else:
                pass
        except Exception as e:
            msg += f"\nFail to access model({llm['llm_name']})." + str(e)
    elif llm["model_type"] == LLMType.TTS:
        mdl = TTSModel[factory](
            key=llm["api_key"], model_name=llm["llm_name"], base_url=llm["api_base"]
        )
        try:
            for resp in mdl.tts("Hello~ Ragflower!"):
                pass
        except RuntimeError as e:
            msg += f"\nFail to access model({llm['llm_name']})." + str(e)
    else:
        # TODO: check other type of models
        pass

    if msg:
        return get_data_error_result(retmsg=msg)

    if not TenantLLMService.filter_update(
            [TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == factory, TenantLLM.llm_name == llm["llm_name"]], llm):
        TenantLLMService.save(**llm)

    return get_json_result(data=True)


@manager.route('/delete_llm', methods=['POST'])
@login_required
@validate_request("llm_factory", "llm_name")
def delete_llm():
    req = request.json
    TenantLLMService.filter_delete(
            [TenantLLM.tenant_id == current_user.id, TenantLLM.llm_factory == req["llm_factory"], TenantLLM.llm_name == req["llm_name"]])
    return get_json_result(data=True)


@manager.route('/my_llms', methods=['GET'])
@login_required
def my_llms():
    try:
        res = {}
        for o in TenantLLMService.get_my_llms(current_user.id):
            if o["llm_factory"] not in res:
                res[o["llm_factory"]] = {
                    "tags": o["tags"],
                    "llm": []
                }
            res[o["llm_factory"]]["llm"].append({
                "type": o["model_type"],
                "name": o["llm_name"],
                "used_token": o["used_tokens"]
            })
        return get_json_result(data=res)
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['GET'])
@login_required
def list_app():
    model_type = request.args.get("model_type")
    try:
        objs = TenantLLMService.query(tenant_id=current_user.id)
        facts = set([o.to_dict()["llm_factory"] for o in objs if o.api_key])
        llms = LLMService.get_all()
        llms = [m.to_dict()
                for m in llms if m.status == StatusEnum.VALID.value]
        for m in llms:
            m["available"] = m["fid"] in facts or m["llm_name"].lower() == "flag-embedding" or m["fid"] in ["Youdao","FastEmbed", "BAAI"]

        llm_set = set([m["llm_name"] for m in llms])
        for o in objs:
            if not o.api_key:continue
            if o.llm_name in llm_set:continue
            llms.append({"llm_name": o.llm_name, "model_type": o.model_type, "fid": o.llm_factory, "available": True})

        res = {}
        for m in llms:
            if model_type and m["model_type"].find(model_type)<0:
                continue
            if m["fid"] not in res:
                res[m["fid"]] = []
            res[m["fid"]].append(m)

        return get_json_result(data=res)
    except Exception as e:
        return server_error_response(e)
