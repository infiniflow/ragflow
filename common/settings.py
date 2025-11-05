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
import os
import json
import secrets
from datetime import date
from common.constants import RAG_FLOW_SERVICE_NAME
from common.file_utils import get_project_base_directory
from common.config_utils import get_base_config, decrypt_database_config

import rag.utils
import rag.utils.es_conn
import rag.utils.infinity_conn
import rag.utils.opensearch_conn
from rag.nlp import search

LLM = None
LLM_FACTORY = None
LLM_BASE_URL = None
CHAT_MDL = ""
EMBEDDING_MDL = ""
RERANK_MDL = ""
ASR_MDL = ""
IMAGE2TEXT_MDL = ""


CHAT_CFG = ""
EMBEDDING_CFG = ""
RERANK_CFG = ""
ASR_CFG = ""
IMAGE2TEXT_CFG = ""
API_KEY = None
PARSERS = None
HOST_IP = None
HOST_PORT = None
SECRET_KEY = None
FACTORY_LLM_INFOS = None
ALLOWED_LLM_FACTORIES = None

DATABASE_TYPE = os.getenv("DB_TYPE", "mysql")
DATABASE = decrypt_database_config(name=DATABASE_TYPE)

# authentication
AUTHENTICATION_CONF = None

# client
CLIENT_AUTHENTICATION = None
HTTP_APP_KEY = None
GITHUB_OAUTH = None
FEISHU_OAUTH = None
OAUTH_CONFIG = None
DOC_ENGINE = os.getenv('DOC_ENGINE', 'elasticsearch')

docStoreConn = None

retriever = None
kg_retriever = None

# user registration switch
REGISTER_ENABLED = 1


# sandbox-executor-manager
SANDBOX_HOST = None
STRONG_TEST_COUNT = int(os.environ.get("STRONG_TEST_COUNT", "8"))

SMTP_CONF = None
MAIL_SERVER = ""
MAIL_PORT = 000
MAIL_USE_SSL = True
MAIL_USE_TLS = False
MAIL_USERNAME = ""
MAIL_PASSWORD = ""
MAIL_DEFAULT_SENDER = ()
MAIL_FRONTEND_URL = ""

# move from rag.settings
ES = {}
INFINITY = {}
AZURE = {}
S3 = {}
MINIO = {}
OSS = {}
OS = {}
REDIS = {}

STORAGE_IMPL_TYPE = os.getenv('STORAGE_IMPL', 'MINIO')

# Initialize the selected configuration data based on environment variables to solve the problem of initialization errors due to lack of configuration
if DOC_ENGINE == 'elasticsearch':
    ES = get_base_config("es", {})
elif DOC_ENGINE == 'opensearch':
    OS = get_base_config("os", {})
elif DOC_ENGINE == 'infinity':
    INFINITY = get_base_config("infinity", {"uri": "infinity:23817"})

if STORAGE_IMPL_TYPE in ['AZURE_SPN', 'AZURE_SAS']:
    AZURE = get_base_config("azure", {})
elif STORAGE_IMPL_TYPE == 'AWS_S3':
    S3 = get_base_config("s3", {})
elif STORAGE_IMPL_TYPE == 'MINIO':
    MINIO = decrypt_database_config(name="minio")
elif STORAGE_IMPL_TYPE == 'OSS':
    OSS = get_base_config("oss", {})

try:
    REDIS = decrypt_database_config(name="redis")
except Exception:
    try:
        REDIS = get_base_config("redis", {})
    except Exception:
        REDIS = {}



def _get_or_create_secret_key():
    secret_key = os.environ.get("RAGFLOW_SECRET_KEY")
    if secret_key and len(secret_key) >= 32:
        return secret_key

    # Check if there's a configured secret key
    configured_key = get_base_config(RAG_FLOW_SERVICE_NAME, {}).get("secret_key")
    if configured_key and configured_key != str(date.today()) and len(configured_key) >= 32:
        return configured_key

    # Generate a new secure key and warn about it
    import logging

    new_key = secrets.token_hex(32)
    logging.warning(f"SECURITY WARNING: Using auto-generated SECRET_KEY. Generated key: {new_key}")
    return new_key


def init_settings():
    global DATABASE_TYPE, DATABASE
    DATABASE_TYPE = os.getenv("DB_TYPE", "mysql")
    DATABASE = decrypt_database_config(name=DATABASE_TYPE)

    global ALLOWED_LLM_FACTORIES, LLM_FACTORY, LLM_BASE_URL
    llm_settings = get_base_config("user_default_llm", {}) or {}
    llm_default_models = llm_settings.get("default_models", {}) or {}
    LLM_FACTORY = llm_settings.get("factory", "") or ""
    LLM_BASE_URL = llm_settings.get("base_url", "") or ""
    ALLOWED_LLM_FACTORIES = llm_settings.get("allowed_factories", None)

    global REGISTER_ENABLED
    try:
        REGISTER_ENABLED = int(os.environ.get("REGISTER_ENABLED", "1"))
    except Exception:
        pass

    global FACTORY_LLM_INFOS
    try:
        with open(os.path.join(get_project_base_directory(), "conf", "llm_factories.json"), "r") as f:
            FACTORY_LLM_INFOS = json.load(f)["factory_llm_infos"]
    except Exception:
        FACTORY_LLM_INFOS = []

    global API_KEY
    API_KEY = llm_settings.get("api_key")

    global PARSERS
    PARSERS = llm_settings.get(
        "parsers", "naive:General,qa:Q&A,resume:Resume,manual:Manual,table:Table,paper:Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One,audio:Audio,email:Email,tag:Tag"
    )

    global CHAT_MDL, EMBEDDING_MDL, RERANK_MDL, ASR_MDL, IMAGE2TEXT_MDL
    chat_entry = _parse_model_entry(llm_default_models.get("chat_model", CHAT_MDL))
    embedding_entry = _parse_model_entry(llm_default_models.get("embedding_model", EMBEDDING_MDL))
    rerank_entry = _parse_model_entry(llm_default_models.get("rerank_model", RERANK_MDL))
    asr_entry = _parse_model_entry(llm_default_models.get("asr_model", ASR_MDL))
    image2text_entry = _parse_model_entry(llm_default_models.get("image2text_model", IMAGE2TEXT_MDL))

    global CHAT_CFG, EMBEDDING_CFG, RERANK_CFG, ASR_CFG, IMAGE2TEXT_CFG
    CHAT_CFG = _resolve_per_model_config(chat_entry, LLM_FACTORY, API_KEY, LLM_BASE_URL)
    EMBEDDING_CFG = _resolve_per_model_config(embedding_entry, LLM_FACTORY, API_KEY, LLM_BASE_URL)
    RERANK_CFG = _resolve_per_model_config(rerank_entry, LLM_FACTORY, API_KEY, LLM_BASE_URL)
    ASR_CFG = _resolve_per_model_config(asr_entry, LLM_FACTORY, API_KEY, LLM_BASE_URL)
    IMAGE2TEXT_CFG = _resolve_per_model_config(image2text_entry, LLM_FACTORY, API_KEY, LLM_BASE_URL)

    CHAT_MDL = CHAT_CFG.get("model", "") or ""
    EMBEDDING_MDL = os.getenv("TEI_MODEL", "BAAI/bge-small-en-v1.5") if "tei-" in os.getenv("COMPOSE_PROFILES", "") else ""
    RERANK_MDL = RERANK_CFG.get("model", "") or ""
    ASR_MDL = ASR_CFG.get("model", "") or ""
    IMAGE2TEXT_MDL = IMAGE2TEXT_CFG.get("model", "") or ""

    global HOST_IP, HOST_PORT
    HOST_IP = get_base_config(RAG_FLOW_SERVICE_NAME, {}).get("host", "127.0.0.1")
    HOST_PORT = get_base_config(RAG_FLOW_SERVICE_NAME, {}).get("http_port")

    global SECRET_KEY
    SECRET_KEY = _get_or_create_secret_key()


    # authentication
    authentication_conf = get_base_config("authentication", {})

    global CLIENT_AUTHENTICATION, HTTP_APP_KEY, GITHUB_OAUTH, FEISHU_OAUTH, OAUTH_CONFIG
    # client
    CLIENT_AUTHENTICATION = authentication_conf.get("client", {}).get("switch", False)
    HTTP_APP_KEY = authentication_conf.get("client", {}).get("http_app_key")
    GITHUB_OAUTH = get_base_config("oauth", {}).get("github")
    FEISHU_OAUTH = get_base_config("oauth", {}).get("feishu")
    OAUTH_CONFIG = get_base_config("oauth", {})

    global DOC_ENGINE, docStoreConn
    DOC_ENGINE = os.environ.get("DOC_ENGINE", "elasticsearch")
    # DOC_ENGINE = os.environ.get('DOC_ENGINE', "opensearch")
    lower_case_doc_engine = DOC_ENGINE.lower()
    if lower_case_doc_engine == "elasticsearch":
        docStoreConn = rag.utils.es_conn.ESConnection()
    elif lower_case_doc_engine == "infinity":
        docStoreConn = rag.utils.infinity_conn.InfinityConnection()
    elif lower_case_doc_engine == "opensearch":
        docStoreConn = rag.utils.opensearch_conn.OSConnection()
    else:
        raise Exception(f"Not supported doc engine: {DOC_ENGINE}")

    global retriever, kg_retriever
    retriever = search.Dealer(docStoreConn)
    from graphrag import search as kg_search

    kg_retriever = kg_search.KGSearch(docStoreConn)

    global SANDBOX_HOST
    if int(os.environ.get("SANDBOX_ENABLED", "0")):
        SANDBOX_HOST = os.environ.get("SANDBOX_HOST", "sandbox-executor-manager")

    global SMTP_CONF
    SMTP_CONF = get_base_config("smtp", {})

    global MAIL_SERVER, MAIL_PORT, MAIL_USE_SSL, MAIL_USE_TLS, MAIL_USERNAME, MAIL_PASSWORD, MAIL_DEFAULT_SENDER, MAIL_FRONTEND_URL
    MAIL_SERVER = SMTP_CONF.get("mail_server", "")
    MAIL_PORT = SMTP_CONF.get("mail_port", 000)
    MAIL_USE_SSL = SMTP_CONF.get("mail_use_ssl", True)
    MAIL_USE_TLS = SMTP_CONF.get("mail_use_tls", False)
    MAIL_USERNAME = SMTP_CONF.get("mail_username", "")
    MAIL_PASSWORD = SMTP_CONF.get("mail_password", "")
    mail_default_sender = SMTP_CONF.get("mail_default_sender", [])
    if mail_default_sender and len(mail_default_sender) >= 2:
        MAIL_DEFAULT_SENDER = (mail_default_sender[0], mail_default_sender[1])
    MAIL_FRONTEND_URL = SMTP_CONF.get("mail_frontend_url", "")


def _parse_model_entry(entry):
    if isinstance(entry, str):
        return {"name": entry, "factory": None, "api_key": None, "base_url": None}
    if isinstance(entry, dict):
        name = entry.get("name") or entry.get("model") or ""
        return {
            "name": name,
            "factory": entry.get("factory"),
            "api_key": entry.get("api_key"),
            "base_url": entry.get("base_url"),
        }
    return {"name": "", "factory": None, "api_key": None, "base_url": None}


def _resolve_per_model_config(entry_dict, backup_factory, backup_api_key, backup_base_url):
    name = (entry_dict.get("name") or "").strip()
    m_factory = entry_dict.get("factory") or backup_factory or ""
    m_api_key = entry_dict.get("api_key") or backup_api_key or ""
    m_base_url = entry_dict.get("base_url") or backup_base_url or ""

    if name and "@" not in name and m_factory:
        name = f"{name}@{m_factory}"

    return {
        "model": name,
        "factory": m_factory,
        "api_key": m_api_key,
        "base_url": m_base_url,
    }
