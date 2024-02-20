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
    llm_infos = [
        # ---------------------- OpenAI ------------------------
        {
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-3.5-turbo",
            "tags": "LLM,CHAT,4K",
            "max_tokens": 4096,
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-3.5-turbo-16k-0613",
            "tags": "LLM,CHAT,16k",
            "max_tokens": 16385,
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "text-embedding-ada-002",
            "tags": "TEXT EMBEDDING,8K",
            "max_tokens": 8191,
            "model_type": LLMType.EMBEDDING.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "whisper-1",
            "tags": "SPEECH2TEXT",
            "max_tokens": 25*1024*1024,
            "model_type": LLMType.SPEECH2TEXT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-4",
            "tags": "LLM,CHAT,8K",
            "max_tokens": 8191,
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-4-32k",
            "tags": "LLM,CHAT,32K",
            "max_tokens": 32768,
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-4-vision-preview",
            "tags": "LLM,CHAT,IMAGE2TEXT",
            "max_tokens": 765,
            "model_type": LLMType.IMAGE2TEXT.value
        },
        # ----------------------- Qwen -----------------------
        {
            "fid": factory_infos[1]["name"],
            "llm_name": "qwen-turbo",
            "tags": "LLM,CHAT,8K",
            "max_tokens": 8191,
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[1]["name"],
            "llm_name": "qwen-plus",
            "tags": "LLM,CHAT,32K",
            "max_tokens": 32768,
            "model_type": LLMType.CHAT.value
        },{
            "fid": factory_infos[1]["name"],
            "llm_name": "text-embedding-v2",
            "tags": "TEXT EMBEDDING,2K",
            "max_tokens": 2048,
            "model_type": LLMType.EMBEDDING.value
        },{
            "fid": factory_infos[1]["name"],
            "llm_name": "paraformer-realtime-8k-v1",
            "tags": "SPEECH2TEXT",
            "max_tokens": 25*1024*1024,
            "model_type": LLMType.SPEECH2TEXT.value
        },{
            "fid": factory_infos[1]["name"],
            "llm_name": "qwen-vl-max",
            "tags": "LLM,CHAT,IMAGE2TEXT",
            "max_tokens": 765,
            "model_type": LLMType.IMAGE2TEXT.value
        },
        # ---------------------- ZhipuAI ----------------------
        {
            "fid": factory_infos[2]["name"],
            "llm_name": "glm-3-turbo",
            "tags": "LLM,CHAT,",
            "max_tokens": 128 * 1000,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[2]["name"],
            "llm_name": "glm-4",
            "tags": "LLM,CHAT,",
            "max_tokens": 128 * 1000,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[2]["name"],
            "llm_name": "glm-4v",
            "tags": "LLM,CHAT,IMAGE2TEXT",
            "max_tokens": 2000,
            "model_type": LLMType.IMAGE2TEXT.value
        },
        {
            "fid": factory_infos[2]["name"],
            "llm_name": "embedding-2",
            "tags": "TEXT EMBEDDING",
            "max_tokens": 512,
            "model_type": LLMType.SPEECH2TEXT.value
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