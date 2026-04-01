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
import hashlib
import logging
import json
import os
import time
import uuid
from copy import deepcopy

from peewee import IntegrityError

from api.db import UserTenantRole
from api.db.db_models import init_database_tables as init_web_db, LLMFactories, LLM, TenantLLM, Knowledgebase, Dialog, Memory
from api.db.services import UserService
from api.db.services.canvas_service import CanvasTemplateService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.memory_service import MemoryService
from api.db.services.tenant_llm_service import LLMFactoriesService, TenantLLMService
from api.db.services.llm_service import LLMService, LLMBundle, get_init_tenant_llm
from api.db.services.user_service import TenantService, UserTenantService
from api.db.services.system_settings_service import SystemSettingsService
from api.db.services.dialog_service import DialogService
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
    usr_tenant = {
        "tenant_id": user_info["id"],
        "user_id": user_info["id"],
        "invited_by": user_info["id"],
        "role": role
    }

    tenant_llm = get_init_tenant_llm(user_info["id"])

    try:
        if not UserService.save(**user_info):
            logging.error("can't init admin.")
            return
    except IntegrityError:
        logging.info("User with email %s already exists, skipping.", email)
        return
    TenantService.insert(**tenant)
    UserTenantService.insert(**usr_tenant)
    TenantLLMService.insert_many(tenant_llm)
    logging.info(
        f"Super user initialized. email: {email},A default password has been set; changing the password after login is strongly recommended.")

    if tenant["llm_id"]:
        chat_model_config = get_tenant_default_model_by_type(tenant["id"], LLMType.CHAT)
        chat_mdl = LLMBundle(tenant["id"], chat_model_config)
        msg = asyncio.run(chat_mdl.async_chat(system="", history=[{"role": "user", "content": "Hello!"}], gen_conf={}))
        if msg.find("ERROR: ") == 0:
            logging.error("'{}' doesn't work. {}".format( tenant["llm_id"], msg))

    if tenant["embd_id"]:
        embd_model_config = get_tenant_default_model_by_type(tenant["id"], LLMType.EMBEDDING)
        embd_mdl = LLMBundle(tenant["id"], embd_model_config)
        v, c = embd_mdl.encode(["Hello!"])
        if c == 0:
            logging.error("'{}' doesn't work!".format(tenant["embd_id"]))


_LLM_FACTORY_HASH_KEY = "__llm_factory_hash__"
_GRAPH_TEMPLATES_HASH_KEY = "__graph_templates_hash__"


def _get_llm_factory_hash():
    """Compute hash of llm factory config to detect changes."""
    factory_llm_infos = settings.FACTORY_LLM_INFOS
    content = json.dumps(factory_llm_infos, sort_keys=True, ensure_ascii=False)
    return hashlib.md5(content.encode()).hexdigest()


def _get_stored_hash():
    """Get stored hash from system_settings table."""
    try:
        rows = list(SystemSettingsService.get_by_name(_LLM_FACTORY_HASH_KEY))
        if rows:
            return rows[0].value if hasattr(rows[0], "value") else None
    except Exception:
        pass
    return None


def _set_stored_hash(hash_value):
    """Store hash in system_settings table."""
    try:
        rows = list(SystemSettingsService.get_by_name(_LLM_FACTORY_HASH_KEY))
        if rows:
            SystemSettingsService.update_by_name(_LLM_FACTORY_HASH_KEY, {"value": hash_value})
        else:
            SystemSettingsService.save(name=_LLM_FACTORY_HASH_KEY, value=hash_value, source="system", data_type="string")
    except Exception:
        pass


def _sync_kb_doc_counts():
    """Sync document counts for all knowledge bases efficiently."""
    doc_count = DocumentService.get_all_kb_doc_count()
    kb_ids = KnowledgebaseService.get_all_ids()
    if not kb_ids:
        return
    from api.db.db_models import DB, Knowledgebase as KBModel
    from common.time_utils import current_timestamp, datetime_format
    from datetime import datetime
    ts = current_timestamp()
    dt = datetime_format(datetime.now())
    with DB.atomic():
        for kb_id in kb_ids:
            count = doc_count.get(kb_id, 0)
            KBModel.update(
                doc_num=count,
                update_time=ts,
                update_date=dt
            ).where(KBModel.id == kb_id).execute()
    logging.info("Synced doc_num for %d knowledge bases.", len(kb_ids))


def init_llm_factory():
    current_hash = _get_llm_factory_hash()
    stored_hash = _get_stored_hash()

    if stored_hash == current_hash:
        logging.info("LLM factory data unchanged (hash=%s), skipping full rebuild.", current_hash)
        _sync_kb_doc_counts()
        return

    logging.info("LLM factory data changed (stored=%s, current=%s), rebuilding...", stored_hash, current_hash)
    LLMFactoriesService.filter_delete([1 == 1])
    factory_llm_infos = settings.FACTORY_LLM_INFOS
    for factory_llm_info in factory_llm_infos:
        info = deepcopy(factory_llm_info)
        llm_infos = info.pop("llm")
        try:
            LLMFactoriesService.save(**info)
        except Exception:
            pass
        LLMService.filter_delete([LLM.fid == factory_llm_info["name"]])
        for llm_info in llm_infos:
            llm_info["fid"] = factory_llm_info["name"]
            try:
                LLMService.save(**llm_info)
            except Exception:
                pass

    LLMFactoriesService.filter_delete([(LLMFactories.name == "Local") | (LLMFactories.name == "novita.ai")])
    LLMService.filter_delete([LLM.fid == "Local"])
    LLMService.filter_delete([LLM.llm_name == "qwen-vl-max"])
    LLMService.filter_delete([LLM.fid == "Moonshot", LLM.llm_name == "flag-embedding"])
    TenantLLMService.filter_delete([TenantLLM.llm_factory == "Moonshot", TenantLLM.llm_name == "flag-embedding"])
    LLMFactoriesService.filter_delete([LLMFactoriesService.model.name == "QAnything"])
    LLMService.filter_delete([LLMService.model.fid == "QAnything"])
    TenantLLMService.filter_update([TenantLLMService.model.llm_factory == "QAnything"], {"llm_factory": "Youdao"})
    TenantLLMService.filter_update([TenantLLMService.model.llm_factory == "cohere"], {"llm_factory": "Cohere"})
    TenantService.filter_update([1 == 1], {
        "parser_ids": "naive:General,qa:Q&A,resume:Resume,manual:Manual,table:Table,paper:Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One,audio:Audio,email:Email,tag:Tag"})
    ## insert openai two embedding models to the current openai user.
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
    _sync_kb_doc_counts()
    _set_stored_hash(current_hash)
    logging.info("LLM factory rebuild done.")



def _get_graph_templates_hash():
    """Compute hash of all agent template files to detect changes."""
    dir = os.path.join(get_project_base_directory(), "agent", "templates")
    if not os.path.exists(dir):
        return ""
    h = hashlib.md5()
    for fnm in sorted(os.listdir(dir)):
        fpath = os.path.join(dir, fnm)
        try:
            with open(fpath, "r", encoding="utf-8") as f:
                content = f.read()
            h.update(fnm.encode())
            h.update(content.encode())
        except Exception:
            pass
    return h.hexdigest()


def add_graph_templates():
    dir = os.path.join(get_project_base_directory(), "agent", "templates")
    if not os.path.exists(dir):
        logging.warning("Missing agent templates!")
        return

    current_hash = _get_graph_templates_hash()
    stored_hash = None
    try:
        rows = list(SystemSettingsService.get_by_name(_GRAPH_TEMPLATES_HASH_KEY))
        if rows:
            stored_hash = rows[0].value if hasattr(rows[0], "value") else None
    except Exception:
        pass

    if stored_hash == current_hash and current_hash:
        logging.info("Agent templates unchanged (hash=%s), skipping rebuild.", current_hash)
        return

    logging.info("Agent templates changed (stored=%s, current=%s), rebuilding...", stored_hash, current_hash)
    CanvasTemplateService.filter_delete([1 == 1])

    for fnm in os.listdir(dir):
        try:
            cnvs = json.load(open(os.path.join(dir, fnm), "r",encoding="utf-8"))
            try:
                CanvasTemplateService.save(**cnvs)
            except Exception:
                CanvasTemplateService.update_by_id(cnvs["id"], cnvs)
        except Exception as e:
            logging.exception(f"Add agent templates error: {e}")

    try:
        rows = list(SystemSettingsService.get_by_name(_GRAPH_TEMPLATES_HASH_KEY))
        if rows:
            SystemSettingsService.update_by_name(_GRAPH_TEMPLATES_HASH_KEY, {"value": current_hash})
        else:
            SystemSettingsService.save(name=_GRAPH_TEMPLATES_HASH_KEY, value=current_hash, source="system", data_type="string")
    except Exception:
        pass
    logging.info("Agent templates rebuild done.")


def init_web_data():
    start_time = time.time()

    init_table()

    init_llm_factory()
    # if not UserService.get_all().count():
    #    init_superuser()

    add_graph_templates()
    init_message_id_sequence()
    init_memory_size_cache()
    fix_missing_tokenized_memory()
    fix_empty_tenant_model_id()
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


def fix_empty_tenant_model_id():
    logging.info("fix_empty_tenant_model_id: checking knowledgebase...")
    # knowledgebase
    empty_tenant_embd_id_kbs = KnowledgebaseService.get_null_tenant_embd_id_row()
    if empty_tenant_embd_id_kbs:
        logging.info(f"Found {len(empty_tenant_embd_id_kbs)} empty tenant_embd_id knowledgebase.")
        kb_groups: dict = {}
        for obj in empty_tenant_embd_id_kbs:
            if kb_groups.get((obj.tenant_id, obj.embd_id)):
                kb_groups[(obj.tenant_id, obj.embd_id)].append(obj.id)
            else:
                kb_groups[(obj.tenant_id, obj.embd_id)] = [obj.id]
        update_cnt = 0
        for k, v in kb_groups.items():
            tenant_llm = TenantLLMService.get_api_key(k[0], k[1])
            if tenant_llm:
                update_cnt += KnowledgebaseService.filter_update([Knowledgebase.id.in_(v)], {"tenant_embd_id": tenant_llm.id})
        logging.info(f"Update {update_cnt} tenant_embd_id in table knowledgebase.")
    # dialog
    empty_tenant_llm_id_dialog = DialogService.get_null_tenant_llm_id_row()
    if empty_tenant_llm_id_dialog:
        logging.info(f"Found {len(empty_tenant_llm_id_dialog)} empty tenant_llm_id dialogs.")
        dialog_groups: dict = {}
        for obj in empty_tenant_llm_id_dialog:
            if dialog_groups.get((obj.tenant_id, obj.llm_id)):
                dialog_groups[(obj.tenant_id, obj.llm_id)].append(obj.id)
            else:
                dialog_groups[(obj.tenant_id, obj.llm_id)] = [obj.id]
        update_cnt = 0
        for k, v in dialog_groups.items():
            tenant_llm = TenantLLMService.get_api_key(k[0], k[1])
            if tenant_llm:
                update_cnt += DialogService.filter_update([Dialog.id.in_(v)], {"tenant_llm_id": tenant_llm.id})
        logging.info(f"Update {update_cnt} tenant_llm_id in table dialog.")

    empty_tenant_rerank_id_dialog = DialogService.get_null_tenant_rerank_id_row()
    if empty_tenant_rerank_id_dialog:
        logging.info(f"Found {len(empty_tenant_rerank_id_dialog)} empty tenant_rerank_id dialogs.")
        dialog_groups: dict = {}
        for obj in empty_tenant_rerank_id_dialog:
            if dialog_groups.get((obj.tenant_id, obj.rerank_id)):
                dialog_groups[(obj.tenant_id, obj.rerank_id)].append(obj.id)
            else:
                dialog_groups[(obj.tenant_id, obj.rerank_id)] = [obj.id]
        update_cnt = 0
        for k, v in dialog_groups.items():
            tenant_llm = TenantLLMService.get_api_key(k[0], k[1])
            if tenant_llm:
                update_cnt += DialogService.filter_update([Dialog.id.in_(v)], {"tenant_rerank_id": tenant_llm.id})
        logging.info(f"Update {update_cnt} tenant_rerank_id in table dialog.")
    logging.info("fix_empty_tenant_model_id: checking memory...")
    # memory
    empty_tenant_embd_id_memories = MemoryService.get_null_tenant_embd_id_row()
    if empty_tenant_embd_id_memories:
        logging.info(f"Found {len(empty_tenant_embd_id_memories)} empty tenant_embd_id memories.")
        memory_groups: dict = {}
        for obj in empty_tenant_embd_id_memories:
            if memory_groups.get((obj.tenant_id, obj.embd_id)):
                memory_groups[(obj.tenant_id, obj.embd_id)].append(obj.id)
            else:
                memory_groups[(obj.tenant_id, obj.embd_id)] = [obj.id]
        update_cnt = 0
        for k, v in memory_groups.items():
            tenant_llm = TenantLLMService.get_api_key(k[0], k[1])
            if tenant_llm:
                update_cnt += MemoryService.filter_update([Memory.id.in_(v)], {"tenant_embd_id": tenant_llm.id})
        logging.info(f"Update {update_cnt} tenant_embd_id in table memory.")

    logging.info("fix_empty_tenant_model_id: checking memory llm_id...")
    empty_tenant_llm_id_memories = MemoryService.get_null_tenant_llm_id_row()
    if empty_tenant_llm_id_memories:
        logging.info(f"Found {len(empty_tenant_llm_id_memories)} empty tenant_llm_id memories.")
        memory_groups: dict = {}
        for obj in empty_tenant_llm_id_memories:
            if memory_groups.get((obj.tenant_id, obj.llm_id)):
                memory_groups[(obj.tenant_id, obj.llm_id)].append(obj.id)
            else:
                memory_groups[(obj.tenant_id, obj.llm_id)] = [obj.id]
        update_cnt = 0
        for k, v in memory_groups.items():
            tenant_llm = TenantLLMService.get_api_key(k[0], k[1])
            if tenant_llm:
                update_cnt += MemoryService.filter_update([Memory.id.in_(v)], {"tenant_llm_id": tenant_llm.id})
        logging.info(f"Update {update_cnt} tenant_llm_id in table memory.")
    logging.info("fix_empty_tenant_model_id: checking tenant...")
    # tenant
    empty_tenant_model_id_tenants = TenantService.get_null_tenant_model_id_rows()
    if empty_tenant_model_id_tenants:
        logging.info(f"Found {len(empty_tenant_model_id_tenants)} empty tenant_model_id tenants.")
        update_cnt = 0
        for obj in empty_tenant_model_id_tenants:
            tenant_dict = obj.to_dict()
            update_dict = {}
            for key in ["llm_id", "embd_id", "asr_id", "img2txt_id", "rerank_id", "tts_id"]:
                if tenant_dict.get(key) and not tenant_dict.get(f"tenant_{key}"):
                    tenant_model = TenantLLMService.get_api_key(tenant_dict["id"], tenant_dict[key])
                    if tenant_model:
                        update_dict.update({f"tenant_{key}": tenant_model.id})
            if update_dict:
                update_cnt += TenantService.update_by_id(tenant_dict["id"], update_dict)
        logging.info(f"Update {update_cnt} tenant_model_id in table tenant.")
    logging.info("Fix empty tenant_model_id done.")

if __name__ == '__main__':
    init_web_db()
    init_web_data()
