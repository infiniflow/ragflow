#
#  Copyright 2019 The InfiniFlow Authors. All Rights Reserved.
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
import time
import uuid

from api.db import LLMType
from api.db.db_models import init_database_tables as init_web_db
from api.db.services import UserService
from api.db.services.llm_service import LLMFactoriesService, LLMService


def init_superuser():
    user_info = {
        "id": uuid.uuid1().hex,
        "password": "admin",
        "nickname": "admin",
        "is_superuser": True,
        "email": "kai.hu@infiniflow.org",
        "creator": "system",
        "status": "1",
    }
    UserService.save(**user_info)


def init_llm_factory():
    factory_infos = [{
            "name": "OpenAI",
            "logo": "",
            "tags": "LLM,TEXT EMBEDDING,SPEECH2TEXT,MODERATION",
            "status": "1",
        },{
            "name": "通义千问",
            "logo": "",
            "tags": "LLM,TEXT EMBEDDING,SPEECH2TEXT,MODERATION",
            "status": "1",
        },{
            "name": "智普AI",
            "logo": "",
            "tags": "LLM,TEXT EMBEDDING,SPEECH2TEXT,MODERATION",
            "status": "1",
        },{
            "name": "文心一言",
            "logo": "",
            "tags": "LLM,TEXT EMBEDDING,SPEECH2TEXT,MODERATION",
            "status": "1",
        },
    ]
    llm_infos = [{
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-3.5-turbo",
            "tags": "LLM,CHAT,4K",
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-3.5-turbo-16k-0613",
            "tags": "LLM,CHAT,16k",
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "text-embedding-ada-002",
            "tags": "TEXT EMBEDDING,8K",
            "model_type": LLMType.EMBEDDING.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "whisper-1",
            "tags": "SPEECH2TEXT",
            "model_type": LLMType.SPEECH2TEXT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-4",
            "tags": "LLM,CHAT,8K",
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-4-32k",
            "tags": "LLM,CHAT,32K",
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-4-vision-preview",
            "tags": "LLM,CHAT,IMAGE2TEXT",
            "model_type": LLMType.IMAGE2TEXT.value
        },{
            "fid": factory_infos[1]["name"],
            "llm_name": "qwen-turbo",
            "tags": "LLM,CHAT,8K",
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[1]["name"],
            "llm_name": "qwen-plus",
            "tags": "LLM,CHAT,32K",
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[1]["name"],
            "llm_name": "text-embedding-v2",
            "tags": "TEXT EMBEDDING,2K",
            "model_type": LLMType.EMBEDDING.value
        },{
            "fid": factory_infos[1]["name"],
            "llm_name": "paraformer-realtime-8k-v1",
            "tags": "SPEECH2TEXT",
            "model_type": LLMType.SPEECH2TEXT.value
        },{
            "fid": factory_infos[1]["name"],
            "llm_name": "qwen_vl_chat_v1",
            "tags": "LLM,CHAT,IMAGE2TEXT",
            "model_type": LLMType.IMAGE2TEXT.value
        },
    ]
    for info in factory_infos:
        LLMFactoriesService.save(**info)
    for info in llm_infos:
        LLMService.save(**info)


def init_web_data():
    start_time = time.time()
    if not UserService.get_all().count():
        init_superuser()

    if not LLMService.get_all().count():init_llm_factory()

    print("init web data success:{}".format(time.time() - start_time))


if __name__ == '__main__':
    init_web_db()
    init_web_data()