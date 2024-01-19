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
from api.db.services.user_service import TenantService
from rag.llm import EmbeddingModel, CvModel
from api.db import LLMType
from api.db.db_models import DB, UserTenant
from api.db.db_models import LLMFactories, LLM, TenantLLM
from api.db.services.common_service import CommonService
from api.db import StatusEnum


class LLMFactoriesService(CommonService):
    model = LLMFactories


class LLMService(CommonService):
    model = LLM


class TenantLLMService(CommonService):
    model = TenantLLM

    @classmethod
    @DB.connection_context()
    def get_api_key(cls, tenant_id, model_name):
        objs = cls.query(tenant_id=tenant_id, llm_name=model_name)
        if not objs: return
        return objs[0]

    @classmethod
    @DB.connection_context()
    def get_my_llms(cls, tenant_id):
        fields = [cls.model.llm_factory, LLMFactories.logo, LLMFactories.tags, cls.model.model_type, cls.model.llm_name]
        objs = cls.model.select(*fields).join(LLMFactories, on=(cls.model.llm_factory == LLMFactories.name)).where(
            cls.model.tenant_id == tenant_id).dicts()

        return list(objs)

    @classmethod
    @DB.connection_context()
    def model_instance(cls, tenant_id, llm_type):
        e,tenant = TenantService.get_by_id(tenant_id)
        if not e: raise LookupError("Tenant not found")

        if llm_type == LLMType.EMBEDDING.value: mdlnm = tenant.embd_id
        elif llm_type == LLMType.SPEECH2TEXT.value: mdlnm = tenant.asr_id
        elif llm_type == LLMType.IMAGE2TEXT.value: mdlnm = tenant.img2txt_id
        elif llm_type == LLMType.CHAT.value: mdlnm = tenant.llm_id
        else: assert False, "LLM type error"

        model_config = cls.get_api_key(tenant_id, mdlnm)
        if not model_config: raise LookupError("Model({}) not found".format(mdlnm))
        model_config = model_config[0].to_dict()
        if llm_type == LLMType.EMBEDDING.value:
            if model_config["llm_factory"] not in EmbeddingModel: return
            return EmbeddingModel[model_config["llm_factory"]](model_config["api_key"], model_config["llm_name"])

        if llm_type == LLMType.IMAGE2TEXT.value:
            if model_config["llm_factory"] not in CvModel: return
            return CvModel[model_config["llm_factory"]](model_config["api_key"], model_config["llm_name"])
