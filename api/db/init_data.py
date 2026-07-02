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
import asyncio
import logging
import json
import os
import time
import uuid

from peewee import IntegrityError
from api.db import UserTenantRole
from api.db.db_models import init_database_tables as init_web_db
from api.db.services import UserService
from api.db.services.canvas_service import CanvasTemplateService
from api.db.services.compilation_template_service import CompilationTemplateService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.services.user_service import TenantService, UserTenantService
from api.db.services.system_settings_service import SystemSettingsService
from api.db.template_utils import normalize_canvas_template_categories
from api.db.joint_services.memory_message_service import init_message_id_sequence, init_memory_size_cache, fix_missing_tokenized_memory
from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type
from common.constants import LLMType
from common.file_utils import get_project_base_directory
from common import settings
from api.common.base64 import encode_to_base64

DEFAULT_SUPERUSER_NICKNAME = os.getenv("DEFAULT_SUPERUSER_NICKNAME", "admin")
DEFAULT_SUPERUSER_EMAIL = os.getenv("DEFAULT_SUPERUSER_EMAIL", "admin@ragflow.io")
DEFAULT_SUPERUSER_PASSWORD = os.getenv("DEFAULT_SUPERUSER_PASSWORD", "admin")


def init_superuser(nickname=DEFAULT_SUPERUSER_NICKNAME, email=DEFAULT_SUPERUSER_EMAIL, password=DEFAULT_SUPERUSER_PASSWORD, role=UserTenantRole.OWNER):
    if UserService.query(email=email):
        logging.info("User with email %s already exists, skipping initialization.", email)
        return

    user_info = {
        "id": uuid.uuid1().hex,
        "password": encode_to_base64(password),
        "nickname": nickname,
        "is_superuser": True,
        "email": email,
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
        "img2txt_id": settings.IMAGE2TEXT_MDL,
        "rerank_id": settings.RERANK_MDL,
    }
    usr_tenant = {"tenant_id": user_info["id"], "user_id": user_info["id"], "invited_by": user_info["id"], "role": role}

    try:
        if not UserService.save(**user_info):
            logging.error("can't init admin.")
            return
    except IntegrityError:
        logging.info("User with email %s already exists, skipping.", email)
        return
    TenantService.insert(**tenant)
    UserTenantService.insert(**usr_tenant)
    logging.info(f"Super user initialized. email: {email},A default password has been set; changing the password after login is strongly recommended.")

    if tenant["llm_id"]:
        chat_model_config = get_tenant_default_model_by_type(tenant["id"], LLMType.CHAT)
        chat_mdl = LLMBundle(tenant["id"], chat_model_config)
        msg = asyncio.run(chat_mdl.async_chat(system="", history=[{"role": "user", "content": "Hello!"}], gen_conf={}))
        if msg.find("ERROR: ") == 0:
            logging.error("'{}' doesn't work. {}".format(tenant["llm_id"], msg))

    if tenant["embd_id"]:
        embd_model_config = get_tenant_default_model_by_type(tenant["id"], LLMType.EMBEDDING)
        embd_mdl = LLMBundle(tenant["id"], embd_model_config)
        v, c = embd_mdl.encode(["Hello!"])
        if c == 0:
            # Don't log the model identifier verbatim: CodeQL flags it
            # as potential sensitive data in clear text. The ID itself
            # is non-sensitive, but the pattern matches any string
            # sourced from tenant config that could carry credentials.
            logging.error("embedding model failed sanity-check encode")


def update_document_number_in_init():
    doc_count = DocumentService.get_all_kb_doc_count()
    for kb_id in KnowledgebaseService.get_all_ids():
        KnowledgebaseService.update_document_number_in_init(kb_id=kb_id, doc_num=doc_count.get(kb_id, 0))


def add_graph_templates():
    dir = os.path.join(get_project_base_directory(), "agent", "templates")
    CanvasTemplateService.filter_delete([1 == 1])
    if not os.path.exists(dir):
        logging.warning("Missing agent templates!")
        return

    for fnm in sorted(os.listdir(dir)):
        if not fnm.endswith(".json"):
            logging.debug("Skipping non-json template file in %s: %s", dir, fnm)
            continue
        template_path = os.path.join(dir, fnm)
        try:
            with open(template_path, "r", encoding="utf-8") as f:
                cnvs = normalize_canvas_template_categories(json.load(f))
            logging.info("Loaded and normalized template file: %s", template_path)
            try:
                CanvasTemplateService.save(**cnvs)
            except Exception:
                CanvasTemplateService.update_by_id(cnvs["id"], cnvs)
        except Exception as e:
            logging.exception("Add agent templates error for %s: %s", template_path, e)


def add_compilation_templates():
    CompilationTemplateService.seed_builtins_from_files()


def init_web_data():
    start_time = time.time()

    init_table()

    # init_llm_factory()
    update_document_number_in_init()
    # if not UserService.get_all().count():
    #    init_superuser()

    add_graph_templates()
    add_compilation_templates()
    init_message_id_sequence()
    init_memory_size_cache()
    fix_missing_tokenized_memory()
    logging.info("init web data success:{}".format(time.time() - start_time))


def init_table():
    # init system_settings
    with open(os.path.join(get_project_base_directory(), "conf", "system_settings.json"), "r") as f:
        records_from_file = json.load(f)["system_settings"]

    record_index = {}
    records_from_db = SystemSettingsService.get_all()
    for index, record in enumerate(records_from_db):
        record_index[record.name] = index

    to_save = []
    for record in records_from_file:
        setting_name = record["name"]
        if setting_name not in record_index:
            to_save.append(record)

    len_to_save = len(to_save)
    if len_to_save > 0:
        # not initialized
        try:
            SystemSettingsService.insert_many(to_save, len_to_save)
        except Exception as e:
            logging.exception("System settings init error: {}".format(e))
            raise e


if __name__ == "__main__":
    init_web_db()
    init_web_data()
