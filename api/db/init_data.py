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
import base64
import json
import os
import time
import uuid
from copy import deepcopy

from api.db import LLMType, UserTenantRole
from api.db.db_models import init_database_tables as init_web_db, LLMFactories, LLM, TenantLLM
from api.db.services import UserService
from api.db.services.canvas_service import CanvasTemplateService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMFactoriesService, LLMService, TenantLLMService, LLMBundle
from api.db.services.user_service import TenantService, UserTenantService
from api import settings
from api.utils.file_utils import get_project_base_directory


def encode_to_base64(input_string):
    base64_encoded = base64.b64encode(input_string.encode('utf-8'))
    return base64_encoded.decode('utf-8')


def init_superuser():
    user_info = {
        "id": uuid.uuid1().hex,
        "password": encode_to_base64("admin"),
        "nickname": "admin",
        "is_superuser": True,
        "email": "admin@ragflow.io",
        "creator": "system",
        "status": "1",
    }
    tenant = {
        "id": user_info["id"],
        "name": user_info["nickname"] + "‘s Kingdom",
        "llm_id": settings.CHAT_MDL,
        "embd_id": settings.EMBEDDING_MDL,
        "asr_id": settings.ASR_MDL,
        "parser_ids": settings.PARSERS,
        "img2txt_id": settings.IMAGE2TEXT_MDL
    }
    usr_tenant = {
        "tenant_id": user_info["id"],
        "user_id": user_info["id"],
        "invited_by": user_info["id"],
        "role": UserTenantRole.OWNER
    }
    tenant_llm = []
    for llm in LLMService.query(fid=settings.LLM_FACTORY):
        tenant_llm.append(
            {"tenant_id": user_info["id"], "llm_factory": settings.LLM_FACTORY, "llm_name": llm.llm_name,
             "model_type": llm.model_type,
             "api_key": settings.API_KEY, "api_base": settings.LLM_BASE_URL})

    if not UserService.save(**user_info):
        logging.error("can't init admin.")
        return
    TenantService.insert(**tenant)
    UserTenantService.insert(**usr_tenant)
    TenantLLMService.insert_many(tenant_llm)
    logging.info(
        "Super user initialized. email: admin@ragflow.io, password: admin. Changing the password after login is strongly recommended.")

    chat_mdl = LLMBundle(tenant["id"], LLMType.CHAT, tenant["llm_id"])
    msg = chat_mdl.chat(system="", history=[
        {"role": "user", "content": "Hello!"}], gen_conf={})
    if msg.find("ERROR: ") == 0:
        logging.error(
            "'{}' dosen't work. {}".format(
                tenant["llm_id"],
                msg))
    embd_mdl = LLMBundle(tenant["id"], LLMType.EMBEDDING, tenant["embd_id"])
    v, c = embd_mdl.encode(["Hello!"])
    if c == 0:
        logging.error(
            "'{}' dosen't work!".format(
                tenant["embd_id"]))


def init_llm_factory():
    try:
        LLMService.filter_delete([(LLM.fid == "MiniMax" or LLM.fid == "Minimax")])
        LLMService.filter_delete([(LLM.fid == "cohere")])
        LLMFactoriesService.filter_delete([LLMFactories.name == "cohere"])
    except Exception:
        pass

    factory_llm_infos = json.load(
        open(
            os.path.join(get_project_base_directory(), "conf", "llm_factories.json"),
            "r",
        )
    )
    for factory_llm_info in factory_llm_infos["factory_llm_infos"]:
        llm_infos = factory_llm_info.pop("llm")
        try:
            LLMFactoriesService.save(**factory_llm_info)
        except Exception:
            pass
        LLMService.filter_delete([LLM.fid == factory_llm_info["name"]])
        for llm_info in llm_infos:
            llm_info["fid"] = factory_llm_info["name"]
            try:
                LLMService.save(**llm_info)
            except Exception:
                pass

    LLMFactoriesService.filter_delete([LLMFactories.name == "Local"])
    LLMService.filter_delete([LLM.fid == "Local"])
    LLMService.filter_delete([LLM.llm_name == "qwen-vl-max"])
    LLMService.filter_delete([LLM.fid == "Moonshot", LLM.llm_name == "flag-embedding"])
    TenantLLMService.filter_delete([TenantLLM.llm_factory == "Moonshot", TenantLLM.llm_name == "flag-embedding"])
    LLMFactoriesService.filter_delete([LLMFactoriesService.model.name == "QAnything"])
    LLMService.filter_delete([LLMService.model.fid == "QAnything"])
    TenantLLMService.filter_update([TenantLLMService.model.llm_factory == "QAnything"], {"llm_factory": "Youdao"})
    TenantLLMService.filter_update([TenantLLMService.model.llm_factory == "cohere"], {"llm_factory": "Cohere"})
    TenantService.filter_update([1 == 1], {
        "parser_ids": "naive:General,qa:Q&A,resume:Resume,manual:Manual,table:Table,paper:Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One,audio:Audio,knowledge_graph:Knowledge Graph,email:Email"})
    ## insert openai two embedding models to the current openai user.
    # print("Start to insert 2 OpenAI embedding models...")
    tenant_ids = set([row["tenant_id"] for row in TenantLLMService.get_openai_models()])
    for tid in tenant_ids:
        for row in TenantLLMService.query(llm_factory="OpenAI", tenant_id=tid):
            row = row.to_dict()
            row["model_type"] = LLMType.EMBEDDING.value
            row["llm_name"] = "text-embedding-3-small"
            row["used_tokens"] = 0
            try:
                TenantLLMService.save(**row)
                row = deepcopy(row)
                row["llm_name"] = "text-embedding-3-large"
                TenantLLMService.save(**row)
            except Exception:
                pass
            break
    for kb_id in KnowledgebaseService.get_all_ids():
        KnowledgebaseService.update_by_id(kb_id, {"doc_num": DocumentService.get_kb_doc_count(kb_id)})
    """
    drop table llm;
    drop table llm_factories;
    update tenant set parser_ids='naive:General,qa:Q&A,resume:Resume,manual:Manual,table:Table,paper:Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One,audio:Audio,knowledge_graph:Knowledge Graph';
    alter table knowledgebase modify avatar longtext;
    alter table user modify avatar longtext;
    alter table dialog modify icon longtext;
    """


def add_graph_templates():
    dir = os.path.join(get_project_base_directory(), "agent", "templates")
    for fnm in os.listdir(dir):
        try:
            cnvs = json.load(open(os.path.join(dir, fnm), "r"))
            try:
                CanvasTemplateService.save(**cnvs)
            except:
                CanvasTemplateService.update_by_id(cnvs["id"], cnvs)
        except Exception:
            logging.exception("Add graph templates error: ")


def init_web_data():
    start_time = time.time()

    init_llm_factory()
    # if not UserService.get_all().count():
    #    init_superuser()

    add_graph_templates()
    logging.info("init web data success:{}".format(time.time() - start_time))


if __name__ == '__main__':
    init_web_db()
    init_web_data()
