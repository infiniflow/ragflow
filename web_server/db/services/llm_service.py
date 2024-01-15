#
#  Copyright 2019 The FATE Authors. All Rights Reserved.
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
import peewee
from werkzeug.security import generate_password_hash, check_password_hash

from web_server.db.db_models import DB, UserTenant
from web_server.db.db_models import LLMFactories, LLM, TenantLLM
from web_server.db.services.common_service import CommonService
from web_server.utils import get_uuid, get_format_time
from web_server.db.db_utils import StatusEnum


class LLMFactoriesService(CommonService):
    model = LLMFactories


class LLMService(CommonService):
    model = LLM


class TenantLLMService(CommonService):
    model = TenantLLM

    @classmethod
    @DB.connection_context()
    def get_api_key(cls, tenant_id, model_type):
        objs = cls.query(tenant_id=tenant_id, model_type=model_type)
        if objs and len(objs)>0 and objs[0].llm_name:
            return objs[0]

        fields = [LLM.llm_name, cls.model.llm_factory, cls.model.api_key]
        objs = cls.model.select(*fields).join(LLM, on=(LLM.fid == cls.model.llm_factory)).where(
            (cls.model.tenant_id == tenant_id),
            (cls.model.model_type == model_type),
            (LLM.status == StatusEnum.VALID)
        )

        if not objs:return
        return objs[0]

