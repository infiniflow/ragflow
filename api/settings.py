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
import os
from datetime import date
from enum import IntEnum, Enum
import rag.utils.es_conn
import rag.utils.infinity_conn

import rag.utils
from rag.nlp import search
from graphrag import search as kg_search
from api.utils import get_base_config, decrypt_database_config

API_VERSION = "v1"
RAG_FLOW_SERVICE_NAME = "ragflow"
LIGHTEN = int(os.environ.get('LIGHTEN', "0"))

REQUEST_WAIT_SEC = 2
REQUEST_MAX_WAIT_SEC = 300

LLM = get_base_config("user_default_llm", {})
LLM_FACTORY = LLM.get("factory", "Tongyi-Qianwen")
LLM_BASE_URL = LLM.get("base_url")

CHAT_MDL = EMBEDDING_MDL = RERANK_MDL = ASR_MDL = IMAGE2TEXT_MDL = ""
if not LIGHTEN:
    default_llm = {
        "Tongyi-Qianwen": {
            "chat_model": "qwen-plus",
            "embedding_model": "text-embedding-v2",
            "image2text_model": "qwen-vl-max",
            "asr_model": "paraformer-realtime-8k-v1",
        },
        "OpenAI": {
            "chat_model": "gpt-3.5-turbo",
            "embedding_model": "text-embedding-ada-002",
            "image2text_model": "gpt-4-vision-preview",
            "asr_model": "whisper-1",
        },
        "Azure-OpenAI": {
            "chat_model": "gpt-35-turbo",
            "embedding_model": "text-embedding-ada-002",
            "image2text_model": "gpt-4-vision-preview",
            "asr_model": "whisper-1",
        },
        "ZHIPU-AI": {
            "chat_model": "glm-3-turbo",
            "embedding_model": "embedding-2",
            "image2text_model": "glm-4v",
            "asr_model": "",
        },
        "Ollama": {
            "chat_model": "qwen-14B-chat",
            "embedding_model": "flag-embedding",
            "image2text_model": "",
            "asr_model": "",
        },
        "Moonshot": {
            "chat_model": "moonshot-v1-8k",
            "embedding_model": "",
            "image2text_model": "",
            "asr_model": "",
        },
        "DeepSeek": {
            "chat_model": "deepseek-chat",
            "embedding_model": "",
            "image2text_model": "",
            "asr_model": "",
        },
        "VolcEngine": {
            "chat_model": "",
            "embedding_model": "",
            "image2text_model": "",
            "asr_model": "",
        },
        "BAAI": {
            "chat_model": "",
            "embedding_model": "BAAI/bge-large-zh-v1.5",
            "image2text_model": "",
            "asr_model": "",
            "rerank_model": "BAAI/bge-reranker-v2-m3",
        }
    }

    if LLM_FACTORY:
        CHAT_MDL = default_llm[LLM_FACTORY]["chat_model"] + f"@{LLM_FACTORY}"
        ASR_MDL = default_llm[LLM_FACTORY]["asr_model"] + f"@{LLM_FACTORY}"
        IMAGE2TEXT_MDL = default_llm[LLM_FACTORY]["image2text_model"] + f"@{LLM_FACTORY}"
    EMBEDDING_MDL = default_llm["BAAI"]["embedding_model"] + "@BAAI"
    RERANK_MDL = default_llm["BAAI"]["rerank_model"] + "@BAAI"

API_KEY = LLM.get("api_key", "")
PARSERS = LLM.get(
    "parsers",
    "naive:General,qa:Q&A,resume:Resume,manual:Manual,table:Table,paper:Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One,audio:Audio,knowledge_graph:Knowledge Graph,email:Email")

HOST = get_base_config(RAG_FLOW_SERVICE_NAME, {}).get("host", "127.0.0.1")
HTTP_PORT = get_base_config(RAG_FLOW_SERVICE_NAME, {}).get("http_port")

SECRET_KEY = get_base_config(
    RAG_FLOW_SERVICE_NAME,
    {}).get("secret_key", str(date.today()))

DATABASE_TYPE = os.getenv("DB_TYPE", 'mysql')
DATABASE = decrypt_database_config(name=DATABASE_TYPE)

# authentication
AUTHENTICATION_CONF = get_base_config("authentication", {})

# client
CLIENT_AUTHENTICATION = AUTHENTICATION_CONF.get(
    "client", {}).get(
    "switch", False)
HTTP_APP_KEY = AUTHENTICATION_CONF.get("client", {}).get("http_app_key")
GITHUB_OAUTH = get_base_config("oauth", {}).get("github")
FEISHU_OAUTH = get_base_config("oauth", {}).get("feishu")

DOC_ENGINE = os.environ.get('DOC_ENGINE', "elasticsearch")
if DOC_ENGINE == "elasticsearch":
    docStoreConn = rag.utils.es_conn.ESConnection()
elif DOC_ENGINE == "infinity":
    docStoreConn = rag.utils.infinity_conn.InfinityConnection()
else:
    raise Exception(f"Not supported doc engine: {DOC_ENGINE}")

retrievaler = search.Dealer(docStoreConn)
kg_retrievaler = kg_search.KGSearch(docStoreConn)


class CustomEnum(Enum):
    @classmethod
    def valid(cls, value):
        try:
            cls(value)
            return True
        except BaseException:
            return False

    @classmethod
    def values(cls):
        return [member.value for member in cls.__members__.values()]

    @classmethod
    def names(cls):
        return [member.name for member in cls.__members__.values()]


class PythonDependenceName(CustomEnum):
    Rag_Source_Code = "python"
    Python_Env = "miniconda"


class ModelStorage(CustomEnum):
    REDIS = "redis"
    MYSQL = "mysql"


class RetCode(IntEnum, CustomEnum):
    SUCCESS = 0
    NOT_EFFECTIVE = 10
    EXCEPTION_ERROR = 100
    ARGUMENT_ERROR = 101
    DATA_ERROR = 102
    OPERATING_ERROR = 103
    CONNECTION_ERROR = 105
    RUNNING = 106
    PERMISSION_ERROR = 108
    AUTHENTICATION_ERROR = 109
    UNAUTHORIZED = 401
    SERVER_ERROR = 500
    FORBIDDEN = 403
    NOT_FOUND = 404
