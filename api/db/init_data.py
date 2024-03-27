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

from api.db import LLMType, UserTenantRole
from api.db.db_models import init_database_tables as init_web_db
from api.db.services import UserService
from api.db.services.llm_service import LLMFactoriesService, LLMService, TenantLLMService, LLMBundle
from api.db.services.user_service import TenantService, UserTenantService
from api.settings import CHAT_MDL, EMBEDDING_MDL, ASR_MDL, IMAGE2TEXT_MDL, PARSERS, LLM_FACTORY, API_KEY


def init_superuser():
    user_info = {
        "id": uuid.uuid1().hex,
        "password": "admin",
        "nickname": "admin",
        "is_superuser": True,
        "email": "admin@ragflow.io",
        "creator": "system",
        "status": "1",
    }
    tenant = {
        "id": user_info["id"],
        "name": user_info["nickname"] + "‘s Kingdom",
        "llm_id": CHAT_MDL,
        "embd_id": EMBEDDING_MDL,
        "asr_id": ASR_MDL,
        "parser_ids": PARSERS,
        "img2txt_id": IMAGE2TEXT_MDL
    }
    usr_tenant = {
        "tenant_id": user_info["id"],
        "user_id": user_info["id"],
        "invited_by": user_info["id"],
        "role": UserTenantRole.OWNER
    }
    tenant_llm = []
    for llm in LLMService.query(fid=LLM_FACTORY):
        tenant_llm.append(
            {"tenant_id": user_info["id"], "llm_factory": LLM_FACTORY, "llm_name": llm.llm_name, "model_type": llm.model_type,
             "api_key": API_KEY})

    if not UserService.save(**user_info):
        print("\033[93m【ERROR】\033[0mcan't init admin.")
        return
    TenantService.insert(**tenant)
    UserTenantService.insert(**usr_tenant)
    TenantLLMService.insert_many(tenant_llm)
    print(
        "【INFO】Super user initialized. \033[93memail: admin@ragflow.io, password: admin\033[0m. Changing the password after logining is strongly recomanded.")

    chat_mdl = LLMBundle(tenant["id"], LLMType.CHAT, tenant["llm_id"])
    msg = chat_mdl.chat(system="", history=[
                        {"role": "user", "content": "Hello!"}], gen_conf={})
    if msg.find("ERROR: ") == 0:
        print(
            "\33[91m【ERROR】\33[0m: ",
            "'{}' dosen't work. {}".format(
                tenant["llm_id"],
                msg))
    embd_mdl = LLMBundle(tenant["id"], LLMType.EMBEDDING, tenant["embd_id"])
    v, c = embd_mdl.encode(["Hello!"])
    if c == 0:
        print(
            "\33[91m【ERROR】\33[0m:",
            " '{}' dosen't work!".format(
                tenant["embd_id"]))


factory_infos = [{
    "name": "OpenAI",
    "logo": "",
    "tags": "LLM,TEXT EMBEDDING,SPEECH2TEXT,MODERATION",
    "status": "1",
}, {
    "name": "Tongyi-Qianwen",
    "logo": "",
    "tags": "LLM,TEXT EMBEDDING,SPEECH2TEXT,MODERATION",
    "status": "1",
}, {
    "name": "ZHIPU-AI",
    "logo": "",
    "tags": "LLM,TEXT EMBEDDING,SPEECH2TEXT,MODERATION",
    "status": "1",
},
    {
    "name": "Local",
    "logo": "",
    "tags": "LLM,TEXT EMBEDDING,SPEECH2TEXT,MODERATION",
        "status": "1",
}, {
        "name": "Moonshot",
    "logo": "",
    "tags": "LLM,TEXT EMBEDDING",
    "status": "1",
}
    # {
    #     "name": "文心一言",
    #     "logo": "",
    #     "tags": "LLM,TEXT EMBEDDING,SPEECH2TEXT,MODERATION",
    #     "status": "1",
    # },
]


def init_llm_factory():
    llm_infos = [
        # ---------------------- OpenAI ------------------------
        {
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-3.5-turbo",
            "tags": "LLM,CHAT,4K",
            "max_tokens": 4096,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-3.5-turbo-16k-0613",
            "tags": "LLM,CHAT,16k",
            "max_tokens": 16385,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[0]["name"],
            "llm_name": "text-embedding-ada-002",
            "tags": "TEXT EMBEDDING,8K",
            "max_tokens": 8191,
            "model_type": LLMType.EMBEDDING.value
        }, {
            "fid": factory_infos[0]["name"],
            "llm_name": "whisper-1",
            "tags": "SPEECH2TEXT",
            "max_tokens": 25 * 1024 * 1024,
            "model_type": LLMType.SPEECH2TEXT.value
        }, {
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-4",
            "tags": "LLM,CHAT,8K",
            "max_tokens": 8191,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[0]["name"],
            "llm_name": "gpt-4-32k",
            "tags": "LLM,CHAT,32K",
            "max_tokens": 32768,
            "model_type": LLMType.CHAT.value
        }, {
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
        }, {
            "fid": factory_infos[1]["name"],
            "llm_name": "qwen-plus",
            "tags": "LLM,CHAT,32K",
            "max_tokens": 32768,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[1]["name"],
            "llm_name": "qwen-max-1201",
            "tags": "LLM,CHAT,6K",
            "max_tokens": 5899,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[1]["name"],
            "llm_name": "text-embedding-v2",
            "tags": "TEXT EMBEDDING,2K",
            "max_tokens": 2048,
            "model_type": LLMType.EMBEDDING.value
        }, {
            "fid": factory_infos[1]["name"],
            "llm_name": "paraformer-realtime-8k-v1",
            "tags": "SPEECH2TEXT",
            "max_tokens": 25 * 1024 * 1024,
            "model_type": LLMType.SPEECH2TEXT.value
        }, {
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
            "model_type": LLMType.EMBEDDING.value
        },
        # ---------------------- 本地 ----------------------
        {
            "fid": factory_infos[3]["name"],
            "llm_name": "qwen-14B-chat",
            "tags": "LLM,CHAT,",
            "max_tokens": 4096,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[3]["name"],
            "llm_name": "flag-embedding",
            "tags": "TEXT EMBEDDING,",
            "max_tokens": 128 * 1000,
            "model_type": LLMType.EMBEDDING.value
        },
        # ------------------------ Moonshot -----------------------
        {
            "fid": factory_infos[4]["name"],
            "llm_name": "moonshot-v1-8k",
            "tags": "LLM,CHAT,",
            "max_tokens": 7900,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[4]["name"],
            "llm_name": "flag-embedding",
            "tags": "TEXT EMBEDDING,",
            "max_tokens": 128 * 1000,
            "model_type": LLMType.EMBEDDING.value
        }, {
            "fid": factory_infos[4]["name"],
            "llm_name": "moonshot-v1-32k",
            "tags": "LLM,CHAT,",
            "max_tokens": 32768,
            "model_type": LLMType.CHAT.value
        }, {
            "fid": factory_infos[4]["name"],
            "llm_name": "moonshot-v1-128k",
            "tags": "LLM,CHAT",
            "max_tokens": 128 * 1000,
            "model_type": LLMType.CHAT.value
        },
    ]
    for info in factory_infos:
        try:
            LLMFactoriesService.save(**info)
        except Exception as e:
            pass
    for info in llm_infos:
        try:
            LLMService.save(**info)
        except Exception as e:
            pass

    """
    modify service_config
    drop table llm;
    drop table llm_factories;
    update tenant_llm set llm_factory='Tongyi-Qianwen' where llm_factory='通义千问';
    update tenant_llm set llm_factory='ZHIPU-AI' where llm_factory='智谱AI';
    update tenant set parser_ids='naive:General,qa:Q&A,resume:Resume,manual:Manual,table:Table,paper:Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One';
    alter table knowledgebase modify avatar longtext;
    alter table user modify avatar longtext;
    alter table dialog modify icon longtext;
    """


def init_web_data():
    start_time = time.time()

    if LLMFactoriesService.get_all().count() != len(factory_infos):
        init_llm_factory()
    if not UserService.get_all().count():
        init_superuser()

    print("init web data success:{}".format(time.time() - start_time))


if __name__ == '__main__':
    init_web_db()
    init_web_data()
